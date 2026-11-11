package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/axfinn/music/backend/config"
	"github.com/axfinn/music/backend/models"

	"github.com/gin-gonic/gin"
)

// ── 音乐库(Music Library) ──
// 上传本地音频 → 流式播放(Range)→ 元数据/歌词/封面/收藏/播放次数管理。
// 音频与封面落盘 ./data/music(容器内 /app/data 命名卷,重建不丢),元数据落 SQLite music_tracks。
// 列表/播放/收藏是公开只读;增删改由 routes 层的管理门保护。

const (
	musicMaxAudio     = 200 << 20 // 单曲默认上限（music.audio_max_bytes 可覆盖）
	musicMaxCover     = 15 << 20  // 封面默认上限（music.cover_max_bytes 可覆盖）
	musicMaxLyrics    = 128 << 10 // 歌词文本上限 128KB
	musicMaxPageSize  = 2000      // /tracks 单页上限(精简载荷,可比旧的 500 放宽)
	musicSnapshotCap  = 20000     // /snapshot 全量上限(防御性,超出由前端继续分页)
	musicFileIndexTTL = 30 * time.Second
	musicStatsTTL     = 2 * time.Second
)

// 扩展名 → Content-Type 白名单,流式响应按此返回。
var musicAudioTypes = map[string]string{
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/opus",
	".webm": "audio/webm",
	".wma":  "audio/x-ms-wma",
	".aiff": "audio/aiff",
	".aif":  "audio/aiff",
	".ape":  "audio/x-ape",
}

var musicImageTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".avif": "image/avif",
}

type MusicHandler struct {
	db    *models.DB
	cfg   *config.Config // 站点与对外接口设置（/ping 要报限流额度）
	files *musicFileIndex

	// 音频与封面落盘目录,由 config.DataDir 驱动；上传上限由 config.Music 驱动。
	musicFileDir  string
	musicCoverDir string
	audioMaxBytes int64
	coverMaxBytes int64

	// rev 是「曲库数据版本」。任何写操作 +1,用于 snapshot 的条件请求:
	// 数据没变时后端只回几十字节的 not_modified,不再重传整份曲库。
	rev atomic.Uint64

	// stats 全表 SUM 在每次 load 都要跑一次,按 rev + TTL 缓存。
	statsMu  sync.Mutex
	statsVal *models.MusicStats
	statsRev uint64
	statsAt  time.Time
}

// NewMusicHandler 构造音乐库 handler,并按 config 计算音频/封面落盘目录并建好。
func NewMusicHandler(db *models.DB, cfg *config.Config) *MusicHandler {
	fileDir := filepath.Join(cfg.DataDir, "music")
	coverDir := filepath.Join(cfg.DataDir, "music/covers")
	// 确保目录存在,避免首次上传因目录缺失而失败。
	if err := os.MkdirAll(fileDir, 0o755); err != nil {
		log.Printf("创建音频目录 %s 失败: %v", fileDir, err)
	}
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		log.Printf("创建封面目录 %s 失败: %v", coverDir, err)
	}
	audioMax, coverMax := cfg.Music.AudioMaxBytes, cfg.Music.CoverMaxBytes
	if audioMax <= 0 {
		audioMax = musicMaxAudio
	}
	if coverMax <= 0 {
		coverMax = musicMaxCover
	}
	return &MusicHandler{
		db:            db,
		cfg:           cfg,
		files:         &musicFileIndex{dir: fileDir},
		musicFileDir:  fileDir,
		musicCoverDir: coverDir,
		audioMaxBytes: audioMax,
		coverMaxBytes: coverMax,
	}
}

// bumpRevision 元数据变了(标题/收藏/播放数/关联等)。
func (h *MusicHandler) bumpRevision() { h.rev.Add(1) }

// bumpFiles 磁盘文件集合也变了,目录快照必须失效。
func (h *MusicHandler) bumpFiles() {
	h.rev.Add(1)
	h.files.invalidate()
}

