package handler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/i18n"
	"github.com/hackliu/DNSLogHub/internal/logger"
	"github.com/hackliu/DNSLogHub/internal/middleware"
	"github.com/hackliu/DNSLogHub/internal/service"
)

type RegisterReq struct {
	Token      string `json:"token"`
	Subdomain  string `json:"subdomain"`
	Domain     string `json:"domain"`
	InviteCode string `json:"inviteCode"`
}

// ipLimiter 基于来源 IP 的简单滑动窗口限流器（按分钟）。
type ipLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *ipLimiter) Allow(ip string, limitPerMinute int) bool {
	if limitPerMinute <= 0 {
		return true
	}
	now := time.Now()
	window := now.Add(-time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(window) {
			kept = append(kept, t)
		}
	}
	l.hits[ip] = kept
	if len(kept) >= limitPerMinute {
		return false
	}
	l.hits[ip] = append(kept, now)
	return true
}

var registerLimiter = &ipLimiter{}

// Register 处理用户注册：邀请码校验 → 原子创建用户 → 消耗邀请码（失败回滚）。
func Register(c *gin.Context) {
	lang := i18n.Detect(c.GetHeader("Accept-Language"))
	ip := c.ClientIP()
	reqID := middleware.GetRequestID(c)

	if !config.Config.RegisterEnabled() {
		respond(c, http.StatusForbidden, i18n.T(lang, "register.disabled"), nil)
		return
	}

	if !registerLimiter.Allow(ip, config.Config.RegisterRateLimit()) {
		logger.Audit("user.register.rate_limited", zap.String("ip", ip), zap.String("request_id", reqID))
		respond(c, http.StatusTooManyRequests, i18n.T(lang, "register.rate_limited"), nil)
		return
	}

	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respond(c, http.StatusBadRequest, i18n.T(lang, "register.bad_request"), nil)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.Subdomain = strings.ToLower(strings.TrimSpace(req.Subdomain))
	req.InviteCode = strings.TrimSpace(req.InviteCode)
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" {
		req.Domain = config.Config.Dns.Domain
	}

	// 校验域名在配置的域名列表中
	domainValid := false
	for _, d := range config.Config.AllDomains() {
		if d == req.Domain {
			domainValid = true
			break
		}
	}
	if !domainValid {
		respond(c, http.StatusBadRequest, i18n.T(lang, "register.invalid_domain"), nil)
		return
	}

	if !service.ValidToken(req.Token) {
		respond(c, http.StatusBadRequest, i18n.T(lang, "register.invalid_token"), nil)
		return
	}
	if !service.ValidSubdomain(req.Subdomain) {
		respond(c, http.StatusBadRequest, i18n.T(lang, "register.invalid_subdomain"), nil)
		return
	}
	if _, exists := config.Config.GetUser(req.Token); exists {
		respond(c, http.StatusConflict, i18n.T(lang, "register.token_taken"), nil)
		return
	}
	if service.SubdomainExists(req.Subdomain) {
		respond(c, http.StatusConflict, i18n.T(lang, "register.subdomain_taken"), nil)
		return
	}

	inv, err := service.VerifyInvite(req.InviteCode, req.Subdomain)
	if err != nil {
		logger.Audit("user.register.invalid_invite", zap.String("ip", ip), zap.String("request_id", reqID), zap.String("subdomain", req.Subdomain))
		respond(c, http.StatusForbidden, i18n.T(lang, "register.invalid_invite"), nil)
		return
	}

	// 先创建用户（失败则不消耗邀请码，保证一致性）
	if err := service.AddUser(req.Token, req.Subdomain, req.Domain); err != nil {
		logger.Audit("user.register.failed", zap.String("ip", ip), zap.String("request_id", reqID), zap.String("error", err.Error()))
		respond(c, http.StatusInternalServerError, i18n.T(lang, "register.internal_error"), nil)
		return
	}

	// 用户创建成功后消耗邀请码；失败则回滚用户（补偿流程），杜绝半启用状态
	if err := service.ConsumeInvite(inv.Hash); err != nil {
		service.RemoveUser(req.Token)
		logger.Audit("user.register.compensated", zap.String("ip", ip), zap.String("request_id", reqID), zap.String("error", err.Error()))
		respond(c, http.StatusInternalServerError, i18n.T(lang, "register.internal_error"), nil)
		return
	}

	logger.Audit("user.register.success",
		zap.String("ip", ip),
		zap.String("request_id", reqID),
		zap.String("token", logger.MaskToken(req.Token)),
		zap.String("subdomain", req.Subdomain),
	)
	respondOK(c, gin.H{
		"subdomain": req.Subdomain + "." + config.Config.Dns.Domain,
		"token":     req.Token,
	})
}
