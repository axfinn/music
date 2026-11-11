package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/axfinn/music/backend/config"
	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 歌词库(Lyrics Library) ──
// 私人歌词档案:收录 → 检索 → 摘句 → 拼装成生成音乐的 prompt。
// 与音乐库(music_tracks)分开:音乐库存音轨,歌词库存储"读过的词",服务于创作参考。

const (
	lyricMaxLyricsLen = 64 << 10 // 单首歌词上限 64KB
	lyricMaxNoteLen   = 4 << 10  // 备注上限 4KB
	lyricMaxBulkItems = 200      // 单次批量导入上限
)

type LyricsHandler struct {
	db        *models.DB
	cfg       *config.Config
	fetcher   *LyricsFetcher // 在线抓取器，可为 nil（未启用/环境缺 python 时接口会明确报错）
	discovery *discoveryPool // 歌词发现的在线候选池（进程内状态）
}

func NewLyricsHandler(db *models.DB, cfg *config.Config) *LyricsHandler {
	return &LyricsHandler{
		db:        db,
		cfg:       cfg,
		fetcher:   NewLyricsFetcher(cfg.Lyrics),
		discovery: &discoveryPool{},
	}
}

// ── 与音乐库联动(反向) ──

// GetMusicMatch GET /api/lyrics/songs/:id/music-match
// 这首歌在音乐库里有没有关联的音轨?有则返回(取第一条),供"在音乐库播放"。
func (h *LyricsHandler) GetMusicMatch(c *gin.Context) {
	id := c.Param("id")
	tracks, err := h.db.MusicTracksByLyricSong(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	if len(tracks) == 0 {
		c.JSON(http.StatusOK, gin.H{"music_track": nil})
		return
	}
	t := tracks[0]
	c.JSON(http.StatusOK, gin.H{"music_track": gin.H{
		"id":     t.ID,
		"title":  t.Title,
		"artist": t.Artist,
	}})
}

// ListMusicLinks GET /api/lyrics/music-links
// 全部关联摘要(lyric_song_id → music_track_id),供歌词库页给已关联的歌打标记。
func (h *LyricsHandler) ListMusicLinks(c *gin.Context) {
	links, err := h.db.MusicLinkSummaries()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": links})
}

// ── 歌曲 CRUD ──

type lyricSongPayload struct {
	Title    string   `json:"title"`
	Artist   string   `json:"artist"`
	Album    string   `json:"album"`
	Language string   `json:"language"`
	Genre    string   `json:"genre"`
	Era      string   `json:"era"`
	Tags     []string `json:"tags"`
	Lyrics   string   `json:"lyrics"`
	Note     string   `json:"note"`
	Source   string   `json:"source"`
	Favorite bool     `json:"favorite"`
}

func (p *lyricSongPayload) apply(s *models.LyricSong) error {
	if strings.TrimSpace(p.Title) == "" {
		return errors.New("歌名不能为空")
	}
	if len(p.Lyrics) > lyricMaxLyricsLen {
		return fmt.Errorf("歌词过长(上限 %dKB)", lyricMaxLyricsLen>>10)
	}
	if len(p.Note) > lyricMaxNoteLen {
		return fmt.Errorf("备注过长(上限 %dKB)", lyricMaxNoteLen>>10)
	}
	s.Title = p.Title
	s.Artist = p.Artist
	s.Album = p.Album
	s.Language = p.Language
	s.Genre = p.Genre
	s.Era = p.Era
	s.Tags = p.Tags
	s.Lyrics = p.Lyrics
	s.Note = p.Note
	s.Source = p.Source
	s.Favorite = p.Favorite
	// 没选语种时按正文粗判一个，用户随后可改；省得在线抓取进来一堆"未分类"。
	if strings.TrimSpace(s.Language) == "" {
		s.Language = DetectLyricLanguage(s.Lyrics)
	}
	return nil
}

// ListSongs GET /api/lyrics/songs
func (h *LyricsHandler) ListSongs(c *gin.Context) {
	filter := models.LyricFilter{
		Keyword:  c.Query("keyword"),
		Artist:   c.Query("artist"),
		Language: c.Query("language"),
		Genre:    c.Query("genre"),
		Tag:      c.Query("tag"),
		Era:      c.Query("era"),
		FavOnly:  c.Query("fav") == "1" || c.Query("fav") == "true",
		Sort:     c.Query("sort"),
		Limit:    parseIntDefault(c.Query("limit"), 50),
		Offset:   parseIntDefault(c.Query("offset"), 0),
	}
	items, total, err := h.db.ListLyricSongs(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// GetSong GET /api/lyrics/songs/:id
func (h *LyricsHandler) GetSong(c *gin.Context) {
	song, err := h.db.GetLyricSong(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "歌词不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": song})
}

// CreateSong POST /api/lyrics/songs
func (h *LyricsHandler) CreateSong(c *gin.Context) {
	var p lyricSongPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	song := &models.LyricSong{}
	if err := p.apply(song); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.CreateLyricSong(song); err != nil {
		if errors.Is(err, models.ErrLyricSongDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": "「" + song.Artist + " - " + song.Title + "」已在库里了"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": song})
}

// BulkCreateSongs POST /api/lyrics/songs/bulk — 批量导入,逐条落库,返回成功/跳过明细
func (h *LyricsHandler) BulkCreateSongs(c *gin.Context) {
	var req struct {
		Items []lyricSongPayload `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可导入的内容"})
		return
	}
	if len(req.Items) > lyricMaxBulkItems {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("单次最多导入 %d 首", lyricMaxBulkItems)})
		return
	}
	created := make([]*models.LyricSong, 0, len(req.Items))
	skipped := make([]gin.H, 0)
	for i, p := range req.Items {
		song := &models.LyricSong{}
		if err := p.apply(song); err != nil {
			skipped = append(skipped, gin.H{"index": i, "title": p.Title, "reason": err.Error()})
			continue
		}
		if err := h.db.CreateLyricSong(song); err != nil {
			reason := err.Error()
			if errors.Is(err, models.ErrLyricSongDuplicate) {
				reason = "已存在同名歌曲"
			}
			skipped = append(skipped, gin.H{"index": i, "title": p.Title, "reason": reason})
			continue
		}
		created = append(created, song)
	}
	c.JSON(http.StatusOK, gin.H{
		"created_count": len(created),
		"skipped_count": len(skipped),
		"items":         created,
		"skipped":       skipped,
	})
}

// UpdateSong PATCH /api/lyrics/songs/:id — 只更新传入字段
func (h *LyricsHandler) UpdateSong(c *gin.Context) {
	song, err := h.db.GetLyricSong(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "歌词不存在"})
		return
	}
	var req struct {
		Title    *string   `json:"title"`
		Artist   *string   `json:"artist"`
		Album    *string   `json:"album"`
		Language *string   `json:"language"`
		Genre    *string   `json:"genre"`
		Era      *string   `json:"era"`
		Tags     *[]string `json:"tags"`
		Lyrics   *string   `json:"lyrics"`
		Note     *string   `json:"note"`
		Source   *string   `json:"source"`
		Favorite *bool     `json:"favorite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "歌名不能为空"})
			return
		}
		song.Title = *req.Title
	}
	if req.Artist != nil {
		song.Artist = *req.Artist
	}
	if req.Album != nil {
		song.Album = *req.Album
	}
	if req.Language != nil {
		song.Language = *req.Language
	}
	if req.Genre != nil {
		song.Genre = *req.Genre
	}
	if req.Era != nil {
		song.Era = *req.Era
	}
	if req.Tags != nil {
		song.Tags = *req.Tags
	}
	if req.Lyrics != nil {
		if len(*req.Lyrics) > lyricMaxLyricsLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "歌词过长"})
			return
		}
		song.Lyrics = *req.Lyrics
	}
	if req.Note != nil {
		if len(*req.Note) > lyricMaxNoteLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "备注过长"})
			return
		}
		song.Note = *req.Note
	}
	if req.Source != nil {
		song.Source = *req.Source
	}
	if req.Favorite != nil {
		song.Favorite = *req.Favorite
	}
	if err := h.db.UpdateLyricSong(song); err != nil {
		if errors.Is(err, models.ErrLyricSongDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": "该艺人已有一首同名歌词"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": song})
}

