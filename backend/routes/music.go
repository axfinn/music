package routes

import (
	"net/http"

	"music-lyrics-archive/backend/handlers"
	"music-lyrics-archive/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterMusicRoutes 音乐库接口(均在 /api/music 下)。
// 播放/列表公开只读;上传走独立限流。
func RegisterMusicRoutes(api *gin.RouterGroup, h *handlers.MusicHandler, limiter *middleware.RateLimiter) {
	m := api.Group("/music")
	{
		m.GET("/tracks", h.ListTracks)
		m.GET("/tracks/:id", h.GetTrack)
		m.GET("/stats", h.GetStats)
		// 首屏一次性拿齐全量精简曲库 + 统计;带 rev 可命中增量(只回 not_modified)
		m.GET("/snapshot", h.Snapshot)
		m.POST("/tracks", limiter.Middleware(), h.UploadTrack)
		m.PATCH("/tracks/:id", h.UpdateTrack)
		m.POST("/tracks/:id/favorite", h.ToggleFavorite)
		m.POST("/tracks/:id/played", h.MarkPlayed)
		m.POST("/tracks/:id/cover", limiter.Middleware(), h.UploadCover)
		m.DELETE("/tracks/:id", h.DeleteTrack)
		m.GET("/stream/:id", h.StreamTrack)
		m.GET("/cover/:id", h.ServeCover)
		// 歌词库联动
		m.GET("/tracks/:id/lyric-match", h.LyricMatch)
		m.POST("/tracks/:id/lyrics", h.LinkLyricSong)
		m.POST("/tracks/:id/lyrics/auto", h.AutoMatchLyric)
		m.DELETE("/tracks/:id/lyrics", h.UnlinkLyricSong)
		// 批量按歌名自动匹配(只处理尚未关联的曲目)
		m.POST("/lyrics/auto-match", h.AutoMatchAllLyrics)
		// 音频文件体检:回填缺失的 filename,报告无主文件(不删)
		m.POST("/maintenance/repair", h.RepairFiles)
	}

	// 启动时后台跑一次体检,把历史 filename 为空/对不上的记录自动修好。
	// 失败/无目录都静默:自愈是兜底手段,不能影响服务启动。
	go h.SweepMusicFiles()
}

// RegisterMusicAPIRoutes 对外接口 /api/v1/music/*（与站内 /api/music 刻意分开）。
// 对称于歌词库：/ping 是公开自检（只挂 CORS，不鉴权），其余全部走 API Key 鉴权。
// 鉴权与 CORS 复用 lyric_api_keys 表与 lyrics 同一套逻辑。
func RegisterMusicAPIRoutes(api *gin.RouterGroup, h *handlers.MusicHandler) {
	pub := api.Group("/v1/music", h.V1CORS())
	pub.GET("/ping", h.V1Ping)

	v1 := api.Group("/v1/music", h.V1CORS(), h.APIKeyAuth())
	{
		// 浏览器预检请求不带密钥，必须在鉴权前就地返回；
		// Gin 只有命中路由才会跑组中间件，所以补一条 OPTIONS 兜底。
		v1.OPTIONS("/*any", func(c *gin.Context) { c.AbortWithStatus(http.StatusNoContent) })
		v1.GET("/tracks", h.V1ListTracks)
		v1.GET("/tracks/:id", h.V1GetTrack)
		v1.GET("/snapshot", h.V1Snapshot)
		v1.GET("/search", h.V1Search)
		v1.GET("/stream/:id", h.V1Stream)
	}
}
