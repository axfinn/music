package handlers

import (
	"net/http"

	"github.com/axfinn/music/backend/middleware"

	"github.com/gin-gonic/gin"
)

// AdminHandler 后台登录接口。
//
// 站点分两种身份：访客只能浏览与播放音乐库；管理额外能进后台
// （歌词库、歌词发现、曲库增删改）。这里只负责「证明你是管理」，
// 具体哪些接口要管理员，由路由层挂 middleware.Admin.Middleware() 决定。
type AdminHandler struct {
	admin *middleware.Admin
}

// NewAdminHandler 构造后台登录处理器。
func NewAdminHandler(admin *middleware.Admin) *AdminHandler {
	return &AdminHandler{admin: admin}
}

// Login POST /api/admin/login — 口令正确则下发会话 Cookie。
func (h *AdminHandler) Login(c *gin.Context) {
	if !h.admin.Enabled() {
		c.JSON(http.StatusForbidden, gin.H{"error": "未配置 admin.password，后台管理已禁用"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if !h.admin.CheckPassword(body.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "口令不正确"})
		return
	}
	h.admin.Issue(c)
	c.JSON(http.StatusOK, gin.H{"authenticated": true})
}

// Logout POST /api/admin/logout — 清掉会话 Cookie。
func (h *AdminHandler) Logout(c *gin.Context) {
	h.admin.Clear(c)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

// Session GET /api/admin/session — 当前是不是管理。
//
// 前端用它决定要不要显示「后台」入口：enabled=false 表示压根没配口令，
// 这时即便登录页也不该出现（登录了也没用）。
func (h *AdminHandler) Session(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":       h.admin.Enabled(),
		"authenticated": h.admin.Authenticated(c),
	})
}