// revisionToken 把「数据版本 + 筛选条件」合成一个不透明 token。
// 筛选条件参与哈希:否则换关键词/排序时客户端拿旧 token 会被误判成未修改。
// 客户端只需原样回传,不需要理解格式。
func (h *MusicHandler) revisionToken(keyword string, favOnly bool, sortBy string) string {
	hsh := fnv.New64a()
	_, _ = hsh.Write([]byte(keyword))
	_, _ = hsh.Write([]byte{0})
	if favOnly {
		_, _ = hsh.Write([]byte{'1'})
	}
	_, _ = hsh.Write([]byte{0})
	_, _ = hsh.Write([]byte(sortBy))
	return strconv.FormatUint(h.rev.Load(), 10) + "-" + strconv.FormatUint(hsh.Sum64(), 16)
}

// musicFileSnapshot 是一次 os.ReadDir 的结果:文件名集合 + 按 id 前缀的索引。
// 以前列表里每首曲目都要 os.Stat 一次,filename 对不上时还会整目录 ReadDir,
// 2597 首就是 2597 次系统调用(最坏 O(N²) 次目录遍历),现在全部变成 O(1) 内存查表。
type musicFileSnapshot struct {
	names map[string]struct{}
	byID  map[string]string
}

type musicFileIndex struct {
	dir     string // 可注入，单测用 t.TempDir()
	mu      sync.Mutex
	at      time.Time
	current musicFileSnapshot
}

func (f *musicFileIndex) invalidate() {
	f.mu.Lock()
	f.at = time.Time{}
	f.mu.Unlock()
}

// get 返回目录快照。force=true 用于启动体检/手动修复;平时按 TTL 复用。
// 即使 ReadDir 失败也缓存空快照,避免目录暂时不可用时每个请求都去撞磁盘。
func (f *musicFileIndex) get(force bool) musicFileSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !force && !f.at.IsZero() && time.Since(f.at) < musicFileIndexTTL {
		return f.current
	}
	s := musicFileSnapshot{
		names: make(map[string]struct{}),
		byID:  make(map[string]string),
	}
	dir := f.dir
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue // covers 子目录
			}
			name := e.Name()
			s.names[name] = struct{}{}
			// 文件名格式固定为 "<id>_<hex><ext>"。id 形如 mus_1a2b3c,本身带下划线,
			// 所以必须按「最后一个」下划线切,不能用第一个(会全归到 "mus")。
			if i := strings.LastIndexByte(name, '_'); i > 0 {
				id := name[:i]
				if _, exists := s.byID[id]; !exists {
					s.byID[id] = name
				}
			}
		}
	}
	f.current = s
	f.at = time.Now()
	return s
}

// cachedStats 按数据版本缓存统计;版本一变立刻失效,不需要等 TTL。
func (h *MusicHandler) cachedStats() *models.MusicStats {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	rev := h.rev.Load()
	if h.statsVal != nil && h.statsRev == rev && time.Since(h.statsAt) < musicStatsTTL {
		return h.statsVal
	}
	s, err := h.db.MusicTrackStats()
	if err != nil {
		return h.statsVal // 查询失败时退回上一次的值,不让统计拖垮主流程
	}
	h.statsVal, h.statsRev, h.statsAt = s, rev, time.Now()
	return s
}

