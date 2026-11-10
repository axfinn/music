package handlers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"music-lyrics-archive/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 歌词抓取与对外接口的 HTTP 层 ──
// 站内(/api/lyrics/fetch*)沿用项目里 `{"item": ...}` 的朴素返回；
// 对外(/api/v1/lyrics/*)统一 `{"ok": bool, "data": ..., "meta": ...}`，
// 因为外系统按这个信封写解析更省事，失败时 `error` 一定有值。

const lyricAPIKeyCtx = "lyric_api_key"

// AttachFetcher 注入抓取器；不注入则相关接口返回"组件不可用"而不 panic。
func (h *LyricsHandler) AttachFetcher(f *LyricsFetcher) *LyricsHandler {
	h.fetcher = f
	return h
}

// ── 站内：抓取 ──

type lyricFetchRequestBody struct {
	Song         string   `json:"song"`
	Artist       string   `json:"artist"`
	Engine       string   `json:"engine"`
	RawLRC       bool     `json:"raw_lrc"`
	Page         int      `json:"page"`
	StripCredits *bool    `json:"strip_credits"`
	Save         bool     `json:"save"`
	Language     string   `json:"language"`
	Genre        string   `json:"genre"`
	Era          string   `json:"era"`
	Tags         []string `json:"tags"`
	Note         string   `json:"note"`
}

// FetchStatus GET /api/lyrics/fetch/status — 前端据此决定是否显示"在线抓取"。
func (h *LyricsHandler) FetchStatus(c *gin.Context) {
	ready := h.fetcher != nil && h.fetcher.Ready()
	engines := []string{"auto", "baidu", "card", "page", "netease", "qq"}
	c.JSON(http.StatusOK, gin.H{
		"ready":   ready,
		"script":  h.fetcher.ScriptInfo(),
		"engines": engines,
	})
}

// FetchLyrics POST /api/lyrics/fetch — 抓一首，可选直接入库。
func (h *LyricsHandler) FetchLyrics(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "歌词抓取组件未就绪：找不到 lyrics_finder.py 或 python 解释器",
		})
		return
	}
	var body lyricFetchRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	result, err := h.fetcher.Fetch(c.Request.Context(), lyricFetchQuery{
		Song: body.Song, Artist: body.Artist, Engine: body.Engine,
		RawLRC: body.RawLRC, Page: body.Page,
	})
	if err != nil {
		writeLyricFetchError(c, err)
		return
	}
	strip := true
	if body.StripCredits != nil {
		strip = *body.StripCredits
	}
	applyLyricCleanup(result, strip, body.Song, body.Artist)

	resp := gin.H{"item": result, "saved": nil, "duplicate": false, "existing_id": ""}
	if body.Save {
		song, dupExisting, saveErr := h.saveFetchedSong(result, body)
		if saveErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "歌词已抓到，但入库失败: " + saveErr.Error(), "item": result})
			return
		}
		if dupExisting != "" {
			resp["duplicate"] = true
			resp["existing_id"] = dupExisting
		} else {
			resp["saved"] = song
		}
	}
	c.JSON(http.StatusOK, resp)
}

