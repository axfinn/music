package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// ── 邮箱验证码 ──
// 验证码只存哈希（绑定邮箱与用途），明文只在发信那一刻存在；
// 每个邮箱+用途只保留一枚有效码，发新码会作废旧码；
// 校验错误累加尝试次数，超限即作废，防止 6 位码被在线穷举。

var (
	ErrCodeInvalid      = errors.New("验证码不正确或已失效")
	ErrCodeExpired      = errors.New("验证码已过期，请重新获取")
	ErrCodeTooManyTries = errors.New("验证码错误次数过多，请重新获取")
	ErrCodeRateLimited  = errors.New("获取太频繁，请稍后再试")
)

const (
	codePurposeRegister = "register"
	codePurposeLogin    = "login"
	codePurposeReset    = "reset"
	codeMaxAttempts     = 5
	codeDigits          = 6
)

func init() {
	RegisterInit("邮箱验证码(email_codes)", (*DB).InitEmailCodes)
}

// EmailCode 一枚验证码记录（不含明文）。
// 过期时间用 Unix 秒存 INTEGER，不走 DATETIME/TS 往返——避免驱动把无时区的
// DATETIME 当 UTC 解析，导致「已过期」被误判成「未过期」。
type EmailCode struct {
	ID        string `json:"-"`
	Email     string `json:"-"`
	Purpose   string `json:"-"`
	Attempts  int    `json:"-"`
	ExpiresAt int64  `json:"-"`
	CreatedAt TS     `json:"-"`
}

func (db *DB) InitEmailCodes() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS email_codes (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			purpose TEXT NOT NULL,
			code_hash TEXT NOT NULL,
			attempts INTEGER DEFAULT 0,
			consumed INTEGER DEFAULT 0,
			expires_at INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_email_codes_lookup ON email_codes(email, purpose, consumed, created_at DESC);
	`)
	return err
}

// hashEmailCode 把验证码与邮箱、用途绑定后做 sha256。
// 绑定上下文能防止一枚码被挪到别的邮箱/用途上复用，也让离线彩虹表失效。
func hashEmailCode(email, purpose, code string) string {
	sum := sha256.Sum256([]byte(email + "|" + purpose + "|" + code))
	return hex.EncodeToString(sum[:])
}

// randomCode 生成 n 位数字验证码（用 crypto/rand，避免 math/rand 可预测）。
func randomCode(n int) (string, error) {
	buf := make([]byte, n)
	for i := range buf {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		buf[i] = byte('0' + v.Int64())
	}
	return string(buf), nil
}

// CreateEmailCode 为新请求生成验证码。
// 同一邮箱+用途的旧未消费码会先作废；返回明文供发信，库里只留哈希。
// ttl 为有效期；rateLimit<=0 表示不检查频率。
func (db *DB) CreateEmailCode(email, purpose string, ttl time.Duration, rateLimit int) (string, error) {
	email = NormalizeEmail(email)
	if email == "" || (purpose != codePurposeRegister && purpose != codePurposeLogin && purpose != codePurposeReset) {
		return "", errors.New("无效的验证码请求")
	}
	now := time.Now()

	// 频率检查：统计最近 1 分钟内的发送次数（含已消费/作废的，防止绕过）。
	if rateLimit > 0 {
		var n int
		since := TS(now.Add(-time.Minute))
		err := db.conn.QueryRow(`SELECT COUNT(*) FROM email_codes
			WHERE email = ? AND purpose = ? AND created_at > ?`, email, purpose, since).Scan(&n)
		if err != nil {
			return "", err
		}
		if n >= rateLimit {
			return "", ErrCodeRateLimited
		}
	}

	plain, err := randomCode(codeDigits)
	if err != nil {
		return "", err
	}
	// 单连接串行写：作废旧码 + 插入新码放在一个事务里，避免并发下出现两枚有效码。
	tx, err := db.conn.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE email_codes SET consumed = 1
		WHERE email = ? AND purpose = ? AND consumed = 0`, email, purpose); err != nil {
		return "", err
	}
	_, err = tx.Exec(`INSERT INTO email_codes (id, email, purpose, code_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"ec_"+generateID(16), email, purpose, hashEmailCode(email, purpose, plain), now.Add(ttl).Unix(), TS(now))
	if err != nil {
		return "", err
	}
	return plain, tx.Commit()
}

// VerifyEmailCode 校验并消费一枚验证码。成功返回 nil，失败返回对应 ErrCode*。
// 校验通过后立即消费，防止同一枚码重复使用。
func (db *DB) VerifyEmailCode(email, purpose, code string) error {
	email = NormalizeEmail(email)
	var (
		id       string
		hash     string
		attempts int
		expires  int64
		consumed int
	)
	err := db.conn.QueryRow(`SELECT id, code_hash, attempts, expires_at, consumed
		FROM email_codes WHERE email = ? AND purpose = ? AND consumed = 0
		ORDER BY created_at DESC LIMIT 1`, email, purpose).
		Scan(&id, &hash, &attempts, &expires, &consumed)
	if errors.Is(err, sql.ErrNoRows) || consumed != 0 {
		return ErrCodeInvalid
	}
	if err != nil {
		return err
	}
	if time.Now().Unix() > expires {
		return ErrCodeExpired
	}
	if attempts >= codeMaxAttempts {
		_, _ = db.conn.Exec(`UPDATE email_codes SET consumed = 1 WHERE id = ?`, id)
		return ErrCodeTooManyTries
	}
	want, err := hex.DecodeString(hash)
	if err != nil {
		return ErrCodeInvalid
	}
	got := sha256.Sum256([]byte(email + "|" + purpose + "|" + code))
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		_, _ = db.conn.Exec(`UPDATE email_codes SET attempts = attempts + 1 WHERE id = ?`, id)
		return ErrCodeInvalid
	}
	res, err := db.conn.Exec(`UPDATE email_codes SET consumed = 1 WHERE id = ? AND consumed = 0`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCodeInvalid // 并发下已被别人消费
	}
	return nil
}

// Purpose 常量导出供 handlers 使用（避免上层写魔法字符串）。
const (
	CodePurposeRegister = codePurposeRegister
	CodePurposeLogin    = codePurposeLogin
	CodePurposeReset    = codePurposeReset
)

// String 便于日志/调试输出（不含验证码内容）。
func (e *EmailCode) String() string {
	return fmt.Sprintf("EmailCode{email:%s purpose:%s}", e.Email, e.Purpose)
}
