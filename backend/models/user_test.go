package models

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("建测试库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if hash == "" || len(hash) < 20 {
		t.Fatalf("哈希串异常: %q", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("正确密码应通过校验")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("错误密码不应通过校验")
	}
	// 两次哈希同密码应不同（随机盐）。
	hash2, _ := HashPassword("correct horse battery staple")
	if hash == hash2 {
		t.Fatal("两次哈希不应相同（盐应随机）")
	}
	// 损坏格式一律当作不匹配，不 panic。
	for _, bad := range []string{"", "garbage", "pbkdf2$notint$xx$yy", "pbkdf2$210000$nothex$nothex"} {
		if VerifyPassword("anything", bad) {
			t.Fatalf("损坏的哈希串 %q 不应通过校验", bad)
		}
	}
}

func TestCreateUserAndNormalizeEmail(t *testing.T) {
	db := newTestDB(t)
	hash, _ := HashPassword("secret123")
	u, err := db.CreateUser("  Alice@Example.COM ", hash, "爱丽丝")
	if err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("邮箱应规范化为小写，got %q", u.Email)
	}
	if u.Nickname != "爱丽丝" {
		t.Fatalf("昵称不符: %q", u.Nickname)
	}
	// 大小写不同也算重复。
	_, err = db.CreateUser("ALICE@example.com", hash, "")
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("重复邮箱应返回 ErrUserExists, got %v", err)
	}
	got, err := db.GetUserByEmail("alice@example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("按邮箱查回用户失败: %v %+v", err, got)
	}
	if !VerifyPassword("secret123", got.PasswordHash) {
		t.Fatal("库里密码哈希应能验证原密码")
	}
	if _, err := db.GetUserByEmail("nobody@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("不存在的邮箱应返回 ErrUserNotFound, got %v", err)
	}
}

func TestSetPasswordAndNickname(t *testing.T) {
	db := newTestDB(t)
	hash, _ := HashPassword("old-pass")
	u, _ := db.CreateUser("bob@example.com", hash, "")
	newHash, _ := HashPassword("new-pass")
	if err := db.SetPassword(u.ID, newHash); err != nil {
		t.Fatalf("改密码失败: %v", err)
	}
	got, _ := db.GetUserByEmail("bob@example.com")
	if !VerifyPassword("new-pass", got.PasswordHash) || VerifyPassword("old-pass", got.PasswordHash) {
		t.Fatal("改密码后应只有新密码有效")
	}
	if err := db.UpdateNickname(u.ID, "鲍勃"); err != nil {
		t.Fatalf("改昵称失败: %v", err)
	}
	got, _ = db.GetUserByEmail("bob@example.com")
	if got.Nickname != "鲍勃" {
		t.Fatalf("昵称不符: %q", got.Nickname)
	}
}

func TestEmailCodeLifecycle(t *testing.T) {
	db := newTestDB(t)
	email := "cindy@example.com"

	// 频率限制：每邮箱每用途每分钟 3 次。
	for i := 0; i < 3; i++ {
		if _, err := db.CreateEmailCode(email, CodePurposeRegister, 10*time.Minute, 3); err != nil {
			t.Fatalf("第 %d 次发码失败: %v", i+1, err)
		}
	}
	if _, err := db.CreateEmailCode(email, CodePurposeRegister, 10*time.Minute, 3); !errors.Is(err, ErrCodeRateLimited) {
		t.Fatalf("第 4 次应被限流, got %v", err)
	}

	// 重新发一枚（不同用途不受同邮箱 register 限流影响），旧码作废。
	code, err := db.CreateEmailCode(email, CodePurposeLogin, 10*time.Minute, 3)
	if err != nil {
		t.Fatalf("发登录码失败: %v", err)
	}
	if err := db.VerifyEmailCode(email, CodePurposeRegister, code); !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("登录码不能当注册码用, got %v", err)
	}

	// 错误码累加，5 次后作废。
	for i := 0; i < codeMaxAttempts; i++ {
		if err := db.VerifyEmailCode(email, CodePurposeLogin, "000999"); !errors.Is(err, ErrCodeInvalid) {
			t.Fatalf("第 %d 次错误码应返回 ErrCodeInvalid, got %v", i+1, err)
		}
	}
	if err := db.VerifyEmailCode(email, CodePurposeLogin, code); !errors.Is(err, ErrCodeTooManyTries) {
		t.Fatalf("超限后应返回 ErrCodeTooManyTries, got %v", err)
	}
}

func TestEmailCodeSuccessAndSingleUse(t *testing.T) {
	db := newTestDB(t)
	email := "dan@example.com"
	code, err := db.CreateEmailCode(email, CodePurposeReset, 10*time.Minute, 3)
	if err != nil {
		t.Fatalf("发码失败: %v", err)
	}
	if err := db.VerifyEmailCode(email, CodePurposeReset, code); err != nil {
		t.Fatalf("正确码应通过: %v", err)
	}
	// 消费后不能重复使用。
	if err := db.VerifyEmailCode(email, CodePurposeReset, code); !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("验证码应为一次性, got %v", err)
	}
}

func TestEmailCodeExpiry(t *testing.T) {
	db := newTestDB(t)
	email := "eve@example.com"
	// 已经过期的码。
	code, err := db.CreateEmailCode(email, CodePurposeRegister, -time.Minute, 0)
	if err != nil {
		t.Fatalf("发码失败: %v", err)
	}
	if err := db.VerifyEmailCode(email, CodePurposeRegister, code); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("过期码应返回 ErrCodeExpired, got %v", err)
	}
}
