package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"graycrow/internal/httpx"
)

const (
	contextRequestID = "gc_request_id"
	csrfHeader       = "X-Requested-With"
	csrfValue        = "graycrow"
)

var sensitiveKeys = []string{"token", "password", "secret", "key", "authorization", "code"}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			buf := make([]byte, 8)
			if _, err := rand.Read(buf); err == nil {
				id = hex.EncodeToString(buf)
			}
		}
		c.Set(contextRequestID, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		log.Printf("[panic] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "服务器内部错误")
	})
}

// AccessLog 只记录方法与路径，查询串中的敏感参数做脱敏，绝不记录请求体。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s%s %d %s", c.Request.Method, c.Request.URL.Path, redactQuery(c.Request.URL.RawQuery), c.Writer.Status(), time.Since(start).Round(time.Millisecond))
	}
}

func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	for i, p := range parts {
		k, _, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		for _, s := range sensitiveKeys {
			if strings.Contains(strings.ToLower(k), s) {
				parts[i] = k + "=***"
				break
			}
		}
	}
	return "?" + strings.Join(parts, "&")
}

// WriteGuard 对写操作校验自定义头，配合 SameSite=Lax Cookie 形成同源 CSRF 防线。
func WriteGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if c.GetHeader(csrfHeader) != csrfValue {
				httpx.Fail(c, http.StatusForbidden, httpx.CodeCSRF, "缺少 "+csrfHeader+" 请求头")
				return
			}
		}
		c.Next()
	}
}

func RequestIDOf(c *gin.Context) string {
	if v, ok := c.Get(contextRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
