// Package config 提供 github.com/axfinn/music 的配置加载。
//
// 配置来源优先级：环境变量 > config.yaml > 默认值。
// 项目独立部署后，站点身份（site）与对外接口（api）是顶层能力，
// 不再挂在歌词库名下；旧的 lyrics.admin_password / lyrics.api_key_rate_limit /
// lyrics.allow_origins 仍会被自动迁移，升级不需要改配置文件。
package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// 站点与对外接口的默认值。改这里等于改「开箱即用」的站点身份。
const (
	DefaultSiteName        = "music"
	DefaultSiteTagline     = "独立音乐站"
	DefaultSiteDescription = "私人曲库与歌词档案：上传、流式播放、行级歌词检索与发现流。"
	DefaultSiteBaseURL     = "https://music.jaxiu.cn"
	DefaultAPIKeyRateLimit = 60

	// 匿名访客的读取额度（次 / 分钟 / IP）。分级是为了让「翻页听歌」
	// 依然顺畅，但「一次拉走整个曲库」的批量接口被单独卡住。
	DefaultAntiCrawlReadRate   = 180
	DefaultAntiCrawlBulkRate   = 40
	DefaultAntiCrawlStreamRate = 300
)

// Config 全局配置。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DataDir  string         `yaml:"data_dir"` // 音频/封面/SQLite 根目录，默认 ./data
	Admin    AdminConfig    `yaml:"admin"`
	Site     SiteConfig     `yaml:"site"`
	Music    MusicConfig    `yaml:"music"`
	Lyrics   LyricsConfig   `yaml:"lyrics"`
	API      APIConfig      `yaml:"api"`
	Security SecurityConfig `yaml:"security"`
}

