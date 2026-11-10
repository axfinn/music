package models

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func init() {
	RegisterInit("音乐库(music_tracks)", (*DB).InitMusicTracks)
}

// MusicTrack 音乐库曲目。音频与封面文件落盘 ./data/music(容器内挂 /app/data 持久卷),
// 数据库存元数据;lyrics 支持纯文本或 LRC 格式,由前端播放器解析同步。
type MusicTrack struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Artist        string `json:"artist"`
	Album         string `json:"album"`
	Lyrics        string `json:"lyrics"`
	Note          string `json:"note"`
	Filename      string `json:"filename"`
	OriginalName  string `json:"original_name"`
	ContentType   string `json:"content_type"`
	CoverFilename string `json:"cover_filename"`
	SizeBytes     int64  `json:"size_bytes"`
	DurationSec   float64 `json:"duration_sec"`
	Favorite      bool   `json:"favorite"`
	PlayCount     int    `json:"play_count"`
	LyricSongID   string `json:"lyric_song_id"`
	CreatedAt     TS     `json:"created_at"`
	UpdatedAt     TS     `json:"updated_at"`
	// Playable 不是数据库字段,由 handler 按「磁盘上能否找到音频文件」实时计算,
	// 供前端把坏掉的曲目标出来,而不是点了没反应/静默跳到下一首。
	Playable *bool `json:"playable,omitempty"`
}

// NewMusicTrackID 预生成曲目 ID。上传流程要「先落盘、再写库」,
// 磁盘文件名需要带 ID 前缀(id_随机.m4a),所以 ID 必须在落盘前就定好。
func NewMusicTrackID() string {
	return "mus_" + generateID(12)
}

