package routes

import (
	"github.com/axfinn/music/backend/handlers"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 会员身份接口（均在 /api/auth 下）。
//
// 与管理员的 /api/admin 完全隔离：这里签发的是会员 Cookie，
// 只能证明「你是谁」，不能进后台。发码/注册/登录/找回/会话查询
// 本身必须公开——否则前端连「我是谁」都问不出来。
func RegisterAuthRoutes(api *gin.RouterGroup, h *handlers.AuthHandler) {
	a := api.Group("/auth")
	{
		a.POST("/code", h.SendCode)
		a.POST("/register", h.Register)
		a.POST("/login", h.Login)
		a.POST("/login/code", h.LoginByCode)
		a.POST("/reset", h.ResetPassword)
		a.GET("/session", h.Session)
		a.POST("/logout", h.Logout)
		a.POST("/profile", h.UpdateProfile)
	}
}
