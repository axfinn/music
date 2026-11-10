// Package models 提供 SQLite 连接底座与音乐/歌词模块的表初始化注册机制。
//
// 与 devtools 的 models 包保持相同的 RegisterInit / InitAll 设计，但：
//   - 驱动换成 modernc.org/sqlite（纯 Go，免 CGO，跨平台二进制一行编译）；
//   - 不再使用 mattn 的 _parse_time=true，改用自定义 TS 时间戳类型解析 SQLite 文本时间。
package models

import (
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB 封装 *sql.DB，所有表初始化通过 RegisterInit 注册后由 InitAll 统一跑。
type DB struct {
	conn *sql.DB
}

var initRegistry []struct {
	name string
	fn   func(*DB) error
}

// RegisterInit 注册一张表的初始化函数，NewDB 时统一执行。
func RegisterInit(name string, fn func(*DB) error) {
	initRegistry = append(initRegistry, struct {
		name string
		fn   func(*DB) error
	}{name, fn})
}

// InitAll 执行所有已注册表的初始化。
func (db *DB) InitAll() error {
	for _, r := range initRegistry {
		if err := r.fn(db); err != nil {
			return fmt.Errorf("初始化 %s 失败: %w", r.name, err)
		}
	}
	return nil
}

// NewDB 打开 SQLite（modernc，纯 Go），并跑全部表初始化。
func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// 单连接写友好：SQLite 默认连接池在多写场景下易锁；适度放宽忙等待。
	conn.SetMaxOpenConns(1)
	db := &DB{conn: conn}
	if err := db.InitAll(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

// Conn 暴露底层 *sql.DB（供需要在主连接上跑自定义语句的场景）。
func (db *DB) Conn() *sql.DB { return db.conn }

// Close 关闭连接。
func (db *DB) Close() error { return db.conn.Close() }

// generateID 生成带前缀的随机 ID：prefix + n 个十六进制字符（约 4n 比特）。
func generateID(n int) string {
	b := make([]byte, n/2+1)
	if _, err := rand.Read(b); err != nil {
		// 极不可能失败；失败退化为时间戳哈希。
		return fmt.Sprintf("%x%d", []byte(time.Now().String()), n)
	}
	return strings.TrimRight(hex.EncodeToString(b), "0")[:n]
}

// ── TS：SQLite 文本时间戳 ↔ time.Time 的安全转换 ──
//
// modernc 不识别 mattn 的 _parse_time=true，SQLite 的 DATETIME 默认以
// "2006-01-02 15:04:05"（本地时区）文本存储。Scan 时按多格式兼容解析，
// Value 落库统一用同一布局；JSON 仍按 RFC3339 输出，保持 API 契约不变。
type TS time.Time

const tsLayout = "2006-01-02 15:04:05"

// Scan 兼容 SQLite 的 "2006-01-02 15:04:05" 与 RFC3339 / 带时区格式。
func (t *TS) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*t = TS(time.Time{})
	case time.Time:
		*t = TS(x)
	case []byte:
		return t.parse(string(x))
	case string:
		return t.parse(x)
	default:
		return fmt.Errorf("TS.Scan: 不支持的类型 %T", v)
	}
	return nil
}

func (t *TS) parse(s string) error {
	s = strings.TrimSpace(s)
	for _, layout := range []string{tsLayout, time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			*t = TS(parsed)
			return nil
		}
	}
	return fmt.Errorf("TS.parse: 无法解析时间 %q", s)
}

// Value 落库统一用 tsLayout 文本。
func (t TS) Value() (driver.Value, error) {
	tm := time.Time(t)
	if tm.IsZero() {
		return nil, nil
	}
	return tm.Format(tsLayout), nil
}

// Time 取底层 time.Time。
func (t TS) Time() time.Time { return time.Time(t) }

// MarshalJSON 按 RFC3339 输出，与原来 time.Time 的 JSON 一致。
func (t TS) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).Format(time.RFC3339) + `"`), nil
}

// UnmarshalJSON 接受 RFC3339 文本。
func (t *TS) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*t = TS(time.Time{})
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return t.parse(s)
	}
	*t = TS(parsed)
	return nil
}
