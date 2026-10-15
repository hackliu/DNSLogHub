package handler

import (
	"net/http"
	"strings"

	"github.com/hackliu/DNSLogHub/internal/logger"
	"github.com/hackliu/DNSLogHub/internal/middleware"
	"github.com/hackliu/DNSLogHub/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserResetSubdomain 用户重置自己的专属子域名。
func UserResetSubdomain(c *gin.Context) {
	var req struct {
		Subdomain string `json:"subdomain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "invalid request", nil)
		return
	}
	req.Subdomain = strings.ToLower(strings.TrimSpace(req.Subdomain))
	oldToken := c.GetString("token")
	if err := service.ResetUserSubdomain(oldToken, req.Subdomain); err != nil {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	logger.Audit("user.reset_subdomain.self",
		zap.String("token", logger.MaskToken(oldToken)),
		zap.String("subdomain", req.Subdomain),
		zap.String("request_id", middleware.GetRequestID(c)),
	)
	respondOK(c, gin.H{"subdomain": req.Subdomain})
}

// UserResetToken 用户重置自己的认证 Token。
func UserResetToken(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "invalid request", nil)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	oldToken := c.GetString("token")
	if err := service.ResetUserToken(oldToken, req.Token); err != nil {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	logger.Audit("user.reset_token.self",
		zap.String("old_token", logger.MaskToken(oldToken)),
		zap.String("new_token", logger.MaskToken(req.Token)),
		zap.String("request_id", middleware.GetRequestID(c)),
	)
	respondOK(c, gin.H{"token": req.Token})
}
