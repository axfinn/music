package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// 小于 1KB 的响应压了反而更大（gzip 头尾约 20~30B），也白搭 CPU。
const gzipMinSize = 1024

var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// GzipHandler 包在 gin.Engine 外层，压缩 API 的文本类响应。
//
// 为什么不是 gin 中间件：gin 的 Context.Status() 直接操作内部 writermem，
// 替换 c.Writer 拿不到真实状态码；包在 http.Handler 这一层只需面对标准的
// http.ResponseWriter，也能顺手避开 c.File / ServeContent 的流式响应。
//
// 曲库首屏 /snapshot 有 1.1MB 明文 JSON，gzip 后约 170KB。外层反代与 CDN
// 默认只压 text/html，不压 application/json，所以必须在应用内解决。
//
// 跳过规则（任一命中即原样透传）：
//   - 客户端未在 Accept-Encoding 声明 gzip，或 q=0 明确拒绝
//   - HEAD / 带 Range 的请求（音频拖进度条）
//   - 非 /api/ 路径（静态前端交给 CDN，c.File 是流式的）
//   - /stream/ 与 /cover/（音频、图片本身已压缩，且要支持 Range）
//   - 响应体 < 1KB、非 2xx、或 Content-Type 不是 JSON / 文本
func GzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

func shouldGzip(r *http.Request) bool {
	if !acceptsGzip(r.Header.Get("Accept-Encoding")) ||
		r.Method == http.MethodHead ||
		r.Header.Get("Range") != "" {
		return false
	}
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/") &&
		!strings.Contains(p, "/stream/") &&
		!strings.Contains(p, "/cover/")
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		pieces := strings.SplitN(strings.TrimSpace(part), ";", 2)
		if !strings.EqualFold(strings.TrimSpace(pieces[0]), "gzip") {
			continue
		}
		if len(pieces) > 1 {
			q := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(pieces[1]), "q="))
			if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
				return false
			}
		}
		return true
	}
	return false
}

func compressibleContentType(ct string) bool {
	if ct == "" {
		return false // 没声明类型宁可不压，避免误伤二进制
	}
	media := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return media == "application/json" ||
		strings.HasSuffix(media, "+json") ||
		strings.HasPrefix(media, "text/") ||
		media == "application/javascript" ||
		media == "application/xml" ||
		media == "image/svg+xml"
}

// gzipResponseWriter 延迟提交响应头：WriteHeader 只记状态码，等第一次 Write
// 才知道响应体大小，再决定要不要压。这样小响应和错误页都零开销透传。
type gzipResponseWriter struct {
	http.ResponseWriter
	status        int
	decided       bool
	compress      bool
	headerWritten bool
	gz            *gzip.Writer
}

// Unwrap 让 http.NewResponseController 能穿过包装拿到底层 writer。
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *gzipResponseWriter) WriteHeader(code int) {
	if !w.headerWritten {
		w.status = code
	}
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.headerWritten {
		w.decide(len(p))
	}
	if w.compress {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *gzipResponseWriter) Flush() {
	if !w.headerWritten {
		w.decide(0) // 流式响应拿不到总长度，一律不压
	}
	if w.compress {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *gzipResponseWriter) decide(sizeHint int) {
	if w.decided {
		return
	}
	w.decided = true
	w.compress = w.status >= 200 && w.status < 300 &&
		sizeHint >= gzipMinSize &&
		compressibleContentType(w.Header().Get("Content-Type"))
	if w.compress {
		// 站内 CORS 中间件会 Set("Vary", "Origin") 覆盖掉外层加的值，
		// 所以在真正提交响应头之前补一次，避免 CDN 把 gzip 响应发给不支持的客户端。
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		w.gz = gzipWriterPool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
	}
	w.headerWritten = true
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *gzipResponseWriter) finish() {
	if !w.headerWritten {
		w.decide(0) // 空响应 / 只写了状态码：补提交响应头
	}
	if w.gz != nil {
		_ = w.gz.Close()
		gzipWriterPool.Put(w.gz)
		w.gz = nil
	}
}
