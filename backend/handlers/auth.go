package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	stdmail "net/mail"
	"strings"
	"time"

	"github.com/axfinn/music/backend/config"
	"github.com/axfinn/music/backend/mail"
	"github.com/axfinn/music/backend/middleware"
	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// AuthHandler 邮箱会员认证接口（注册 / 登录 / 找回 / 会话）。
//
// 混合模式：邮箱验证码注册时顺带设置密码；之后可用密码登录，
// 忘记密码走验证码找回；也支持「验证码登录」——已注册用户收码即可登，免记密码。
// 会员与管理员完全隔离：这里的成功只签发会员 Cookie，不给后台任何权限。
type AuthHandler struct {
	db      *models.DB
	cfg     *config.Config
	mailer  *mail.Client
	member  *middleware.Member
	codeTTL time.Duration
	// ipLimiter 按 IP 限制发码频率，防止脚本对海量邮箱狂发邮件。
	ipLimiter *middleware.RateLimiter
}

// NewAuthHandler 构造会员认证处理器。
func NewAuthHandler(db *models.DB, cfg *config.Config, mailer *mail.Client, member *middleware.Member) *AuthHandler {
	return &AuthHandler{
		db:        db,
		cfg:       cfg,
		mailer:    mailer,
		member:    member,
		codeTTL:   time.Duration(cfg.Auth.CodeTTLMinutes) * time.Minute,
		ipLimiter: middleware.NewRateLimiter(cfg.Auth.CodeRateLimit*4, time.Minute),
	}
}

// Enabled 认证入口是否开放（配置了邮件服务才算可用）。
func (h *AuthHandler) Available() bool {
	return h.cfg.MailConfigured()
}

// ── 请求体 ──

type codeRequest struct {
	Email   string `json:"email"`
	Purpose string `json:"purpose"` // register / login / reset
}

type registerRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

type passwordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type codeLoginRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resetRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

// SendCode POST /api/auth/code — 给邮箱发一枚验证码。
func (h *AuthHandler) SendCode(c *gin.Context) {
	if !h.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱登录尚未开放（服务端未配置邮件服务）"})
		return
	}
	var body codeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	email := models.NormalizeEmail(body.Email)
	if !validEmail(email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不正确"})
		return
	}
	if body.Purpose != models.CodePurposeRegister &&
		body.Purpose != models.CodePurposeLogin &&
		body.Purpose != models.CodePurposeReset {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码用途不正确"})
		return
	}

	_, err := h.db.GetUserByEmail(email)
	exists := err == nil
	if err != nil && !errors.Is(err, models.ErrUserNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	// 语义检查：注册要求邮箱未被占用；登录/找回要求已注册。
	// 对未注册邮箱发「登录码」没有意义，还会让人白等一封永远收不到的邮件。
	switch body.Purpose {
	case models.CodePurposeRegister:
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已注册，请直接登录"})
			return
		}
	case models.CodePurposeLogin, models.CodePurposeReset:
		if !exists {
			// 不泄漏邮箱是否注册：对外统一说「验证码已发送」，
			// 但根本不发信，避免被人用来枚举已注册邮箱。
			c.JSON(http.StatusOK, gin.H{"sent": true, "ttl": h.cfg.Auth.CodeTTLMinutes * 60})
			return
		}
	}

	if !h.ipLimiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "获取太频繁，请稍后再试"})
		return
	}
	code, err := h.db.CreateEmailCode(email, body.Purpose, h.codeTTL, h.cfg.Auth.CodeRateLimit)
	if err != nil {
		if errors.Is(err, models.ErrCodeRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成验证码失败"})
		return
	}

	subject := fmt.Sprintf("[%s] 邮箱验证码", h.cfg.Site.Name)
	bodyText := verificationEmailBody(code, h.cfg.Auth.CodeTTLMinutes)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.mailer.Send(ctx, []string{email}, subject, bodyText); err != nil {
		// 上游拒信要如实回显（例如 from 不在白名单），否则运维以为是网络抖动。
		c.JSON(http.StatusBadGateway, gin.H{"error": "验证码邮件发送失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true, "ttl": h.cfg.Auth.CodeTTLMinutes * 60})
}

// verificationEmailBody 生成验证码邮件正文。
// 码和有效期之间不能只靠换行分隔：部分邮件网关会把纯文本里的换行吞掉，
// 只写 "123456\n\n10 分钟" 展示时就粘成 "12345610 分钟"。用【】和「有效期」
// 双重标识，即使换行全丢也不会混在一起。
func verificationEmailBody(code string, ttlMinutes int) string {
	return fmt.Sprintf("你的验证码是：【%s】\n\n有效期 %d 分钟，请勿转发给他人。\n如果不是你本人操作，请忽略这封邮件。\n",
		code, ttlMinutes)
}

// Register POST /api/auth/register — 验证码 + 密码注册，成功即登录。
func (h *AuthHandler) Register(c *gin.Context) {
	if !h.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱注册尚未开放"})
		return
	}
	var body registerRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	email := models.NormalizeEmail(body.Email)
	if !validEmail(email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不正确"})
		return
	}
	if err := validPassword(body.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.VerifyEmailCode(email, models.CodePurposeRegister, body.Code); err != nil {
		c.JSON(codeErrorStatus(err), gin.H{"error": codeErrorMessage(err)})
		return
	}
	hash, err := models.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码处理失败"})
		return
	}
	user, err := h.db.CreateUser(email, hash, body.Nickname)
	if err != nil {
		if errors.Is(err, models.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已注册，请直接登录"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败"})
		return
	}
	h.db.TouchLogin(user.ID)
	h.member.Issue(c, user.ID)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": publicUser(user)})
}

// Login POST /api/auth/login — 邮箱 + 密码登录。
func (h *AuthHandler) Login(c *gin.Context) {
	var body passwordLoginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	email := models.NormalizeEmail(body.Email)
	user, err := h.db.GetUserByEmail(email)
	if err != nil {
		// 不区分「邮箱不存在」和「密码错」，避免枚举。
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码不正确"})
		return
	}
	if !models.VerifyPassword(body.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码不正确"})
		return
	}
	h.db.TouchLogin(user.ID)
	h.member.Issue(c, user.ID)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": publicUser(user)})
}

// LoginByCode POST /api/auth/login/code — 验证码登录（免密）。
func (h *AuthHandler) LoginByCode(c *gin.Context) {
	if !h.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱登录尚未开放"})
		return
	}
	var body codeLoginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	email := models.NormalizeEmail(body.Email)
	if err := h.db.VerifyEmailCode(email, models.CodePurposeLogin, body.Code); err != nil {
		c.JSON(codeErrorStatus(err), gin.H{"error": codeErrorMessage(err)})
		return
	}
	user, err := h.db.GetUserByEmail(email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号不存在"})
		return
	}
	h.db.TouchLogin(user.ID)
	h.member.Issue(c, user.ID)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": publicUser(user)})
}