func randHexName(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("f_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func sanitizeExt(name string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(name)))
	if len(ext) > 12 || strings.ContainsAny(ext, "/\\") {
		return ""
	}
	return ext
}

// ── 上传 ──

// UploadTrack POST /api/music/tracks  multipart: file(必填) + title/artist/album/lyrics/note/duration
func (h *MusicHandler) UploadTrack(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少音频文件字段 file"})
		return
	}
	if fileHeader.Size > h.audioMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("文件超过 %dMB 上限", h.audioMaxBytes>>20)})
		return
	}
	ext := sanitizeExt(fileHeader.Filename)
	contentType, ok := musicAudioTypes[ext]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的音频格式,允许:mp3/wav/flac/m4a/aac/ogg/opus/webm/wma/aiff/ape"})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		base := filepath.Base(fileHeader.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	lyrics := c.PostForm("lyrics")
	if len(lyrics) > musicMaxLyrics {
		c.JSON(http.StatusBadRequest, gin.H{"error": "歌词过长(上限 128KB)"})
		return
	}

	track := &models.MusicTrack{
		ID:           models.NewMusicTrackID(),
		Title:        title,
		Artist:       strings.TrimSpace(c.PostForm("artist")),
		Album:        strings.TrimSpace(c.PostForm("album")),
		Lyrics:       lyrics,
		Note:         strings.TrimSpace(c.PostForm("note")),
		OriginalName: fileHeader.Filename,
		ContentType:  contentType,
		SizeBytes:    fileHeader.Size,
		DurationSec:  parsePositiveFloat(c.PostForm("duration")),
	}

	// 文件名 = id_随机 hex + 扩展名,避免路径穿越与重名。
	// 顺序很关键:先把文件落盘,再带着最终 filename 一次性入库。
	// 旧实现是「先插空 filename 的行 → 落盘 → UPDATE 回填」,一旦 UPDATE 失败或进程
	// 在中间重启,库里就留下 filename='' 的坏记录 → 播放 404(2026-10-30 实际踩到)。
	filename := track.ID + "_" + randHexName(8) + ext
	dest := filepath.Join(h.musicFileDir, filename)
	if err := saveMultipartFile(fileHeader, dest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存音频文件失败: " + err.Error()})
		return
	}
	track.Filename = filename
	if err := h.db.CreateMusicTrack(track); err != nil {
		_ = os.Remove(dest) // 入库失败则回滚文件,不留无主文件
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入数据库失败: " + err.Error()})
		return
	}
	h.bumpFiles() // 新音频落盘 + 新记录,目录快照与数据版本都要刷新
	track.Playable = boolPtr(true)
	// 上传后按歌名尝试自动匹配歌词库:AI 曲目的艺人名不可靠,只认歌名;
	// 仅唯一精确命中才关联,失败静默(用户可随时手动关联/改)。
	if song, _, mErr := h.matchLyricByTitle(track.Title); mErr == nil && song != nil {
		if lErr := h.db.UpdateMusicTrackLyricLink(track.ID, song.ID); lErr == nil {
			track.LyricSongID = song.ID
		}
	}
	c.JSON(http.StatusCreated, gin.H{"item": track})
}

// UploadCover POST /api/music/tracks/:id/cover  multipart: cover
func (h *MusicHandler) UploadCover(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("cover")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少封面字段 cover"})
		return
	}
	if fileHeader.Size > h.coverMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("封面超过 %dMB 上限", h.coverMaxBytes>>20)})
		return
	}
	ext := sanitizeExt(fileHeader.Filename)
	if _, ok := musicImageTypes[ext]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的图片格式,允许:jpg/png/webp/gif/avif"})
		return
	}
	// 删旧封面
	if track.CoverFilename != "" {
		_ = os.Remove(filepath.Join(h.musicCoverDir, track.CoverFilename))
	}
	filename := track.ID + "_" + randHexName(8) + ext
	if err := saveMultipartFile(fileHeader, filepath.Join(h.musicCoverDir, filename)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存封面失败: " + err.Error()})
		return
	}
	if err := h.db.UpdateMusicTrackCover(track.ID, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新数据库失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"cover_filename": filename})
}

// ── 查询 ──

// parseListParams 统一解析列表/快照的筛选参数,sort 的合法性由 models 层白名单兜底。
func parseListParams(c *gin.Context, defaultLimit int) (keyword string, favOnly bool, sortBy string, limit, offset int) {
	keyword = strings.TrimSpace(c.Query("keyword"))
	favOnly = c.Query("fav") == "1" || c.Query("fav") == "true"
	sortBy = c.Query("sort")
	limit = parseIntDefault(c.Query("limit"), defaultLimit)
	offset = parseIntDefault(c.Query("offset"), 0)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > musicMaxPageSize {
		limit = musicMaxPageSize
	}
	return
}

// ListTracks GET /api/music/tracks?keyword=&fav=&sort=&limit=&offset=&full=&with_stats=
// 默认返回精简曲目(不含歌词/备注正文);full=1 保留旧的完整形态,供外部脚本/老客户端兼容。
func (h *MusicHandler) ListTracks(c *gin.Context) {
	keyword, favOnly, sortBy, limit, offset := parseListParams(c, 100)
	token := h.revisionToken(keyword, favOnly, sortBy)
	c.Header("X-Music-Revision", token)

	if c.Query("full") == "1" {
		items, err := h.db.ListMusicTracks(keyword, favOnly, sortBy, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
			return
		}
		h.annotatePlayable(items)
		total, err := h.db.CountMusicTracks(keyword, favOnly)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "revision": token})
		return
	}

	items, err := h.db.ListMusicTrackBriefs(keyword, favOnly, sortBy, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	h.annotateBriefPlayable(items)
	total, err := h.db.CountMusicTracks(keyword, favOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败: " + err.Error()})
		return
	}
	resp := gin.H{"items": items, "total": total, "revision": token}
	if c.Query("with_stats") == "1" {
		resp["stats"] = h.cachedStats()
	}
	c.JSON(http.StatusOK, resp)
}

