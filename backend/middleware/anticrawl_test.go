package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testBrowserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func enabledAntiCrawlOptions() AntiCrawlOptions {
	return AntiCrawlOptions{
		Enabled:           true,
		AnonymousReadRate: 180,
		BulkReadRate:      40,
		StreamRate:        300,
	}
}

// antiCrawlRouter 复刻 main.go 的挂载顺序：全局 NoIndex → /robots.txt → /api Guard。
func antiCrawlRouter(opts AntiCrawlOptions) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(NoIndex())
	r.GET("/robots.txt", RobotsTxt())

	guard := NewAntiCrawl(opts)
	api := r.Group("/api")
	api.Use(guard.Guard())
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	api.GET("/health", ok)
	api.GET("/site", ok)
	api.GET("/music/tracks", ok)
	api.GET("/music/tracks/:id", ok)
	api.GET("/music/snapshot", ok)
	api.GET("/music/stream/:id", ok)
	api.GET("/music/cover/:id", ok)
	api.GET("/lyrics/export", ok)
	api.GET("/v1/music/tracks", ok)
	api.OPTIONS("/music/tracks", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return r
}

func call(r *gin.Engine, path string, headers map[string]string) *httptest.ResponseRecorder {
	return callMethod(r, http.MethodGet, path, headers)
}

