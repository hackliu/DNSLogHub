package config

import (
	"os"
	"sync"

	"gopkg.in/yaml.v2"
)

var Config = &config{}

// userMu 保护 User 映射的并发读写（注册功能会在运行时增删用户）
var userMu sync.RWMutex

// reloadMu 保护可热加载字段的并发读写
var reloadMu sync.RWMutex

type config struct {
	Http     HttpConfig     `yaml:"Http"`
	User     UserConfig     `yaml:"User"`
	Dns      DnsConfig      `yaml:"Dns"`
	Register RegisterConfig `yaml:"Register"`
	Log      LogConfig      `yaml:"Log"`
	Data     DataConfig     `yaml:"Data"`
	Admin    AdminConfig    `yaml:"Admin"`

	userDomains map[string]string // token -> 域名（运行时，空 = 主域名）
}

type HttpConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	ConsoleDisable bool   `yaml:"consoleDisable"`
}

type UserConfig map[string]string

type DnsConfig struct {
	Domain  string            `yaml:"domain"`  // 主域名（兼容旧配置）
	Domains []string          `yaml:"domains"` // 多域名列表（可选）
	ARecord map[string]string `yaml:"ARecord"` // ARecord 也是键值对，使用 map[string]string
}

// AllDomains 返回全部根域名（未配置 Domains 时回退到单个 Domain）。
func (c *config) AllDomains() []string {
	if len(c.Dns.Domains) > 0 {
		return c.Dns.Domains
	}
	if c.Dns.Domain != "" {
		return []string{c.Dns.Domain}
	}
	return nil
}

type RegisterConfig struct {
	Enabled            bool `yaml:"enabled"`
	RateLimitPerMinute int  `yaml:"rateLimitPerMinute"`
}

type LogConfig struct {
	Level      string `yaml:"level"`      // debug / info / warn / error
	Dir        string `yaml:"dir"`        // 日志目录
	MaxSize    int    `yaml:"maxSize"`    // 单文件大小上限 (MB)
	MaxBackups int    `yaml:"maxBackups"` // 保留的历史文件数
	MaxAge     int    `yaml:"maxAge"`     // 历史文件最长保留天数
}

type DataConfig struct {
	DbFile        string `yaml:"dbFile"`
	FlushInterval int    `yaml:"flushInterval"` // 秒
}

type AdminConfig struct {
	Token         string `yaml:"token"`         // 管理员令牌（非空则启用管理接口）
	LoginPath     string `yaml:"loginPath"`     // 后台登录页路径
	DashboardPath string `yaml:"dashboardPath"` // 后台管理页路径
}

// GetUserBySubdomainAndDomain 按子域名前缀 + 所属域名反查 token。
func (c *config) GetUserBySubdomainAndDomain(subdomain, domain string) string {
	userMu.RLock()
	defer userMu.RUnlock()
	for token, sub := range c.User {
		if sub != subdomain {
			continue
		}
		ud := c.userDomains[token]
		if ud == "" {
			ud = c.Dns.Domain
		}
		if ud == domain {
			return token
		}
	}
	return "other"
}

// GetUserDomain 返回用户所属域名（未设置时为主域名）。
func (c *config) GetUserDomain(token string) string {
	userMu.RLock()
	defer userMu.RUnlock()
	if d, ok := c.userDomains[token]; ok && d != "" {
		return d
	}
	return c.Dns.Domain
}

// SetUserDomain 设置用户所属域名。
func (c *config) SetUserDomain(token, domain string) {
	userMu.Lock()
	defer userMu.Unlock()
	if c.userDomains == nil {
		c.userDomains = map[string]string{}
	}
	c.userDomains[token] = domain
}

// GetUser 并发安全地读取用户子域名
func (c *config) GetUser(token string) (string, bool) {
	userMu.RLock()
	defer userMu.RUnlock()
	v, ok := c.User[token]
	return v, ok
}

// SetUser 并发安全地写入用户
func (c *config) SetUser(token, subdomain string) {
	userMu.Lock()
	defer userMu.Unlock()
	if c.User == nil {
		c.User = map[string]string{}
	}
	c.User[token] = subdomain
}

// DeleteUser 并发安全地删除用户
func (c *config) DeleteUser(token string) {
	userMu.Lock()
	defer userMu.Unlock()
	delete(c.User, token)
}

// UserSnapshot 返回 User 映射的快照副本
func (c *config) UserSnapshot() map[string]string {
	userMu.RLock()
	defer userMu.RUnlock()
	m := make(map[string]string, len(c.User))
	for k, v := range c.User {
		m[k] = v
	}
	return m
}

// Reload 从 config.yaml 重新加载，并原子地应用可热加载字段。
// 可热加载：Register.enabled、Register.rateLimitPerMinute。
// 其余字段（Http/Dns/路径/Admin）需重启生效。
// 解析或校验失败返回错误且不改变任何运行中配置（等价回滚）。
func (c *config) Reload() error {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return err
	}
	var fresh config
	if err := yaml.Unmarshal(data, &fresh); err != nil {
		return err
	}
	fresh.Normalize()

	reloadMu.Lock()
	c.Register.Enabled = fresh.Register.Enabled
	c.Register.RateLimitPerMinute = fresh.Register.RateLimitPerMinute
	reloadMu.Unlock()
	return nil
}

// RegisterEnabled 并发安全地读取注册开关
func (c *config) RegisterEnabled() bool {
	reloadMu.RLock()
	defer reloadMu.RUnlock()
	return c.Register.Enabled
}

// RegisterRateLimit 并发安全地读取注册限流值
func (c *config) RegisterRateLimit() int {
	reloadMu.RLock()
	defer reloadMu.RUnlock()
	return c.Register.RateLimitPerMinute
}

// Normalize 填充缺失配置的默认值
func (c *config) Normalize() {
	if len(c.Dns.Domains) > 0 && c.Dns.Domain == "" {
		c.Dns.Domain = c.Dns.Domains[0]
	}
	if c.Register.RateLimitPerMinute <= 0 {
		c.Register.RateLimitPerMinute = 5
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Dir == "" {
		c.Log.Dir = "logs"
	}
	if c.Log.MaxSize <= 0 {
		c.Log.MaxSize = 10
	}
	if c.Log.MaxBackups <= 0 {
		c.Log.MaxBackups = 7
	}
	if c.Log.MaxAge <= 0 {
		c.Log.MaxAge = 30
	}
	if c.Data.DbFile == "" {
		c.Data.DbFile = "data/dnsloghub.db"
	}
	if c.Data.FlushInterval <= 0 {
		c.Data.FlushInterval = 10
	}
	if c.Admin.LoginPath == "" {
		c.Admin.LoginPath = "/admin"
	}
	if c.Admin.DashboardPath == "" {
		c.Admin.DashboardPath = "/admin/dashboard"
	}
}