// Snapshot GET /api/music/snapshot?keyword=&fav=&sort=&rev=
// 首屏一次拿齐「全量精简曲目 + 总数 + 统计」,替掉以前 14 次逐页补齐。
// 带上一次返回的 rev:数据版本与筛选条件都没变时只回 not_modified,零载荷。
func (h *MusicHandler) Snapshot(c *gin.Context) {
	keyword, favOnly, sortBy, _, _ := parseListParams(c, musicSnapshotCap)
	token := h.revisionToken(keyword, favOnly, sortBy)
	c.Header("X-Music-Revision", token)
	c.Header("Cache-Control", "no-store") // 增量判断自己做,不依赖浏览器缓存
	if c.Query("rev") == token {
		c.JSON(http.StatusOK, gin.H{"not_modified": true, "revision": token})
		return
	}
	items, err := h.db.ListAllMusicTrackBriefs(keyword, favOnly, sortBy, musicSnapshotCap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	h.annotateBriefPlayable(items)
	total, err := h.db.CountMusicTracks(keyword, favOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"stats":     h.cachedStats(),
		"revision":  token,
		"truncated": len(items) >= musicSnapshotCap,
	})
}

// GetTrack GET /api/music/tracks/:id — 详情始终返回完整正文(歌词/备注),列表不再带。
func (h *MusicHandler) GetTrack(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	h.annotatePlayable([]*models.MusicTrack{track})
	c.JSON(http.StatusOK, gin.H{"item": track})
}

// GetTrackLyrics GET /api/music/tracks/:id/lyrics — 公开只读。
//
// 访客在音乐库里播放一首歌时,要能看到这首歌的歌词。
// 歌词库(收词/检索/摘句)是后台模块,但「正在听的这一首的歌词」属于听歌本身,
// 所以在音乐库这边开一个只读口子:优先用关联到歌词库的版本(更规整),
// 没有关联就回落到曲目自带的正文。
func (h *MusicHandler) GetTrackLyrics(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	text, source := track.Lyrics, "local"
	if track.LyricSongID != "" {
		if song, err := h.db.GetLyricSong(track.LyricSongID); err == nil && song.Lyrics != "" {
			text, source = song.Lyrics, "library"
		}
	}
	c.JSON(http.StatusOK, gin.H{"text": text, "source": source})
}

