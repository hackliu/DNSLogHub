package service

import (
	"errors"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/store"
)

// ListUsersAdmin 列出全部注册用户。
func ListUsersAdmin() ([]store.User, error) { return DB.ListUsers() }

// ListInvitesAdmin 列出全部邀请码。
func ListInvitesAdmin() ([]store.Invite, error) { return DB.ListInvites() }

// RevokeInvite 作废邀请码。
func RevokeInvite(id int64) error { return DB.RevokeInvite(id) }

// SetUserEnabled 启用/禁用用户（禁用立即从运行配置与内存映射移除，会话即刻失效）。
func SetUserEnabled(token string, enabled bool) error {
	u, ok, err := DB.GetUser(token)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("user not found")
	}
	if err := DB.SetUserEnabled(token, enabled); err != nil {
		return err
	}
	if enabled {
		config.Config.SetUser(u.Token, u.Subdomain)
		model.UserDnsDataMap.AddUser(u.Token, u.Subdomain)
	} else {
		config.Config.DeleteUser(u.Token)
		model.UserDnsDataMap.RemoveUser(u.Token)
	}
	return nil
}

// ResetUserSubdomain 重置用户子域名（校验唯一性/合法性并同步 DNS 映射）。
func ResetUserSubdomain(token, newSubdomain string) error {
	if !ValidSubdomain(newSubdomain) {
		return errors.New("invalid subdomain")
	}
	if SubdomainExists(newSubdomain) {
		return errors.New("subdomain already exists")
	}
	if _, ok, _ := DB.GetUser(token); !ok {
		return errors.New("not a registered user")
	}
	if err := DB.UpdateUserSubdomain(token, newSubdomain); err != nil {
		return err
	}
	config.Config.SetUser(token, newSubdomain)
	model.UserDnsDataMap.RemoveUser(token)
	model.UserDnsDataMap.AddUser(token, newSubdomain)
	return nil
}

// ResetUserToken 重置用户认证 Token（仅注册用户，事务内更新）。
func ResetUserToken(oldToken, newToken string) error {
	if !ValidToken(newToken) {
		return errors.New("invalid token")
	}
	if _, exists := config.Config.GetUser(newToken); exists {
		return errors.New("token already exists")
	}
	sub, ok := config.Config.GetUser(oldToken)
	if !ok {
		return errors.New("user not found")
	}
	u, ok, err := DB.GetUser(oldToken)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("not a registered user")
	}
	if err := DB.ResetUserToken(oldToken, newToken); err != nil {
		return err
	}
	config.Config.DeleteUser(oldToken)
	config.Config.SetUser(newToken, sub)
	config.Config.SetUserDomain(newToken, u.Domain)
	model.UserDnsDataMap.RemoveUser(oldToken)
	model.UserDnsDataMap.AddUser(newToken, sub)
	return nil
}
