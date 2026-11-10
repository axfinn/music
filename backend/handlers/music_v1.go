package handlers

import (
	"net/http"
	"strings"

	"music-lyrics-archive/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 音乐库对外接口 /api/v1/music ──
// 与歌词库对外接口对称：同样的 API Key 鉴权（复用 lyrics 的 lyric_api_keys 表）、
// 同样的 CORS 策略（读 h.cfg.Lyrics.AllowOrigins）、同样的 {"ok":bool,"data":...} 信封。
// 站内 /api/music/* 不变，这里只新增一套"给别的系统用"的对称接口。

const musicAPIKeyCtx = "music_api_key"

// V1CORS 对外接口的跨域放行，逻辑照搬歌词库 V1CORS：
// 凭据是密钥而不是 Cookie，默认放开来源；配置 lyrics.allow_origins 可收紧白名单。
func (h *MusicHandler) V1CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := "*"
		if list := h.cfg.Lyrics.AllowOrigins; len(list) > 0 {
			allowed = ""
			for _, o := range list {
				if strings.EqualFold(strings.TrimSpace(o), origin) || strings.TrimSpace(o) == "*" {
					allowed = origin
					break
				}
			}
		}
		if allowed != "" {
			c.Header("Access-Control-Allow-Origin", allowed)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// APIKeyAuth 对外接口的密钥门：复用 lyric_api_keys 表与 lyrics 同一套校验方法
// （models.VerifyLyricAPIKey 会校验有效性、是否撤销，并累加调用次数）。
// 支持 `X-API-Key` 与 `Authorization: Bearer`，失败返回 401。
func (h *MusicHandler) APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractLyricAPIKey(c)
		if raw == "" {
			v1Fail(c, http.StatusUnauthorized, "缺少密钥：请用 X-API-Key 或 Authorization: Bearer <key>")
			return
		}
		key, err := h.db.VerifyLyricAPIKey(raw)
		if err != nil {
			v1Fail(c, http.StatusUnauthorized, "密钥无效或已撤销")
			return
		}
		if h.keyLimiter != nil && !h.keyLimiter.Allow(key.ID) {
			v1Fail(c, http.StatusTooManyRequests, "该密钥请求过于频繁，请稍后再试")
			return
		}
		c.Set(musicAPIKeyCtx, key)
		c.Next()
	}
}

// V1Ping GET /api/v1/music/ping — 公开自检（只挂 CORS，不鉴权）。
func (h *MusicHandler) V1Ping(c *gin.Context) {
	v1OK(c, gin.H{"ready": true}, nil)
}

// V1ListTracks GET /api/v1/music/tracks — 精简列表（keyword/fav/sort/limit/offset）。
func (h *MusicHandler) V1ListTracks(c *gin.Context) {
	keyword, favOnly, sortBy, limit, offset := parseListParams(c, 100)
	items, err := h.db.ListMusicTrackBriefs(keyword, favOnly, sortBy, limit, offset)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	h.annotateBriefPlayable(items)
	total, err := h.db.CountMusicTracks(keyword, favOnly)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "统计失败: "+err.Error())
		return
	}
	v1OK(c, gin.H{"items": items, "total": total}, nil)
}

// V1Snapshot GET /api/v1/music/snapshot — 全量精简曲库 + 统计，带 rev 增量语义（照搬 Snapshot）。
func (h *MusicHandler) V1Snapshot(c *gin.Context) {
	keyword, favOnly, sortBy, _, _ := parseListParams(c, musicSnapshotCap)
	token := h.revisionToken(keyword, favOnly, sortBy)
	c.Header("X-Music-Revision", token)
	if c.Query("rev") == token {
		v1OK(c, gin.H{"not_modified": true, "revision": token}, nil)
		return
	}
	items, err := h.db.ListAllMusicTrackBriefs(keyword, favOnly, sortBy, musicSnapshotCap)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	h.annotateBriefPlayable(items)
	total, err := h.db.CountMusicTracks(keyword, favOnly)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "统计失败: "+err.Error())
		return
	}
	v1OK(c, gin.H{
		"items":     items,
		"total":     total,
		"stats":     h.cachedStats(),
		"revision":  token,
		"truncated": len(items) >= musicSnapshotCap,
	}, nil)
}

// V1Search GET /api/v1/music/search?keyword= — 按关键词检索曲目，返回 items/total。
func (h *MusicHandler) V1Search(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		keyword = strings.TrimSpace(c.Query("q"))
	}
	limit := parseIntDefault(c.Query("limit"), 50)
	offset := parseIntDefault(c.Query("offset"), 0)
	if limit > musicMaxPageSize {
		limit = musicMaxPageSize
	}
	items, err := h.db.ListMusicTrackBriefs(keyword, false, "", limit, offset)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "检索失败: "+err.Error())
		return
	}
	h.annotateBriefPlayable(items)
	total, err := h.db.CountMusicTracks(keyword, false)
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "统计失败: "+err.Error())
		return
	}
	v1OK(c, gin.H{"items": items, "total": total, "keyword": keyword}, nil)
}

// V1GetTrack GET /api/v1/music/tracks/:id — 详情（含歌词/备注正文）。
func (h *MusicHandler) V1GetTrack(c *gin.Context) {
	track, err := h.db.GetMusicTrack(c.Param("id"))
	if err != nil {
		v1Fail(c, http.StatusNotFound, "曲目不存在")
		return
	}
	h.annotatePlayable([]*models.MusicTrack{track})
	v1OK(c, track, nil)
}

// V1Stream GET /api/v1/music/stream/:id — 复用 StreamTrack 的 Range 流式播放逻辑。
// 音视频流无法套 JSON 信封，直接复用内部实现（其自身已处理 404/Range/ServeContent）。
func (h *MusicHandler) V1Stream(c *gin.Context) {
	h.StreamTrack(c)
}
