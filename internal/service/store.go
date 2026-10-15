package service

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/logger"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/store"

	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

var (
	DB       *store.Store
	recordCh chan store.RecordEntry
)

// InitStore 打开 SQLite、迁移 schema、导入旧数据并加载到内存。
func InitStore(dbFile string) error {
	s, err := store.Open(dbFile)
	if err != nil {
		return err
	}
	DB = s

	if err := migrateLegacy(); err != nil {
		logger.Logger.Warn("legacy data migration failed", zap.Error(err))
	}
	if err := loadUsers(); err != nil {
		return err
	}
	if err := loadRecords(); err != nil {
		return err
	}

	recordCh = make(chan store.RecordEntry, 100000)
	model.UserDnsDataMap.OnSet = func(token string, info model.DnsInfo) {
		recordCh <- store.RecordEntry{Token: token, Info: info}
	}
	return nil
}

func loadUsers() error {
	users, err := DB.ListUsers()
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.Enabled {
			config.Config.SetUser(u.Token, u.Subdomain)
			config.Config.SetUserDomain(u.Token, u.Domain)
			model.UserDnsDataMap.AddUser(u.Token, u.Subdomain)
		}
	}
	return nil
}

func loadRecords() error {
	loaded := make(map[string][]model.DnsInfo)
	for token := range config.Config.UserSnapshot() {
		recs, err := DB.RecordsByToken(token)
		if err != nil {
			return err
		}
		loaded[token] = recs
	}
	model.UserDnsDataMap.Load(loaded)
	return nil
}

// StartPersistence 周期批量落盘 + 退出信号时 flush。
func StartPersistence(flushInterval time.Duration) {
	if DB == nil || flushInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		for range ticker.C {
			flushRecords()
		}
	}()
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		flushRecords()
		os.Exit(0)
	}()
}

func flushRecords() {
	if recordCh == nil {
		return
	}
	batch := make([]store.RecordEntry, 0, len(recordCh))
	for {
		select {
		case e := <-recordCh:
			batch = append(batch, e)
		default:
			goto done
		}
	}
done:
	if len(batch) == 0 {
		return
	}
	if err := DB.InsertRecords(batch); err != nil {
		logger.Logger.Error("insert records error", zap.Error(err))
		// 失败时重新入队，避免丢数据（DB 持续失败会背压到写入方）
		for _, e := range batch {
			recordCh <- e
		}
	}
}

// ClearRecords 清空某用户记录（内存 + DB）。
func ClearRecords(token string) error {
	model.UserDnsDataMap.Clear(token)
	return ClearDBRecords(token)
}

// ClearDBRecords 仅清空 DB 中某用户记录。
func ClearDBRecords(token string) error {
	if DB != nil {
		return DB.ClearRecords(token)
	}
	return nil
}

// migrateLegacy 一次性迁移旧文件数据（JSON/YAML → SQLite），成功后改名 .bak。
func migrateLegacy() error {
	if data, err := os.ReadFile("data/users.yaml"); err == nil {
		var f struct {
			Users map[string]string `yaml:"users"`
		}
		if yaml.Unmarshal(data, &f) == nil {
			for token, sub := range f.Users {
				_ = DB.SaveUser(store.User{Token: token, Subdomain: sub, Enabled: true, CreatedAt: time.Now().Unix()})
			}
			_ = os.Rename("data/users.yaml", "data/users.yaml.bak")
		}
	}
	if data, err := os.ReadFile("data/invites.yaml"); err == nil {
		var f struct {
			Invites []struct {
				Hash           string `yaml:"hash"`
				RemainingUses  int    `yaml:"remainingUses"`
				ExpiresAt      string `yaml:"expiresAt"`
				BoundSubdomain string `yaml:"boundSubdomain"`
				Note           string `yaml:"note"`
			} `yaml:"invites"`
		}
		if yaml.Unmarshal(data, &f) == nil {
			for _, iv := range f.Invites {
				_ = DB.AddInvite(store.Invite{Hash: iv.Hash, RemainingUses: iv.RemainingUses, ExpiresAt: iv.ExpiresAt, BoundSubdomain: iv.BoundSubdomain, Note: iv.Note})
			}
			_ = os.Rename("data/invites.yaml", "data/invites.yaml.bak")
		}
	}
	if data, err := os.ReadFile("data/records.json"); err == nil {
		var f struct {
			Records map[string][]model.DnsInfo `json:"records"`
		}
		if json.Unmarshal(data, &f) == nil {
			for token, recs := range f.Records {
				entries := make([]store.RecordEntry, 0, len(recs))
				for _, r := range recs {
					entries = append(entries, store.RecordEntry{Token: token, Info: r})
				}
				_ = DB.InsertRecords(entries)
			}
			_ = os.Rename("data/records.json", "data/records.json.bak")
		}
	}
	return nil
}
