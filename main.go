// music 后端入口。
//
// 单二进制同时承载 API（音乐库 / 歌词库 / 歌词发现 / v1 开放接口）与静态前端。
// 开发期前端由 Vite 单独跑（默认 5173）并代理 /api 到后端；
// 生产期后端直接用 ./frontend/dist 提供 SPA，未命中 /api 的路由回落 index.html。
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/axfinn/music/backend/config"
	"github.com/axfinn/music/backend/handlers"
	"github.com/axfinn/music/backend/middleware"
	"github.com/axfinn/music/backend/models"
	"github.com/axfinn/music/backend/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.LoadOrDefault("config.yaml")

	// 数据目录与 SQLite 路径。
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "music.db")
	db, err := models.NewDB(dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	r := gin.Default()

	// ── 跨域：音乐库 / 歌词库只读接口 ──
	// devtools 的魔法宠物运行在另一个源(devtools 站点)，需要跨域读取本站的
	// 音乐与歌词；这些都是公开只读数据、无凭证，故用 * 放行并处理预检 OPTIONS。
	r.Use(corsForPublicRead())

	// ── 全站反爬 ──
	// NoIndex 对所有响应生效（含 404、静态文件、音频流）；robots.txt 走显式路由，
	// 免得被 SPA 回落成一张 HTML。/api 下的匿名数据接口再挂一层 Guard。
	r.Use(middleware.NoIndex())
	r.GET("/robots.txt", middleware.RobotsTxt())
	antiCrawl := middleware.NewAntiCrawl(middleware.AntiCrawlOptions{
		Enabled:           cfg.Security.AntiCrawl,
		AnonymousReadRate: cfg.Security.AnonymousReadRate,
		BulkReadRate:      cfg.Security.BulkReadRate,
		StreamRate:        cfg.Security.StreamRate,
		BlockUserAgents:   cfg.Security.BlockUserAgents,
		AllowUserAgents:   cfg.Security.AllowUserAgents,
	})

	// ── API ──
	api := r.Group("/api")
	// 必须在注册任何子路由之前 Use：gin 的 Group 会在创建时复制父组 handlers。
	api.Use(antiCrawl.Guard())
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "music"})
		})
		// 站点身份：前端用它渲染品牌/标题，外部系统用它发现接口地址。
		routes.RegisterSiteRoutes(api, cfg)

		// 管理员身份:口令换算成会话 Cookie。
		// 留空 = 纯访客模式(只有音乐库可听,后台接口全关)。
		admin := middleware.NewAdmin(cfg.Admin.Password)

		// 后台登录/登出/会话查询。
		routes.RegisterAdminRoutes(api, handlers.NewAdminHandler(admin))

		// 对外 v1 公共设施：音乐与歌词两套 v1 共用同一把密钥、CORS 与限流。
		// 密钥表仍叫 lyric_api_keys（历史表名，不动生产库），但语义已是全站密钥。
		apiV1 := middleware.NewV1API(middleware.V1APIOptions{
			AllowOrigins: cfg.API.AllowOrigins,
			KeyRateLimit: cfg.API.KeyRateLimit,
			Verify: func(raw string) (middleware.APIKeyInfo, error) {
				key, err := db.VerifyAPIKey(raw)
				if err != nil {
					return middleware.APIKeyInfo{}, err
				}
				return middleware.APIKeyInfo{
					ID: key.ID, Name: key.Name, Prefix: key.Prefix, CallCount: key.CallCount,
				}, nil
			},
		})

		// 音乐库路由(上传/封面走独立限流:200 次/分钟)。
		musicHandler := handlers.NewMusicHandler(db, cfg)
		routes.RegisterMusicRoutes(api, musicHandler, middleware.NewRateLimiter(200, time.Minute), admin)

		// 歌词库路由(创建/摘句等写操作单独限流:60 次/分钟)。
		lyricsHandler := handlers.NewLyricsHandler(db, cfg)
		routes.RegisterLyricsRoutes(api, lyricsHandler, middleware.NewRateLimiter(60, time.Minute), admin)
		routes.RegisterLyricsAPIRoutes(api, lyricsHandler, apiV1)

		// 音乐库对外对称接口(/api/v1/music)。站内 /api/music 不变。
		routes.RegisterMusicAPIRoutes(api, musicHandler, apiV1)
	}

	// ── 静态前端 + SPA 回落 ──
	dist := "./frontend/dist"
	if info, err := os.Stat(dist); err == nil && info.IsDir() {
		r.Static("/assets", filepath.Join(dist, "assets"))
		r.NoRoute(spaFallback(dist))
	} else {
		// 前端还没构建。注意不能直接把 frontend/index.html 返回给浏览器 ——
		// 那是 Vite 的开发入口，里面引用 /src/main.js，未经打包浏览器跑不起来，
		// 打开只会是一张白页。这里给一张说明页，把两条正确姿势写清楚。
		log.Printf("未找到 %s，将只提供 API（前端开发请用 Vite，生产请先 npm run build）", dist)
		r.NoRoute(devPlaceholder())
	}

	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	log.Printf("music 监听 %s (data=%s)", addr, cfg.DataDir)
	// 用 http.Server 而不是 r.Run()：把 gzip 包在 gin.Engine 外层，对 /api 的
	// JSON 响应做透明压缩。曲库首屏 /snapshot 是 1.1MB 明文，压缩后约 170KB；
	// 外层 nginx / CDN 默认只压 text/html，不压 application/json，只能在应用内做。
	srv := &http.Server{
		Addr:              addr,
		Handler:           middleware.GzipHandler(r),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// corsForPublicRead 为音乐库 / 歌词库的只读接口放开跨域。
// 只作用于 /api/music 与 /api/lyrics 前缀；其余路由直接放行，不受影响。
func corsForPublicRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api/music") && !strings.HasPrefix(path, "/api/lyrics") {
			c.Next()
			return
		}
		// 标记为本站「魔法宠物」跨站只读通道：放行反爬的 cross-site / 浏览器信号校验，
		// 但限流(读/流媒体额度)依旧生效，避免被当成拖库口子。
		c.Set("companionRead", true)
		if origin := c.GetHeader("Origin"); origin != "" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// spaFallback 返回 SPA 回落处理器：非 /api 路由一律返回 index.html。
