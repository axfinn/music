// Package middleware 提供纯内存的极简 HTTP 限流。
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 基于滑动窗口的计数限流，状态保存在进程内存中。
// key 可以是 IP 也可以是 API Key ID；对个人/小团队部署足够，
// 多实例部署时各实例各自计数（额度不共享），需要共享可换成集中式存储。
type RateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	hits      map[string][]time.Time
	lastSweep time.Time
}

// NewRateLimiter 创建限流器：limit 为窗口内最大请求数。
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		limit:     limit,
		window:    window,
		hits:      make(map[string][]time.Time),
		lastSweep: time.Now(),
	}
}

// Allow 判断某 key 在窗口内是否仍可放行。
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)
	// 每个窗口至少整体清一次：只裁切当前 key 会让长期不再出现的 IP / 密钥
	// 永远留在 map 里，站点跑久了就是一条缓慢的内存泄漏。
	if now.Sub(rl.lastSweep) >= rl.window {
		rl.sweep(cutoff)
		rl.lastSweep = now
	}
	times := rl.hits[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	rl.hits[key] = kept
	if len(kept) >= rl.limit {
		return false
	}
	rl.hits[key] = append(rl.hits[key], now)
	return true
}

// sweep 删除所有窗口内已无记录的 key（调用方持有锁）。
func (rl *RateLimiter) sweep(cutoff time.Time) {
	for key, times := range rl.hits {
		kept := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(rl.hits, key)
			continue
		}
		rl.hits[key] = kept
	}
}

// Middleware 返回 gin 中间件：超限返回 429。
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后再试",
				"code":  429,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
