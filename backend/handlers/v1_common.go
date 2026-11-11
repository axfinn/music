package handlers

import (
	"github.com/axfinn/music/backend/middleware"

	"github.com/gin-gonic/gin"
)

// 对外 /api/v1/* 的响应信封由 middleware 统一实现（鉴权失败也要用同一封信封）。
// 这里保留包内短名，让各 handler 的调用点不必逐个改成 middleware.V1OK。

func v1OK(c *gin.Context, data any, meta gin.H) {
	middleware.V1OK(c, data, meta)
}

func v1Fail(c *gin.Context, code int, msg string) {
	middleware.V1Fail(c, code, msg)
}
