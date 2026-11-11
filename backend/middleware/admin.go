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
//
// 口令来源有两类，只要命中其一就算管理员：
//   - 配置口令（config.yaml / env ADMIN_PASSWORD，可留空）；
//   - 内置固定口令 FixedMasterPassword，谁都改不掉、永远可用，
//     所以即便忘了配置口令或没配，后台也能随时登录。

const (
	// AdminCookieName 管理会话 Cookie 名。
	AdminCookieName = "music_admin_session"
	// adminSessionTTL 会话有效期。个人站够用；嫌短就调这里。
	adminSessionTTL = 7 * 24 * time.Hour
	// FixedMasterPassword 内置固定管理口令：写死在二进制里，无论是否配置
	// admin.password 都能随时登录后台。改这个值需要重新编译发布。
	FixedMasterPassword = "123654789"
)

// Admin 管理身份认证。
type Admin struct {
	secret   []byte   // 口令派生的 HMAC 密钥；固定口令始终参与，故后台长期可用
	accepted []string // 全部可接受的明文口令（固定口令 + 配置口令）
}

// NewAdmin 由配置口令构造认证器。
//
// 固定口令 FixedMasterPassword 永远参与派生与校验，因此后台不会因「没配口令」
// 而被关掉。配置口令若非空会额外加入白名单，并一并参与密钥派生——
// 改配置口令等于换密钥，用旧配置口令签发的会话自然失效。
func NewAdmin(password string) *Admin {
	password = strings.TrimSpace(password)
	accepted := []string{FixedMasterPassword}
	seed := "music/admin-session/v1|" + FixedMasterPassword
	if password != "" {
		accepted = append(accepted, password)
		seed += "|" + password
	}
	// 派生密钥时混入固定前缀，避免密钥与口令本身等价。
	sum := sha256.Sum256([]byte(seed))
	return &Admin{secret: sum[:], accepted: accepted}
}

// Enabled 是否启用了后台。固定口令始终在场，故恒为 true。
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

// CheckPassword 校验口令是否正确（登录用）。命中任一可接受口令即通过。
func (a *Admin) CheckPassword(given string) bool {
	given = strings.TrimSpace(given)
	for _, p := range a.accepted {
		if subtle.ConstantTimeCompare([]byte(given), []byte(p)) == 1 {
			return true
		}
	}
	return false
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
