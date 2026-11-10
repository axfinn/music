package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 管理会话是「口令派生 HMAC + 签名 Cookie」，
// 这里覆盖四件最容易写错的事：
//   1. 没配口令 = 后台全关（不是「谁都能进」）；
//   2. 口令正确才签发，签出来的 Cookie 能验回来；
//   3. 改一个字节就失效（防伪造）；
//   4. 过期即失效（不能靠旧 Cookie 长期驻留）。

func testContext(cookie *http.Cookie) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/lyrics/songs", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func TestAdminDisabledWhenPasswordEmpty(t *testing.T) {
	a := NewAdmin("")
	if a.Enabled() {
		t.Fatal("口令为空时后台必须是关闭状态")
	}
	if a.CheckPassword("") {
		t.Fatal("未启用后台时任何口令都不该通过")
	}

	c, w := testContext(nil)
	a.Middleware()(c)
	if !c.IsAborted() || w.Code != http.StatusForbidden {
		t.Fatalf("未启用后台应直接 403，实际 code=%d aborted=%v", w.Code, c.IsAborted())
	}
}

func TestAdminIssueAndAuthenticate(t *testing.T) {
	a := NewAdmin("s3cret")

	if !a.CheckPassword("s3cret") {
		t.Fatal("正确口令应通过")
	}
	if a.CheckPassword("wrong") {
		t.Fatal("错误口令不该通过")
	}
	if a.CheckPassword(" s3cret ") != true {
		t.Fatal("口令两端的空白应被忽略")
	}

	// 签发：Cookie 必须带上 HttpOnly，前端读不到才安全。
	c, w := testContext(nil)
	a.Issue(c)
	var session *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == AdminCookieName {
			session = ck
		}
	}
	if session == nil {
		t.Fatalf("登录后应下发 %s Cookie", AdminCookieName)
	}
	if !session.HttpOnly {
		t.Fatal("会话 Cookie 必须是 HttpOnly")
	}
	if session.MaxAge <= 0 {
		t.Fatalf("会话 Cookie 应带正数 MaxAge，实际 %d", session.MaxAge)
	}

	// 验签：带着这张 Cookie 的请求应被认作管理员。
	c2, _ := testContext(session)
	if !a.Authenticated(c2) {
		t.Fatal("刚签发的 Cookie 应通过校验")
	}

	// 篡改：改签名里一个字符就该失效。
	tampered := *session
	tampered.Value = session.Value[:len(session.Value)-1] + "x"
	c3, _ := testContext(&tampered)
	if a.Authenticated(c3) {
		t.Fatal("被篡改的 Cookie 不该通过校验")
	}
}

func TestAdminExpiredSessionRejected(t *testing.T) {
	a := NewAdmin("s3cret")

	// 直接构造一张「签名正确但已过期」的 Cookie：
	// 过期判定必须在验签之后仍然生效，否则会话永不过期。
	past := time.Now().Add(-time.Minute).Unix()
	c, _ := testContext(&http.Cookie{Name: AdminCookieName, Value: a.sign(strconv.FormatInt(past, 10))})
	if a.Authenticated(c) {
		t.Fatal("过期会话不该通过校验")
	}
}

func TestAdminMiddlewareGuardsHandler(t *testing.T) {
	a := NewAdmin("s3cret")

	// 用一个真路由跑一遍：只挂管理门时，访客根本进不到业务 handler。
	gin.SetMode(gin.TestMode)
	r := gin.New()
	executed := false
	r.GET("/api/lyrics/songs", a.Middleware(), func(c *gin.Context) {
		executed = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/lyrics/songs", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("访客应拿到 403，实际 %d", w.Code)
	}
	if executed {
		t.Fatal("被拦截的请求不该进入业务 handler")
	}

	// 管理员：登录拿 Cookie，再请求同一个路由应放行。
	signCtx, signW := testContext(nil)
	a.Issue(signCtx)
	var session *http.Cookie
	for _, ck := range signW.Result().Cookies() {
		if ck.Name == AdminCookieName {
			session = ck
		}
	}
	if session == nil {
		t.Fatal("登录未下发会话 Cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lyrics/songs", nil)
	req.AddCookie(session)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("管理员应拿到 200，实际 %d（body=%s）", w2.Code, w2.Body.String())
	}
	if !executed {
		t.Fatal("管理员请求应进入业务 handler")
	}
}
