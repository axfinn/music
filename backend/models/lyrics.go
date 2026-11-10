package models

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

		CREATE TABLE IF NOT EXISTS lyric_fragments (
			id TEXT PRIMARY KEY,
			song_id TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			kind TEXT DEFAULT 'line',
			tags TEXT DEFAULT '',
			note TEXT DEFAULT '',
			favorite INTEGER DEFAULT 0,
			use_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_lyric_fragments_song ON lyric_fragments(song_id);
		CREATE INDEX IF NOT EXISTS idx_lyric_fragments_created ON lyric_fragments(created_at DESC);
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

// ── 以下为 Phase 3 补齐的完整歌词模型 ──

var (
	// ErrLyricSongDuplicate 同一艺人下已存在同名歌曲。
	ErrLyricSongDuplicate = errors.New("该艺人的这首歌词已收录")
)

// ── 文本度量 ──

// CountLyricLines 统计非空行数(纯 LRC 时间戳行也算一行)。
func CountLyricLines(lyrics string) int {
	n := 0
	for _, line := range strings.Split(lyrics, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// CountLyricChars 统计有效字符数(去除所有空白)。
func CountLyricChars(lyrics string) int {
	n := 0
	for _, r := range lyrics {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

func JoinLyricTags(tags []string) string {
	return strings.Join(SplitLyricTags(strings.Join(tags, lyricTagSep)), lyricTagSep)
}

// ── 写入 ──

func normalizeLyricSong(s *LyricSong) {
	s.Title = strings.TrimSpace(s.Title)
	s.Artist = strings.TrimSpace(s.Artist)
	s.Album = strings.TrimSpace(s.Album)
	s.Language = strings.ToLower(strings.TrimSpace(s.Language))
	s.Genre = strings.ToLower(strings.TrimSpace(s.Genre))
	s.Era = strings.TrimSpace(s.Era)
	s.Source = strings.TrimSpace(s.Source)
	s.Note = strings.TrimSpace(s.Note)
	s.LineCount = CountLyricLines(s.Lyrics)
	s.CharCount = CountLyricChars(s.Lyrics)
}

func (db *DB) CreateLyricSong(s *LyricSong) error {
	if s == nil {
		return errors.New("song is nil")
	}
	normalizeLyricSong(s)
	if s.Title == "" {
		return errors.New("歌名不能为空")
	}
	if existing, err := db.FindLyricSongByArtistTitle(s.Artist, s.Title); err == nil && existing != nil {
		return ErrLyricSongDuplicate
	}
	if s.ID == "" {
		s.ID = "lyr_" + generateID(12)
	}
	now := TS(time.Now())
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := db.conn.Exec(`
		INSERT INTO lyric_songs (
			id, title, artist, album, language, genre, era, tags, lyrics, note, source,
			favorite, line_count, char_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.Title, s.Artist, s.Album, s.Language, s.Genre, s.Era, JoinLyricTags(s.Tags),
		s.Lyrics, s.Note, s.Source, boolToInt(s.Favorite), s.LineCount, s.CharCount,
		s.CreatedAt, s.UpdatedAt)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrLyricSongDuplicate
	}
	return err
}

// UpdateLyricSong 全字段更新(调用方负责先读后改,避免部分字段丢失)。
func (db *DB) UpdateLyricSong(s *LyricSong) error {
	if s == nil || s.ID == "" {
		return errors.New("song is nil")
	}
	normalizeLyricSong(s)
	if s.Title == "" {
		return errors.New("歌名不能为空")
	}
	// 改名/改艺人后可能与他人冲突
	if existing, err := db.FindLyricSongByArtistTitle(s.Artist, s.Title); err == nil &&
		existing != nil && existing.ID != s.ID {
		return ErrLyricSongDuplicate
	}
	s.UpdatedAt = TS(time.Now())
	_, err := db.conn.Exec(`
		UPDATE lyric_songs
		SET title = ?, artist = ?, album = ?, language = ?, genre = ?, era = ?, tags = ?,
			lyrics = ?, note = ?, source = ?, favorite = ?, line_count = ?, char_count = ?, updated_at = ?
		WHERE id = ?
	`, s.Title, s.Artist, s.Album, s.Language, s.Genre, s.Era, JoinLyricTags(s.Tags),
		s.Lyrics, s.Note, s.Source, boolToInt(s.Favorite), s.LineCount, s.CharCount,
		s.UpdatedAt, s.ID)
	return err
}

func (db *DB) SetLyricSongFavorite(id string, fav bool) error {
	_, err := db.conn.Exec(`UPDATE lyric_songs SET favorite = ?, updated_at = ? WHERE id = ?`,
		boolToInt(fav), TS(time.Now()), id)
	return err
}

// DeleteLyricSong 删歌时保留摘句(置空 song_id),避免连带丢失创作素材。
func (db *DB) DeleteLyricSong(id string) error {
	if _, err := db.conn.Exec(`UPDATE lyric_fragments SET song_id = '' WHERE song_id = ?`, id); err != nil {
		return err
	}
	_, err := db.conn.Exec(`DELETE FROM lyric_songs WHERE id = ?`, id)
	return err
}

// FindLyricSongByArtistTitle 用于入库前去重。
func (db *DB) FindLyricSongByArtistTitle(artist, title string) (*LyricSong, error) {
	row := db.conn.QueryRow(
		`SELECT `+lyricSongColumns+` FROM lyric_songs WHERE artist = ? COLLATE NOCASE AND title = ? COLLATE NOCASE LIMIT 1`,
		strings.TrimSpace(artist), strings.TrimSpace(title))
	s, err := scanLyricSong(row)
	if err != nil {
		return nil, ErrLyricSongNotFound
	}
	return s, nil
}

// ── 行级检索 ──

// LyricLineHit 命中行:定位到某首歌的第 N 行(1 起),供"这句在哪儿"检索。
type LyricLineHit struct {
	SongID    string `json:"song_id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	LineNo    int    `json:"line_no"`
	LineText  string `json:"line_text"`
	HasLyrics bool   `json:"has_lyrics"`
}

// SearchLyricLines 在歌词正文里逐行匹配关键词(大小写不敏感),返回命中行。
func (db *DB) SearchLyricLines(keyword string, limit int) ([]LyricLineHit, int, error) {
	keywords := strings.Fields(strings.ToLower(strings.TrimSpace(keyword)))
	if len(keywords) == 0 {
		return []LyricLineHit{}, 0, nil
	}
	if limit <= 0 || limit > 300 {
		limit = 120
	}
	where, args := LyricFilter{Keyword: keyword}.where()
	rows, err := db.conn.Query(
		`SELECT id, title, artist, lyrics FROM lyric_songs WHERE `+where+` ORDER BY created_at DESC LIMIT 400`,
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	hits := make([]LyricLineHit, 0, limit)
	total := 0
	for rows.Next() {
		var id, title, artist, lyrics string
		if err := rows.Scan(&id, &title, &artist, &lyrics); err != nil {
			return nil, 0, err
		}
		for i, line := range strings.Split(lyrics, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			matched := true
			for _, kw := range keywords {
				if !strings.Contains(lower, kw) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			total++
			if len(hits) < limit {
				hits = append(hits, LyricLineHit{
					SongID: id, Title: title, Artist: artist,
					LineNo: i + 1, LineText: trimmed, HasLyrics: true,
				})
			}
		}
	}
	return hits, total, rows.Err()
}

// ── 聚合统计 ──

type LyricCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type LyricStats struct {
	TotalSongs      int          `json:"total_songs"`
	TotalArtists    int          `json:"total_artists"`
	TotalLines      int          `json:"total_lines"`
	TotalChars      int          `json:"total_chars"`
	FavoriteSongs   int          `json:"favorite_songs"`
	FragmentCount   int          `json:"fragment_count"`
	Languages       []LyricCount `json:"languages"`
	Genres          []LyricCount `json:"genres"`
	TopArtists      []LyricCount `json:"top_artists"`
	TopTags         []LyricCount `json:"top_tags"`
	RecentSongs     []LyricCount `json:"recent_songs"`
	AvgLinesPerSong float64      `json:"avg_lines_per_song"`
}

func (db *DB) LyricStats() (*LyricStats, error) {
	s := &LyricStats{}
	err := db.conn.QueryRow(`
		SELECT COUNT(*),
			COUNT(DISTINCT CASE WHEN artist <> '' THEN artist END),
			COALESCE(SUM(line_count), 0),
			COALESCE(SUM(char_count), 0),
			COALESCE(SUM(favorite), 0)
		FROM lyric_songs
	`).Scan(&s.TotalSongs, &s.TotalArtists, &s.TotalLines, &s.TotalChars, &s.FavoriteSongs)
	if err != nil {
		return nil, err
	}
	if s.TotalSongs > 0 {
		s.AvgLinesPerSong = float64(s.TotalLines) / float64(s.TotalSongs)
	}
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM lyric_fragments`).Scan(&s.FragmentCount); err != nil {
		return nil, err
	}

	groupCount := func(column string, limit int) ([]LyricCount, error) {
		q := `SELECT ` + column + `, COUNT(*) c FROM lyric_songs WHERE ` + column + ` <> '' GROUP BY ` + column + ` ORDER BY c DESC LIMIT ?`
		rows, err := db.conn.Query(q, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]LyricCount, 0, limit)
		for rows.Next() {
			var item LyricCount
			if err := rows.Scan(&item.Name, &item.Count); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	}

	if s.Languages, err = groupCount("language", 12); err != nil {
		return nil, err
	}
	if s.Genres, err = groupCount("genre", 12); err != nil {
		return nil, err
	}
	if s.TopArtists, err = groupCount("artist", 12); err != nil {
		return nil, err
	}

	// 标签是逗号串,在 Go 端拆开统计(避免再建关联表)。
	rows, err := db.conn.Query(`SELECT tags FROM lyric_songs WHERE tags <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tagCounter := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, t := range SplitLyricTags(raw) {
			tagCounter[t]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.TopTags = topCounts(tagCounter, 30)

	recent, err := db.conn.Query(`SELECT title || ' · ' || COALESCE(NULLIF(artist,''),'未知'), 1 FROM lyric_songs ORDER BY created_at DESC LIMIT 8`)
	if err != nil {
		return nil, err
	}
	defer recent.Close()
	s.RecentSongs = make([]LyricCount, 0, 8)
	for recent.Next() {
		var item LyricCount
		if err := recent.Scan(&item.Name, &item.Count); err != nil {
			return nil, err
		}
		s.RecentSongs = append(s.RecentSongs, item)
	}
	return s, rows.Err()
}

func topCounts(counter map[string]int, limit int) []LyricCount {
	out := make([]LyricCount, 0, len(counter))
	for k, v := range counter {
		out = append(out, LyricCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ListLyricArtists 艺人列表(名字 + 收录首数),供侧边栏艺人视图。
func (db *DB) ListLyricArtists() ([]LyricCount, error) {
	rows, err := db.conn.Query(`
		SELECT COALESCE(NULLIF(artist,''),'未知艺人') a, COUNT(*) c
		FROM lyric_songs GROUP BY a ORDER BY c DESC, a COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LyricCount, 0, 32)
	for rows.Next() {
		var item LyricCount
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ── 高频字词(意象分析) ──

// LyricWordCount 高频字词。CJK 用相邻双字(bigram)近似词,CJK 单字与拉丁词分别统计。
type LyricWordCount struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
	Type  string `json:"type"` // cjk2 | cjk1 | word
}

// 中文常见虚词/代词/助词,统计时过滤,避免 "我的""一个" 这类噪声占据榜首。
var lyricStopBigrams = map[string]bool{
	"的时": true, "时候": true, "我们": true, "你们": true, "他们": true, "自己": true,
	"什么": true, "没有": true, "就是": true, "可以": true, "这个": true, "那个": true,
	"一个": true, "一样": true, "已经": true, "还是": true, "因为": true, "所以": true,
	"但是": true, "如果": true, "不会": true, "不能": true, "不要": true, "知道": true,
	"觉得": true, "真的": true, "不是": true, "在我": true, "你的": true, "我的": true,
	"他的": true, "她的": true, "是我": true, "是你": true, "了我": true, "了你": true,
}

var lyricStopRunes = map[rune]bool{
	'的': true, '了': true, '在': true, '是': true, '我': true, '你': true, '他': true, '她': true,
	'它': true, '们': true, '这': true, '那': true, '就': true, '都': true, '也': true, '不': true,
	'有': true, '和': true, '与': true, '把': true, '被': true, '给': true, '对': true, '从': true,
	'到': true, '着': true, '过': true, '要': true, '会': true, '能': true, '说': true, '很': true,
	'太': true, '还': true, '又': true, '再': true, '才': true, '只': true, '让': true, '吗': true,
	'呢': true, '吧': true, '啊': true, '呀': true, '哦': true, '嗯': true, '一': true, '个': true,
	'上': true, '下': true, '里': true, '中': true, '么': true, '什': true, '得': true, '地': true,
}

var lyricStopWords = map[string]bool{
	"the": true, "and": true, "you": true, "that": true, "was": true, "for": true, "are": true,
	"with": true, "his": true, "her": true, "she": true, "him": true, "but": true, "not": true,
	"all": true, "have": true, "this": true, "from": true, "they": true, "will": true, "just": true,
	"when": true, "what": true, "like": true, "your": true, "can": true, "our": true, "out": true,
}

// LyricWordFrequency 扫描全库歌词统计高频字词。个人库量级(万行以内)全扫可接受。
func (db *DB) LyricWordFrequency(limit int) ([]LyricWordCount, error) {
	if limit <= 0 || limit > 300 {
		limit = 80
	}
	rows, err := db.conn.Query(`SELECT lyrics FROM lyric_songs WHERE lyrics <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bigram := map[string]int{}
	single := map[string]int{}
	words := map[string]int{}

	for rows.Next() {
		var lyrics string
		if err := rows.Scan(&lyrics); err != nil {
			return nil, err
		}
		for _, line := range strings.Split(lyrics, "\n") {
			// 去掉 LRC 时间戳与括号注记,避免 "[00:12.34]" 干扰统计
			clean := stripLRCAndBrackets(line)
			countLyricText(clean, bigram, single, words)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]LyricWordCount, 0, limit*3)
	for w, c := range bigram {
		out = append(out, LyricWordCount{Word: w, Count: c, Type: "cjk2"})
	}
	for w, c := range words {
		if c < 2 {
			continue
		}
		out = append(out, LyricWordCount{Word: w, Count: c, Type: "word"})
	}
	for w, c := range single {
		out = append(out, LyricWordCount{Word: w, Count: c, Type: "cjk1"})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Word < out[j].Word
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func countLyricText(text string, bigram, single, words map[string]int) {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if isCJK(r) {
			if !lyricStopRunes[r] {
				single[string(r)]++
			}
			if i+1 < len(runes) && isCJK(runes[i+1]) {
				bg := string(runes[i : i+2])
				if !lyricStopBigrams[bg] && !lyricStopRunes[r] && !lyricStopRunes[runes[i+1]] {
					bigram[bg]++
				}
			}
			continue
		}
		if isLatinLetter(r) {
			j := i
			for j < len(runes) && isLatinLetter(runes[j]) {
				j++
			}
			w := strings.ToLower(string(runes[i:j]))
			if utf8.RuneCountInString(w) >= 3 && !lyricStopWords[w] {
				words[w]++
			}
			i = j - 1
		}
	}
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func isLatinLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '\''
}

// stripLRCAndBrackets 移除 LRC 时间戳 [mm:ss.xx] 与圆/方括号注记。
func stripLRCAndBrackets(line string) string {
	var b strings.Builder
	depthSquare, depthRound := 0, 0
	for _, r := range line {
		switch r {
		case '[':
			depthSquare++
			continue
		case ']':
			if depthSquare > 0 {
				depthSquare--
				continue
			}
		case '（', '(':
			depthRound++
			continue
		case '）', ')':
			if depthRound > 0 {
				depthRound--
				continue
			}
		}
		if depthSquare > 0 || depthRound > 0 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ── 摘句 ──

// LyricFragment 摘句:从某首歌里摘出来的句子,单独成条便于检索与随机取用。
type LyricFragment struct {
	ID        string    `json:"id"`
	SongID    string    `json:"song_id"` // 可为空(自由摘句)
	SongTitle string    `json:"song_title"`
	Artist    string    `json:"artist"`
	Text      string    `json:"text"`
	Kind      string    `json:"kind"` // hook/chorus/verse/bridge/line
	Tags      []string  `json:"tags"`
	Note      string    `json:"note"`
	Favorite  bool      `json:"favorite"`
	UseCount  int       `json:"use_count"`
	CreatedAt TS        `json:"created_at"`
	UpdatedAt TS        `json:"updated_at"`
}

const lyricFragmentColumns = `id, song_id, text, kind, tags, note, favorite, use_count, created_at, updated_at`

func scanLyricFragment(row interface{ Scan(...any) error }) (*LyricFragment, error) {
	f := &LyricFragment{}
	var tags string
	var favorite int
	err := row.Scan(&f.ID, &f.SongID, &f.Text, &f.Kind, &tags, &f.Note, &favorite, &f.UseCount,
		&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	f.Favorite = favorite != 0
	f.Tags = SplitLyricTags(tags)
	if f.Tags == nil {
		f.Tags = []string{}
	}
	return f, nil
}

func (db *DB) ListLyricFragments(keyword, tag, songID string, favOnly bool, limit, offset int) ([]*LyricFragment, int, error) {
	conds := make([]string, 0, 5)
	args := make([]any, 0, 8)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		conds = append(conds, `(text LIKE ? OR tags LIKE ? OR note LIKE ?)`)
		args = append(args, like, like, like)
	}
	if t := strings.TrimSpace(tag); t != "" {
		conds = append(conds, `(',' || tags || ',') LIKE ?`)
		args = append(args, "%,"+strings.ToLower(t)+",%")
	}
	if sid := strings.TrimSpace(songID); sid != "" {
		conds = append(conds, `song_id = ?`)
		args = append(args, sid)
	}
	if favOnly {
		conds = append(conds, `favorite = 1`)
	}
	where := "1 = 1"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}
	var total int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM lyric_fragments WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.conn.Query(
		`SELECT `+lyricFragmentColumns+` FROM lyric_fragments WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*LyricFragment, 0, limit)
	for rows.Next() {
		f, err := scanLyricFragment(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := db.attachFragmentSongInfo(items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// attachFragmentSongInfo 用一次查询补齐摘句所属歌名/艺人。
func (db *DB) attachFragmentSongInfo(items []*LyricFragment) error {
	ids := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		if it.SongID != "" && !seen[it.SongID] {
			seen[it.SongID] = true
			ids = append(ids, it.SongID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.conn.Query(`SELECT id, title, artist FROM lyric_songs WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	meta := map[string][2]string{}
	for rows.Next() {
		var id, title, artist string
		if err := rows.Scan(&id, &title, &artist); err != nil {
			return err
		}
		meta[id] = [2]string{title, artist}
	}
	for _, it := range items {
		if m, ok := meta[it.SongID]; ok {
			it.SongTitle = m[0]
			it.Artist = m[1]
		}
	}
	return rows.Err()
}

func (db *DB) CreateLyricFragment(f *LyricFragment) error {
	if f == nil {
		return errors.New("fragment is nil")
	}
	f.Text = strings.TrimSpace(f.Text)
	if f.Text == "" {
		return errors.New("摘句内容不能为空")
	}
	if f.Kind == "" {
		f.Kind = "line"
	}
	if f.ID == "" {
		f.ID = "lyf_" + generateID(12)
	}
	now := TS(time.Now())
	f.CreatedAt = now
	f.UpdatedAt = now
	_, err := db.conn.Exec(`
		INSERT INTO lyric_fragments (id, song_id, text, kind, tags, note, favorite, use_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID, f.SongID, f.Text, f.Kind, JoinLyricTags(f.Tags), strings.TrimSpace(f.Note),
		boolToInt(f.Favorite), f.UseCount, f.CreatedAt, f.UpdatedAt)
	return err
}

func (db *DB) GetLyricFragment(id string) (*LyricFragment, error) {
	row := db.conn.QueryRow(`SELECT `+lyricFragmentColumns+` FROM lyric_fragments WHERE id = ?`, id)
	f, err := scanLyricFragment(row)
	if err != nil {
		return nil, errors.New("摘句不存在")
	}
	items := []*LyricFragment{f}
	if err := db.attachFragmentSongInfo(items); err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) UpdateLyricFragment(f *LyricFragment) error {
	if f == nil || f.ID == "" {
		return errors.New("fragment is nil")
	}
	f.Text = strings.TrimSpace(f.Text)
	if f.Text == "" {
		return errors.New("摘句内容不能为空")
	}
	f.UpdatedAt = TS(time.Now())
	_, err := db.conn.Exec(`
		UPDATE lyric_fragments
		SET song_id = ?, text = ?, kind = ?, tags = ?, note = ?, favorite = ?, updated_at = ?
		WHERE id = ?
	`, f.SongID, f.Text, f.Kind, JoinLyricTags(f.Tags), strings.TrimSpace(f.Note),
		boolToInt(f.Favorite), f.UpdatedAt, f.ID)
	return err
}

func (db *DB) DeleteLyricFragment(id string) error {
	_, err := db.conn.Exec(`DELETE FROM lyric_fragments WHERE id = ?`, id)
	return err
}

func (db *DB) MarkLyricFragmentUsed(id string) error {
	_, err := db.conn.Exec(`UPDATE lyric_fragments SET use_count = use_count + 1 WHERE id = ?`, id)
	return err
}

// RandomLyricFragment 随机取一条摘句(可按标签过滤),用于"灵感一下"。
func (db *DB) RandomLyricFragment(tag string) (*LyricFragment, error) {
	q := `SELECT ` + lyricFragmentColumns + ` FROM lyric_fragments`
	args := make([]any, 0, 1)
	if t := strings.TrimSpace(tag); t != "" {
		q += ` WHERE (',' || tags || ',') LIKE ?`
		args = append(args, "%,"+strings.ToLower(t)+",%")
	}
	q += ` ORDER BY RANDOM() LIMIT 1`
	row := db.conn.QueryRow(q, args...)
	f, err := scanLyricFragment(row)
	if err != nil {
		return nil, errors.New("摘句库还是空的")
	}
	items := []*LyricFragment{f}
	_ = db.attachFragmentSongInfo(items)
	return f, nil
}

// RandomLyricSong 随机取一首歌(可按艺人/风格过滤),用于"随手翻一首"。
func (db *DB) RandomLyricSong(artist, genre string) (*LyricSong, error) {
	q := `SELECT ` + lyricSongColumns + ` FROM lyric_songs WHERE lyrics <> ''`
	args := make([]any, 0, 2)
	if a := strings.TrimSpace(artist); a != "" {
		q += ` AND artist = ? COLLATE NOCASE`
		args = append(args, a)
	}
	if g := strings.TrimSpace(genre); g != "" {
		q += ` AND genre = ?`
		args = append(args, g)
	}
	q += ` ORDER BY RANDOM() LIMIT 1`
	row := db.conn.QueryRow(q, args...)
	s, err := scanLyricSong(row)
	if err != nil {
		return nil, ErrLyricSongNotFound
	}
	return s, nil
}