// ResetPassword POST /api/auth/reset — 验证码找回密码。
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	if !h.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱找回尚未开放"})
		return
	}
	var body resetRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := validPassword(body.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := models.NormalizeEmail(body.Email)
	if err := h.db.VerifyEmailCode(email, models.CodePurposeReset, body.Code); err != nil {
		c.JSON(codeErrorStatus(err), gin.H{"error": codeErrorMessage(err)})
		return
	}
	user, err := h.db.GetUserByEmail(email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}
	hash, err := models.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码处理失败"})
		return
	}
	if err := h.db.SetPassword(user.ID, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置密码失败"})
		return
	}
	h.member.Issue(c, user.ID)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": publicUser(user)})
}

// Session GET /api/auth/session — 当前会员是谁（未登录也 200，前端据此渲染）。
func (h *AuthHandler) Session(c *gin.Context) {
	uid := h.member.UserID(c)
	if uid == "" {
		c.JSON(http.StatusOK, gin.H{"authenticated": false, "available": h.Available(), "user": nil})
		return
	}
	user, err := h.db.GetUserByID(uid)
	if err != nil {
		h.member.Clear(c)
		c.JSON(http.StatusOK, gin.H{"authenticated": false, "available": h.Available(), "user": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"authenticated": true, "available": h.Available(), "user": publicUser(user),
	})
}

// Logout POST /api/auth/logout — 清会员 Cookie。
func (h *AuthHandler) Logout(c *gin.Context) {
	h.member.Clear(c)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

// UpdateProfile POST /api/auth/profile — 改昵称（需登录）。
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	uid := h.member.UserID(c)
	if uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if len([]rune(body.Nickname)) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "昵称最多 32 个字"})
		return
	}
	if err := h.db.UpdateNickname(uid, body.Nickname); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改失败"})
		return
	}
	user, err := h.db.GetUserByID(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取用户信息失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": publicUser(user)})
}

// ── 辅助 ──

func publicUser(u *models.User) gin.H {
	return gin.H{
		"id":         u.ID,
		"email":      u.Email,
		"nickname":   u.Nickname,
		"created_at": u.CreatedAt,
	}
}

func validEmail(email string) bool {
	if email == "" || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	addr, err := stdmail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func validPassword(password string) error {
	n := len([]rune(password))
	if n < 6 {
		return errors.New("密码至少 6 位")
	}
	if n > 128 {
		return errors.New("密码最多 128 位")
	}
	return nil
}

func codeErrorStatus(err error) int {
	switch {
	case errors.Is(err, models.ErrCodeExpired):
		return http.StatusGone
	case errors.Is(err, models.ErrCodeTooManyTries):
		return http.StatusTooManyRequests
	default:
		return http.StatusBadRequest
	}
}

func codeErrorMessage(err error) string {
	if err != nil {
		return err.Error()
	}
	return "验证码无效"
}
