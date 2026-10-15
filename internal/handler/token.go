package handler

import (
	"net/http"

	"github.com/hackliu/DNSLogHub/internal/config"

	"github.com/gin-gonic/gin"
)

type VerifyTokenReq struct {
	Token string `json:"token"`
}

// VerifyToken 验证 Token
func VerifyToken(c *gin.Context) {
	var req VerifyTokenReq
	if err := c.BindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	if req.Token == "" {
		respond(c, http.StatusBadRequest, "Token cannot be empty", nil)
		return
	}
	subdomain, ok := config.Config.GetUser(req.Token)
	if !ok {
		respond(c, http.StatusUnauthorized, "Invalid token", nil)
		return
	}
	respondOK(c, gin.H{
		"subdomain": subdomain + "." + config.Config.GetUserDomain(req.Token),
		"token":     req.Token,
	})
}

// Domains 返回配置的全部根域名（公开，供注册页选择）。
func Domains(c *gin.Context) {
	respondOK(c, gin.H{"domains": config.Config.AllDomains()})
}
