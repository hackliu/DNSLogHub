package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hackliu/DNSLogHub/internal/config"
	"net/http"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("token")
		_, ok := config.Config.GetUser(token)
		if token == "" || !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"msg":     "Invalid token",
				"message": "Invalid token",
				"traceId": GetRequestID(c),
			})
			c.Abort()
			return
		}
		c.Set("token", token)
		c.Next()
	}
}