func callMethod(r *gin.Engine, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func webHeaders() map[string]string {
	return map[string]string{"User-Agent": testBrowserUA, BrowserClientHeader: BrowserClientValue}
}

func TestRobotsTxtAndNoIndex(t *testing.T) {
	r := antiCrawlRouter(enabledAntiCrawlOptions())

	w := call(r, "/robots.txt", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Disallow: /") {
		t.Fatalf("robots.txt 应全站 Disallow，实际 %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatal("robots.txt 必须是纯文本，不能被 SPA 回落成 HTML")
	}

	w = call(r, "/api/music/tracks", webHeaders())
	if got := w.Header().Get("X-Robots-Tag"); got != noIndexHeader {
		t.Fatalf("API 响应应带 noindex，实际 %q", got)
	}
	w = call(r, "/robots.txt", nil)
	if got := w.Header().Get("X-Robots-Tag"); got != noIndexHeader {
		t.Fatalf("静态响应也应带 noindex，实际 %q", got)
	}
}

// 这一组是「禁止爬虫抓数据」的主线：脚本、HTTP 库、爬虫与无头浏览器一律 403。
func TestGuardBlocksScriptAndCrawlerUA(t *testing.T) {
	r := antiCrawlRouter(enabledAntiCrawlOptions())
	blocked := []string{
		"",
		"curl/8.4.0",
		"Wget/1.21",
		"python-requests/2.31.0",
		"Go-http-client/1.1",
		"Scrapy/2.11 (+https://scrapy.org)",
		"Mozilla/5.0 (compatible; Bytespider/1.0)",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 HeadlessChrome/120 Safari/537.36",
	}
	for _, ua := range blocked {
		h := webHeaders()
		h["User-Agent"] = ua
		w := call(r, "/api/music/tracks", h)
		if w.Code != http.StatusForbidden {
			t.Errorf("UA %q 应被拒绝，实际 %d", ua, w.Code)
		}
	}
	if w := call(r, "/api/music/tracks", webHeaders()); w.Code != http.StatusOK {
		t.Fatalf("正常浏览器应放行，实际 %d", w.Code)
	}
}

// 只伪造 UA 还不够：匿名数据接口必须同时带一个浏览器来源信号。
func TestGuardRequiresBrowserSignalOnDataAPIs(t *testing.T) {
	r := antiCrawlRouter(enabledAntiCrawlOptions())

	bare := map[string]string{"User-Agent": testBrowserUA}
	if w := call(r, "/api/music/tracks", bare); w.Code != http.StatusForbidden {
		t.Fatalf("无浏览器信号的数据请求应 403，实际 %d", w.Code)
	}
	withFetch := map[string]string{"User-Agent": testBrowserUA, "Sec-Fetch-Site": "same-origin"}
	if w := call(r, "/api/music/tracks", withFetch); w.Code != http.StatusOK {
		t.Fatalf("Sec-Fetch-Site: same-origin 应放行，实际 %d", w.Code)
	}
	if w := call(r, "/api/music/tracks", webHeaders()); w.Code != http.StatusOK {
		t.Fatalf("自家前端标识应放行，实际 %d", w.Code)
	}
	// <audio> / <img> 是浏览器直接发的，老浏览器可能不带任何 Sec-Fetch 头，
	// 不能因为反爬把播放和封面弄坏。
	for _, path := range []string{"/api/music/stream/mus_1", "/api/music/cover/mus_1"} {
		if w := call(r, path, bare); w.Code != http.StatusOK {
			t.Errorf("%s 应只按 UA 放行，实际 %d", path, w.Code)
		}
	}
	if w := call(r, "/api/music/stream/mus_1", map[string]string{"User-Agent": "curl/8.4.0"}); w.Code != http.StatusForbidden {
		t.Fatalf("脚本批量下载音频应被拒绝，实际 %d", w.Code)
	}
}

func TestGuardRejectsCrossSite(t *testing.T) {
	r := antiCrawlRouter(enabledAntiCrawlOptions())
	h := webHeaders()
	h["Sec-Fetch-Site"] = "cross-site"
	if w := call(r, "/api/music/tracks", h); w.Code != http.StatusForbidden {
		t.Fatalf("跨站取数应 403，实际 %d", w.Code)
	}
	h["Sec-Fetch-Site"] = "none" // 地址栏直接打开
	if w := call(r, "/api/music/tracks", h); w.Code != http.StatusOK {
		t.Fatalf("直接导航应放行，实际 %d", w.Code)
	}
}

// 健康检查、站点信息、预检与 API Key 授权通道不能被误伤：
// healthcheck 用的是 wget，v1 的合法调用方本来就是脚本。
func TestGuardExemptPaths(t *testing.T) {
	r := antiCrawlRouter(enabledAntiCrawlOptions())
	wget := map[string]string{"User-Agent": "Wget/1.21"}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/site"},
		{http.MethodGet, "/api/v1/music/tracks"},
		{http.MethodOptions, "/api/music/tracks"},
	} {
		w := callMethod(r, tc.method, tc.path, wget)
		if w.Code == http.StatusForbidden {
			t.Errorf("%s %s 不应被反爬拦截，实际 %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestGuardBulkRateLimit(t *testing.T) {
	opts := enabledAntiCrawlOptions()
	opts.BulkReadRate = 2
	r := antiCrawlRouter(opts)

	for i := 0; i < 2; i++ {
		if w := call(r, "/api/music/snapshot", webHeaders()); w.Code != http.StatusOK {
			t.Fatalf("第 %d 次全量读取应放行，实际 %d", i+1, w.Code)
		}
	}
	if w := call(r, "/api/music/snapshot", webHeaders()); w.Code != http.StatusTooManyRequests {
		t.Fatalf("超额的 snapshot 应 429，实际 %d", w.Code)
	}
	// 大分页也归到批量档，换个 URL 绕不过去。
	if w := call(r, "/api/music/tracks?limit=2000", webHeaders()); w.Code != http.StatusTooManyRequests {
		t.Fatalf("大分页应与 snapshot 共用批量额度，实际 %d", w.Code)
	}
	// 普通读不受批量额度牵连。
	if w := call(r, "/api/music/tracks?limit=100", webHeaders()); w.Code != http.StatusOK {
		t.Fatalf("普通列表应有独立额度，实际 %d", w.Code)
	}
}

func TestGuardStreamHasOwnRate(t *testing.T) {
	opts := enabledAntiCrawlOptions()
	opts.BulkReadRate = 1
	opts.StreamRate = 2
	r := antiCrawlRouter(opts)

	for i := 0; i < 2; i++ {
		if w := call(r, "/api/music/stream/mus_1", map[string]string{"User-Agent": testBrowserUA}); w.Code != http.StatusOK {
			t.Fatalf("第 %d 次音频请求应放行，实际 %d", i+1, w.Code)
		}
	}
	if w := call(r, "/api/music/stream/mus_1", map[string]string{"User-Agent": testBrowserUA}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("超额音频请求应 429，实际 %d", w.Code)
	}
}

func TestGuardAllowUserAgentsAndDisableSwitch(t *testing.T) {
	opts := enabledAntiCrawlOptions()
	opts.AllowUserAgents = []string{"my-monitor"}
	h := webHeaders()
	h["User-Agent"] = "my-monitor/1.0 curl/8.0"
	if w := call(antiCrawlRouter(opts), "/api/music/tracks", h); w.Code != http.StatusOK {
		t.Fatalf("白名单 UA 应放行，实际 %d", w.Code)
	}
	if w := call(antiCrawlRouter(enabledAntiCrawlOptions()), "/api/music/tracks", h); w.Code != http.StatusForbidden {
		t.Fatalf("不在白名单的脚本 UA 应拒绝，实际 %d", w.Code)
	}

	opts = enabledAntiCrawlOptions()
	opts.Enabled = false
	r := antiCrawlRouter(opts)
	w := call(r, "/api/music/snapshot", map[string]string{"User-Agent": "curl/8.0"})
	if w.Code != http.StatusOK {
		t.Fatalf("关闭开关后应全放行，实际 %d", w.Code)
	}
	if got := w.Header().Get("X-Robots-Tag"); got != noIndexHeader {
		t.Fatalf("关闭开关后声明层仍应生效，实际 %q", got)
	}
}

func TestClassifyReadClasses(t *testing.T) {
	cases := []struct {
		path  string
		query url.Values
		want  readClass
	}{
		{"/api/music/tracks", nil, classRead},
		{"/api/music/tracks", url.Values{"limit": {"100"}}, classRead},
		{"/api/music/tracks", url.Values{"limit": {"500"}}, classBulk},
		{"/api/music/tracks", url.Values{"full": {"1"}}, classBulk},
		{"/api/music/snapshot", nil, classBulk},
		{"/api/lyrics/export", nil, classBulk},
		{"/api/music/stream/mus_1", nil, classStream},
		{"/api/music/cover/mus_1", nil, classStream},
		{"/api/music/tracks/mus_1", nil, classRead},
	}
	for _, tc := range cases {
		if got := classify(tc.path, tc.query); got != tc.want {
			t.Errorf("classify(%q, %v) = %d，期望 %d", tc.path, tc.query, got, tc.want)
		}
	}
}
