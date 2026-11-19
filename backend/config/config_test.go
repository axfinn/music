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

// ── 反爬 / 防批量拖库配置 ──
// 默认必须是「开启」：这是私人曲库，漏配一个开关就等于全库裸奔。

func TestSecurityDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if !cfg.Security.AntiCrawl {
		t.Fatal("反爬默认应开启")
	}
	if cfg.Security.AnonymousReadRate != DefaultAntiCrawlReadRate ||
		cfg.Security.BulkReadRate != DefaultAntiCrawlBulkRate ||
		cfg.Security.StreamRate != DefaultAntiCrawlStreamRate {
		t.Fatalf("分级限流默认值不对: %+v", cfg.Security)
	}
}

func TestTrustedDomainsDefaultFollowSiteBaseURL(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(cfg.Security.TrustedDomains) != 1 || cfg.Security.TrustedDomains[0] != "jaxiu.cn" {
		t.Fatalf("默认应信任站点主域 jaxiu.cn，实际 %v", cfg.Security.TrustedDomains)
	}

	t.Setenv("SITE_BASE_URL", "https://music.example.com")
	cfg, err = Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(cfg.Security.TrustedDomains) != 1 || cfg.Security.TrustedDomains[0] != "example.com" {
		t.Fatalf("换域名后信任域应跟着变，实际 %v", cfg.Security.TrustedDomains)
	}
}

func TestTrustedDomainsFromYAMLAndEnv(t *testing.T) {
	path := writeTemp(t, "security:\n  trusted_domains: [\"*.foo.cn\", \"https://bar.example.org/path\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(cfg.Security.TrustedDomains) != 2 ||
		cfg.Security.TrustedDomains[0] != "foo.cn" || cfg.Security.TrustedDomains[1] != "bar.example.org" {
		t.Fatalf("YAML 可信域未规范化: %v", cfg.Security.TrustedDomains)
	}

	t.Setenv("ANTI_CRAWL_TRUSTED_DOMAINS", "demo.jaxiu.cn, other.example.com")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(cfg.Security.TrustedDomains) != 2 || cfg.Security.TrustedDomains[0] != "demo.jaxiu.cn" {
		t.Fatalf("环境变量应覆盖 YAML: %v", cfg.Security.TrustedDomains)
	}
}

func TestSecurityFromYAMLAndNormalized(t *testing.T) {
	path := writeTemp(t, "security:\n  anti_crawl: false\n  bulk_read_rate: 3\n  stream_rate: 0\n  allow_user_agents: [\"my-monitor/1.0\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Security.AntiCrawl {
		t.Fatal("YAML 里显式关闭应生效")
	}
	if cfg.Security.BulkReadRate != 3 || len(cfg.Security.AllowUserAgents) != 1 {
		t.Fatalf("security 段未生效: %+v", cfg.Security)
	}
	// 0 不能解释成「不限流」，否则一个误配就放开全库。
	if cfg.Security.StreamRate != DefaultAntiCrawlStreamRate {
		t.Fatalf("非法额度应回填默认值，实际 %d", cfg.Security.StreamRate)
	}
}

func TestSecurityFromEnv(t *testing.T) {
	t.Setenv("ANTI_CRAWL", "false")
	t.Setenv("ANTI_CRAWL_BULK_RATE", "7")
	t.Setenv("ANTI_CRAWL_BLOCK_UA", "my-scanner, another-bot")
	t.Setenv("ANTI_CRAWL_ALLOW_UA", "my-monitor/1.0")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Security.AntiCrawl || cfg.Security.BulkReadRate != 7 {
		t.Fatalf("反爬环境变量未生效: %+v", cfg.Security)
	}
	if len(cfg.Security.BlockUserAgents) != 2 || cfg.Security.BlockUserAgents[1] != "another-bot" {
		t.Fatalf("UA 黑名单解析不对: %v", cfg.Security.BlockUserAgents)
	}
	if len(cfg.Security.AllowUserAgents) != 1 {
		t.Fatalf("UA 白名单解析不对: %v", cfg.Security.AllowUserAgents)
	}
}

func TestAuthAndMailFromEnv(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "false")
	t.Setenv("AUTH_CODE_TTL_MINUTES", "5")
	t.Setenv("AUTH_SESSION_TTL_HOURS", "48")
	t.Setenv("MAIL_CLIENT_ID", "mt_test")
	t.Setenv("MAIL_CLIENT_SECRET", "secret-test")
	t.Setenv("MAIL_FROM", " sender@example.com ")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Auth.Enabled || cfg.Auth.CodeTTLMinutes != 5 || cfg.Auth.SessionTTLHours != 48 {
		t.Fatalf("认证环境变量未生效: %+v", cfg.Auth)
	}
	if cfg.Mail.ClientID != "mt_test" || cfg.Mail.ClientSecret != "secret-test" {
		t.Fatalf("邮件凭据未生效: %+v", cfg.Mail)
	}
	if cfg.Mail.From != "sender@example.com" {
		t.Fatalf("发件人应去空白, got %q", cfg.Mail.From)
	}
	// AUTH_ENABLED=false 时，即使凭据齐全也不开放会员入口。
	if cfg.MailConfigured() {
		t.Fatal("AUTH_ENABLED=false 时不应开放会员认证")
	}
}

func TestMailConfiguredRequiresCredentials(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MailConfigured() {
		t.Fatal("未配凭据时不应开放会员认证")
	}
	cfg.Mail.ClientID = "mt_x"
	if cfg.MailConfigured() {
		t.Fatal("只有 client_id 时不应开放")
	}
	cfg.Mail.ClientSecret = "s"
	if !cfg.MailConfigured() {
		t.Fatal("凭据齐全后应开放")
	}
}

func TestAuthInvalidValuesFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.CodeTTLMinutes = -1
	cfg.Auth.SessionTTLHours = 0
	cfg.Mail.BaseURL = ""
	normalizeAuth(cfg)
	if cfg.Auth.CodeTTLMinutes != DefaultAuthCodeTTLMinutes || cfg.Auth.SessionTTLHours != DefaultAuthSessionTTLHours {
		t.Fatalf("非法认证参数应回填默认值: %+v", cfg.Auth)
	}
	if cfg.Mail.BaseURL != DefaultMailBaseURL {
		t.Fatalf("空 base_url 应回填默认值, got %q", cfg.Mail.BaseURL)
	}
}
