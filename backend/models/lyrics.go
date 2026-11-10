package models

import (
	"errors"
	"strings"
)

// ── 歌词库(Lyrics Library,最小子集) ──
// 本文件只移植「让音乐库编译通过」所需的最小子集:歌词歌曲表 + 跨表查询方法。
// 摘句 / 检索 / 统计 / 抓取等接口为后续 Phase 3 预留,不在此处。

var (
	// ErrLyricSongNotFound 歌曲不存在。
	ErrLyricSongNotFound = errors.New("歌词不存在")
)

func init() {
	RegisterInit("歌词库(lyric_songs)", (*DB).InitLyrics)
}

// LyricSong 收录的一首歌词。
type LyricSong struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	Language  string `json:"language"` // zh/yue/en/ja/ko/other
	Genre     string `json:"genre"`    // pop/rock/folk/rap/rnb/electronic/ballad/other
	Era       string `json:"era"`      // 年代,如 1990s / 2000s
	Tags      []string `json:"tags"`   // 自由标签:意象/手法/情绪
	Lyrics    string `json:"lyrics"`
	Note      string `json:"note"` // 创作笔记:为什么收录、句式特点
	Source    string `json:"source"`
	Favorite  bool   `json:"favorite"`
	LineCount int    `json:"line_count"`
	CharCount int    `json:"char_count"`
	CreatedAt TS     `json:"created_at"`
	UpdatedAt TS     `json:"updated_at"`
}

func (db *DB) InitLyrics() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS lyric_songs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL DEFAULT '',
			album TEXT DEFAULT '',
			language TEXT DEFAULT '',
			genre TEXT DEFAULT '',
			era TEXT DEFAULT '',
			tags TEXT DEFAULT '',
			lyrics TEXT DEFAULT '',
			note TEXT DEFAULT '',
			source TEXT DEFAULT '',
			favorite INTEGER DEFAULT 0,
			line_count INTEGER DEFAULT 0,
			char_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_lyric_songs_artist ON lyric_songs(artist COLLATE NOCASE);
		CREATE INDEX IF NOT EXISTS idx_lyric_songs_created ON lyric_songs(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_lyric_songs_favorite ON lyric_songs(favorite, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_lyric_songs_language ON lyric_songs(language);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_lyric_songs_artist_title
			ON lyric_songs(artist COLLATE NOCASE, title COLLATE NOCASE);
	`)
	return err
}

// ── 标签序列化 ──

const lyricTagSep = ","

// SplitLyricTags 把 "a, b,,c" 规整为去空去重的小写标签。
func SplitLyricTags(raw string) []string {
	out := make([]string, 0, 6)
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '/'
	}) {
		t := strings.ToLower(strings.TrimSpace(part))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// ── 查询 ──

const lyricSongColumns = `id, title, artist, album, language, genre, era, tags, lyrics, note, source,
	favorite, line_count, char_count, created_at, updated_at`

func scanLyricSong(row interface{ Scan(...any) error }) (*LyricSong, error) {
	s := &LyricSong{}
	var tags string
	var favorite int
	err := row.Scan(&s.ID, &s.Title, &s.Artist, &s.Album, &s.Language, &s.Genre, &s.Era, &tags,
		&s.Lyrics, &s.Note, &s.Source, &favorite, &s.LineCount, &s.CharCount, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Favorite = favorite != 0
	s.Tags = SplitLyricTags(tags)
	if s.Tags == nil {
		s.Tags = []string{}
	}
	return s, nil
}

// LyricFilter 列表过滤条件。Keyword 支持多词 AND,跨歌名/艺人/专辑/歌词/标签/笔记。
type LyricFilter struct {
	Keyword  string
	Artist   string
	Language string
	Genre    string
	Tag      string
	Era      string
	FavOnly  bool
	Sort     string // created|title|artist|lines|updated
	Limit    int
	Offset   int
}

func (f LyricFilter) where() (string, []any) {
	conds := make([]string, 0, 8)
	args := make([]any, 0, 12)
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		for _, word := range strings.Fields(kw) {
			like := "%" + word + "%"
			conds = append(conds, `(title LIKE ? OR artist LIKE ? OR album LIKE ? OR lyrics LIKE ? OR tags LIKE ? OR note LIKE ?)`)
			args = append(args, like, like, like, like, like, like)
		}
	}
	if a := strings.TrimSpace(f.Artist); a != "" {
		conds = append(conds, `artist = ? COLLATE NOCASE`)
		args = append(args, a)
	}
	if v := strings.TrimSpace(f.Language); v != "" {
		conds = append(conds, `language = ?`)
		args = append(args, v)
	}
	if v := strings.TrimSpace(f.Genre); v != "" {
		conds = append(conds, `genre = ?`)
		args = append(args, v)
	}
	if v := strings.TrimSpace(f.Era); v != "" {
		conds = append(conds, `era = ?`)
		args = append(args, v)
	}
	if v := strings.TrimSpace(f.Tag); v != "" {
		// tags 以逗号包裹存储,前后加逗号做整词匹配,避免 "爱" 命中 "爱情"。
		conds = append(conds, `(',' || tags || ',') LIKE ?`)
		args = append(args, "%,"+strings.ToLower(v)+",%")
	}
	if f.FavOnly {
		conds = append(conds, `favorite = 1`)
	}
	if len(conds) == 0 {
		return "1 = 1", args
	}
	return strings.Join(conds, " AND "), args
}

// ListLyricSongs 分页列表。歌词正文可能很长,列表返回完整内容(个人库量级可接受),
// 前端按需展示折叠。
func (db *DB) ListLyricSongs(f LyricFilter) ([]*LyricSong, int, error) {
	where, args := f.where()
	var total int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM lyric_songs WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := ` ORDER BY created_at DESC`
	switch f.Sort {
	case "title":
		order = ` ORDER BY title COLLATE NOCASE ASC`
	case "artist":
		order = ` ORDER BY artist COLLATE NOCASE ASC, title COLLATE NOCASE ASC`
	case "lines":
		order = ` ORDER BY line_count DESC`
	case "updated":
		order = ` ORDER BY updated_at DESC`
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q := `SELECT ` + lyricSongColumns + ` FROM lyric_songs WHERE ` + where + order + ` LIMIT ? OFFSET ?`
	rows, err := db.conn.Query(q, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*LyricSong, 0, limit)
	for rows.Next() {
		s, err := scanLyricSong(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

func (db *DB) GetLyricSong(id string) (*LyricSong, error) {
	row := db.conn.QueryRow(`SELECT `+lyricSongColumns+` FROM lyric_songs WHERE id = ?`, id)
	s, err := scanLyricSong(row)
	if err != nil {
		return nil, ErrLyricSongNotFound
	}
	return s, nil
}
