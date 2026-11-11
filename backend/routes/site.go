package routes

import (
	"net/http"

	"github.com/axfinn/music/backend/config"

	"github.com/gin-gonic/gin"
)

// RegisterSiteRoutes 公开站点信息。
//
// 独立站的品牌（站名 / 标语 / 域名）由配置决定，前端启动后拉一次这个接口来渲染
// 顶栏、document.title 与分享卡片，换站名不用重新构建前端。
// 这里只回公开信息，不回管理口令、密钥、磁盘路径等任何私有数据。
func RegisterSiteRoutes(api *gin.RouterGroup, cfg *config.Config) {
	api.GET("/site", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":          cfg.Site.Name,
			"tagline":       cfg.Site.Tagline,
			"description":   cfg.Site.Description,
			"base_url":      cfg.Site.BaseURL,
			"admin_enabled": cfg.Admin.Password != "",
			"api": gin.H{
				"music":     "/api/music",
				"lyrics":    "/api/lyrics",
				"v1_music":  "/api/v1/music",
				"v1_lyrics": "/api/v1/lyrics",
			},
		})
	})
}
