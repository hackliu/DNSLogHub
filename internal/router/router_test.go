package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hackliu/DNSLogHub/internal/config"
)

func TestSetupRouter(t *testing.T) {
	config.Config.Http.ConsoleDisable = false
	config.Config.Register.Enabled = true
	config.Config.Dns.Domain = "example.com"
	config.Config.SetUser("admin", "dnslog")

	// SetupRouter 内部用 template.Must 解析全部模板，若模板有语法错误会 panic。
	r := SetupRouter()

	paths := []string{"/", "/welcome", "/register", "/console", "/docs", "/admin", "/admin/dashboard", "/js/theme.js", "/js/i18n.js", "/css/theme.css"}
	for _, path := range paths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s => %d, want 200 (body: %.200s)", path, w.Code, w.Body.String())
		}
	}
}
