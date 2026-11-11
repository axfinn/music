// Package models 提供 SQLite 连接底座与音乐/歌词模块的表初始化注册机制。
//
// 用 RegisterInit / InitAll 让各模块在建表阶段自行注册，避免一个中心文件
// 里堆满 CREATE TABLE。两个刻意的选型：
//   - 驱动用 modernc.org/sqlite（纯 Go，免 CGO，跨平台二进制一行编译）；
//   - 不依赖驱动的 parse_time，改用自定义 TS 时间戳类型兼容历史库里的多种文本格式。
package models

import (
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
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

// 历史数据里的时间有四种写法，都得认：
//
//	"2026-11-10 15:30:06"                       本项目写入的规范格式
//	"2026-11-10 15:30:06.560582875+08:00"       旧系统迁移过来的原始文本
//	"2026-11-10 15:30:06.560582875 +0800 +0800" Go time.String() 的时区重复写法
//	"2026-11-10T15:30:06Z" / "2026-11-05"       RFC3339 / 纯日期
//
// 一条脏记录不能让整页列表 500，所以解析要宽容，落库仍用规范格式。
var (
	tsDateTimeRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:([Zz])|[ ]?([+-])(\d{2}):?(\d{2}))?`)
	tsDateOnlyRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
)

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
	if s == "" {
		*t = TS(time.Time{})
		return nil
	}
	if m := tsDateTimeRe.FindStringSubmatch(s); m != nil {
		tm := time.Date(
			tsAtoi(m[1]), time.Month(tsAtoi(m[2])), tsAtoi(m[3]),
			tsAtoi(m[4]), tsAtoi(m[5]), tsAtoi(m[6]),
			tsFraction(m[7]), tsLocation(m[8], m[9], m[10], m[11]),
		)
		*t = TS(tm)
		return nil
	}
	if m := tsDateOnlyRe.FindStringSubmatch(s); m != nil {
		*t = TS(time.Date(tsAtoi(m[1]), time.Month(tsAtoi(m[2])), tsAtoi(m[3]), 0, 0, 0, 0, time.Local))
		return nil
	}
	// 兜底：交给标准库再试一遍（不常见的布局）。
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, tsLayout} {
		if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			*t = TS(parsed)
			return nil
		}
	}
	return fmt.Errorf("TS.parse: 无法解析时间 %q", s)
}

func tsAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// tsFraction 把任意位数的秒小数补/截成纳秒。
func tsFraction(s string) int {
	if s == "" {
		return 0
	}
	if len(s) > 9 {
		s = s[:9]
	}
	for len(s) < 9 {
		s += "0"
	}
	return tsAtoi(s)
}

// tsLocation 有时区偏移就按固定时区解释，没有则按本地时区（历史数据都是本地时间）。
func tsLocation(z, sign, hour, min string) *time.Location {
	if z != "" {
		return time.UTC
	}
	if sign == "" {
		return time.Local
	}
	off := tsAtoi(hour)*3600 + tsAtoi(min)*60
	if sign == "-" {
		off = -off
	}
	return time.FixedZone("", off)
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

// IsZero 判断是否为零值（无对应时间）。
func (t TS) IsZero() bool {
	return time.Time(t).IsZero()
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
