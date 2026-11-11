package handlers

import (
	"net/http"
	"strings"

	"github.com/axfinn/music/backend/middleware"
	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 音乐库对外接口 /api/v1/music ──
// 与歌词库 v1 共用 middleware.V1API 这一个实例：同一把密钥、同一套 CORS 白名单、
// 同一份 {"ok":bool,"data":...} 信封。这里只写音乐库自己的数据端点。

// V1Ping GET /api/v1/music/ping — 公开自检；带合法密钥时附带这把 key 的用量。
func (h *MusicHandler) V1Ping(c *gin.Context) {
	info := gin.H{"ready": true}
	if key, ok := middleware.APIKeyFromContext(c); ok {
		info["key_name"] = key.Name
		info["key_prefix"] = key.Prefix
		info["call_count"] = key.CallCount
		info["rate_limit_per_minute"] = h.cfg.API.KeyRateLimit
	}
	v1OK(c, info, nil)
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
