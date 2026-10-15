package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/i18n"
	"github.com/hackliu/DNSLogHub/internal/model"
	"github.com/hackliu/DNSLogHub/internal/service"
	"github.com/hackliu/DNSLogHub/internal/store"
)

func TestRegisterFlowEndToEnd(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")

	config.Config.Register.Enabled = true
	config.Config.Dns.Domain = "example.com"
	config.Config.Http.ConsoleDisable = false
	model.UserDnsDataMap = model.NewDnsDataMap(config.UserConfig{})
	i18n.Init()
	if err := service.InitStore(dbFile); err != nil {
		t.Fatal(err)
	}
	defer service.DB.Close()

	plain, err := service.GenerateInviteCode(26)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := service.HashInviteCode(plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AddInvite(store.Invite{Hash: hash, RemainingUses: 2}); err != nil {
		t.Fatal(err)
	}

	r := SetupRouter()

	do := func(method, path, body, token string) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("token", token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// 1) 正常注册
	w := do(http.MethodPost, "/api/register", `{"token":"testtoken123","subdomain":"mylog","inviteCode":"`+plain+`"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("register => %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Subdomain string `json:"subdomain"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 200 || resp.Data.Subdomain != "mylog.example.com" {
		t.Fatalf("unexpected register resp: %+v (body %s)", resp, w.Body.String())
	}

	// 2) 注册后 token 可鉴权访问
	w = do(http.MethodGet, "/api/getDnsData", "", "testtoken123")
	if w.Code != http.StatusOK {
		t.Fatalf("getDnsData with new token => %d, want 200", w.Code)
	}

	// 3) 无效 token → 401
	w = do(http.MethodGet, "/api/getDnsData", "", "bad")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token => %d, want 401", w.Code)
	}

	// 4) 重复 token → 409
	w = do(http.MethodPost, "/api/register", `{"token":"testtoken123","subdomain":"other","inviteCode":"`+plain+`"}`, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("dup token => %d, want 409", w.Code)
	}

	// 5) 重复子域 → 409
	w = do(http.MethodPost, "/api/register", `{"token":"anothertoken","subdomain":"mylog","inviteCode":"`+plain+`"}`, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("dup subdomain => %d, want 409", w.Code)
	}

	// 6) 页面可渲染
	for _, p := range []string{"/", "/register", "/docs", "/console"} {
		w = do(http.MethodGet, p, "", "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s => %d, want 200", p, w.Code)
		}
	}

	// 7) 用户已持久化到 SQLite
	users, err := service.DB.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range users {
		if u.Token == "testtoken123" && u.Subdomain == "mylog" {
			found = true
		}
	}
	if !found {
		t.Fatal("registered user should be persisted in SQLite")
	}
}