func spaFallback(dist string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// SPA 入口与根目录静态文件（favicon.svg 等）都没有内容哈希。
		// 不给 Cache-Control 时，外层 CDN 会按默认规则缓存住旧 index.html，
		// 发完新版用户仍看到旧前端；旧版把 /favicon.svg 回落成的 HTML 也会被一直缓存。
		// no-cache = 可以缓存但每次必须回源验证，配合 Last-Modified 命中 304，
		// 既不会让发布卡在旧缓存，也不会每次都传整个文件。
		c.Header("Cache-Control", "no-cache")
		target := filepath.Join(dist, c.Request.URL.Path)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			c.File(target)
			return
		}
		c.File(filepath.Join(dist, "index.html"))
	}
}

// devPlaceholder 前端未构建时的说明页。
//
// 只对非 /api 路由生效；API 仍按正常 404 JSON 返回，
// 免得脚本探测接口时拿到一坨 HTML。
func devPlaceholder() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(devPlaceholderHTML))
	}
}

const devPlaceholderHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>music · 前端尚未构建</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; min-height: 100dvh; display: grid; place-items: center;
    background: #0b0d12; color: #e8eaf0;
    font-family: system-ui, -apple-system, "PingFang SC", sans-serif; }
  .card { width: min(520px, calc(100vw - 40px)); padding: 30px 28px;
    border-radius: 18px; background: #151921;
    border: 1px solid rgba(255,255,255,.08); }
  h1 { margin: 0 0 6px; font-size: 20px; }
  p { margin: 0 0 16px; font-size: 13px; line-height: 1.7; color: #8a93a5; }
  h2 { margin: 18px 0 8px; font-size: 13px; color: #c9d1e0; }
  code, pre { font-family: "SF Mono", Menlo, monospace; }
  pre { margin: 0; padding: 12px 14px; border-radius: 10px;
    background: #0e1117; border: 1px solid rgba(255,255,255,.07);
    color: #ffb98f; font-size: 12px; line-height: 1.7; overflow-x: auto; }
  a { color: #ff8b5c; }
</style>
</head>
<body>
  <div class="card">
    <h1>后端已启动，前端还没打包</h1>
    <p>当前目录下没有 <code>frontend/dist</code>，所以这里暂时只有接口可用。
       想看到界面，二选一：</p>

    <h2>开发模式（改代码即时生效）</h2>
    <pre>cd frontend &amp;&amp; npm install &amp;&amp; npm run dev
# 打开 http://localhost:5173</pre>

    <h2>生产模式（后端直接托管）</h2>
    <pre>cd frontend &amp;&amp; npm install &amp;&amp; npm run build
# 回到项目根目录重启后端，再刷新本页</pre>

    <p style="margin-top:16px">接口自检：<a href="/api/health">/api/health</a></p>
  </div>
</body>
</html>`