// ToggleFavorite POST /api/lyrics/songs/:id/favorite
func (h *LyricsHandler) ToggleFavorite(c *gin.Context) {
	song, err := h.db.GetLyricSong(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "歌词不存在"})
		return
	}
	var req struct {
		Favorite *bool `json:"favorite"`
	}
	_ = c.ShouldBindJSON(&req)
	target := !song.Favorite
	if req.Favorite != nil {
		target = *req.Favorite
	}
	if err := h.db.SetLyricSongFavorite(song.ID, target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"favorite": target})
}

// DeleteSong DELETE /api/lyrics/songs/:id
func (h *LyricsHandler) DeleteSong(c *gin.Context) {
	if _, err := h.db.GetLyricSong(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "歌词不存在"})
		return
	}
	if err := h.db.DeleteLyricSong(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── 检索与聚合 ──

// SearchLines GET /api/lyrics/search/lines?keyword=&limit=
func (h *LyricsHandler) SearchLines(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入检索关键词"})
		return
	}
	hits, total, err := h.db.SearchLyricLines(keyword, parseIntDefault(c.Query("limit"), 120))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检索失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": hits, "total": total, "keyword": keyword})
}

// GetStats GET /api/lyrics/stats
func (h *LyricsHandler) GetStats(c *gin.Context) {
	stats, err := h.db.LyricStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ListArtists GET /api/lyrics/artists
func (h *LyricsHandler) ListArtists(c *gin.Context) {
	items, err := h.db.ListLyricArtists()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GetWordFrequency GET /api/lyrics/words?limit=80 — 高频字词(意象分析)
func (h *LyricsHandler) GetWordFrequency(c *gin.Context) {
	items, err := h.db.LyricWordFrequency(parseIntDefault(c.Query("limit"), 80))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RandomSong GET /api/lyrics/random?artist=&genre=
func (h *LyricsHandler) RandomSong(c *gin.Context) {
	song, err := h.db.RandomLyricSong(c.Query("artist"), c.Query("genre"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "库还是空的,先收录几首吧"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": song})
}

// ── 摘句 ──

type lyricFragmentPayload struct {
	SongID   string   `json:"song_id"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"`
	Tags     []string `json:"tags"`
	Note     string   `json:"note"`
	Favorite bool     `json:"favorite"`
}

// ListFragments GET /api/lyrics/fragments
func (h *LyricsHandler) ListFragments(c *gin.Context) {
	items, total, err := h.db.ListLyricFragments(
		c.Query("keyword"), c.Query("tag"), c.Query("song_id"),
		c.Query("fav") == "1" || c.Query("fav") == "true",
		parseIntDefault(c.Query("limit"), 50),
		parseIntDefault(c.Query("offset"), 0))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// CreateFragment POST /api/lyrics/fragments
func (h *LyricsHandler) CreateFragment(c *gin.Context) {
	var p lyricFragmentPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	f := &models.LyricFragment{
		SongID: p.SongID, Text: p.Text, Kind: p.Kind,
		Tags: p.Tags, Note: p.Note, Favorite: p.Favorite,
	}
	if err := h.db.CreateLyricFragment(f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": f})
}

// UpdateFragment PATCH /api/lyrics/fragments/:id
func (h *LyricsHandler) UpdateFragment(c *gin.Context) {
	f, err := h.db.GetLyricFragment(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摘句不存在"})
		return
	}
	var req struct {
		SongID   *string   `json:"song_id"`
		Text     *string   `json:"text"`
		Kind     *string   `json:"kind"`
		Tags     *[]string `json:"tags"`
		Note     *string   `json:"note"`
		Favorite *bool     `json:"favorite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if req.SongID != nil {
		f.SongID = *req.SongID
	}
	if req.Text != nil {
		f.Text = *req.Text
	}
	if req.Kind != nil {
		f.Kind = *req.Kind
	}
	if req.Tags != nil {
		f.Tags = *req.Tags
	}
	if req.Note != nil {
		f.Note = *req.Note
	}
	if req.Favorite != nil {
		f.Favorite = *req.Favorite
	}
	if err := h.db.UpdateLyricFragment(f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": f})
}

// DeleteFragment DELETE /api/lyrics/fragments/:id
func (h *LyricsHandler) DeleteFragment(c *gin.Context) {
	if _, err := h.db.GetLyricFragment(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "摘句不存在"})
		return
	}
	if err := h.db.DeleteLyricFragment(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkFragmentUsed POST /api/lyrics/fragments/:id/use
func (h *LyricsHandler) MarkFragmentUsed(c *gin.Context) {
	if err := h.db.MarkLyricFragmentUsed(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RandomFragment GET /api/lyrics/fragments/random?tag=
func (h *LyricsHandler) RandomFragment(c *gin.Context) {
	f, err := h.db.RandomLyricFragment(c.Query("tag"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": f})
}

// ── 导出与生成 prompt ──

// ExportSongs GET /api/lyrics/export?ids=a,b&format=md
// 不传 ids 则导出全部(受 limit 保护),输出可直接喂给模型或存盘的 Markdown。
func (h *LyricsHandler) ExportSongs(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "md")))
	idsParam := strings.TrimSpace(c.Query("ids"))
	songs := make([]*models.LyricSong, 0, 32)
	if idsParam != "" {
		for _, id := range strings.Split(idsParam, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if song, err := h.db.GetLyricSong(id); err == nil {
				songs = append(songs, song)
			}
		}
	} else {
		items, _, err := h.db.ListLyricSongs(models.LyricFilter{Limit: 200, Sort: "artist"})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "导出失败: " + err.Error()})
			return
		}
		songs = items
	}
	if len(songs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "没有可导出的歌词"})
		return
	}
	if format == "txt" {
		var b strings.Builder
		for i, s := range songs {
			if i > 0 {
				b.WriteString("\n\n" + strings.Repeat("=", 48) + "\n\n")
			}
			b.WriteString(s.Title + "\n")
			meta := make([]string, 0, 3)
			if s.Artist != "" {
				meta = append(meta, s.Artist)
			}
			if s.Album != "" {
				meta = append(meta, s.Album)
			}
			if s.Era != "" {
				meta = append(meta, s.Era)
			}
			if len(meta) > 0 {
				b.WriteString(strings.Join(meta, " · ") + "\n")
			}
			b.WriteString("\n" + s.Lyrics + "\n")
		}
		c.Header("Content-Disposition", `attachment; filename="lyrics.txt"`)
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b.String()))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="lyrics.md"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(buildLyricsMarkdown(songs)))
}

func buildLyricsMarkdown(songs []*models.LyricSong) string {
	var b strings.Builder
	b.WriteString("# 歌词库导出\n\n")
	b.WriteString(fmt.Sprintf("共 %d 首 · 导出于 %s\n\n---\n\n", len(songs), nowStamp()))
	for _, s := range songs {
		b.WriteString("## " + s.Title + "\n\n")
		meta := make([]string, 0, 5)
		if s.Artist != "" {
			meta = append(meta, "艺人:"+s.Artist)
		}
		if s.Album != "" {
			meta = append(meta, "专辑:"+s.Album)
		}
		if s.Language != "" {
			meta = append(meta, "语种:"+s.Language)
		}
		if s.Genre != "" {
			meta = append(meta, "风格:"+s.Genre)
		}
		if s.Era != "" {
			meta = append(meta, "年代:"+s.Era)
		}
		if len(s.Tags) > 0 {
			meta = append(meta, "标签:"+strings.Join(s.Tags, " / "))
		}
		if len(meta) > 0 {
			b.WriteString(strings.Join(meta, " · ") + "\n\n")
		}
		b.WriteString("```text\n" + s.Lyrics + "\n```\n\n")
		if s.Note != "" {
			b.WriteString("> 笔记:" + strings.ReplaceAll(s.Note, "\n", " ") + "\n\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String()
}

// BuildPrompt GET /api/lyrics/songs/:id/prompt
// 把一首歌词拼成"可生成音乐"的三段:风格 prompt、带结构标注的歌词、完整 Markdown。
// 已带 [Verse]/[Chorus] 标注的保留原标注,否则按空行分块推断结构。
func (h *LyricsHandler) BuildPrompt(c *gin.Context) {
	song, err := h.db.GetLyricSong(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "歌词不存在"})
		return
	}
	styleHint := strings.TrimSpace(c.Query("style"))
	c.JSON(http.StatusOK, gin.H{
		"item":              song,
		"style_prompt":      buildStylePrompt(song, styleHint),
		"structured_lyrics": buildStructuredLyrics(song.Lyrics),
		"markdown":          buildPromptMarkdown(song, styleHint),
	})
}

// 语种/风格/年代 → 模型能懂的英文风格描述(生成音乐上游基本认英文风格词)。
var lyricLangHint = map[string]string{
	"zh": "Mandarin Chinese vocals", "yue": "Cantonese vocals",
	"en": "English vocals", "ja": "Japanese vocals", "ko": "Korean vocals",
}

var lyricGenreHint = map[string]string{
	"pop": "mandopop, catchy hook, bright production", "rock": "rock band, distorted guitars, live drums",
	"folk": "acoustic folk, fingerpicked guitar, intimate", "rap": "hip-hop, boom bap drums, rhythmic flow",
	"rnb": "R&B, smooth groove, warm keys", "electronic": "synth-pop, pulsing arpeggios, wide reverb",
	"ballad": "slow ballad, piano and strings, emotional build", "other": "",
}

func buildStylePrompt(song *models.LyricSong, extra string) string {
	parts := make([]string, 0, 6)
	if g := lyricGenreHint[song.Genre]; g != "" {
		parts = append(parts, g)
	}
	if l := lyricLangHint[song.Language]; l != "" {
		parts = append(parts, l)
	}
	if song.Era != "" {
		parts = append(parts, song.Era+" production")
	}
	if song.Artist != "" {
		parts = append(parts, "in the spirit of "+song.Artist)
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	if len(parts) == 0 {
		return "contemporary pop, clear vocal, radio-ready mix"
	}
	return strings.Join(parts, ", ")
}

// buildStructuredLyrics 给歌词加段落标注。识别已有 [xxx] 标记则原样返回。
func buildStructuredLyrics(lyrics string) string {
	raw := strings.Split(strings.ReplaceAll(lyrics, "\r\n", "\n"), "\n")
	if strings.Contains(strings.ToLower(lyrics), "[verse") || strings.Contains(strings.ToLower(lyrics), "[chorus") {
		return strings.Join(raw, "\n")
	}
	blocks := make([][]string, 0, 8)
	current := make([]string, 0, 8)
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = make([]string, 0, 8)
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	if len(blocks) == 0 {
		return ""
	}
	var b strings.Builder
	verseNo, chorusNo := 0, 0
	for i, block := range blocks {
		label := ""
		if i == 0 {
			verseNo++
			label = fmt.Sprintf("[Verse %d]", verseNo)
		} else if i == len(blocks)-1 && len(blocks) > 2 {
			label = "[Outro]"
		} else if i%2 == 1 {
			chorusNo++
			label = "[Chorus]"
		} else {
			verseNo++
			label = fmt.Sprintf("[Verse %d]", verseNo)
		}
		b.WriteString(label + "\n")
		b.WriteString(strings.Join(block, "\n"))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func buildPromptMarkdown(song *models.LyricSong, extra string) string {
	var b strings.Builder
	b.WriteString("# " + song.Title)
	if song.Artist != "" {
		b.WriteString(" — " + song.Artist)
	}
	b.WriteString("\n\n**Style**\n\n" + buildStylePrompt(song, extra) + "\n\n")
	b.WriteString("**Lyrics**\n\n" + buildStructuredLyrics(song.Lyrics) + "\n")
	if song.Note != "" {
		b.WriteString("\n**参考笔记**\n\n" + song.Note + "\n")
	}
	return b.String()
}

// ── 内部工具 ──

func nowStamp() string {
	return time.Now().Format("2006-01-02 15:04")
}
