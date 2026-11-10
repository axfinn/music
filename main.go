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

	// ── API ──
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "music"})
		})

		// 管理员身份:口令换算成会话 Cookie。
		// 留空 = 纯访客模式(只有音乐库可听,后台接口全关)。
		admin := middleware.NewAdmin(cfg.Admin.Password)

		// 后台登录/登出/会话查询。
		routes.RegisterAdminRoutes(api, handlers.NewAdminHandler(admin))

		// 音乐库路由(上传/封面走独立限流:200 次/分钟)。
		musicHandler := handlers.NewMusicHandler(db, cfg)
		routes.RegisterMusicRoutes(api, musicHandler, middleware.NewRateLimiter(200, time.Minute), admin)

		// 歌词库路由(创建/摘句等写操作单独限流:60 次/分钟)。
		lyricsHandler := handlers.NewLyricsHandler(db, cfg)
		routes.RegisterLyricsRoutes(api, lyricsHandler, middleware.NewRateLimiter(60, time.Minute), admin)
		routes.RegisterLyricsAPIRoutes(api, lyricsHandler)

		// 音乐库对外对称接口(/api/v1/music)。站内 /api/music 不变。
		routes.RegisterMusicAPIRoutes(api, musicHandler)
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

	addr := cfg.Server.Host + ":" + itoa(cfg.Server.Port)
	log.Printf("music 监听 %s (data=%s)", addr, cfg.DataDir)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// spaFallback 返回 SPA 回落处理器：非 /api 路由一律返回 index.html。
func spaFallback(dist string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
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
<title>音乐库 · 前端尚未构建</title>
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
