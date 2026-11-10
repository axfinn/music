package routes

import (
	"net/http"

	"github.com/axfinn/music/backend/handlers"
	"github.com/axfinn/music/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterLyricsRoutes 歌词库接口(均在 /api/lyrics 下)。
// 读接口不加限流(个人内网工具);写接口复用统一的 createRateLimiter。
func RegisterLyricsRoutes(api *gin.RouterGroup, h *handlers.LyricsHandler, createRateLimiter *middleware.RateLimiter) {
	l := api.Group("/lyrics")
	{
		l.GET("/songs", h.ListSongs)
		l.POST("/songs", createRateLimiter.Middleware(), h.CreateSong)
		// 批量导入走独立路径,避免与 /songs/:id 在同一路径段混合静态与通配。
		l.POST("/import", createRateLimiter.Middleware(), h.BulkCreateSongs)
		l.GET("/songs/:id", h.GetSong)
		l.PATCH("/songs/:id", h.UpdateSong)
		l.DELETE("/songs/:id", h.DeleteSong)
		l.POST("/songs/:id/favorite", h.ToggleFavorite)
		l.GET("/songs/:id/prompt", h.BuildPrompt)
		// 与音乐库联动(反向查找)
		l.GET("/songs/:id/music-match", h.GetMusicMatch)
		l.GET("/music-links", h.ListMusicLinks)

		l.GET("/search/lines", h.SearchLines)
		l.GET("/stats", h.GetStats)
		l.GET("/artists", h.ListArtists)
		l.GET("/words", h.GetWordFrequency)
		l.GET("/random", h.RandomSong)
		l.GET("/export", h.ExportSongs)

		l.GET("/fragments", h.ListFragments)
		l.POST("/fragments", createRateLimiter.Middleware(), h.CreateFragment)
		l.GET("/fragments/random", h.RandomFragment)
		l.PATCH("/fragments/:id", h.UpdateFragment)
		l.DELETE("/fragments/:id", h.DeleteFragment)
		l.POST("/fragments/:id/use", h.MarkFragmentUsed)

		// ── 在线抓取(内置 lyrics_finder.py) ──
		// 站内沿用现有开放策略(与歌词库其他接口一致);抓取要起外部进程,单独限流。
		l.GET("/fetch/status", h.FetchStatus)
		l.POST("/fetch", createRateLimiter.Middleware(), h.FetchLyrics)
		l.POST("/fetch/batch", createRateLimiter.Middleware(), h.FetchLyricsBatch)

		// ── 对外 API 密钥管理(管理员密码门,密码在 lyrics.admin_password) ──
		l.GET("/api-keys", h.ListAPIKeys)
		l.POST("/api-keys", h.CreateAPIKey)
		l.PATCH("/api-keys/:id", h.UpdateAPIKey)
		l.DELETE("/api-keys/:id", h.DeleteAPIKey)
	}

	// ── 歌词发现 ──
	// 单独一个组而不是塞进 /lyrics：发现是"逛"，
	// 不是"查某一首歌"，路径语义不一样，接口版本演进也能分开走。
	d := api.Group("/discovery")
	{
		d.GET("/feed", h.DiscoveryFeed)           // 本地 + 在线混合的发现流
		d.GET("/status", h.DiscoveryStatus)       // 池状态与种子配置
		d.GET("/online", h.DiscoveryOnlineSearch) // 按歌名/艺人在线搜
		d.POST("/save", h.DiscoverySave)          // 把在线结果收进库
		d.POST("/refresh", h.DiscoveryRefresh)    // 强制刷新候选池（异步）
	}
}

// RegisterLyricsAPIRoutes 对外接口 /api/v1/lyrics/*。
// 与站内接口刻意分开：站内是"自己用"，这里是"给别的系统用"，签名/限流/返回信封都不同。
// 注意：/ping 是公开自检接口（无需密钥，便于外部探活），单独挂在只带 CORS 的组下；
// 其余接口全部走 API Key 鉴权。
func RegisterLyricsAPIRoutes(api *gin.RouterGroup, h *handlers.LyricsHandler) {
	pub := api.Group("/v1/lyrics", h.V1CORS())
	pub.GET("/ping", h.V1Ping)

	v1 := api.Group("/v1/lyrics", h.V1CORS(), h.APIKeyAuth())
	{
		// 浏览器的预检请求不带密钥，必须在鉴权之前就地返回。
		// Gin 只有命中路由才会跑组中间件，所以这里补一条 OPTIONS 兜底。
		v1.OPTIONS("/*any", func(c *gin.Context) { c.AbortWithStatus(http.StatusNoContent) })
		v1.POST("/fetch", h.V1Fetch)
		v1.POST("/fetch/batch", h.V1FetchBatch)
		v1.GET("/songs", h.V1ListSongs)
		v1.POST("/songs", h.V1CreateSong)
		v1.GET("/songs/:id", h.V1GetSong)
		v1.GET("/search", h.V1Search)
		v1.GET("/stats", h.V1Stats)
	}
}
