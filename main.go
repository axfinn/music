// music-lyrics-archive 后端入口。
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

	"music-lyrics-archive/backend/config"
	"music-lyrics-archive/backend/handlers"
	"music-lyrics-archive/backend/middleware"
	"music-lyrics-archive/backend/models"
	"music-lyrics-archive/backend/routes"

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
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "music-lyrics-archive"})
		})

		// 音乐库路由(上传/封面走独立限流:200 次/分钟)。
		musicHandler := handlers.NewMusicHandler(db, cfg)
		routes.RegisterMusicRoutes(api, musicHandler, middleware.NewRateLimiter(200, time.Minute))

		// 歌词库路由(创建/摘句等写操作单独限流:60 次/分钟)。
		lyricsHandler := handlers.NewLyricsHandler(db, cfg)
		routes.RegisterLyricsRoutes(api, lyricsHandler, middleware.NewRateLimiter(60, time.Minute))
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
		// 尚未构建前端时，回退到仓库内占位 index.html（便于本地先跑通后端）。
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.File("./frontend/index.html")
		})
	}

	addr := cfg.Server.Host + ":" + itoa(cfg.Server.Port)
	log.Printf("music-lyrics-archive 监听 %s (data=%s)", addr, cfg.DataDir)
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
