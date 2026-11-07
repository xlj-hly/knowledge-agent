package httpapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go-server/internal/domain/saas"
)

// 平台接口必须同时经过平台域名入口和独立平台身份验证。
func platformEntry(s interface{ IsPlatformHost(string) bool }) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/platform/") && !s.IsPlatformHost(c.Request.Host) {
			reply(c, nil, domain.Forbidden())
			c.Abort()
			return
		}
		c.Next()
	}
}

func security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
}
func recoverRequest() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		logFailure(c.FullPath(), nil)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "服务暂时不可用，请稍后再试"}})
	})
}
func cors(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, origin := range origins {
		if origin != "" {
			allowed[origin] = true
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if allowed[origin] {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}
		c.Next()
	}
}
