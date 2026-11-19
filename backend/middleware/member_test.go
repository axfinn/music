package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func memberContext(cookie *http.Cookie) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func TestMemberIssueAndAuthenticate(t *testing.T) {
	m := NewMember("test-secret-high-entropy", 24*time.Hour)
	c, w := memberContext(nil)
	m.Issue(c, "usr_abc123")

	var issued *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == MemberCookieName {
			issued = ck
		}
	}
	if issued == nil {
		t.Fatal("应签发会员 Cookie")
	}
	if !issued.HttpOnly {
		t.Fatal("会员 Cookie 必须 HttpOnly")
	}

	c2, _ := memberContext(issued)
	if got := m.UserID(c2); got != "usr_abc123" {
		t.Fatalf("用户 ID 应能验回, got %q", got)
	}
}

func TestMemberCookieTamperAndExpiry(t *testing.T) {
	m := NewMember("test-secret-high-entropy", 24*time.Hour)

	// 篡改签名应失效。
	c, w := memberContext(nil)
	m.Issue(c, "usr_abc123")
	var issued *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == MemberCookieName {
			issued = ck
		}
	}
	tampered := &http.Cookie{Name: MemberCookieName, Value: issued.Value + "x"}
	c2, _ := memberContext(tampered)
	if m.UserID(c2) != "" {
		t.Fatal("篡改的 Cookie 不应通过")
	}

	// 直接签一个过期时间戳的 payload（负 TTL 会被构造函数回退成默认值，不能用 Issue 构造）。
	expiredValue := m.sign("usr_abc123|" + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10))
	expired := &http.Cookie{Name: MemberCookieName, Value: expiredValue}
	c4, _ := memberContext(expired)
	if m.UserID(c4) != "" {
		t.Fatal("过期会话不应通过")
	}
}

func TestMemberAndAdminCookiesIsolated(t *testing.T) {
	// 会员 Cookie 不能被管理员认证器识别，反之亦然。
	m := NewMember("mail-secret", 24*time.Hour)
	a := NewAdmin("admin-pass")
	c, w := memberContext(nil)
	m.Issue(c, "usr_abc123")
	var memberCookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == MemberCookieName {
			memberCookie = ck
		}
	}
	c2, _ := testContext(memberCookie)
	if a.Authenticated(c2) {
		t.Fatal("会员 Cookie 不应让管理员认证通过")
	}
}

func TestMemberClear(t *testing.T) {
	m := NewMember("mail-secret", 24*time.Hour)
	c, w := memberContext(nil)
	m.Clear(c)
	for _, ck := range w.Result().Cookies() {
		if ck.Name == MemberCookieName && ck.MaxAge >= 0 {
			t.Fatalf("清除 Cookie 应设 MaxAge<0, got %d", ck.MaxAge)
		}
	}
}