// FetchLyricsBatch POST /api/lyrics/fetch/batch — 批量抓取，一次进程多首。
func (h *LyricsHandler) FetchLyricsBatch(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "歌词抓取组件未就绪：找不到 lyrics_finder.py 或 python 解释器",
		})
		return
	}
	var req struct {
		Songs        []string `json:"songs"`
		Engine       string   `json:"engine"`
		RawLRC       bool     `json:"raw_lrc"`
		StripCredits *bool    `json:"strip_credits"`
		Save         bool     `json:"save"`
		Language     string   `json:"language"`
		Genre        string   `json:"genre"`
		Era          string   `json:"era"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if len(req.Songs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要抓取的歌曲"})
		return
	}
	if len(req.Songs) > lyricFetchMaxSongs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次最多抓取 " + strconv.Itoa(lyricFetchMaxSongs) + " 首"})
		return
	}

	queries := make([]lyricFetchQuery, 0, len(req.Songs))
	for _, raw := range req.Songs {
		song, artist := splitSongLine(raw)
		queries = append(queries, lyricFetchQuery{
			Song: song, Artist: artist, Engine: req.Engine, RawLRC: req.RawLRC,
		})
	}

	items, err := h.fetcher.FetchBatch(c.Request.Context(), queries)
	if err != nil {
		writeLyricFetchError(c, err)
		return
	}

	strip := true
	if req.StripCredits != nil {
		strip = *req.StripCredits
	}
	out := make([]gin.H, 0, len(items))
	okCount, failCount, savedCount := 0, 0, 0
	for i, it := range items {
		entry := gin.H{"query": it.Query, "ok": it.Error == "", "item": nil, "error": it.Error, "saved": nil, "duplicate": false}
		if it.Error == "" {
			res := it.toResult()
			applyLyricCleanup(res, strip, queries[i].Song, queries[i].Artist)
			entry["item"] = res
			okCount++
			if req.Save {
				song, dupExisting, saveErr := h.saveFetchedSong(res, lyricFetchRequestBody{
					Song: res.Song, Artist: res.Singer, Language: req.Language,
					Genre: req.Genre, Era: req.Era, Save: true,
				})
				switch {
				case saveErr != nil:
					entry["error"] = "入库失败: " + saveErr.Error()
				case dupExisting != "":
					entry["duplicate"] = true
					entry["existing_id"] = dupExisting
				default:
					entry["saved"] = song
					savedCount++
				}
			}
		} else {
			failCount++
		}
		out = append(out, entry)
	}
	c.JSON(http.StatusOK, gin.H{
		"items": out, "ok_count": okCount, "failed_count": failCount, "saved_count": savedCount,
	})
}

func writeLyricFetchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrLyricFetchTimeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": err.Error()})
	case errors.Is(err, ErrLyricFetchUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, ErrLyricFetchNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		// 参数类错误（歌名空、引擎非法等）
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

// applyLyricCleanup 在结果上做统一清洗：剔署名行、按需重算行数、粗判语种。
func applyLyricCleanup(res *lyricFetchResult, strip bool, querySong, queryArtist string) {
	if res == nil {
		return
	}
	if strip {
		res.Lyrics = CleanLyricLines(res.Lyrics, firstNonEmpty(res.Song, querySong), firstNonEmpty(res.Singer, queryArtist))
	}
	res.LineCount = models.CountLyricLines(res.Lyrics)
	if res.Song == "" {
		res.Song = querySong
	}
	if res.Singer == "" {
		res.Singer = queryArtist
	}
}

// saveFetchedSong 把抓取结果落库；已存在同名歌时返回其 id（不覆盖用户编辑过的内容）。
func (h *LyricsHandler) saveFetchedSong(res *lyricFetchResult, body lyricFetchRequestBody) (*models.LyricSong, string, error) {
	title := strings.TrimSpace(firstNonEmpty(res.Song, body.Song))
	artist := strings.TrimSpace(firstNonEmpty(res.Singer, body.Artist))
	if title == "" {
		return nil, "", errors.New("抓取结果没有歌名")
	}
	if existing, err := h.db.FindLyricSongByArtistTitle(artist, title); err == nil && existing != nil {
		return nil, existing.ID, nil
	}
	source := "在线抓取"
	if res.Source != "" {
		source = "在线抓取 · " + res.Source
	}
	if res.Engine != "" {
		source += " [" + res.Engine + "]"
	}
	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "自动抓取于 " + time.Now().Format("2006-01-02")
		if res.URL != "" {
			note += "，来源 " + res.URL
		}
	}
	song := &models.LyricSong{
		Title:    title,
		Artist:   artist,
		Album:    res.Album,
		Language: firstNonEmpty(body.Language, DetectLyricLanguage(res.Lyrics)),
		Genre:    body.Genre,
		Era:      body.Era,
		Tags:     body.Tags,
		Lyrics:   res.Lyrics,
		Note:     note,
		Source:   source,
	}
	if err := h.db.CreateLyricSong(song); err != nil {
		if errors.Is(err, models.ErrLyricSongDuplicate) {
			if existing, findErr := h.db.FindLyricSongByArtistTitle(artist, title); findErr == nil && existing != nil {
				return nil, existing.ID, nil
			}
		}
		return nil, "", err
	}
	return song, "", nil
}

// ── 站内：API 密钥管理（管理员密码门） ──

func (h *LyricsHandler) requireLyricAdmin(c *gin.Context) bool {
	pwd := strings.TrimSpace(h.cfg.Lyrics.AdminPassword)
	if pwd == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "未配置 lyrics.admin_password，密钥管理接口已禁用"})
		return false
	}
	given := c.GetHeader("X-Admin-Password")
	if given == "" {
		given = c.Query("admin_password")
	}
	if subtle.ConstantTimeCompare([]byte(given), []byte(pwd)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "管理员密码不正确"})
		return false
	}
	return true
}

// ListAPIKeys GET /api/lyrics/api-keys
func (h *LyricsHandler) ListAPIKeys(c *gin.Context) {
	if !h.requireLyricAdmin(c) {
		return
	}
	keys, err := h.db.ListLyricAPIKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": keys, "base_url": "/api/v1/lyrics"})
}

// CreateAPIKey POST /api/lyrics/api-keys — 明文只在这里返回一次。
func (h *LyricsHandler) CreateAPIKey(c *gin.Context) {
	if !h.requireLyricAdmin(c) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "未命名"
	}
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60])
	}
	key, plain, err := h.db.CreateLyricAPIKey(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": key, "plain": plain})
}

// UpdateAPIKey PATCH /api/lyrics/api-keys/:id — 目前只支持启用/撤销。
func (h *LyricsHandler) UpdateAPIKey(c *gin.Context) {
	if !h.requireLyricAdmin(c) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请传 enabled"})
		return
	}
	if err := h.db.SetLyricAPIKeyEnabled(c.Param("id"), *req.Enabled); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "密钥不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": *req.Enabled})
}

// DeleteAPIKey DELETE /api/lyrics/api-keys/:id
func (h *LyricsHandler) DeleteAPIKey(c *gin.Context) {
	if !h.requireLyricAdmin(c) {
		return
	}
	if err := h.db.DeleteLyricAPIKey(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "密钥不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── 对外 /api/v1/lyrics ──

// APIKeyAuth 对外接口的密钥门。支持 `X-API-Key` 与 `Authorization: Bearer`。
// 限流按密钥计数（同一把 key 多台机器共用也共享额度）。
func (h *LyricsHandler) APIKeyAuth() gin.HandlerFunc {
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
		c.Set(lyricAPIKeyCtx, key)
		c.Next()
	}
}

func extractLyricAPIKey(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(c.Query("api_key"))
}

func v1OK(c *gin.Context, data interface{}, meta gin.H) {
	body := gin.H{"ok": true, "data": data}
	if meta != nil {
		body["meta"] = meta
	}
	c.JSON(http.StatusOK, body)
}

func v1Fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"ok": false, "error": msg, "code": code})
}

// V1CORS 对外接口的跨域放行：凭据是密钥而不是 Cookie，所以默认放开来源。
// 配置 lyrics.allow_origins 可收紧白名单。
func (h *LyricsHandler) V1CORS() gin.HandlerFunc {
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

// V1Ping GET /api/v1/lyrics/ping — 鉴权自检，同时看到这把 key 的用量。
func (h *LyricsHandler) V1Ping(c *gin.Context) {
	key, _ := c.Get(lyricAPIKeyCtx)
	info := gin.H{"ready": h.fetcher != nil && h.fetcher.Ready()}
	if k, ok := key.(*models.LyricAPIKey); ok && k != nil {
		info["key_name"] = k.Name
		info["key_prefix"] = k.Prefix
		info["call_count"] = k.CallCount
		info["rate_limit_per_minute"] = h.cfg.Lyrics.APIKeyRateLimit
	}
	v1OK(c, info, nil)
}

// V1Fetch POST /api/v1/lyrics/fetch
func (h *LyricsHandler) V1Fetch(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		v1Fail(c, http.StatusServiceUnavailable, "抓取组件未就绪")
		return
	}
	var body lyricFetchRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		v1Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	started := time.Now()
	result, err := h.fetcher.Fetch(c.Request.Context(), lyricFetchQuery{
		Song: body.Song, Artist: body.Artist, Engine: body.Engine,
		RawLRC: body.RawLRC, Page: body.Page,
	})
	if err != nil {
		writeV1FetchError(c, err)
		return
	}
	strip := true
	if body.StripCredits != nil {
		strip = *body.StripCredits
	}
	applyLyricCleanup(result, strip, body.Song, body.Artist)

	data := gin.H{"item": result, "saved": nil, "duplicate": false, "existing_id": ""}
	if body.Save {
		song, dupExisting, saveErr := h.saveFetchedSong(result, body)
		if saveErr != nil {
			v1Fail(c, http.StatusInternalServerError, "入库失败: "+saveErr.Error())
			return
		}
		if dupExisting != "" {
			data["duplicate"] = true
			data["existing_id"] = dupExisting
		} else {
			data["saved"] = song
		}
	}
	v1OK(c, data, gin.H{"engine": result.Engine, "elapsed_ms": time.Since(started).Milliseconds()})
}

// V1FetchBatch POST /api/v1/lyrics/fetch/batch
func (h *LyricsHandler) V1FetchBatch(c *gin.Context) {
	if h.fetcher == nil || !h.fetcher.Ready() {
		v1Fail(c, http.StatusServiceUnavailable, "抓取组件未就绪")
		return
	}
	var req struct {
		Songs        []string `json:"songs"`
		Engine       string   `json:"engine"`
		RawLRC       bool     `json:"raw_lrc"`
		StripCredits *bool    `json:"strip_credits"`
		Save         bool     `json:"save"`
		Language     string   `json:"language"`
		Genre        string   `json:"genre"`
		Era          string   `json:"era"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		v1Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	if len(req.Songs) == 0 {
		v1Fail(c, http.StatusBadRequest, "songs 不能为空")
		return
	}
	if len(req.Songs) > lyricFetchMaxSongs {
		v1Fail(c, http.StatusBadRequest, "单次最多 "+strconv.Itoa(lyricFetchMaxSongs)+" 首")
		return
	}
	queries := make([]lyricFetchQuery, 0, len(req.Songs))
	for _, raw := range req.Songs {
		song, artist := splitSongLine(raw)
		queries = append(queries, lyricFetchQuery{Song: song, Artist: artist, Engine: req.Engine, RawLRC: req.RawLRC})
	}
	started := time.Now()
	items, err := h.fetcher.FetchBatch(c.Request.Context(), queries)
	if err != nil {
		writeV1FetchError(c, err)
		return
	}
	strip := true
	if req.StripCredits != nil {
		strip = *req.StripCredits
	}
	out := make([]gin.H, 0, len(items))
	okCount, failCount, savedCount := 0, 0, 0
	for i, it := range items {
		entry := gin.H{"query": it.Query, "ok": it.Error == "", "error": it.Error, "saved": nil, "duplicate": false}
		if it.Error == "" {
			res := it.toResult()
			applyLyricCleanup(res, strip, queries[i].Song, queries[i].Artist)
			entry["item"] = res
			okCount++
			if req.Save {
				song, dupExisting, saveErr := h.saveFetchedSong(res, lyricFetchRequestBody{
					Song: res.Song, Artist: res.Singer, Language: req.Language, Genre: req.Genre, Era: req.Era, Save: true,
				})
				switch {
				case saveErr != nil:
					entry["error"] = "入库失败: " + saveErr.Error()
				case dupExisting != "":
					entry["duplicate"] = true
					entry["existing_id"] = dupExisting
				default:
					entry["saved"] = song
					savedCount++
				}
			}
		} else {
			failCount++
		}
		out = append(out, entry)
	}
	v1OK(c, gin.H{"items": out}, gin.H{
		"ok_count": okCount, "failed_count": failCount, "saved_count": savedCount,
		"elapsed_ms": time.Since(started).Milliseconds(),
	})
}