// AdminConfig 后台管理口令。
//
// 站点分两种身份：
//   - 访客：浏览与播放曲库、看当前这首的歌词；
//   - 管理：额外能进后台操作歌词库、歌词发现、以及曲库的增删改。
//
// 留空 = 纯访客模式，后台接口一律关闭。
type AdminConfig struct {
	Password string `yaml:"password"` // 管理口令；留空=禁用后台
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// SiteConfig 站点身份：前端拿它渲染品牌、标题与分享卡片，外部系统拿它拼 canonical。
// 全部可覆盖；base_url 只用于生成绝对链接，不参与路由或鉴权。
type SiteConfig struct {
	Name        string `yaml:"name"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
	BaseURL     string `yaml:"base_url"`
}

// APIConfig 对外 /api/v1/* 的公共设置。
// 音乐与歌词两套 v1 接口共用同一把密钥、同一套 CORS 白名单与限流额度。
type APIConfig struct {
	KeyRateLimit int      `yaml:"key_rate_limit"` // 单密钥每分钟上限，默认 60
	AllowOrigins []string `yaml:"allow_origins"`  // 空 = 允许任意来源
}

// SecurityConfig 反爬 / 防批量拖库设置。
//
// 这套防护只针对「匿名站内接口」：/api/v1/* 走 API Key 授权，属于被允许的
// 程序化访问，不在这里拦；后台接口另有一层管理员会话。默认开启，
// 需要对接脚本或做压测时可整体关掉，或把指定 UA 加进 allow_user_agents。
type SecurityConfig struct {
	// 总开关。关掉后只保留 robots.txt / noindex 这类声明式约定。
	AntiCrawl bool `yaml:"anti_crawl"`

	AnonymousReadRate int `yaml:"anonymous_read_rate"` // 普通读接口：次/分钟/IP，默认 180
	BulkReadRate      int `yaml:"bulk_read_rate"`      // 全量/导出类接口：次/分钟/IP，默认 40
	StreamRate        int `yaml:"stream_rate"`         // 音频与封面：次/分钟/IP，默认 300

	// 额外要拦的 UA 片段（大小写不敏感），追加在内置黑名单之后。
	BlockUserAgents []string `yaml:"block_user_agents"`
	// 命中黑名单但仍要放行的 UA 片段，例如自家监控、备份脚本。
	AllowUserAgents []string `yaml:"allow_user_agents"`
}

// MusicConfig 音乐库配置。
type MusicConfig struct {
	AudioMaxBytes int64 `yaml:"audio_max_bytes"` // 单曲上限，默认 200MB
	CoverMaxBytes int64 `yaml:"cover_max_bytes"` // 封面上限，默认 15MB
}

// LyricsConfig 歌词库配置：站内在线抓取（lyrics_finder.py）与歌词发现候选池。
//
// 对外接口的密钥表由 models 维护、配置在 APIConfig，管理口令在 AdminConfig，
// 三者互不相干。旧配置里写在这一段的 api_key_rate_limit / allow_origins
// 会在 Load 时迁移到 APIConfig（见 migrateLegacyAPI）。
type LyricsConfig struct {
	PythonBin      string   `yaml:"python_bin"`      // 解释器路径；留空自动探测
	ScriptPath     string   `yaml:"script_path"`     // lyrics_finder.py 路径；留空自动探测
	TimeoutSeconds int      `yaml:"timeout_seconds"` // 单次抓取总超时，默认 45
	MaxConcurrent  int      `yaml:"max_concurrent"`  // 同时抓取进程上限，默认 3
	DiscoverySeeds []string `yaml:"discovery_seeds"` // 种子歌单（"歌名 - 艺人"）；留空用内置默认

	DiscoveryPoolSize       int `yaml:"discovery_pool_size"`       // 在线候选池上限，默认 24
	DiscoveryBatchSize      int `yaml:"discovery_batch_size"`      // 每次刷新抓几首，默认 4
	DiscoveryRefreshMinutes int `yaml:"discovery_refresh_minutes"` // 池刷新间隔（分钟），默认 30
}

// DefaultConfig 返回带合理默认值的配置。
func DefaultConfig() *Config {
	return &Config{
		Server:  ServerConfig{Host: "0.0.0.0", Port: 8080},
		DataDir: "./data",
		Site: SiteConfig{
			Name:        DefaultSiteName,
			Tagline:     DefaultSiteTagline,
			Description: DefaultSiteDescription,
			BaseURL:     DefaultSiteBaseURL,
		},
		Music: MusicConfig{AudioMaxBytes: 200 << 20, CoverMaxBytes: 15 << 20},
		Lyrics: LyricsConfig{
			TimeoutSeconds: 45,
			MaxConcurrent:  3,
		},
		API: APIConfig{KeyRateLimit: DefaultAPIKeyRateLimit},
		Security: SecurityConfig{
			AntiCrawl:         true,
			AnonymousReadRate: DefaultAntiCrawlReadRate,
			BulkReadRate:      DefaultAntiCrawlBulkRate,
			StreamRate:        DefaultAntiCrawlStreamRate,
		},
	}
}

// Load 读取 yaml 配置，yaml 缺省项用默认值补齐；再叠加环境变量覆盖。
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	var raw []byte
	if path != "" {
		var err error
		raw, err = os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(raw, cfg); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	migrateLegacyAdmin(cfg, raw)
	migrateLegacyAPI(cfg, raw)
	applyEnv(cfg)
	normalizeSecurity(cfg)
	return cfg, nil
}

// normalizeSecurity 把非法额度回填默认值。0 不解释成「不限流」——
// 想彻底放开请显式写 anti_crawl: false，免得一个误配把曲库变成裸奔。
func normalizeSecurity(cfg *Config) {
	if cfg.Security.AnonymousReadRate <= 0 {
		cfg.Security.AnonymousReadRate = DefaultAntiCrawlReadRate
	}
	if cfg.Security.BulkReadRate <= 0 {
		cfg.Security.BulkReadRate = DefaultAntiCrawlBulkRate
	}
	if cfg.Security.StreamRate <= 0 {
		cfg.Security.StreamRate = DefaultAntiCrawlStreamRate
	}
}

// migrateLegacyAdmin 兼容旧配置项 lyrics.admin_password。
//
// 早期「签发 API 密钥」用的就是这个口令；现在后台管理用同一个口令，
// 所以老配置直接沿用，不强迫用户改 config.yaml。
func migrateLegacyAdmin(cfg *Config, raw []byte) {
	if cfg.Admin.Password != "" || len(raw) == 0 {
		return
	}
	var legacy struct {
		Lyrics struct {
			AdminPassword string `yaml:"admin_password"`
		} `yaml:"lyrics"`
	}
	if err := yaml.Unmarshal(raw, &legacy); err != nil {
		return
	}
	cfg.Admin.Password = strings.TrimSpace(legacy.Lyrics.AdminPassword)
}

// migrateLegacyAPI 兼容旧的 lyrics.api_key_rate_limit / lyrics.allow_origins。
//
// 指针字段用来区分「没写」和「写了 0」：只有顶层 api 段没显式给值时，
// 旧值才生效，免得升级后新配置被老配置悄悄盖掉。
func migrateLegacyAPI(cfg *Config, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var probe struct {
		API struct {
			KeyRateLimit *int     `yaml:"key_rate_limit"`
			AllowOrigins []string `yaml:"allow_origins"`
		} `yaml:"api"`
		Lyrics struct {
			LegacyKeyRateLimit *int     `yaml:"api_key_rate_limit"`
			AllowOrigins       []string `yaml:"allow_origins"`
		} `yaml:"lyrics"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return
	}
	if probe.API.KeyRateLimit == nil && probe.Lyrics.LegacyKeyRateLimit != nil {
		cfg.API.KeyRateLimit = *probe.Lyrics.LegacyKeyRateLimit
	}
	if len(probe.API.AllowOrigins) == 0 && len(probe.Lyrics.AllowOrigins) > 0 {
		cfg.API.AllowOrigins = probe.Lyrics.AllowOrigins
	}
	if cfg.API.KeyRateLimit <= 0 {
		cfg.API.KeyRateLimit = DefaultAPIKeyRateLimit
	}
}