// GetStats GET /api/music/stats
func (h *MusicHandler) GetStats(c *gin.Context) {
	stats := h.cachedStats()
	if stats == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ── 修改 ──

// UpdateTrack PATCH /api/music/tracks/:id  JSON: title/artist/album/lyrics/note/duration_sec
func (h *MusicHandler) UpdateTrack(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	var req struct {
		Title       *string  `json:"title"`
		Artist      *string  `json:"artist"`
		Album       *string  `json:"album"`
		Lyrics      *string  `json:"lyrics"`
		Note        *string  `json:"note"`
		DurationSec *float64 `json:"duration_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "标题不能为空"})
			return
		}
		track.Title = title
	}
	if req.Artist != nil {
		track.Artist = strings.TrimSpace(*req.Artist)
	}
	if req.Album != nil {
		track.Album = strings.TrimSpace(*req.Album)
	}
	if req.Lyrics != nil {
		if len(*req.Lyrics) > musicMaxLyrics {
			c.JSON(http.StatusBadRequest, gin.H{"error": "歌词过长(上限 128KB)"})
			return
		}
		track.Lyrics = *req.Lyrics
	}
	if req.Note != nil {
		track.Note = strings.TrimSpace(*req.Note)
	}
	if req.DurationSec != nil && *req.DurationSec > 0 {
		track.DurationSec = *req.DurationSec
	}
	if err := h.db.UpdateMusicTrackMeta(track); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"item": track})
}

// ToggleFavorite POST /api/music/tracks/:id/favorite  JSON: {favorite: bool} 缺省取反
func (h *MusicHandler) ToggleFavorite(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	var req struct {
		Favorite *bool `json:"favorite"`
	}
	_ = c.ShouldBindJSON(&req) // body 可省略
	target := !track.Favorite
	if req.Favorite != nil {
		target = *req.Favorite
	}
	if err := h.db.SetMusicTrackFavorite(track.ID, target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"favorite": target})
}

// MarkPlayed POST /api/music/tracks/:id/played
func (h *MusicHandler) MarkPlayed(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	if err := h.db.IncrementMusicTrackPlayCount(track.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteTrack DELETE /api/music/tracks/:id — 删元数据并清理音频/封面文件
func (h *MusicHandler) DeleteTrack(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	if err := h.db.DeleteMusicTrack(track.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	// 按 ID 前缀清理,而不是只删 track.Filename:
	// 坏记录(filename 为空)以前删完会留下音频文件,越攒越多成为无主文件。
	removed := h.removeMusicFilesByPrefix(track.ID)
	if track.Filename != "" && !removed {
		_ = os.Remove(filepath.Join(h.musicFileDir, track.Filename))
	}
	if track.CoverFilename != "" {
		_ = os.Remove(filepath.Join(h.musicCoverDir, track.CoverFilename))
	}
	h.bumpFiles()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// removeMusicFilesByPrefix 删掉某曲目名下所有音频文件,返回是否有删除。
func (h *MusicHandler) removeMusicFilesByPrefix(id string) bool {
	entries, err := os.ReadDir(h.musicFileDir)
	if err != nil {
		return false
	}
	prefix := id + "_"
	removed := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if os.Remove(filepath.Join(h.musicFileDir, e.Name())) == nil {
			removed = true
		}
	}
	return removed
}

// ── 歌词库联动 ──

// LinkLyricSong POST /api/music/tracks/:id/lyrics  body: {lyric_song_id}
// 把本曲目关联到歌词库的一首歌(之后音乐库里能直接看到收录的歌词)。
func (h *MusicHandler) LinkLyricSong(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	var req struct {
		LyricSongID string `json:"lyric_song_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.LyricSongID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 lyric_song_id"})
		return
	}
	// 校验歌词库条目存在(不存在的 id 直接拦下,避免脏关联)
	if _, err := h.db.GetLyricSong(req.LyricSongID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "歌词库里没有这首(可能 id 有误)"})
		return
	}
	if err := h.db.UpdateMusicTrackLyricLink(track.ID, strings.TrimSpace(req.LyricSongID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关联失败: " + err.Error()})
		return
	}
	updated, err := h.db.GetMusicTrack(track.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"item": updated})
}

