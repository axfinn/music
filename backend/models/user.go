package models

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ── 邮箱会员 ──
// 会员是独立于管理员的身份：会员靠邮箱验证码注册/登录，解决「你是谁」；
// 管理员靠 admin 口令进后台。两套身份互不授权，会员 Upgrade 不成管理员。
//
// 密码只存 PBKDF2-HMAC-SHA256 派生值（带随机盐），明文不落库、不进日志。
// 存储格式：pbkdf2$<迭代次数>$<盐 hex>$<派生值 hex>，迭代数写进串里，
// 以后调参不用强制所有老用户改密码。

var (
	// ErrUserExists 邮箱已注册。
	ErrUserExists = errors.New("该邮箱已注册")
	// ErrUserNotFound 用户不存在。
	ErrUserNotFound = errors.New("用户不存在")
	// ErrBadPassword 密码错误。
	ErrBadPassword = errors.New("密码不正确")
)

const (
	userIDPrefix      = "usr_"
	pbkdf2Iterations  = 210000 // OWASP 对 PBKDF2-HMAC-SHA256 的推荐下限
	pbkdf2KeyLen      = 32
	pbkdf2SaltLen     = 16
	userNicknameLimit = 32
)

func init() {
	RegisterInit("邮箱会员(users)", (*DB).InitUsers)
}

// User 一名邮箱会员（不含密码明文/哈希，哈希字段 json 不导出）。
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Nickname     string `json:"nickname"`
	CreatedAt    TS     `json:"created_at"`
	LastLoginAt  *TS    `json:"last_login_at"`
}

func (db *DB) InitUsers() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			nickname TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email COLLATE NOCASE);
	`)
	return err
}

// NormalizeEmail 统一邮箱写法：去空白 + 转小写。注册和登录都走它，
// 保证 A@B.com 与 a@b.com 是同一个人。
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// HashPassword 生成可入库的密码串。 errors 只在系统熵源失败时返回。
func HashPassword(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iterations,
		hex.EncodeToString(salt), hex.EncodeToString(derived)), nil
}

// VerifyPassword 校验明文密码与库中串是否匹配。格式损坏一律当作不匹配，
// 不把错误详情透给调用方（避免变成密码格式探针）。
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter < 10000 || iter > 10000000 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// CreateUser 注册新会员。邮箱重复返回 ErrUserExists。
func (db *DB) CreateUser(email, passwordHash, nickname string) (*User, error) {
	email = NormalizeEmail(email)
	if email == "" || passwordHash == "" {
		return nil, errors.New("邮箱与密码不能为空")
	}
	if len(nickname) > userNicknameLimit {
		nickname = nickname[:userNicknameLimit]
	}
	u := &User{
		ID:           userIDPrefix + generateID(16),
		Email:        email,
		PasswordHash: passwordHash,
		Nickname:     nickname,
		CreatedAt:    TS(time.Now()),
	}
	_, err := db.conn.Exec(`
		INSERT INTO users (id, email, password_hash, nickname, created_at)
		VALUES (?, ?, ?, ?, ?)`, u.ID, u.Email, u.PasswordHash, u.Nickname, u.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

// GetUserByEmail 按邮箱查会员。
func (db *DB) GetUserByEmail(email string) (*User, error) {
	return db.getUser(`SELECT id, email, password_hash, nickname, created_at, last_login_at
		FROM users WHERE email = ? COLLATE NOCASE`, NormalizeEmail(email))
}

// GetUserByID 按 ID 查会员（会话恢复用）。
func (db *DB) GetUserByID(id string) (*User, error) {
	return db.getUser(`SELECT id, email, password_hash, nickname, created_at, last_login_at
		FROM users WHERE id = ?`, id)
}

func (db *DB) getUser(query string, args ...any) (*User, error) {
	u := &User{}
	lastLogin := new(TS)
	err := db.conn.QueryRow(query, args...).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Nickname, &u.CreatedAt, lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if !lastLogin.IsZero() {
		u.LastLoginAt = lastLogin
	}
	return u, nil
}

// SetPassword 重置密码（找回流程用）。
func (db *DB) SetPassword(id, passwordHash string) error {
	if passwordHash == "" {
		return errors.New("密码哈希不能为空")
	}
	res, err := db.conn.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// TouchLogin 记录最近登录时间，顺便让会员列表能看出活跃度。
func (db *DB) TouchLogin(id string) {
	_, _ = db.conn.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, TS(time.Now()), id)
}

// UpdateNickname 修改昵称（空串表示恢复默认）。
func (db *DB) UpdateNickname(id, nickname string) error {
	if len(nickname) > userNicknameLimit {
		nickname = nickname[:userNicknameLimit]
	}
	res, err := db.conn.Exec(`UPDATE users SET nickname = ? WHERE id = ?`, nickname, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}
