package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

// ── 歌词库对外 API 密钥(Lyric API Key) ──
// 对外暴露 /api/v1/lyrics/* 时使用。约定：
//   - 明文只在创建那一刻返回一次，库里只存 sha256(明文) 的十六进制，泄露数据库也拿不到可用 key；
//   - 前缀字段保留头 8 位，便于在列表里辨认是哪一把，不构成凭据；
//   - 撤销用 enabled=0，保留记录与调用次数，便于排查"谁在调"。
// 与站内密码（X-Admin-Password）是两套东西：密码管"改密钥"，密钥管"调接口"。

var (
	// ErrLyricAPIKeyNotFound 密钥不存在。
	ErrLyricAPIKeyNotFound = errors.New("密钥不存在")
	// ErrLyricAPIKeyInvalid 密钥无效、已撤销或格式不对。
	ErrLyricAPIKeyInvalid = errors.New("密钥无效或已撤销")
)

const (
	lyricAPIKeyPrefix   = "lly_"
	lyricAPIKeyRandLen  = 40 // 明文随机部分长度（hex 字符数 → 160 bit 熵）
	lyricAPIKeyHeadKeep = 8
)

func init() {
	RegisterInit("歌词库密钥(lyric_api_keys)", (*DB).InitLyricAPIKeys)
}

// LyricAPIKey 一把对外调用密钥（不含明文）。
type LyricAPIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"` // 形如 lly_1a2b，仅用于辨认
	Hash       string     `json:"-"`
	Enabled    bool       `json:"enabled"`
	CallCount  int        `json:"call_count"`
	CreatedAt  TS         `json:"created_at"`
	LastUsedAt *TS        `json:"last_used_at"`
}

func (db *DB) InitLyricAPIKeys() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS lyric_api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL DEFAULT '',
			hash TEXT NOT NULL UNIQUE,
			enabled INTEGER DEFAULT 1,
			call_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_lyric_api_keys_enabled ON lyric_api_keys(enabled);
	`)
	return err
}

// HashLyricAPIKey 明文 → 存储用哈希。确定性哈希是为了能按 hash 走唯一索引直接查。
func HashLyricAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func newLyricAPIKeyPlain() (string, error) {
	buf := make([]byte, lyricAPIKeyRandLen/2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return lyricAPIKeyPrefix + hex.EncodeToString(buf), nil
}

// CreateLyricAPIKey 新建一把密钥，返回明文（仅此一次）。
func (db *DB) CreateLyricAPIKey(name string) (*LyricAPIKey, string, error) {
	plain, err := newLyricAPIKeyPlain()
	if err != nil {
		return nil, "", err
	}
	key := &LyricAPIKey{
		ID:        "lak_" + generateID(12),
		Name:      name,
		Prefix:    plain[:lyricAPIKeyHeadKeep],
		Hash:      HashLyricAPIKey(plain),
		Enabled:   true,
		CreatedAt: TS(time.Now()),
	}
	_, err = db.conn.Exec(`
		INSERT INTO lyric_api_keys (id, name, prefix, hash, enabled, call_count, created_at)
		VALUES (?, ?, ?, ?, 1, 0, ?)`,
		key.ID, key.Name, key.Prefix, key.Hash, key.CreatedAt)
	if err != nil {
		return nil, "", err
	}
	return key, plain, nil
}

// ListLyricAPIKeys 列出全部密钥（按创建时间倒序）。
func (db *DB) ListLyricAPIKeys() ([]*LyricAPIKey, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, prefix, enabled, call_count, created_at, last_used_at
		FROM lyric_api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*LyricAPIKey, 0, 8)
	for rows.Next() {
		k := &LyricAPIKey{}
		lastUsed := new(TS)
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.Enabled, &k.CallCount, &k.CreatedAt, lastUsed); err != nil {
			return nil, err
		}
		if !lastUsed.IsZero() {
			k.LastUsedAt = lastUsed
		}
		items = append(items, k)
	}
	return items, rows.Err()
}

// SetLyricAPIKeyEnabled 启用/撤销一把密钥。
func (db *DB) SetLyricAPIKeyEnabled(id string, enabled bool) error {
	res, err := db.conn.Exec(`UPDATE lyric_api_keys SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLyricAPIKeyNotFound
	}
	return nil
}

// DeleteLyricAPIKey 物理删除（一般用撤销，删除只用于清理误建）。
func (db *DB) DeleteLyricAPIKey(id string) error {
	res, err := db.conn.Exec(`DELETE FROM lyric_api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLyricAPIKeyNotFound
	}
	return nil
}

// VerifyLyricAPIKey 校验明文密钥，命中则返回记录并累加调用次数。
// 用固定时间比较，避免按字节提前返回带来的时序差异。
func (db *DB) VerifyLyricAPIKey(plain string) (*LyricAPIKey, error) {
	if len(plain) < lyricAPIKeyHeadKeep || len(plain) > 128 {
		return nil, ErrLyricAPIKeyInvalid
	}
	hash := HashLyricAPIKey(plain)
	key := &LyricAPIKey{}
	lastUsed := new(TS)
	err := db.conn.QueryRow(`
		SELECT id, name, prefix, hash, enabled, call_count, created_at, last_used_at
		FROM lyric_api_keys WHERE hash = ?`, hash).
		Scan(&key.ID, &key.Name, &key.Prefix, &key.Hash, &key.Enabled, &key.CallCount, &key.CreatedAt, lastUsed)
	if err != nil {
		return nil, ErrLyricAPIKeyInvalid
	}
	if !lastUsed.IsZero() {
		key.LastUsedAt = lastUsed
	}
	if subtle.ConstantTimeCompare([]byte(key.Hash), []byte(hash)) != 1 || !key.Enabled {
		return nil, ErrLyricAPIKeyInvalid
	}
	now := TS(time.Now())
	_, _ = db.conn.Exec(`UPDATE lyric_api_keys SET call_count = call_count + 1, last_used_at = ? WHERE id = ?`, now, key.ID)
	key.CallCount++
	lu := now
	key.LastUsedAt = &lu
	return key, nil
}