// UnlinkLyricSong DELETE /api/music/tracks/:id/lyrics  解除关联。
func (h *MusicHandler) UnlinkLyricSong(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	if err := h.db.UpdateMusicTrackLyricLink(track.ID, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解除关联失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// normalizeSongTitle 归一化歌名:去首尾空白、转小写、去掉所有空白字符,
// 仅用于「歌名相等」的宽松判断(如 "把夏天 寄给你" 与 "把夏天寄给你")。
func normalizeSongTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stripBracketDecorations 去掉歌名里的括号装饰段(全角/半角/方头/书名号)。
// AI 生成的曲目常把版本说明塞进歌名,如 "诺亚方舟（AI 新唱）"、"温柔 (还你自由版)（AI 新唱）"、
// "因为你所以我（借词新曲·抒情版）";而歌词库里的歌名是干净的。匹配前必须先剥掉这些括号段。
func stripBracketDecorations(s string) string {
	pairs := map[rune]rune{
		'（': '）', '(': ')', '【': '】', '[': ']',
		'「': '」', '『': '』', '《': '》', '<': '>',
	}
	var b strings.Builder
	depth := 0
	var closeFor rune
	for _, r := range s {
		if depth == 0 {
			if close, ok := pairs[r]; ok {
				depth = 1
				closeFor = close
				continue
			}
			b.WriteRune(r)
			continue
		}
		// 处于括号内:只认同种闭合符,其余字符丢弃(不做嵌套)
		if r == closeFor {
			depth = 0
			closeFor = 0
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// songBaseKey 歌名匹配用的键:先剥掉括号装饰,再小写去空白。
func songBaseKey(s string) string {
	return normalizeSongTitle(stripBracketDecorations(s))
}

// lyricSongBriefs 把歌词条目转成前端需要的精简结构(id/歌名/艺人)。
func lyricSongBriefs(songs []*models.LyricSong) []gin.H {
	items := make([]gin.H, 0, len(songs))
	for _, s := range songs {
		items = append(items, gin.H{"id": s.ID, "title": s.Title, "artist": s.Artist})
	}
	return items
}

// matchLyricByTitle 按歌名在歌词库里找最可能的对应条目。
// 返回 (唯一精确匹配, 候选列表, err)。仅当「歌名剥去括号装饰后归一化恰好相等且唯一」才算高置信匹配;
// 0 个或多个相等都交给用户手动挑(候选一并返回)。艺人名不参与过滤——AI 曲目的艺人名
// (如 "YuE2-3B (AI 生成)") 与真实歌手对不上,带上反而搜不到。
func (h *MusicHandler) matchLyricByTitle(title string) (*models.LyricSong, []*models.LyricSong, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil, nil
	}
	// 用剥掉括号装饰的歌名检索:否则 "诺亚方舟（AI 新唱）" 整串 LIKE 会一无所获。
	search := strings.TrimSpace(stripBracketDecorations(title))
	if search == "" {
		search = title
	}
	songs, _, err := h.db.ListLyricSongs(models.LyricFilter{Keyword: search, Limit: 30, Offset: 0})
	if err != nil {
		return nil, nil, err
	}
	want := songBaseKey(title)
	var exact []*models.LyricSong
	for _, s := range songs {
		if songBaseKey(s.Title) == want {
			exact = append(exact, s)
		}
	}
	if len(exact) == 1 {
		return exact[0], songs, nil
	}
	return nil, songs, nil
}

// LyricMatch GET /api/music/tracks/:id/lyric-match?q=  在歌词库里找候选,帮助手动/自动关联。
// 默认只用歌名检索;显式 q 可覆盖(搜艺人/专辑等)。歌名精确相等的候选排在最前。
func (h *MusicHandler) LyricMatch(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		// 默认按歌名匹配:AI 曲目的艺人名不可靠,不参与默认检索。
		q = strings.TrimSpace(track.Title)
	}
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	// 检索前剥掉括号装饰(AI 曲目歌名常带 "（AI 新唱）" 之类),否则整串 LIKE 搜不到。
	search := strings.TrimSpace(stripBracketDecorations(q))
	if search == "" {
		search = q
	}
	songs, _, err := h.db.ListLyricSongs(models.LyricFilter{Keyword: search, Limit: 20, Offset: 0})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检索失败: " + err.Error()})
		return
	}
	want := songBaseKey(q)
	sort.SliceStable(songs, func(i, j int) bool {
		return songBaseKey(songs[i].Title) == want && songBaseKey(songs[j].Title) != want
	})
	c.JSON(http.StatusOK, gin.H{"items": lyricSongBriefs(songs)})
}

// AutoMatchLyric POST /api/music/tracks/:id/lyrics/auto
// 按歌名自动匹配歌词库:歌名唯一精确命中才自动关联,否则返回候选交给用户手动挑。
func (h *MusicHandler) AutoMatchLyric(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	if track.LyricSongID != "" {
		if song, err := h.db.GetLyricSong(track.LyricSongID); err == nil {
			c.JSON(http.StatusOK, gin.H{"matched": true, "already": true, "lyric_song": gin.H{"id": song.ID, "title": song.Title, "artist": song.Artist}})
			return
		}
	}
	song, candidates, err := h.matchLyricByTitle(track.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "匹配失败: " + err.Error()})
		return
	}
	items := lyricSongBriefs(candidates)
	if song == nil {
		c.JSON(http.StatusOK, gin.H{"matched": false, "items": items})
		return
	}
	if err := h.db.UpdateMusicTrackLyricLink(track.ID, song.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关联失败: " + err.Error()})
		return
	}
	h.bumpRevision()
	c.JSON(http.StatusOK, gin.H{"matched": true, "lyric_song": gin.H{"id": song.ID, "title": song.Title, "artist": song.Artist}, "items": items})
}