func (db *DB) InitMusicTracks() error {
	if _, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS music_tracks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT DEFAULT '',
			album TEXT DEFAULT '',
			lyrics TEXT DEFAULT '',
			note TEXT DEFAULT '',
			filename TEXT NOT NULL,
			original_name TEXT DEFAULT '',
			content_type TEXT DEFAULT '',
			cover_filename TEXT DEFAULT '',
			size_bytes INTEGER DEFAULT 0,
			duration_sec REAL DEFAULT 0,
			favorite INTEGER DEFAULT 0,
			play_count INTEGER DEFAULT 0,
			lyric_song_id TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_music_tracks_created_at ON music_tracks(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_music_tracks_favorite ON music_tracks(favorite, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_music_tracks_play_count ON music_tracks(play_count DESC);
		CREATE INDEX IF NOT EXISTS idx_music_tracks_title_nocase ON music_tracks(title COLLATE NOCASE);
		CREATE INDEX IF NOT EXISTS idx_music_tracks_lyric_song ON music_tracks(lyric_song_id);
	`); err != nil {
		return err
	}
	// 旧表兼容:补加 lyric_song_id 列(已存在则忽略)
	db.addColumnIfNotExists("music_tracks", "lyric_song_id", "TEXT DEFAULT ''")
	return nil
}

// addColumnIfNotExists 对已存在的表安全地新增列(CREATE TABLE 已含则 no-op)。
func (db *DB) addColumnIfNotExists(table, col, def string) {
	_, err := db.conn.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + def)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		// 其它错误也忽略,避免初始化中断(列缺失会在查询时报错并暴露)
		_ = err
	}
}

const musicTrackColumns = `id, title, artist, album, lyrics, note, filename, original_name, content_type, cover_filename, size_bytes, duration_sec, favorite, play_count, lyric_song_id, created_at, updated_at`

// MusicTrackBrief 是列表/快照用的精简曲目。
// 歌词与备注是库里最重的两个字段(实测 2597 首合计 1.9MB,占单行内容近九成),
// 列表页只需要知道「有没有」,真正查看/编辑时再走 GET /tracks/:id 取全文。
// 这样一次全量快照不再把整本歌词集反复序列化、传输、JSON.parse。
type MusicTrackBrief struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Artist        string `json:"artist"`
	Album         string `json:"album"`
	HasLyrics     bool   `json:"has_lyrics"`
	HasNote       bool   `json:"has_note"`
	Filename      string `json:"-"`
	OriginalName  string `json:"-"`
	ContentType   string `json:"-"`
	CoverFilename string `json:"cover_filename"`
	SizeBytes     int64  `json:"size_bytes"`
	DurationSec   float64 `json:"duration_sec"`
	Favorite      bool   `json:"favorite"`
	PlayCount     int    `json:"play_count"`
	LyricSongID   string `json:"lyric_song_id"`
	CreatedAt     TS     `json:"created_at"`
	UpdatedAt     TS     `json:"updated_at"`
	Playable      *bool  `json:"playable,omitempty"`
}

// 大字段不进 SELECT,只算布尔标记;filename/original_name/content_type 仍要读出来
// 供 handler 判定可播放性,但不进 JSON(json:"-")。
const musicTrackBriefColumns = `id, title, artist, album,
	(lyrics IS NOT NULL AND lyrics <> ''),
	(note IS NOT NULL AND note <> ''),
	filename, original_name, content_type, cover_filename,
	size_bytes, duration_sec, favorite, play_count, lyric_song_id, created_at, updated_at`

func scanMusicTrackBrief(row interface{ Scan(...any) error }) (*MusicTrackBrief, error) {
	t := &MusicTrackBrief{}
	var favorite, hasLyrics, hasNote int
	var lyricSongID sql.NullString
	err := row.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &hasLyrics, &hasNote,
		&t.Filename, &t.OriginalName, &t.ContentType, &t.CoverFilename,
		&t.SizeBytes, &t.DurationSec, &favorite, &t.PlayCount, &lyricSongID,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.HasLyrics = hasLyrics != 0
	t.HasNote = hasNote != 0
	t.Favorite = favorite != 0
	t.LyricSongID = lyricSongID.String
	return t, nil
}

// musicTrackFilter 统一构造列表查询的 WHERE / ORDER BY。
// 排序值只认白名单,其余一律回落「最新」,避免把 query 直接拼进 SQL。
func musicTrackFilter(keyword string, favOnly bool, sort string) (string, string, []any) {
	where := ` WHERE 1 = 1`
	args := make([]any, 0, 4)
	if keyword != "" {
		where += ` AND (title LIKE ? OR artist LIKE ? OR album LIKE ? OR note LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like, like, like)
	}
	if favOnly {
		where += ` AND favorite = 1`
	}
	order := ` ORDER BY created_at DESC`
	switch sort {
	case "title":
		order = ` ORDER BY title COLLATE NOCASE ASC`
	case "plays":
		order = ` ORDER BY play_count DESC, created_at DESC`
	}
	return where, order, args
}

func scanMusicTrack(row interface{ Scan(...any) error }) (*MusicTrack, error) {
	t := &MusicTrack{}
	var favorite int
	var lyricSongID string
	err := row.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Lyrics, &t.Note, &t.Filename,
		&t.OriginalName, &t.ContentType, &t.CoverFilename, &t.SizeBytes, &t.DurationSec,
		&favorite, &t.PlayCount, &lyricSongID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Favorite = favorite != 0
	t.LyricSongID = lyricSongID
	return t, nil
}

func (db *DB) CreateMusicTrack(t *MusicTrack) error {
	if t == nil {
		return errors.New("track is nil")
	}
	if t.ID == "" {
		t.ID = NewMusicTrackID()
	}
	now := TS(time.Now())
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Title == "" {
		t.Title = "未命名"
	}
	_, err := db.conn.Exec(`
		INSERT INTO music_tracks (
			id, title, artist, album, lyrics, note, filename, original_name, content_type,
			cover_filename, size_bytes, duration_sec, favorite, play_count, lyric_song_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Title, t.Artist, t.Album, t.Lyrics, t.Note, t.Filename, t.OriginalName,
		t.ContentType, t.CoverFilename, t.SizeBytes, t.DurationSec, boolToInt(t.Favorite),
		t.PlayCount, t.LyricSongID, t.CreatedAt, t.UpdatedAt)
	return err
}

func (db *DB) GetMusicTrack(id string) (*MusicTrack, error) {
	row := db.conn.QueryRow(`SELECT `+musicTrackColumns+` FROM music_tracks WHERE id = ?`, id)
	t, err := scanMusicTrack(row)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ListMusicTracks 返回完整曲目(含歌词/备注正文)。仅供详情兼容模式与内部维护使用;
// 面向页面的列表请走 ListMusicTrackBriefs / ListAllMusicTrackBriefs。
// 排序只接受白名单值:created(最新)|title(标题)|plays(最多播放)。
func (db *DB) ListMusicTracks(keyword string, favOnly bool, sort string, limit, offset int) ([]*MusicTrack, error) {
	where, order, args := musicTrackFilter(keyword, favOnly, sort)
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + musicTrackColumns + ` FROM music_tracks` + where + order + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*MusicTrack, 0, limit)
	for rows.Next() {
		t, err := scanMusicTrack(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// ListMusicTrackBriefs 分页精简列表(GET /tracks 默认形态)。
func (db *DB) ListMusicTrackBriefs(keyword string, favOnly bool, sort string, limit, offset int) ([]*MusicTrackBrief, error) {
	if limit <= 0 {
		limit = 100
	}
	return db.listMusicTrackBriefs(keyword, favOnly, sort, limit, offset)
}

// ListAllMusicTrackBriefs 一次性取回全量精简曲目,供 /snapshot 合并成单个请求。
// cap 是防御性上限;返回条数可能小于 cap,调用方用 len 判断,不要假设等于 total。
func (db *DB) ListAllMusicTrackBriefs(keyword string, favOnly bool, sort string, cap int) ([]*MusicTrackBrief, error) {
	if cap <= 0 {
		cap = 20000
	}
	return db.listMusicTrackBriefs(keyword, favOnly, sort, cap, 0)
}

func (db *DB) listMusicTrackBriefs(keyword string, favOnly bool, sort string, limit, offset int) ([]*MusicTrackBrief, error) {
	where, order, args := musicTrackFilter(keyword, favOnly, sort)
	query := `SELECT ` + musicTrackBriefColumns + ` FROM music_tracks` + where + order + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*MusicTrackBrief, 0, limit)
	for rows.Next() {
		t, err := scanMusicTrackBrief(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (db *DB) CountMusicTracks(keyword string, favOnly bool) (int, error) {
	where, _, args := musicTrackFilter(keyword, favOnly, "")
	var total int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM music_tracks`+where, args...).Scan(&total)
	return total, err
}

type MusicStats struct {
	TotalCount    int     `json:"total_count"`
	FavoriteCount int     `json:"favorite_count"`
	TotalDuration float64 `json:"total_duration"`
	TotalBytes    int64   `json:"total_bytes"`
	TotalPlays    int     `json:"total_plays"`
}

func (db *DB) MusicTrackStats() (*MusicStats, error) {
	s := &MusicStats{}
	err := db.conn.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(favorite), 0),
			COALESCE(SUM(duration_sec), 0),
			COALESCE(SUM(size_bytes), 0),
			COALESCE(SUM(play_count), 0)
		FROM music_tracks
	`).Scan(&s.TotalCount, &s.FavoriteCount, &s.TotalDuration, &s.TotalBytes, &s.TotalPlays)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateMusicTrackMeta 更新可编辑字段(标题/歌手/专辑/歌词/备注/时长)。
func (db *DB) UpdateMusicTrackMeta(t *MusicTrack) error {
	if t == nil || t.ID == "" {
		return errors.New("track is nil")
	}
	t.UpdatedAt = TS(time.Now())
	_, err := db.conn.Exec(`
		UPDATE music_tracks
		SET title = ?, artist = ?, album = ?, lyrics = ?, note = ?, duration_sec = ?, updated_at = ?
		WHERE id = ?
	`, t.Title, t.Artist, t.Album, t.Lyrics, t.Note, t.DurationSec, t.UpdatedAt, t.ID)
	return err
}

func (db *DB) UpdateMusicTrackCover(id, coverFilename string) error {
	_, err := db.conn.Exec(`
		UPDATE music_tracks SET cover_filename = ?, updated_at = ? WHERE id = ?
	`, coverFilename, TS(time.Now()), id)
	return err
}

func (db *DB) SetMusicTrackFavorite(id string, fav bool) error {
	_, err := db.conn.Exec(`
		UPDATE music_tracks SET favorite = ?, updated_at = ? WHERE id = ?
	`, boolToInt(fav), TS(time.Now()), id)
	return err
}

// UpdateMusicTrackFilename 上传落盘后回填磁盘文件名(随机名),供流式播放定位文件。
func (db *DB) UpdateMusicTrackFilename(id, filename string) error {
	_, err := db.conn.Exec(`
		UPDATE music_tracks SET filename = ?, updated_at = ? WHERE id = ?
	`, filename, TS(time.Now()), id)
	return err
}

// UpdateMusicTrackLyricLink 关联/解关联歌词库条目(lyric_song_id 为空即解关联)。
func (db *DB) UpdateMusicTrackLyricLink(id, lyricSongID string) error {
	_, err := db.conn.Exec(`
		UPDATE music_tracks SET lyric_song_id = ?, updated_at = ? WHERE id = ?
	`, lyricSongID, TS(time.Now()), id)
	return err
}

func (db *DB) IncrementMusicTrackPlayCount(id string) error {
	_, err := db.conn.Exec(`
		UPDATE music_tracks SET play_count = play_count + 1, updated_at = updated_at WHERE id = ?
	`, id)
	return err
}

func (db *DB) DeleteMusicTrack(id string) error {
	_, err := db.conn.Exec(`DELETE FROM music_tracks WHERE id = ?`, id)
	return err
}

// ── 歌词库联动 ──

// MusicLinkSummary 歌词库 ↔ 音乐库的关联摘要,供歌词页反向查找"这首歌在音乐库有没有音轨"。
type MusicLinkSummary struct {
	LyricSongID  string `json:"lyric_song_id"`
	MusicTrackID string `json:"music_track_id"`
	Title        string `json:"title"`
}

// MusicTracksByLyricSong 找出关联到某个歌词库条目的全部音乐曲目(通常 0~1 条,但不限制)。
func (db *DB) MusicTracksByLyricSong(lyricSongID string) ([]*MusicTrack, error) {
	rows, err := db.conn.Query(`SELECT `+musicTrackColumns+` FROM music_tracks WHERE lyric_song_id = ? ORDER BY created_at DESC`, lyricSongID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*MusicTrack, 0)
	for rows.Next() {
		t, err := scanMusicTrack(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// MusicLinkSummaries 返回所有已关联(lyric_song_id 非空)的音乐曲目摘要,用于歌词库批量打标记。
func (db *DB) MusicLinkSummaries() ([]MusicLinkSummary, error) {
	rows, err := db.conn.Query(`SELECT lyric_song_id, id, title FROM music_tracks WHERE lyric_song_id IS NOT NULL AND lyric_song_id <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MusicLinkSummary, 0)
	for rows.Next() {
		var s MusicLinkSummary
		if err := rows.Scan(&s.LyricSongID, &s.MusicTrackID, &s.Title); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ── 批量维护用轻查询 ──

// MusicTrackFileRef 只带 id + filename,供启动体检与目录快照做内存比对,
// 不再把整张表(含歌词正文)读出来做一次 stat。
type MusicTrackFileRef struct {
	ID       string
	Filename string
}

func (db *DB) ListMusicTrackFileRefs() ([]MusicTrackFileRef, error) {
	rows, err := db.conn.Query(`SELECT id, filename FROM music_tracks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MusicTrackFileRef, 0)
	for rows.Next() {
		var r MusicTrackFileRef
		var filename sql.NullString
		if err := rows.Scan(&r.ID, &filename); err != nil {
			return nil, err
		}
		r.Filename = filename.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// MusicTrackTitle 只带 id + title,供批量歌词匹配。
type MusicTrackTitle struct {
	ID    string
	Title string
}

// ListUnlinkedMusicTracks 取出所有尚未关联歌词库的曲目(不再有 500 首上限,
// 以前超过 500 首的曲目永远轮不到自动匹配)。
func (db *DB) ListUnlinkedMusicTracks() ([]MusicTrackTitle, error) {
	rows, err := db.conn.Query(`SELECT id, title FROM music_tracks WHERE lyric_song_id IS NULL OR lyric_song_id = ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MusicTrackTitle, 0)
	for rows.Next() {
		var t MusicTrackTitle
		if err := rows.Scan(&t.ID, &t.Title); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LyricTitleBrief 批量匹配用的歌词库索引项(绝不 SELECT lyrics 正文)。
type LyricTitleBrief struct {
	ID     string
	Title  string
	Artist string
}

// ListLyricTitleIndex 一次取回歌词库全部 (id, 歌名, 艺人),替代「每首曲目一次 LIKE 全表查询」。
func (db *DB) ListLyricTitleIndex() ([]LyricTitleBrief, error) {
	rows, err := db.conn.Query(`SELECT id, title, artist FROM lyric_songs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LyricTitleBrief, 0)
	for rows.Next() {
		var b LyricTitleBrief
		var artist sql.NullString
		if err := rows.Scan(&b.ID, &b.Title, &artist); err != nil {
			return nil, err
		}
		b.Artist = artist.String
		out = append(out, b)
	}
	return out, rows.Err()
}

// MusicLyricLink 一条批量关联结果。
type MusicLyricLink struct {
	TrackID     string
	LyricSongID string
}

// BulkLinkMusicTracksToLyrics 在一个事务里批量写关联。
// WHERE 里带「仍为空」的条件,避免并发下覆盖用户刚做的手动关联。
func (db *DB) BulkLinkMusicTracksToLyrics(links []MusicLyricLink) (int, error) {
	if len(links) == 0 {
		return 0, nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE music_tracks SET lyric_song_id = ?, updated_at = ?
		WHERE id = ? AND (lyric_song_id IS NULL OR lyric_song_id = '')`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	now := TS(time.Now())
	affected := 0
	for _, l := range links {
		res, err := stmt.Exec(l.LyricSongID, now, l.TrackID)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		affected += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// boolToInt 把布尔转成 SQLite 的 0/1 整型。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
