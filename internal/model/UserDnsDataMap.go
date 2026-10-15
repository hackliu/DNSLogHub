package model

import (
	"sync"

	"github.com/hackliu/DNSLogHub/internal/config"
)

var UserDnsDataMap *DnsDataMap

type DnsDataMap struct {
	userDnsData       map[string][]DnsInfo
	Mu                sync.RWMutex
	TokenSubDomainMap map[string]string
	SubDomainTokenMap map[string]string
	OnSet             func(token string, data DnsInfo) // 可选回调（持久化）
}

func NewDnsDataMap(conf config.UserConfig) *DnsDataMap {
	TokenSubDomainMap := make(map[string]string)
	SubDomainTokenMap := make(map[string]string)
	for token, subdomain := range conf {
		TokenSubDomainMap[token] = subdomain
		SubDomainTokenMap[subdomain] = token
	}
	return &DnsDataMap{
		userDnsData:       make(map[string][]DnsInfo),
		TokenSubDomainMap: TokenSubDomainMap,
		SubDomainTokenMap: SubDomainTokenMap,
	}
}

func (d *DnsDataMap) Get(token string) []DnsInfo {
	d.Mu.RLock()
	defer d.Mu.RUnlock()
	s := d.userDnsData[token]
	out := make([]DnsInfo, len(s))
	copy(out, s)
	return out
}

// Set 追加一条记录（锁内 append，修复原有 Load-在锁外导致的并发丢数据竞态）
func (d *DnsDataMap) Set(token string, data DnsInfo) {
	d.Mu.Lock()
	d.userDnsData[token] = append(d.userDnsData[token], data)
	d.Mu.Unlock()
	if d.OnSet != nil {
		d.OnSet(token, data)
	}
}

func (d *DnsDataMap) Clear(token string) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	delete(d.userDnsData, token)
}

// GetAndClear 原子地“取后即清”：返回当前记录并清空，避免并发下重复消费或丢失数据
func (d *DnsDataMap) GetAndClear(token string) []DnsInfo {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	s := d.userDnsData[token]
	out := make([]DnsInfo, len(s))
	copy(out, s)
	delete(d.userDnsData, token)
	return out
}

// Snapshot 返回全部记录副本（持久化用）
func (d *DnsDataMap) Snapshot() map[string][]DnsInfo {
	d.Mu.RLock()
	defer d.Mu.RUnlock()
	out := make(map[string][]DnsInfo, len(d.userDnsData))
	for k, v := range d.userDnsData {
		c := make([]DnsInfo, len(v))
		copy(c, v)
		out[k] = c
	}
	return out
}

// Load 用快照替换内存数据（启动恢复用）
func (d *DnsDataMap) Load(records map[string][]DnsInfo) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	d.userDnsData = make(map[string][]DnsInfo, len(records))
	for k, v := range records {
		c := make([]DnsInfo, len(v))
		copy(c, v)
		d.userDnsData[k] = c
	}
}

func (d *DnsDataMap) AddUser(token, subdomain string) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	d.TokenSubDomainMap[token] = subdomain
	d.SubDomainTokenMap[subdomain] = token
}

func (d *DnsDataMap) RemoveUser(token string) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	if sub, ok := d.TokenSubDomainMap[token]; ok {
		delete(d.SubDomainTokenMap, sub)
		delete(d.TokenSubDomainMap, token)
	}
	delete(d.userDnsData, token)
}

func (d *DnsDataMap) GetTokenBySubdomain(subdomain string) (string, bool) {
	d.Mu.RLock()
	defer d.Mu.RUnlock()
	token, ok := d.SubDomainTokenMap[subdomain]
	return token, ok
}
