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

// ── 站点身份与对外接口配置 ──
// 独立站把 site / api 提到顶层，旧配置里写在 lyrics 段的值必须继续生效。

func TestSiteDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Site.Name != DefaultSiteName || cfg.Site.BaseURL != DefaultSiteBaseURL {
		t.Fatalf("站点默认值不对: %+v", cfg.Site)
	}
	if cfg.API.KeyRateLimit != DefaultAPIKeyRateLimit {
		t.Fatalf("密钥限流默认值不对: %d", cfg.API.KeyRateLimit)
	}
}

func TestSiteFromYAMLAndEnv(t *testing.T) {
	path := writeTemp(t, "site:\n  name: 我的曲库\n  tagline: 私人电台\n  base_url: https://example.com\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Site.Name != "我的曲库" || cfg.Site.Tagline != "私人电台" {
		t.Fatalf("站点 YAML 未生效: %+v", cfg.Site)
	}
	// 没写 description 时要保留默认值，而不是被零值覆盖。
	if cfg.Site.Description != DefaultSiteDescription {
		t.Fatalf("缺省项应保留默认值，实际 %q", cfg.Site.Description)
	}
	t.Setenv("SITE_NAME", "env-station")
	t.Setenv("SITE_BASE_URL", "https://music.jaxiu.cn")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Site.Name != "env-station" || cfg.Site.BaseURL != "https://music.jaxiu.cn" {
		t.Fatalf("环境变量应覆盖 YAML: %+v", cfg.Site)
	}
}

func TestAPIConfigFromYAML(t *testing.T) {
	path := writeTemp(t, "api:\n  key_rate_limit: 12\n  allow_origins: [\"https://music.jaxiu.cn\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.API.KeyRateLimit != 12 || len(cfg.API.AllowOrigins) != 1 {
		t.Fatalf("api 段未生效: %+v", cfg.API)
	}
}

func TestLegacyAPIConfigMigrated(t *testing.T) {
	path := writeTemp(t, "lyrics:\n  api_key_rate_limit: 17\n  allow_origins: [\"https://old.example.com\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.API.KeyRateLimit != 17 {
		t.Fatalf("旧 lyrics.api_key_rate_limit 应迁移，实际 %d", cfg.API.KeyRateLimit)
	}
	if len(cfg.API.AllowOrigins) != 1 || cfg.API.AllowOrigins[0] != "https://old.example.com" {
		t.Fatalf("旧 lyrics.allow_origins 应迁移，实际 %v", cfg.API.AllowOrigins)
	}
}

func TestNewAPIConfigBeatsLegacy(t *testing.T) {
	path := writeTemp(t, "api:\n  key_rate_limit: 30\n  allow_origins: [\"https://new.example.com\"]\nlyrics:\n  api_key_rate_limit: 17\n  allow_origins: [\"https://old.example.com\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.API.KeyRateLimit != 30 || cfg.API.AllowOrigins[0] != "https://new.example.com" {
		t.Fatalf("顶层 api 应优先于旧配置: %+v", cfg.API)
	}
}

func TestAPIConfigFromEnv(t *testing.T) {
	t.Setenv("API_KEY_RATE_LIMIT", "9")
	t.Setenv("API_ALLOW_ORIGINS", "https://a.example.com, https://b.example.com")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.API.KeyRateLimit != 9 || len(cfg.API.AllowOrigins) != 2 {
		t.Fatalf("API 环境变量未生效: %+v", cfg.API)
	}
	if cfg.API.AllowOrigins[1] != "https://b.example.com" {
		t.Fatalf("逗号分隔的 origin 应去掉空格: %v", cfg.API.AllowOrigins)
	}
}
