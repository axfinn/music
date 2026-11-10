package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 配置这层最容易出的错是「升级后口令读不到」：老部署用的是
// lyrics.admin_password / LYRICS_ADMIN_PASSWORD，新代码只认 admin.password。
// 这里把兼容链路钉住：旧写法继续生效，新写法优先。

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return path
}

func TestAdminPasswordFromYAML(t *testing.T) {
	path := writeTemp(t, "admin:\n  password: yaml-new\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "yaml-new" {
		t.Fatalf("admin.password 未生效，实际 %q", cfg.Admin.Password)
	}
}

func TestLegacyYAMLAdminPasswordMigrated(t *testing.T) {
	path := writeTemp(t, "lyrics:\n  admin_password: yaml-legacy\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "yaml-legacy" {
		t.Fatalf("旧配置项 lyrics.admin_password 应被迁移，实际 %q", cfg.Admin.Password)
	}
}

func TestNewYAMLBeatsLegacy(t *testing.T) {
	path := writeTemp(t, "admin:\n  password: new\nlyrics:\n  admin_password: legacy\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "new" {
		t.Fatalf("admin.password 应优先于 lyrics.admin_password，实际 %q", cfg.Admin.Password)
	}
}

func TestAdminPasswordFromEnv(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "env-new")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "env-new" {
		t.Fatalf("ADMIN_PASSWORD 未生效，实际 %q", cfg.Admin.Password)
	}
}

func TestLegacyEnvFallback(t *testing.T) {
	// 老 compose 里写的是 LYRICS_ADMIN_PASSWORD，升级后必须仍然能进后台。
	t.Setenv("LYRICS_ADMIN_PASSWORD", "env-legacy")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "env-legacy" {
		t.Fatalf("未回退到 LYRICS_ADMIN_PASSWORD，实际 %q", cfg.Admin.Password)
	}

	// 两个都在时，新名字优先。
	t.Setenv("ADMIN_PASSWORD", "env-new")
	cfg, err = Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Admin.Password != "env-new" {
		t.Fatalf("ADMIN_PASSWORD 应优先，实际 %q", cfg.Admin.Password)
	}
}
