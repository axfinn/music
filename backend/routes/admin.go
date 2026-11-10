package routes

import (
	"github.com/axfinn/music/backend/handlers"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 后台身份接口(均在 /api/admin 下)。
//
// 站点分两种身份:访客只能浏览与播放音乐库;管理额外能进后台
// (歌词库 / 歌词发现 / 曲库增删改)。这里只负责「证明你是管理」,
// 哪些接口要管理员由各模块自己挂 middleware.Admin.Middleware()。
//
// 登录/登出/查会话本身必须公开 —— 否则前端连「我是谁」都问不出来。
func RegisterAdminRoutes(api *gin.RouterGroup, h *handlers.AdminHandler) {
	a := api.Group("/admin")
	{
		a.POST("/login", h.Login)
		a.POST("/logout", h.Logout)
		a.GET("/session", h.Session)
	}
}