func writeV1FetchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrLyricFetchTimeout):
		v1Fail(c, http.StatusGatewayTimeout, err.Error())
	case errors.Is(err, ErrLyricFetchUnavailable):
		v1Fail(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrLyricFetchNotFound):
		v1Fail(c, http.StatusNotFound, err.Error())
	default:
		v1Fail(c, http.StatusBadRequest, err.Error())
	}
}

// V1ListSongs GET /api/v1/lyrics/songs
func (h *LyricsHandler) V1ListSongs(c *gin.Context) {
	items, total, err := h.db.ListLyricSongs(models.LyricFilter{
		Keyword: c.Query("keyword"), Artist: c.Query("artist"),
		Language: c.Query("language"), Genre: c.Query("genre"),
		Tag: c.Query("tag"), Era: c.Query("era"),
		Sort:   c.DefaultQuery("sort", ""),
		Limit:  parseIntDefault(c.Query("limit"), 50),
		Offset: parseIntDefault(c.Query("offset"), 0),
	})
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "查询失败: "+err.Error())
		return
	}
	v1OK(c, gin.H{"items": items}, gin.H{"total": total})
}

// V1GetSong GET /api/v1/lyrics/songs/:id
func (h *LyricsHandler) V1GetSong(c *gin.Context) {
	song, err := h.db.GetLyricSong(c.Param("id"))
	if err != nil {
		v1Fail(c, http.StatusNotFound, "歌词不存在")
		return
	}
	v1OK(c, song, nil)
}

