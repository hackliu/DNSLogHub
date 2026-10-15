package service

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"regexp"
	"time"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/store"

	"golang.org/x/crypto/bcrypt"
)

var (
	tokenRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	subdomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// GenerateInviteCode 生成高熵邀请码（base32 大写无填充，约 5bit/字符）。
func GenerateInviteCode(length int) (string, error) {
	if length <= 0 {
		length = 26
	}
	buf := make([]byte, (length*5+7)/8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return s[:length], nil
}

// HashInviteCode 计算邀请码的 bcrypt 哈希。
func HashInviteCode(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ValidToken 校验 token 格式。
func ValidToken(t string) bool { return tokenRe.MatchString(t) }

// ValidSubdomain 校验子域名格式（DNS 标签规则）。
func ValidSubdomain(s string) bool { return subdomainRe.MatchString(s) }

// AddInvite 邀请码入库。
func AddInvite(inv store.Invite) error {
	if DB == nil {
		return errors.New("store not initialized")
	}
	return DB.AddInvite(inv)
}

// VerifyInvite 校验邀请码（不扣减次数），返回命中的邀请码条目。
func VerifyInvite(plain, requestedSubdomain string) (*store.Invite, error) {
	invites, err := DB.ListInvites()
	if err != nil {
		return nil, err
	}
	for i := range invites {
		inv := &invites[i]
		if bcrypt.CompareHashAndPassword([]byte(inv.Hash), []byte(plain)) != nil {
			continue
		}
		if inv.RemainingUses == 0 {
			return nil, errors.New("invite code exhausted")
		}
		if inv.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, inv.ExpiresAt)
			if err != nil || time.Now().After(t) {
				return nil, errors.New("invite code expired")
			}
		}
		if inv.BoundSubdomain != "" && inv.BoundSubdomain != requestedSubdomain {
			return nil, errors.New("invite code not bound to this subdomain")
		}
		return inv, nil
	}
	return nil, errors.New("invalid invite code")
}

// ConsumeInvite 扣减一次使用次数。
func ConsumeInvite(hash string) error { return DB.ConsumeInvite(hash) }

// SubdomainExists 判断子域名是否已被占用。
func SubdomainExists(subdomain string) bool {
	for _, v := range config.Config.UserSnapshot() {
		if v == subdomain {
			return true
		}
	}
	return false
}

// AddUser 原子新增用户：配置 + 内存映射 + SQLite；任一步失败即回滚。
func AddUser(token, subdomain, domain string) error {
	if _, exists := config.Config.GetUser(token); exists {
		return errors.New("token already exists")
	}
	if SubdomainExists(subdomain) {
		return errors.New("subdomain already exists")
	}
	config.Config.SetUser(token, subdomain)
	config.Config.SetUserDomain(token, domain)
	model.UserDnsDataMap.AddUser(token, subdomain)
	if err := DB.SaveUser(store.User{Token: token, Subdomain: subdomain, Domain: domain, Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		config.Config.DeleteUser(token)
		config.Config.SetUserDomain(token, "")
		model.UserDnsDataMap.RemoveUser(token)
		return err
	}
	return nil
}

// RemoveUser 移除用户（补偿/回滚用）。
func RemoveUser(token string) {
	config.Config.DeleteUser(token)
	model.UserDnsDataMap.RemoveUser(token)
	if DB != nil {
		_ = DB.DeleteUser(token)
	}
}
