package models

import (
	"database/sql"
	"errors"
	"strings"
)

// ── 歌词发现（Discovery）所需的查询 ──
//
// 发现流要回答的是"接下来看什么"，所以查询都偏"随机 + 反向缺口"：
// 随机一首歌、随机一句摘句、还有"收了歌却没歌词"这种明显该补的洞。

// ErrDiscoveryEmpty 表示发现流没有任何可用素材（本地库是空的）。
var ErrDiscoveryEmpty = errors.New("发现流还没有素材")

// RandomLyricSongs 随机取 n 首歌（必须有歌词正文，空歌词唱不了也读不了）。
func (db *DB) RandomLyricSongs(limit int) ([]*LyricSong, error) {
	if limit <= 0 {
		limit = 6
	}
	rows, err := db.conn.Query(
		`SELECT `+lyricSongColumns+` FROM lyric_songs WHERE lyrics <> '' ORDER BY RANDOM() LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*LyricSong, 0, limit)
	for rows.Next() {
		s, err := scanLyricSong(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RandomLyricFragments 随机取 n 条摘句。
func (db *DB) RandomLyricFragments(limit int) ([]*LyricFragment, error) {
	if limit <= 0 {
		limit = 4
	}
	rows, err := db.conn.Query(
		`SELECT `+lyricFragmentColumns+` FROM lyric_fragments ORDER BY RANDOM() LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*LyricFragment, 0, limit)
	for rows.Next() {
		f, err := scanLyricFragment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 摘句可能没关联歌曲，补上歌名/艺人，前端才有上下文。
	_ = db.attachFragmentSongInfo(out)
	return out, nil
}

// MusicTrackLyricGap 音乐库里"有歌但没歌词"的曲目。
type MusicTrackLyricGap struct {
	ID        string
	Title     string
	Artist    string
	Album     string
	HasLyrics bool // 曲目自带歌词文本（内嵌歌词），有就不算缺口
}

// ListMusicTracksMissingLyrics 列出"收了但没歌词"的曲目。
// 判定：既没有内嵌歌词文本，也没有关联歌词库里的任何一首歌。
// 这是发现流里最有行动价值的一类：用户明确要过补歌词。
func (db *DB) ListMusicTracksMissingLyrics(limit int) ([]*MusicTrackLyricGap, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.conn.Query(`
		SELECT id, title, artist, album,
			CASE WHEN lyrics IS NULL OR TRIM(lyrics) = '' THEN 0 ELSE 1 END AS has_lyrics
		FROM music_tracks
		WHERE (lyric_song_id IS NULL OR lyric_song_id = '')
		  AND (lyrics IS NULL OR TRIM(lyrics) = '')
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*MusicTrackLyricGap, 0, limit)
	for rows.Next() {
		var g MusicTrackLyricGap
		var artist, album sql.NullString
		if err := rows.Scan(&g.ID, &g.Title, &artist, &album, &g.HasLyrics); err != nil {
			return nil, err
		}
		g.Artist = artist.String
		g.Album = album.String
		out = append(out, &g)
	}
	return out, rows.Err()
}

// CountMusicTracksMissingLyrics 统计"有歌没歌词"的曲目数。
func (db *DB) CountMusicTracksMissingLyrics() (int, error) {
	var n int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM music_tracks
		WHERE (lyric_song_id IS NULL OR lyric_song_id = '')
		  AND (lyrics IS NULL OR TRIM(lyrics) = '')`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// LyricTitleKey 返回用于去重的「艺人-歌名」键。
// 去重时大小写与空白不敏感：同一首歌在不同站点的大小写/空格经常不一致。
func LyricTitleKey(artist, title string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + "\x00" + strings.ToLower(strings.TrimSpace(title))
}

// LyricTitleIndex 返回歌词库里全部 (艺人, 歌名) 的去重键集合，用于在线结果去重。
func (db *DB) LyricTitleIndex() (map[string]bool, error) {
	rows, err := db.conn.Query(`SELECT title, artist FROM lyric_songs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := map[string]bool{}
	for rows.Next() {
		var title string
		var artist sql.NullString
		if err := rows.Scan(&title, &artist); err != nil {
			return nil, err
		}
		index[LyricTitleKey(artist.String, title)] = true
	}
	return index, rows.Err()
}

// MusicTitleIndex 返回音乐库里全部 (艺人, 歌名) 的去重键集合。
func (db *DB) MusicTitleIndex() (map[string]bool, error) {
	rows, err := db.conn.Query(`SELECT title, artist FROM music_tracks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := map[string]bool{}
	for rows.Next() {
		var title string
		var artist sql.NullString
		if err := rows.Scan(&title, &artist); err != nil {
			return nil, err
		}
		index[LyricTitleKey(artist.String, title)] = true
	}
	return index, rows.Err()
}
