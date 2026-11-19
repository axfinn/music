package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/axfinn/music/backend/config"
	"github.com/axfinn/music/backend/mail"
	"github.com/axfinn/music/backend/middleware"
	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

type authTestEnv struct {
	h      *AuthHandler
	member *middleware.Member
	db     *models.DB
	cfg    *config.Config
}

func newAuthTestEnv(t *testing.T) *authTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := models.NewDB(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.DefaultConfig()
	cfg.Mail.ClientID = "mt_test"
	cfg.Mail.ClientSecret = "test-secret"
	member := middleware.NewMember(cfg.Mail.ClientSecret, time.Hour)
	mailer := mail.NewClient(&cfg.Mail)
	return &authTestEnv{
		h:      NewAuthHandler(db, cfg, mailer, member),
		member: member,
		db:     db,
		cfg:    cfg,
	}
}

// doRequest 把一个 JSON 请求打到指定 handler，返回状态码与解析后的 body。
func doRequest(t *testing.T, handler gin.HandlerFunc, method, path string, payload any, cookies ...*http.Cookie) (int, map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler(c)
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result, w
}

func memberCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range w.Result().Cookies() {
		if ck.Name == middleware.MemberCookieName {
			return ck
		}
	}
	return nil
}

func TestAuthLoginSessionLogoutFlow(t *testing.T) {
	env := newAuthTestEnv(t)
	hash, _ := models.HashPassword("mypassword")
	user, err := env.db.CreateUser("frank@example.com", hash, "弗兰克")
	if err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	// 密码错误 → 401，且不泄漏邮箱是否存在。
	status, body, _ := doRequest(t, env.h.Login, http.MethodPost, "/api/auth/login",
		passwordLoginRequest{Email: "frank@example.com", Password: "wrong"})
	if status != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401, got %d %v", status, body)
	}
	status, _, _ = doRequest(t, env.h.Login, http.MethodPost, "/api/auth/login",
		passwordLoginRequest{Email: "nobody@example.com", Password: "mypassword"})
	if status != http.StatusUnauthorized {
		t.Fatalf("不存在的邮箱也应 401（防枚举）, got %d", status)
	}

	// 正确密码 → 200 + 会员 Cookie。
	status, body, w := doRequest(t, env.h.Login, http.MethodPost, "/api/auth/login",
		passwordLoginRequest{Email: "frank@example.com", Password: "mypassword"})
	if status != http.StatusOK || body["authenticated"] != true {
		t.Fatalf("正确密码应登录成功, got %d %v", status, body)
	}
	ck := memberCookie(w)
	if ck == nil || !ck.HttpOnly {
		t.Fatal("登录应签发 HttpOnly 会员 Cookie")
	}

	// 带 Cookie 查会话 → 已登录，拿到当前用户。
	status, body, _ = doRequest(t, env.h.Session, http.MethodGet, "/api/auth/session", nil, ck)
	if status != http.StatusOK || body["authenticated"] != true {
		t.Fatalf("会话应已登录, got %d %v", status, body)
	}
	u, ok := body["user"].(map[string]any)
	if !ok || u["id"] != user.ID || u["email"] != "frank@example.com" {
		t.Fatalf("会话返回的用户不对: %v", body)
	}

	// 未登录查会话 → 200 但未认证。
	status, body, _ = doRequest(t, env.h.Session, http.MethodGet, "/api/auth/session", nil)
	if status != http.StatusOK || body["authenticated"] != false {
		t.Fatalf("匿名会话应返回未登录, got %d %v", status, body)
	}

	// 登出 → Cookie 被清，会话变未登录。
	_, _, w = doRequest(t, env.h.Logout, http.MethodPost, "/api/auth/logout", nil, ck)
	cleared := memberCookie(w)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("登出应清除 Cookie, got %+v", cleared)
	}
}

func TestAuthMemberCannotEnterAdmin(t *testing.T) {
	env := newAuthTestEnv(t)
	hash, _ := models.HashPassword("mypassword")
	_, _ = env.db.CreateUser("grace@example.com", hash, "")
	_, _, w := doRequest(t, env.h.Login, http.MethodPost, "/api/auth/login",
		passwordLoginRequest{Email: "grace@example.com", Password: "mypassword"})
	ck := memberCookie(w)

	// 会员 Cookie 不能让管理员中间件通过（用真实引擎挂一条受保护路由）。
	admin := middleware.NewAdmin("admin-pass")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/protected", admin.Middleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(ck)
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("会员 Cookie 不应进入后台, code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthCodeRequiresConfiguration(t *testing.T) {
	env := newAuthTestEnv(t)
	env.cfg.Mail.ClientID = "" // 模拟未配置邮件服务
	status, body, _ := doRequest(t, env.h.SendCode, http.MethodPost, "/api/auth/code",
		codeRequest{Email: "heidi@example.com", Purpose: models.CodePurposeRegister})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("未配置邮件服务发码应 503, got %d %v", status, body)
	}
}

func TestAuthUpdateProfileRequiresLogin(t *testing.T) {
	env := newAuthTestEnv(t)
	status, _, _ := doRequest(t, env.h.UpdateProfile, http.MethodPost, "/api/auth/profile",
		map[string]string{"nickname": "x"})
	if status != http.StatusUnauthorized {
		t.Fatalf("未登录改昵称应 401, got %d", status)
	}
}