// LoadOrDefault 与 Load 相同，但忽略配置文件不存在的错误（用默认配置启动）。
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		// 配置解析失败不应阻断启动，但必须留下日志，否则运维会以为配置生效了。
		log.Printf("读取配置 %q 失败，已回退默认值: %v", path, err)
		cfg = DefaultConfig()
		applyEnv(cfg)
		normalizeSecurity(cfg)
	}
	return cfg
}

// applyEnv 叠加环境变量。
//
// ADMIN_PASSWORD 是新名字；旧部署（compose 里写的是 LYRICS_ADMIN_PASSWORD）
// 也认，避免升级后后台突然变成「未配置口令」把人锁在外面。
func applyEnv(cfg *Config) {
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	} else if v := os.Getenv("LYRICS_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	// 站点身份：不重启就能换站名/域名，前端从 /api/site 读取。
	for _, item := range []struct {
		env    string
		target *string
	}{
		{"SITE_NAME", &cfg.Site.Name},
		{"SITE_TAGLINE", &cfg.Site.Tagline},
		{"SITE_DESCRIPTION", &cfg.Site.Description},
		{"SITE_BASE_URL", &cfg.Site.BaseURL},
	} {
		if v := strings.TrimSpace(os.Getenv(item.env)); v != "" {
			*item.target = v
		}
	}
	if v := os.Getenv("API_KEY_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.API.KeyRateLimit = n
		}
	}
	if v := os.Getenv("API_ALLOW_ORIGINS"); v != "" {
		var origins []string
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				origins = append(origins, part)
			}
		}
		if len(origins) > 0 {
			cfg.API.AllowOrigins = origins
		}
	}

	// 反爬 / 防批量拖库。环境变量与 security 段一一对应，容器里不用挂配置文件。
	if v := strings.TrimSpace(os.Getenv("ANTI_CRAWL")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Security.AntiCrawl = b
		}
	}
	for _, item := range []struct {
		env    string
		target *int
	}{
		{"ANTI_CRAWL_READ_RATE", &cfg.Security.AnonymousReadRate},
		{"ANTI_CRAWL_BULK_RATE", &cfg.Security.BulkReadRate},
		{"ANTI_CRAWL_STREAM_RATE", &cfg.Security.StreamRate},
	} {
		if v := os.Getenv(item.env); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*item.target = n
			}
		}
	}
	if v := os.Getenv("ANTI_CRAWL_BLOCK_UA"); v != "" {
		cfg.Security.BlockUserAgents = splitList(v)
	}
	if v := os.Getenv("ANTI_CRAWL_ALLOW_UA"); v != "" {
		cfg.Security.AllowUserAgents = splitList(v)
	}
}

// splitList 把逗号分隔的环境变量拆成去空白的非空片段。
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
