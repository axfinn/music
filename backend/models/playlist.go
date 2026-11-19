package models

import (
	"database/sql"
	"errors"
	"time"
)

// 歌单是管理员发布的公开集合，与前端「播放队列」是两个概念。
// 访客自己的歌单存在浏览器 localStorage，不落库（站点没有访客账号）。
//
// 数据分两张表：playlists 存元信息，playlist_tracks 存有序曲目。
// 不用「track_ids 拼一个 TEXT」是因为要支持按曲目反查、删除曲目时清理引用。
func init() {
	RegisterInit("歌单(playlists)", (*DB).InitPlaylists)
}

// Playlist 公开歌单。TrackIDs 只在详情接口填充；列表接口为空，
// 避免 3000 首曲库的歌单列表把几万个 ID 一起序列化。
type Playlist struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TrackCount  int      `json:"track_count"`
	TrackIDs    []string `json:"track_ids,omitempty"`
	CreatedAt   TS       `json:"created_at"`
	UpdatedAt   TS       `json:"updated_at"`
}

func NewPlaylistID() string { return "pls_" + generateID(12) }

// playlistTrackHardCap 是单张歌单的防御性上限，防止误传全量曲库 ID 撑爆请求。
const playlistTrackHardCap = 5000

func (db *DB) InitPlaylists() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS playlists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_playlists_created_at ON playlists(created_at DESC);
		CREATE TABLE IF NOT EXISTS playlist_tracks (
			playlist_id TEXT NOT NULL,
			track_id TEXT NOT NULL,
			position INTEGER DEFAULT 0,
			PRIMARY KEY (playlist_id, track_id)
		);
		CREATE INDEX IF NOT EXISTS idx_playlist_tracks_track ON playlist_tracks(track_id);
	`)
	return err
}

// filterExistingTrackIDs 去重、过滤不存在的曲目，保持传入顺序。
// 必须在事务内调用：DB 只有一条连接，事务期间不能再从 db.conn 开查询。
func filterExistingTrackIDs(tx *sql.Tx, ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		var one int
		err := tx.QueryRow(`SELECT 1 FROM music_tracks WHERE id = ?`, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= playlistTrackHardCap {
			break
		}
	}
	return out, nil
}

func playlistExists(tx *sql.Tx, id string) (bool, error) {
	var one int
	err := tx.QueryRow(`SELECT 1 FROM playlists WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CreatePlaylist 创建歌单并写入初始曲目（可为空）。
func (db *DB) CreatePlaylist(p *Playlist, trackIDs []string) error {
	if p == nil || p.ID == "" {
		return errors.New("playlist is nil")
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := TS(time.Now())
	p.CreatedAt, p.UpdatedAt = now, now
	if _, err = tx.Exec(`
		INSERT INTO playlists (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Description, p.CreatedAt, p.UpdatedAt); err != nil {
		return err
	}

	ids, err := filterExistingTrackIDs(tx, trackIDs)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err = tx.Exec(`
			INSERT OR IGNORE INTO playlist_tracks (playlist_id, track_id, position)
			VALUES (?, ?, ?)
		`, p.ID, id, i); err != nil {
			return err
		}
	}
	p.TrackIDs = ids
	p.TrackCount = len(ids)
	return tx.Commit()
}

// ListPlaylists 歌单列表，带曲目数但不带曲目 ID。
func (db *DB) ListPlaylists() ([]*Playlist, error) {
	rows, err := db.conn.Query(`
		SELECT p.id, p.name, p.description, p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM playlist_tracks pt WHERE pt.playlist_id = p.id)
		FROM playlists p ORDER BY p.created_at DESC, p.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*Playlist, 0)
	for rows.Next() {
		p := &Playlist{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt, &p.TrackCount); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// GetPlaylist 详情：带按添加顺序排列的曲目 ID。
func (db *DB) GetPlaylist(id string) (*Playlist, error) {
	p := &Playlist{}
	err := db.conn.QueryRow(`
		SELECT id, name, description, created_at, updated_at,
			(SELECT COUNT(*) FROM playlist_tracks pt WHERE pt.playlist_id = p.id)
		FROM playlists p WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt, &p.TrackCount)
	if err != nil {
		return nil, err
	}
	rows, err := db.conn.Query(`
		SELECT track_id FROM playlist_tracks WHERE playlist_id = ?
		ORDER BY position ASC, rowid ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p.TrackIDs = make([]string, 0, p.TrackCount)
	for rows.Next() {
		var trackID string
		if err := rows.Scan(&trackID); err != nil {
			return nil, err
		}
		p.TrackIDs = append(p.TrackIDs, trackID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	p.TrackCount = len(p.TrackIDs)
	return p, nil
}

// UpdatePlaylist 只改名/描述，不碰曲目顺序。
func (db *DB) UpdatePlaylist(id, name, description string) (*Playlist, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ok, err := playlistExists(tx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, sql.ErrNoRows
	}
	if _, err = tx.Exec(`
		UPDATE playlists SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`, name, description, TS(time.Now()), id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetPlaylist(id)
}

// DeletePlaylist 删除歌单及其曲目关联。
func (db *DB) DeletePlaylist(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM playlist_tracks WHERE playlist_id = ?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM playlists WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// AddPlaylistTracks 追加曲目；已存在的条目保持原位不重复。
func (db *DB) AddPlaylistTracks(id string, trackIDs []string) (int, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ok, err := playlistExists(tx, id)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, sql.ErrNoRows
	}
	ids, err := filterExistingTrackIDs(tx, trackIDs)
	if err != nil {
		return 0, err
	}
	var maxPos int
	if err = tx.QueryRow(`SELECT COALESCE(MAX(position), -1) FROM playlist_tracks WHERE playlist_id = ?`, id).Scan(&maxPos); err != nil {
		return 0, err
	}
	added := 0
	for _, trackID := range ids {
		res, err := tx.Exec(`
			INSERT OR IGNORE INTO playlist_tracks (playlist_id, track_id, position)
			VALUES (?, ?, ?)
		`, id, trackID, maxPos+1+added)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	if added > 0 {
		if _, err = tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, TS(time.Now()), id); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

// RemovePlaylistTrack 从歌单移除一首，不影响曲库本身。
func (db *DB) RemovePlaylistTrack(id, trackID string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ok, err := playlistExists(tx, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	if _, err = tx.Exec(`DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?`, id, trackID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, TS(time.Now()), id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePlaylistTracksForTrack 曲目被删除时清理所有歌单引用，避免详情里出现幽灵 ID。
func (db *DB) DeletePlaylistTracksForTrack(trackID string) error {
	_, err := db.conn.Exec(`DELETE FROM playlist_tracks WHERE track_id = ?`, trackID)
	return err
}