// V1CreateSong POST /api/v1/lyrics/songs — 外系统把自有歌词直接推进库（不经过抓取）。
func (h *LyricsHandler) V1CreateSong(c *gin.Context) {
	var p lyricSongPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		v1Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	song := &models.LyricSong{}
	if err := p.apply(song); err != nil {
		v1Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if song.Source == "" {
		song.Source = "外部接口"
	}
	if err := h.db.CreateLyricSong(song); err != nil {
		if errors.Is(err, models.ErrLyricSongDuplicate) {
			v1Fail(c, http.StatusConflict, "「"+song.Artist+" - "+song.Title+"」已存在")
			return
		}
		v1Fail(c, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": song})
}

// V1Search GET /api/v1/lyrics/search?q= — 行级检索，返回"哪首歌第几行"。
func (h *LyricsHandler) V1Search(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		keyword = strings.TrimSpace(c.Query("keyword"))
	}
	if keyword == "" {
		v1Fail(c, http.StatusBadRequest, "请传 q")
		return
	}
	hits, total, err := h.db.SearchLyricLines(keyword, parseIntDefault(c.Query("limit"), 50))
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "检索失败: "+err.Error())
		return
	}
	v1OK(c, gin.H{"keyword": keyword, "items": hits}, gin.H{"total": total})
}

// V1Stats GET /api/v1/lyrics/stats
func (h *LyricsHandler) V1Stats(c *gin.Context) {
	stats, err := h.db.LyricStats()
	if err != nil {
		v1Fail(c, http.StatusInternalServerError, "统计失败: "+err.Error())
		return
	}
	v1OK(c, stats, nil)
}

// ── 小工具 ──

// splitSongLine 解析 "歌名 - 艺人" / "歌名|艺人" / "歌名 艺人"，与脚本 parse_song_line 同规则。
func splitSongLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	for _, sep := range []string{" - ", " – ", "|", "——", "\t"} {
		if idx := strings.Index(line, sep); idx > 0 {
			return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+len(sep):])
		}
	}
	return line, ""
}

// firstNonEmpty 取首个非空字符串（逐参 strip 后）。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
