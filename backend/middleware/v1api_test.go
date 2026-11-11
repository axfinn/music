package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func testV1API(limit int, origins ...string) *V1API {
	return NewV1API(V1APIOptions{
		KeyRateLimit: limit,
		AllowOrigins: origins,
		Verify: func(raw string) (APIKeyInfo, error) {
			if raw != "good-key" {
				return APIKeyInfo{}, errors.New("invalid")
			}
			return APIKeyInfo{ID: "k1", Name: "对接系统", Prefix: "lly_1a2b", CallCount: 7}, nil
		},
	})
}

func testV1Router(apiV1 *V1API) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ping", apiV1.CORS(), apiV1.OptionalKey(), func(c *gin.Context) {
		key, ok := APIKeyFromContext(c)
		c.JSON(http.StatusOK, gin.H{"authed": ok, "name": key.Name, "limit": apiV1.KeyRateLimit()})
	})
	r.GET("/data", apiV1.CORS(), apiV1.RequireKey(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.OPTIONS("/data", apiV1.CORS(), apiV1.RequireKey())
	return r
}

func serve(r *gin.Engine, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, w.Body.String())
	}
	return body
}

func TestV1KeyGate(t *testing.T) {
	r := testV1Router(testV1API(60))

	// 需要密钥的端点：缺密钥、无效密钥都是 401，且失败信封结构一致。
	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{"缺密钥", nil},
		{"无效密钥", map[string]string{"X-API-Key": "bad"}},
	} {
		w := serve(r, http.MethodGet, "/data", tc.headers)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s 应返回 401，实际 %d", tc.name, w.Code)
		}
		body := decode(t, w)
		if body["ok"] != false || body["code"] != float64(401) || body["error"] == "" {
			t.Fatalf("%s 失败信封不对: %v", tc.name, body)
		}
	}

	// X-API-Key / Bearer / ?api_key= 三种传法都要认。
	for _, tc := range []struct {
		path    string
		headers map[string]string
	}{
		{"/data", map[string]string{"X-API-Key": "good-key"}},
		{"/data", map[string]string{"Authorization": "Bearer good-key"}},
		{"/data?api_key=good-key", nil},
	} {
		if w := serve(r, http.MethodGet, tc.path, tc.headers); w.Code != http.StatusOK {
			t.Fatalf("%v 应放行，实际 %d", tc, w.Code)
		}
	}
}

func TestV1OptionalKeyPing(t *testing.T) {
	r := testV1Router(testV1API(60))

	// 不带密钥也能探活，但不会有密钥信息。
	w := serve(r, http.MethodGet, "/ping", nil)
	if w.Code != http.StatusOK || decode(t, w)["authed"] != false {
		t.Fatalf("ping 不带密钥应放行: %d %s", w.Code, w.Body.String())
	}
	// 带合法密钥则把用量透出来（这里验证 context 传递，字段在 handler 里组装）。
	w = serve(r, http.MethodGet, "/ping", map[string]string{"X-API-Key": "good-key"})
	body := decode(t, w)
	if body["authed"] != true || body["name"] != "对接系统" || body["limit"] != float64(60) {
		t.Fatalf("ping 带密钥应返回用量: %v", body)
	}
	// 带了密钥就必须有效，不能静默当成匿名。
	w = serve(r, http.MethodGet, "/ping", map[string]string{"X-API-Key": "bad"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("ping 带无效密钥应返回 401，实际 %d", w.Code)
	}
}

func TestV1CORS(t *testing.T) {
	// 默认放开任意来源；预检请求在 CORS 层就地结束，不会撞到密钥门。
	r := testV1Router(testV1API(60))
	w := serve(r, http.MethodOptions, "/data", map[string]string{"Origin": "https://any.example.com"})
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("预检应 204 且放开来源: %d %v", w.Code, w.Header())
	}

	// 配了白名单就只放行名单里的 Origin。
	strict := testV1Router(testV1API(60, "https://music.jaxiu.cn"))
	w = serve(strict, http.MethodGet, "/data", map[string]string{
		"Origin": "https://evil.example.com", "X-API-Key": "good-key",
	})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("白名单外的 Origin 不该被放行: %v", w.Header())
	}
	w = serve(strict, http.MethodGet, "/data", map[string]string{
		"Origin": "https://music.jaxiu.cn", "X-API-Key": "good-key",
	})
	if w.Header().Get("Access-Control-Allow-Origin") != "https://music.jaxiu.cn" {
		t.Fatalf("白名单内的 Origin 应被放行: %v", w.Header())
	}
}

func TestV1KeyRateLimit(t *testing.T) {
	r := testV1Router(testV1API(1))
	if w := serve(r, http.MethodGet, "/data", map[string]string{"X-API-Key": "good-key"}); w.Code != http.StatusOK {
		t.Fatalf("第一次应放行，实际 %d", w.Code)
	}
	w := serve(r, http.MethodGet, "/data", map[string]string{"X-API-Key": "good-key"})
	if w.Code != http.StatusTooManyRequests || decode(t, w)["code"] != float64(429) {
		t.Fatalf("第二次应 429，实际 %d %s", w.Code, w.Body.String())
	}
}