// AutoMatchAllLyrics POST /api/music/lyrics/auto-match
// 批量按歌名匹配所有「尚未关联」的曲目(唯一精确命中才关联),返回统计与命中明细。
//
// 旧实现是「最多 500 首 × 每首一次 LIKE 全表扫描歌词库」,曲库涨到 2597 首后
// 就是几千次全表查询,且第 501 首以后永远轮不到。现在改成:
//  1. 一次取出所有未关联曲目的 (id,title)  —— 走 lyric_song_id 索引,无 500 上限
//  2. 一次取出歌词库的 (id,title,artist)  —— 不读歌词正文
//  3. 内存建归一化歌名索引,O(1) 匹配
//  4. 一个事务批量写回关联
func (h *MusicHandler) AutoMatchAllLyrics(c *gin.Context) {
	tracks, err := h.db.ListUnlinkedMusicTracks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	songs, err := h.db.ListLyricTitleIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "歌词库读取失败: " + err.Error()})
		return
	}
	index := make(map[string][]models.LyricTitleBrief, len(songs))
	for _, s := range songs {
		key := songBaseKey(s.Title)
		if key == "" {
			continue
		}
		index[key] = append(index[key], s)
	}

	links := make([]models.MusicLyricLink, 0)
	linked := make([]gin.H, 0)
	skipped := 0
	for _, t := range tracks {
		matches := index[songBaseKey(t.Title)]
		if len(matches) != 1 { // 0 个或多个同名都交给用户手动挑
			skipped++
			continue
		}
		s := matches[0]
		links = append(links, models.MusicLyricLink{TrackID: t.ID, LyricSongID: s.ID})
		// 明细只用于展示/排错,封顶避免几千条把响应撑大(前端只用统计数字)
		if len(linked) < 200 {
			linked = append(linked, gin.H{
				"track_id":      t.ID,
				"track_title":   t.Title,
				"lyric_song_id": s.ID,
				"lyric_title":   s.Title,
				"lyric_artist":  s.Artist,
			})
		}
	}
	matched, err := h.db.BulkLinkMusicTracksToLyrics(links)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量关联失败: " + err.Error()})
		return
	}
	if matched > 0 {
		h.bumpRevision()
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "total": len(tracks), "matched": matched,
		// 并发下被别人抢先关联的行不算失败,但也归到 skipped 里对外说明
		"skipped": skipped + (len(links) - matched), "linked": linked,
	})
}

// ── 文件服务 ──

// resolveMusicPath 定位曲目的音频文件,并在必要时自愈数据库里的 filename。
// 查询走目录快照(一次 ReadDir 服务 N 首),只有「缓存里确实找不到」时才强制
// 刷新一次兜底 —— 兼顾外部手工放文件的新鲜度,又不会每次播放都扫全目录。
func (h *MusicHandler) resolveMusicPath(track *models.MusicTrack) (string, bool) {
	s := h.files.get(false)
	if track.Filename != "" {
		if _, ok := s.names[track.Filename]; ok {
			return filepath.Join(h.musicFileDir, track.Filename), true
		}
	}
	name, found := s.byID[track.ID]
	if !found {
		s = h.files.get(true)
		name, found = s.byID[track.ID]
	}
	if !found {
		return "", false
	}
	if name != track.Filename {
		if err := h.db.UpdateMusicTrackFilename(track.ID, name); err == nil {
			track.Filename = name
			h.bumpRevision()
		}
	}
	// 库里 content_type 为空时按扩展名补,避免流式响应给错类型
	if track.ContentType == "" {
		if ct, ok := musicAudioTypes[sanitizeExt(name)]; ok {
			track.ContentType = ct
		}
	}
	return filepath.Join(h.musicFileDir, name), true
}

// annotatePlayable 给详情/兼容模式列表补 playable 标记。
// 列表请求是纯读:不再在这里写库自愈,自愈交给播放时与启动体检。
func (h *MusicHandler) annotatePlayable(list []*models.MusicTrack) {
	if len(list) == 0 {
		return
	}
	s := h.files.get(false)
	for _, t := range list {
		_, byName := s.names[t.Filename]
		_, byID := s.byID[t.ID]
		t.Playable = boolPtr(byName || byID)
	}
}

