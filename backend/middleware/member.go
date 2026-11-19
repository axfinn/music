package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 会员会话（独立于管理员）。
//
// 会员靠邮箱验证码登录，令牌里带 user id；管理员靠固定口令，两者 Cookie 不同、
// 密钥不同，互不授权——会员不能因此进后台，管理员也不自动成为某个会员。
//
// 同样用无状态签名 Cookie：重启不用重登，多实例不用共享会话表。
// 密钥来自邮件服务的 client_secret（高熵且仅服务端持有），不硬编码在源码里。

const (
	// MemberCookieName 会员会话 Cookie 名。
	MemberCookieName  = "music_member_session"
	memberSessionSeed = "music/member-session/v1|"
)

// Member 会员身份认证。
type Member struct {
	secret []byte
	ttl    time.Duration
}

// NewMember 由种子与会话时长构造认证器。seed 应传入高熵的服务端密钥。
func NewMember(seed string, ttl time.Duration) *Member {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	sum := sha256.Sum256([]byte(memberSessionSeed + seed))
	return &Member{secret: sum[:], ttl: ttl}
}

// Issue 为指定用户签发会话 Cookie，payload 形如 "usr_xxx|<exp>"。
func (m *Member) Issue(c *gin.Context, userID string) {
	exp := time.Now().Add(m.ttl).Unix()
	payload := userID + "|" + strconv.FormatInt(exp, 10)
	value := m.sign(payload)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     MemberCookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Unix(exp, 0),
		MaxAge:   int(m.ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(c),
	})
}

// UserID 读取当前请求的有效会员 ID；无效/未登录返回空串。
func (m *Member) UserID(c *gin.Context) string {
	cookie, err := c.Request.Cookie(MemberCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	payload, ok := m.verify(cookie.Value)
	if !ok {
		return ""
	}
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) != 2 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

// Clear 清除会员会话 Cookie（登出）。
func (m *Member) Clear(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     MemberCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(c),
	})
}

func (m *Member) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Member) verify(value string) (string, bool) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	fields := strings.SplitN(string(payload), "|", 2)
	if len(fields) != 2 || fields[0] == "" {
		return "", false
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return "", false
	}
	return string(payload), true
}
