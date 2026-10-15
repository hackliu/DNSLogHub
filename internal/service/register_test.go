package service

import (
	"path/filepath"
	"testing"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/store"
)

func TestRegisterFlow(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")

	config.Config.Register.Enabled = true
	config.Config.Dns.Domain = "example.com"
	model.UserDnsDataMap = model.NewDnsDataMap(config.UserConfig{})
	if err := InitStore(dbFile); err != nil {
		t.Fatal(err)
	}
	defer DB.Close()

	// 1) 邀请码生成：长度与唯一性
	plain, err := GenerateInviteCode(26)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(plain) != 26 {
		t.Fatalf("length = %d, want 26", len(plain))
	}
	plain2, _ := GenerateInviteCode(26)
	if plain == plain2 {
		t.Fatal("two generated codes should differ")
	}

	// 2) 哈希 + 入库
	hash, err := HashInviteCode(plain)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := AddInvite(store.Invite{Hash: hash, RemainingUses: 2}); err != nil {
		t.Fatalf("add invite: %v", err)
	}

	// 3) 校验邀请码
	inv, err := VerifyInvite(plain, "mylog")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// 4) 创建用户
	if err := AddUser("testtoken123", "mylog", ""); err != nil {
		t.Fatalf("add user: %v", err)
	}
	if !SubdomainExists("mylog") {
		t.Fatal("subdomain should exist")
	}
	if _, ok := config.Config.GetUser("testtoken123"); !ok {
		t.Fatal("token should exist")
	}

	// 5) 消耗邀请码并校验剩余次数
	if err := ConsumeInvite(inv.Hash); err != nil {
		t.Fatalf("consume: %v", err)
	}
	inv2, err := VerifyInvite(plain, "otherlog")
	if err != nil {
		t.Fatalf("verify after consume: %v", err)
	}
	if inv2.RemainingUses != 1 {
		t.Fatalf("remaining = %d, want 1", inv2.RemainingUses)
	}

	// 6) 唯一性冲突
	if err := AddUser("testtoken123", "other", ""); err == nil {
		t.Fatal("token conflict should return error")
	}
	if err := AddUser("anothertoken", "mylog", ""); err == nil {
		t.Fatal("subdomain conflict should return error")
	}

	// 7) 绑定子域名策略
	boundHash, _ := HashInviteCode("boundcode")
	_ = AddInvite(store.Invite{Hash: boundHash, RemainingUses: 1, BoundSubdomain: "onlythis"})
	if _, err := VerifyInvite("boundcode", "other"); err == nil {
		t.Fatal("bound subdomain should reject mismatch")
	}
	if _, err := VerifyInvite("boundcode", "onlythis"); err != nil {
		t.Fatalf("bound subdomain should accept match: %v", err)
	}

	// 8) 无效邀请码
	if _, err := VerifyInvite("wrong", "mylog"); err == nil {
		t.Fatal("invalid code should return error")
	}
}

func TestValidTokenAndSubdomain(t *testing.T) {
	if !ValidToken("abcd1234") {
		t.Fatal("valid token rejected")
	}
	if ValidToken("short") {
		t.Fatal("short token accepted")
	}
	if ValidToken("bad token!") {
		t.Fatal("token with space accepted")
	}
	if !ValidSubdomain("mylog") {
		t.Fatal("valid subdomain rejected")
	}
	if ValidSubdomain("-mylog") {
		t.Fatal("leading hyphen accepted")
	}
	if ValidSubdomain("my_log") {
		t.Fatal("underscore accepted")
	}
}