// annotateBriefPlayable 给精简列表补 playable 标记:一次目录快照 + O(1) 查表,
// 不再对每首曲目做 os.Stat / os.ReadDir。
func (h *MusicHandler) annotateBriefPlayable(list []*models.MusicTrackBrief) {
	if len(list) == 0 {
		return
	}
	s := h.files.get(false)
	for _, t := range list {
		_, byName := s.names[t.Filename]
		_, byID := s.byID[t.ID]
		t.Playable = boolPtr(byName || byID)
	}
}

// StreamTrack GET /api/music/stream/:id — http.ServeContent 原生支持 Range(拖动进度条/边下边播)
func (h *MusicHandler) StreamTrack(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	path, ok := h.resolveMusicPath(track)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "音频文件缺失", "missing": true})
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "音频文件不存在", "missing": true})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}
	contentType := track.ContentType
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	c.Header("Content-Type", contentType)
	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", "public, max-age=3600")
	http.ServeContent(c.Writer, c.Request, track.Filename, info.ModTime(), file)
}

// ServeCover GET /api/music/cover/:id
func (h *MusicHandler) ServeCover(c *gin.Context) {
	track, ok := h.loadTrack(c)
	if !ok {
		return
	}
	if track.CoverFilename == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "无封面"})
		return
	}
	path := filepath.Join(h.musicCoverDir, track.CoverFilename)
	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "封面不存在"})
		return
	}
	defer file.Close()
	ext := sanitizeExt(track.CoverFilename)
	contentType := musicImageTypes[ext]
	if contentType == "" {
		contentType = "image/jpeg"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	http.ServeContent(c.Writer, c.Request, track.CoverFilename, time.Time{}, file)
}

// ── 内部工具 ──

// SweepMusicFiles 全库体检:回填缺失/错误的 filename,并统计无主文件(有文件没记录)。
// 幂等,启动时跑一次;也可手动 POST /api/music/maintenance/repair 触发。
// 旧实现是「读整张表(含歌词正文) + 每首一次 os.Stat,坏记录还要整目录 ReadDir」;
// 现在只读 (id,filename) 两列 + 一次 ReadDir,剩下全是内存比对。
func (h *MusicHandler) SweepMusicFiles() (fixed int, missing int, orphans []string) {
	refs, err := h.db.ListMusicTrackFileRefs()
	if err != nil {
		return 0, 0, nil
	}
	s := h.files.get(true)
	known := make(map[string]bool, len(refs))
	for _, r := range refs {
		name := r.Filename
		if _, ok := s.names[name]; !ok {
			name = s.byID[r.ID] // filename 为空/对不上时按 ID 前缀兜底
		}
		if name == "" {
			missing++
			continue
		}
		if name != r.Filename {
			if err := h.db.UpdateMusicTrackFilename(r.ID, name); err == nil {
				fixed++
			}
		}
		known[name] = true
	}
	for name := range s.names {
		if !known[name] {
			orphans = append(orphans, name)
		}
	}
	if fixed > 0 {
		h.bumpRevision()
	}
	return fixed, missing, orphans
}

// RepairFiles POST /api/music/maintenance/repair — 手动触发全库体检(只回填,不删文件)。
func (h *MusicHandler) RepairFiles(c *gin.Context) {
	fixed, missing, orphans := h.SweepMusicFiles()
	sort.Strings(orphans)
	c.JSON(http.StatusOK, gin.H{
		"ok":           true,
		"fixed":        fixed,
		"missing":      missing,
		"orphan_count": len(orphans),
		"orphan_files": orphans,
		"note":         "orphan 是磁盘上无记录对应的文件,仅报告不删除",
	})
}

func (h *MusicHandler) loadTrack(c *gin.Context) (*models.MusicTrack, bool) {
	track, err := h.db.GetMusicTrack(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "曲目不存在"})
		return nil, false
	}
	return track, true
}

func boolPtr(b bool) *bool { return &b }

// saveMultipartFile 把上传文件落盘(自动建目录)。
func saveMultipartFile(fh *multipart.FileHeader, dest string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	dst, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil || v < 0 {
		return def
	}
	return v
}

func parsePositiveFloat(s string) float64 {
	if s == "" {
		return 0
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil || v < 0 {
		return 0
	}
	return v
}
