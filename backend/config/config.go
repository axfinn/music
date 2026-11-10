// Package config 提供 github.com/axfinn/music 的极简配置加载。
//
// 只保留音乐库 / 歌词库两大模块需要的字段，不照搬 devtools 那套大而全的配置。
// 配置来源优先级：环境变量 > config.yaml > 默认值。
package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 全局配置。
type Config struct {
	Server  ServerConfig `yaml:"server"`
	DataDir string       `yaml:"data_dir"` // 音频/封面/SQLite 根目录，默认 ./data
	Admin   AdminConfig  `yaml:"admin"`
	Music   MusicConfig  `yaml:"music"`
	Lyrics  LyricsConfig `yaml:"lyrics"`
}

// AdminConfig 后台管理口令。
//
// 站点分两种身份：
//   - 访客：只能浏览与播放音乐库（音乐库是公开内容）；
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

// MusicConfig 音乐库配置。
type MusicConfig struct {
	AudioMaxBytes int64 `yaml:"audio_max_bytes"` // 单曲上限，默认 200MB
	CoverMaxBytes int64 `yaml:"cover_max_bytes"` // 封面上限，默认 15MB
}

// LyricsConfig 歌词库配置：站内在线抓取（lyrics_finder.py）+ 对外 v1 接口鉴权。
//
// 两条鉴权线互不相干：
//   - 管理口令（见 AdminConfig）管「后台管理」这类站内动作（留空则该动作禁用）；
//   - 对外调用本身走密钥表 lyric_api_keys，密钥在页面上签发/撤销，不落配置文件。
type LyricsConfig struct {
	PythonBin       string   `yaml:"python_bin"`         // 解释器路径；留空自动探测
	ScriptPath      string   `yaml:"script_path"`        // lyrics_finder.py 路径；留空自动探测
	TimeoutSeconds  int      `yaml:"timeout_seconds"`    // 单次抓取总超时，默认 45
	MaxConcurrent   int      `yaml:"max_concurrent"`     // 同时抓取进程上限，默认 3
	APIKeyRateLimit int      `yaml:"api_key_rate_limit"` // 单密钥每分钟上限，默认 60
	AllowOrigins    []string `yaml:"allow_origins"`      // 对外接口 CORS 白名单；空=允许任意来源

	// ── 歌词发现 ──
	DiscoverySeeds          []string `yaml:"discovery_seeds"`           // 种子歌单（"歌名 - 艺人"）；留空用内置默认
	DiscoveryPoolSize       int      `yaml:"discovery_pool_size"`       // 在线候选池上限，默认 24
	DiscoveryBatchSize      int      `yaml:"discovery_batch_size"`      // 每次刷新抓几首，默认 4
	DiscoveryRefreshMinutes int      `yaml:"discovery_refresh_minutes"` // 池刷新间隔（分钟），默认 30
}

// DefaultConfig 返回带合理默认值的配置。
func DefaultConfig() *Config {
	return &Config{
		Server:  ServerConfig{Host: "0.0.0.0", Port: 8080},
		DataDir: "./data",
		Music:   MusicConfig{AudioMaxBytes: 200 << 20, CoverMaxBytes: 15 << 20},
		Lyrics: LyricsConfig{
			TimeoutSeconds:  45,
			MaxConcurrent:   3,
			APIKeyRateLimit: 60,
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

// LoadOrDefault 与 Load 相同，但忽略配置文件不存在的错误（用默认配置启动）。
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		// 配置解析失败不应阻断启动；回退默认值并继续。
		cfg = DefaultConfig()
		applyEnv(cfg)
	}
	return cfg
}

// applyEnv 叠加环境变量（PORT / DATA_DIR / ADMIN_PASSWORD）。
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
	}
}
