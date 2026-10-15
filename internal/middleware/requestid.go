package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const (
	RequestIDHeader = "X-Request-ID"
	ctxRequestID    = "request_id"
)

// RequestID 为每个 HTTP 请求生成/继承请求标识，便于审计日志全链路串联。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(ctxRequestID, rid)
		c.Header(RequestIDHeader, rid)
		c.Next()
	}
}

// GetRequestID 从上下文读取请求标识。
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(ctxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
