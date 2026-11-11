package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipHandler(t *testing.T) {
	big := `{"items":[` + strings.Repeat(`{"title":"测试曲目","artist":"某某人","album":"示例专辑"},`, 40) + `{"title":"末"}]}`
	if len(big) < gzipMinSize {
		t.Fatalf("测试体必须大于压缩阈值 %d，实际 %d", gzipMinSize, len(big))
	}

	cases := []struct {
		name        string
		path        string
		method      string
		accept      string
		rangeHdr    string
		contentType string
		status      int
		body        string
		originVary  bool // 模拟站内 CORS 中间件 Set("Vary", "Origin")
		wantGzip    bool
		wantStatus  int
	}{
		{
			name: "大 JSON 压缩", path: "/api/music/snapshot",
			accept: "gzip, deflate, br", contentType: "application/json; charset=utf-8",
			body: big, wantGzip: true, wantStatus: 200,
		},
		{
			name: "CORS 覆盖 Vary 后仍保留 Accept-Encoding", path: "/api/music/tracks",
			accept: "gzip", contentType: "application/json", body: big,
			originVary: true, wantGzip: true, wantStatus: 200,
		},
		{
			name: "客户端不支持 gzip 原样透传", path: "/api/music/snapshot",
			contentType: "application/json", body: big, wantGzip: false, wantStatus: 200,
		},
		{
			name: "q=0 明确拒绝", path: "/api/music/snapshot",
			accept: "gzip;q=0, br", contentType: "application/json", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "音频流不压", path: "/api/music/stream/mus_123",
			accept: "gzip", contentType: "audio/mp4", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "封面不压", path: "/api/music/cover/mus_123",
			accept: "gzip", contentType: "image/jpeg", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "图片类型不压", path: "/api/music/snapshot",
			accept: "gzip", contentType: "image/png", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "小响应不压", path: "/api/health",
			accept: "gzip", contentType: "application/json", body: `{"status":"ok"}`,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "错误响应不压", path: "/api/music/snapshot",
			accept: "gzip", contentType: "application/json", status: 500, body: big,
			wantGzip: false, wantStatus: 500,
		},
		{
			name: "HEAD 不压", path: "/api/music/snapshot", method: http.MethodHead,
			accept: "gzip", contentType: "application/json", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "Range 请求不压", path: "/api/music/snapshot",
			accept: "gzip", rangeHdr: "bytes=0-1023", contentType: "application/json", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "静态前端不压", path: "/index.html",
			accept: "gzip", contentType: "text/html", body: big,
			wantGzip: false, wantStatus: 200,
		},
		{
			name: "只有状态码没有响应体", path: "/api/health",
			accept: "gzip", contentType: "application/json", status: 204, body: "",
			wantGzip: false, wantStatus: 204,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.originVary {
					w.Header().Set("Vary", "Origin")
				}
				w.Header().Set("Content-Type", tc.contentType)
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			})

			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept-Encoding", tc.accept)
			}
			if tc.rangeHdr != "" {
				req.Header.Set("Range", tc.rangeHdr)
			}
			rec := httptest.NewRecorder()
			GzipHandler(next).ServeHTTP(rec, req)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tc.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d", res.StatusCode, tc.wantStatus)
			}
			encoding := res.Header.Get("Content-Encoding")
			if tc.wantGzip {
				if encoding != "gzip" {
					t.Fatalf("Content-Encoding = %q，期望 gzip", encoding)
				}
				// Vary 是多值头：站内 CORS 会写 Origin，这里再 Add 一个，
				// Header.Get() 只返回第一个，必须按 Values 判断。
				vary := strings.Join(res.Header.Values("Vary"), ",")
				if !strings.Contains(vary, "Accept-Encoding") {
					t.Fatalf("Vary = %q，缺少 Accept-Encoding", vary)
				}
				zr, err := gzip.NewReader(res.Body)
				if err != nil {
					t.Fatalf("解压失败: %v", err)
				}
				plain, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("读取解压内容失败: %v", err)
				}
				_ = zr.Close()
				if string(plain) != tc.body {
					t.Fatalf("解压后内容不一致: got %d bytes, want %d", len(plain), len(tc.body))
				}
				if rec.Body.Len() >= len(tc.body)/2 {
					t.Fatalf("压缩率异常: %d -> %d", len(tc.body), rec.Body.Len())
				}
				if tc.originVary && !strings.Contains(vary, "Origin") {
					t.Fatalf("Vary 丢了 Origin: %q", vary)
				}
			} else {
				if encoding != "" {
					t.Fatalf("不该压缩，Content-Encoding = %q", encoding)
				}
				if rec.Body.String() != tc.body {
					t.Fatal("透传内容被改动")
				}
			}
		})
	}
}
