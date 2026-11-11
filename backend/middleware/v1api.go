// Package middleware 里的 v1api.go 是 /api/v1/* 的公共设施。
//
// 音乐与歌词两套对外接口共用同一把密钥、同一套 CORS 白名单、同一份响应信封。
// 以前这些逻辑在两个 handler 里各写一遍（音乐侧甚至跨模块调用歌词文件的私有函数），
// 现在收敛到这里，handler 只负责业务数据。
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// APIKeyInfo 是鉴权成功后交给 handler 的最小密钥视图。
// 不暴露 hash / 明文，也不依赖 models 包，避免中间件层反向依赖数据层。
type APIKeyInfo struct {
	ID        string
	Name      string
	Prefix    string
	CallCount int
}

// APIKeyVerifier 校验一把明文密钥。由 main 用 models.DB 的方法适配后注入。
type APIKeyVerifier func(raw string) (APIKeyInfo, error)

// APIKeyContextKey 是鉴权结果在 gin.Context 里的键。
const APIKeyContextKey = "v1_api_key"

// V1APIOptions 构造对外接口公共设施所需的全部依赖。
type V1APIOptions struct {
	AllowOrigins []string
	KeyRateLimit int
	Verify       APIKeyVerifier
}

// V1API 持有 CORS 白名单与按密钥计数的限流器（两个 v1 路由组共享同一实例）。
type V1API struct {
	origins []string
	limit   int
	verify  APIKeyVerifier
	limiter *RateLimiter
}

// NewV1API 创建对外接口公共设施。
func NewV1API(opts V1APIOptions) *V1API {
	limit := opts.KeyRateLimit
	if limit <= 0 {
		limit = 60
	}
	return &V1API{
		origins: append([]string(nil), opts.AllowOrigins...),
		limit:   limit,
		verify:  opts.Verify,
		limiter: NewRateLimiter(limit, time.Minute),
	}
}

// KeyRateLimit 返回单密钥每分钟额度，供 /ping 应答展示。
func (v *V1API) KeyRateLimit() int { return v.limit }

// CORS 对外接口的跨域放行：凭据是密钥而不是 Cookie，默认放开来源。
// 配了 allow_origins 就只放行白名单里的 Origin；预检请求在这里就地结束。
func (v *V1API) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := "*"
		if len(v.origins) > 0 {
			allowed = ""
			for _, o := range v.origins {
				o = strings.TrimSpace(o)
				if o == "*" || strings.EqualFold(o, origin) {
					allowed = origin
					break
				}
			}
		}
		if allowed != "" {
			c.Header("Access-Control-Allow-Origin", allowed)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequireKey 给需要密钥的端点用（缺密钥直接 401）。
func (v *V1API) RequireKey() gin.HandlerFunc { return v.keyGate(true) }

// OptionalKey 给 /ping 这类自检端点用：不带密钥照样探活，
// 带了密钥就校验并把用量写进 context（无效密钥仍然 401，不静默忽略）。
func (v *V1API) OptionalKey() gin.HandlerFunc { return v.keyGate(false) }

func (v *V1API) keyGate(required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		raw := ExtractAPIKey(c)
		if raw == "" {
			if required {
				V1Fail(c, http.StatusUnauthorized, "缺少密钥：请用 X-API-Key 或 Authorization: Bearer <key>")
				return
			}
			c.Next()
			return
		}
		if v.verify == nil {
			V1Fail(c, http.StatusInternalServerError, "对外接口未配置密钥校验")
			return
		}
		key, err := v.verify(raw)
		if err != nil {
			V1Fail(c, http.StatusUnauthorized, "密钥无效或已撤销")
			return
		}
		if !v.limiter.Allow(key.ID) {
			V1Fail(c, http.StatusTooManyRequests, "该密钥请求过于频繁，请稍后再试")
			return
		}
		c.Set(APIKeyContextKey, key)
		c.Next()
	}
}

// ExtractAPIKey 从请求头或查询串里取密钥（也供测试与文档说明使用）。
func ExtractAPIKey(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(c.Query("api_key"))
}

// APIKeyFromContext 取鉴权后的密钥信息。
func APIKeyFromContext(c *gin.Context) (APIKeyInfo, bool) {
	val, exists := c.Get(APIKeyContextKey)
	if !exists {
		return APIKeyInfo{}, false
	}
	info, ok := val.(APIKeyInfo)
	return info, ok
}

// V1OK 成功信封：{"ok":true,"data":...,"meta":...}。
func V1OK(c *gin.Context, data any, meta gin.H) {
	body := gin.H{"ok": true, "data": data}
	if meta != nil {
		body["meta"] = meta
	}
	c.JSON(http.StatusOK, body)
}

// V1Fail 失败信封：{"ok":false,"error":...,"code":4xx}，error 一定有值。
func V1Fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"ok": false, "error": msg, "code": code})
}
