package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/logger"
	"github.com/hackliu/DNSLogHub/internal/middleware"
	"github.com/hackliu/DNSLogHub/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Ping 校验管理员令牌（供登录页使用）。
func Ping(c *gin.Context) {
	respondOK(c, nil)
}

// Reload 热加载配置（仅可热加载项），失败保持原配置不变并记录审计。
func Reload(c *gin.Context) {
	if err := config.Config.Reload(); err != nil {
		logger.Audit("config.reload.failed",
			zap.String("request_id", middleware.GetRequestID(c)),
			zap.String("error", err.Error()),
		)
		respond(c, http.StatusInternalServerError, "reload failed", nil)
		return
	}
	logger.Audit("config.reload.success",
		zap.String("request_id", middleware.GetRequestID(c)),
		zap.Bool("register_enabled", config.Config.RegisterEnabled()),
		zap.Int("rate_limit_per_minute", config.Config.RegisterRateLimit()),
	)
	respondOK(c, gin.H{
		"enabled":            config.Config.RegisterEnabled(),
		"rateLimitPerMinute": config.Config.RegisterRateLimit(),
	})
}

type inviteView struct {
	ID             int64  `json:"id"`
	HashPrefix     string `json:"hashPrefix"`
	RemainingUses  int    `json:"remainingUses"`
	ExpiresAt      string `json:"expiresAt"`
	BoundSubdomain string `json:"boundSubdomain"`
	Note           string `json:"note"`
	CreatedAt      int64  `json:"createdAt"`
}

// ListInvites 列出邀请码（支持搜索 q、分页 page/pageSize）。
func ListInvites(c *gin.Context) {
	invites, err := service.ListInvitesAdmin()
	if err != nil {
		respond(c, http.StatusInternalServerError, "list invites failed", nil)
		return
	}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	filtered := invites
	if q != "" {
		filtered = nil
		for _, inv := range invites {
			if strings.Contains(strings.ToLower(inv.Note), q) || strings.Contains(strings.ToLower(inv.BoundSubdomain), q) {
				filtered = append(filtered, inv)
			}
		}
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	items := make([]inviteView, 0, end-start)
	for _, inv := range filtered[start:end] {
		p := inv.Hash
		if len(p) > 8 {
			p = p[:8]
		}
		items = append(items, inviteView{
			ID:             inv.ID,
			HashPrefix:     p,
			RemainingUses:  inv.RemainingUses,
			ExpiresAt:      inv.ExpiresAt,
			BoundSubdomain: inv.BoundSubdomain,
			Note:           inv.Note,
			CreatedAt:      inv.CreatedAt,
		})
	}
	respondOK(c, gin.H{"total": total, "items": items})
}

// RevokeInvite 作废邀请码。
func RevokeInvite(c *gin.Context) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		respond(c, http.StatusBadRequest, "invalid id", nil)
		return
	}
	if err := service.RevokeInvite(req.ID); err != nil {
		respond(c, http.StatusInternalServerError, "revoke failed", nil)
		return
	}
	logger.Audit("invite.revoke", zap.Int64("id", req.ID), zap.String("request_id", middleware.GetRequestID(c)))
	respondOK(c, nil)
}

// ListUsers 列出注册用户（token 脱敏）。
func ListUsers(c *gin.Context) {
	users, err := service.ListUsersAdmin()
	if err != nil {
		respond(c, http.StatusInternalServerError, "list users failed", nil)
		return
	}
	type userView struct {
		Token     string `json:"token"`
		Subdomain string `json:"subdomain"`
		Domain    string `json:"domain"`
		Enabled   bool   `json:"enabled"`
		CreatedAt int64  `json:"createdAt"`
	}
	items := make([]userView, 0, len(users))
	for _, u := range users {
		d := u.Domain
		if d == "" {
			d = config.Config.Dns.Domain
		}
		items = append(items, userView{
			Token:     logger.MaskToken(u.Token),
			Subdomain: u.Subdomain,
			Domain:    d,
			Enabled:   u.Enabled,
			CreatedAt: u.CreatedAt,
		})
	}
	respondOK(c, gin.H{"items": items})
}

// DisableUser 禁用用户（会话立即失效）。
func DisableUser(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		respond(c, http.StatusBadRequest, "invalid token", nil)
		return
	}
	if err := service.SetUserEnabled(req.Token, false); err != nil {
		respond(c, http.StatusNotFound, "user not found", nil)
		return
	}
	logger.Audit("user.disable", zap.String("token", logger.MaskToken(req.Token)), zap.String("request_id", middleware.GetRequestID(c)))
	respondOK(c, nil)
}

// EnableUser 启用用户。
func EnableUser(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		respond(c, http.StatusBadRequest, "invalid token", nil)
		return
	}
	if err := service.SetUserEnabled(req.Token, true); err != nil {
		respond(c, http.StatusNotFound, "user not found", nil)
		return
	}
	logger.Audit("user.enable", zap.String("token", logger.MaskToken(req.Token)), zap.String("request_id", middleware.GetRequestID(c)))
	respondOK(c, nil)
}

// AdminResetSubdomain 管理员重置用户子域名。
func AdminResetSubdomain(c *gin.Context) {
	var req struct {
		Token     string `json:"token"`
		Subdomain string `json:"subdomain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || req.Subdomain == "" {
		respond(c, http.StatusBadRequest, "invalid request", nil)
		return
	}
	req.Subdomain = strings.ToLower(strings.TrimSpace(req.Subdomain))
	if err := service.ResetUserSubdomain(req.Token, req.Subdomain); err != nil {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	logger.Audit("user.reset_subdomain", zap.String("token", logger.MaskToken(req.Token)), zap.String("subdomain", req.Subdomain), zap.String("request_id", middleware.GetRequestID(c)))
	respondOK(c, nil)
}
