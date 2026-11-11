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
)

// Config 全局配置。
type Config struct {
	Server  ServerConfig `yaml:"server"`
	DataDir string       `yaml:"data_dir"` // 音频/封面/SQLite 根目录，默认 ./data
	Admin   AdminConfig  `yaml:"admin"`
	Site    SiteConfig   `yaml:"site"`
	Music   MusicConfig  `yaml:"music"`
	Lyrics  LyricsConfig `yaml:"lyrics"`
	API     APIConfig    `yaml:"api"`
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
	return cfg, nil
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
}
