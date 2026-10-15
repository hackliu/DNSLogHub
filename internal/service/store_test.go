package service

import (
	"path/filepath"
	"testing"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/model"
)

func TestRecordsPersistence(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")

	config.Config.Dns.Domain = "example.com"
	model.UserDnsDataMap = model.NewDnsDataMap(config.UserConfig{})
	if err := InitStore(dbFile); err != nil {
		t.Fatal(err)
	}

	if err := AddUser("admin", "dnslog", ""); err != nil {
		t.Fatal(err)
	}
	model.UserDnsDataMap.Set("admin", model.DnsInfo{Type: model.DNS, Subdomain: "a.example.com", Ipaddress: "1.2.3.4", Time: 123})
	model.UserDnsDataMap.Set("admin", model.DnsInfo{Type: model.Http, Subdomain: "/x/y", Ipaddress: "5.6.7.8", Request: "GET / HTTP/1.1", Time: 456})
	flushRecords()

	// 模拟重启：关闭 DB，重置内存，重新打开
	_ = DB.Close()
	model.UserDnsDataMap = model.NewDnsDataMap(config.UserConfig{})
	if err := InitStore(dbFile); err != nil {
		t.Fatal(err)
	}
	defer DB.Close()

	got := model.UserDnsDataMap.Get("admin")
	if len(got) != 2 {
		t.Fatalf("restored %d records, want 2", len(got))
	}
	if got[0].Subdomain != "a.example.com" || got[0].Ipaddress != "1.2.3.4" || got[0].Type != model.DNS {
		t.Fatalf("record 0 mismatch: %+v", got[0])
	}
	if got[1].Request != "GET / HTTP/1.1" || got[1].Type != model.Http {
		t.Fatalf("record 1 mismatch: %+v", got[1])
	}
}
