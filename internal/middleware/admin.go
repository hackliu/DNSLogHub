package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/hackliu/DNSLogHub/internal/config"

	"github.com/gin-gonic/gin"
)

// AdminAuth 管理员鉴权：校验 X-Admin-Token 请求头，常量时间比较防时序侧信道。
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := c.GetHeader("X-Admin-Token")
		cfgToken := config.Config.Admin.Token
		if cfgToken == "" || t == "" || subtle.ConstantTimeCompare([]byte(t), []byte(cfgToken)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "Forbidden",
				"traceId": GetRequestID(c),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
