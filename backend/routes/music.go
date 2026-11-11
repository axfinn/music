package routes

import (
	"net/http"

	"github.com/axfinn/music/backend/handlers"
	"github.com/axfinn/music/backend/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterMusicRoutes 音乐库接口(均在 /api/music 下)。
//
// 分两类:
//   - 公开只读:列表 / 详情 / 统计 / 快照 / 音频流 / 封面 —— 访客听歌要用。
//   - 管理动作:上传 / 改 / 删 / 关联歌词 —— 挂在管理门后面。
//
// 上传与封面单独限流(200 次/分钟),其余管理动作走管理身份。
func RegisterMusicRoutes(api *gin.RouterGroup, h *handlers.MusicHandler, limiter *middleware.RateLimiter, admin *middleware.Admin) {
	m := api.Group("/music")
	{
		// ── 公开:访客听歌 ──
		m.GET("/tracks", h.ListTracks)
		m.GET("/tracks/:id", h.GetTrack)
		m.GET("/stats", h.GetStats)
		// 首屏一次性拿齐全量精简曲库 + 统计;带 rev 可命中增量(只回 not_modified)
		m.GET("/snapshot", h.Snapshot)
		m.GET("/stream/:id", h.StreamTrack)
		m.GET("/cover/:id", h.ServeCover)
		// 正在听的这首歌的歌词:属于听歌本身,访客要能看
		m.GET("/tracks/:id/lyrics", h.GetTrackLyrics)
		// 播放计数:听歌本身的行为,访客也要能累加
		m.POST("/tracks/:id/played", h.MarkPlayed)
		// 收藏:「我喜欢的」是听歌时的顺手操作,访客可以点
		m.POST("/tracks/:id/favorite", h.ToggleFavorite)

		// ── 管理:改曲库 ──
		adminM := admin.Middleware()
		m.POST("/tracks", limiter.Middleware(), adminM, h.UploadTrack)
		m.PATCH("/tracks/:id", adminM, h.UpdateTrack)
		m.POST("/tracks/:id/cover", limiter.Middleware(), adminM, h.UploadCover)
		m.DELETE("/tracks/:id", adminM, h.DeleteTrack)
		// 歌词库联动(管理动作)
		m.GET("/tracks/:id/lyric-match", adminM, h.LyricMatch)
		m.POST("/tracks/:id/lyrics", adminM, h.LinkLyricSong)
		m.POST("/tracks/:id/lyrics/auto", adminM, h.AutoMatchLyric)
		m.DELETE("/tracks/:id/lyrics", adminM, h.UnlinkLyricSong)
		// 批量按歌名自动匹配(只处理尚未关联的曲目)
		m.POST("/lyrics/auto-match", adminM, h.AutoMatchAllLyrics)
		// 音频文件体检:回填缺失的 filename,报告无主文件(不删)
		m.POST("/maintenance/repair", adminM, h.RepairFiles)
	}

	// 启动时后台跑一次体检,把历史 filename 为空/对不上的记录自动修好。
	// 失败/无目录都静默:自愈是兜底手段,不能影响服务启动。
	go h.SweepMusicFiles()
}

// RegisterMusicAPIRoutes 对外接口 /api/v1/music/*（与站内 /api/music 刻意分开）。
// apiV1 与歌词库 v1 是同一个实例：同一把密钥、同一套 CORS、同一个限流额度。
// /ping 走可选鉴权（不带密钥可探活，带了就报密钥用量），其余全部强制鉴权。
func RegisterMusicAPIRoutes(api *gin.RouterGroup, h *handlers.MusicHandler, apiV1 *middleware.V1API) {
	pub := api.Group("/v1/music", apiV1.CORS(), apiV1.OptionalKey())
	pub.GET("/ping", h.V1Ping)

	v1 := api.Group("/v1/music", apiV1.CORS(), apiV1.RequireKey())
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
