package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 管理员会话。
//
// 站点分两种身份：
//   - 访客：浏览与播放音乐库（音乐是公开内容）；
//   - 管理：额外能进后台——歌词库、歌词发现、曲库增删改。
//
// 这里不用服务端会话表，而是「口令换算成 HMAC 密钥 + 签名 Cookie」：
// 进程重启不用重新登录，多实例部署也不用共享存储。
// 改口令等于换密钥，所有旧会话自然失效。

const (
	// AdminCookieName 管理会话 Cookie 名。
	AdminCookieName = "music_admin_session"
	// adminSessionTTL 会话有效期。个人站够用；嫌短就调这里。
	adminSessionTTL = 7 * 24 * time.Hour
)

// Admin 管理身份认证。
type Admin struct {
	secret []byte // 口令派生的 HMAC 密钥；为空表示未启用后台
	plain  string // 原始口令，仅用于恒定时间比较
}

// NewAdmin 由管理口令构造认证器。口令为空 = 纯访客模式，后台接口一律关闭。
func NewAdmin(password string) *Admin {
	password = strings.TrimSpace(password)
	if password == "" {
		return &Admin{}
	}
	// 派生密钥时混入固定前缀，避免密钥与口令本身等价。
	sum := sha256.Sum256([]byte("music/admin-session/v1|" + password))
	return &Admin{secret: sum[:], plain: password}
}

// Enabled 是否启用了后台。
func (a *Admin) Enabled() bool { return len(a.secret) > 0 }

// Authenticated 当前请求是否带有效管理会话。
func (a *Admin) Authenticated(c *gin.Context) bool {
	if !a.Enabled() {
		return false
	}
	cookie, err := c.Request.Cookie(AdminCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return a.verify(cookie.Value)
}

// Middleware 管理门：非管理员返回 403。
//
// 用 403 而不是 401 —— 这里不是「没登录」，是「你本来就不该看到这个接口」，
// 前端据此区分「需要管理员」与「访客权限不足」。
func (a *Admin) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.Enabled() {
			c.JSON(http.StatusForbidden, gin.H{"error": "未配置 admin.password，后台管理接口已禁用"})
			c.Abort()
			return
		}
		if !a.Authenticated(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员身份"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// CheckPassword 校验口令是否正确（登录用）。
func (a *Admin) CheckPassword(given string) bool {
	if !a.Enabled() {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(given)), []byte(a.plain)) == 1
}

// Issue 签发会话 Cookie。
func (a *Admin) Issue(c *gin.Context) {
	exp := time.Now().Add(adminSessionTTL).Unix()
	value := a.sign(strconv.FormatInt(exp, 10))
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AdminCookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Unix(exp, 0),
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(c),
	})
}

// Clear 清除会话 Cookie（登出）。
func (a *Admin) Clear(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AdminCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(c),
	})
}

// sign 对 payload 签名：base64url(payload) + "." + base64url(HMAC)。
func (a *Admin) sign(payload string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verify 校验 Cookie 值：格式、过期时间、签名都要过。
func (a *Admin) verify(value string) bool {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}
	exp, err := strconv.ParseInt(string(payload), 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

// isHTTPS 判断是否走 HTTPS（反代场景看 X-Forwarded-Proto）。
func isHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
