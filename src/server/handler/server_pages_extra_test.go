package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

type fakeMCP struct {
	enabled bool
	tools   []MCPToolInfo
}

func (f fakeMCP) Enabled() bool                 { return f.enabled }
func (f fakeMCP) PublicToolList() []MCPToolInfo { return f.tools }

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, w.Body.String())
	}
	return out
}

func TestGetDPOAPIFallbackChain(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Contact.Admin.Email = "admin@example.com"

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/dpo", nil)
	r.Header.Set("Accept", "application/json")
	r.Host = "wthr.example.com"
	w := httptest.NewRecorder()
	GetDPOAPI(cfg).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even without an address", w.Code)
	}
	body := decodeBody(t, w)
	if body["email"] != "" {
		t.Errorf("admin address leaked: %v", body["email"])
	}
	if !strings.HasSuffix(body["privacy_url"].(string), "/server/privacy") || !strings.HasPrefix(body["privacy_url"].(string), "http") {
		t.Errorf("privacy_url must be absolute, got %v", body["privacy_url"])
	}

	cfg.Server.Contact.General.Email = "general@example.com"
	w = httptest.NewRecorder()
	GetDPOAPI(cfg).ServeHTTP(w, r)
	if got := decodeBody(t, w)["email"]; got != "general@example.com" {
		t.Errorf("general fallback = %v", got)
	}

	cfg.Server.Contact.DPO = config.ContactDPOConfig{Name: "Dana", Email: "dpo@example.com"}
	w = httptest.NewRecorder()
	GetDPOAPI(cfg).ServeHTTP(w, r)
	body = decodeBody(t, w)
	if body["email"] != "dpo@example.com" || body["name"] != "Dana" {
		t.Errorf("dpo = %v", body)
	}
}

func TestMCPHelpDisabledIs404(t *testing.T) {
	cfg := &config.AppConfig{}
	for _, enabled := range []bool{false} {
		w := httptest.NewRecorder()
		GetMCPHelpAPI(cfg, fakeMCP{enabled: enabled}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/server/help/mcp", nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("api status = %d, want 404 when MCP is disabled", w.Code)
		}
		w = httptest.NewRecorder()
		ShowMCPHelpPage(cfg, fakeMCP{enabled: enabled}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/server/help/mcp", nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("page status = %d, want 404 when MCP is disabled", w.Code)
		}
	}
}

func TestMCPHelpAPIContent(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Pages.Help.MCP = "Ask about Berlin."
	src := fakeMCP{enabled: true, tools: []MCPToolInfo{{Name: "getWeather", Description: "Current weather"}}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/help/mcp", nil)
	r.Header.Set("Accept", "application/json")
	r.Host = "wthr.example.com"
	w := httptest.NewRecorder()
	GetMCPHelpAPI(cfg, src).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := decodeBody(t, w)
	if body["ok"] != true {
		t.Fatalf("envelope ok = %v", body["ok"])
	}
	data := body["data"].(map[string]interface{})
	if data["enabled"] != true || data["project"] != "Ask about Berlin." {
		t.Errorf("data = %v", data)
	}
	if !strings.HasSuffix(data["url"].(string), "/api/v1/server/mcp") {
		t.Errorf("url = %v", data["url"])
	}
	if len(data["aliases"].([]interface{})) != 2 || len(data["tools"].([]interface{})) != 1 {
		t.Errorf("aliases/tools = %v / %v", data["aliases"], data["tools"])
	}
	if data["auth"].(map[string]interface{})["scheme"] != "Bearer" {
		t.Errorf("auth = %v", data["auth"])
	}
}

func TestSecurityAPIs(t *testing.T) {
	resetGlobalConfig(t)
	cfg := &config.AppConfig{}
	cfg.Server.Contact.Security.Email = "security@example.com"
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/security", nil)
	r.Header.Set("Accept", "application/json")
	r.Host = "wthr.example.com"
	w := httptest.NewRecorder()
	GetSecurityAPI(cfg).ServeHTTP(w, r)
	body := decodeBody(t, w)
	contacts := body["contacts"].([]interface{})
	if len(contacts) != 2 || contacts[0].(map[string]interface{})["type"] != "form" || contacts[1].(map[string]interface{})["address"] != "security@example.com" {
		t.Errorf("contacts = %v", contacts)
	}
	if body["pgp"].(map[string]interface{})["available"] != false {
		t.Errorf("pgp = %v", body["pgp"])
	}
	for _, key := range []string{"policy_url", "thanks_url", "expires"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing %s", key)
		}
	}

	w = httptest.NewRecorder()
	policyReq := httptest.NewRequest(http.MethodGet, "/api/v1/server/security/policy", nil)
	policyReq.Header.Set("Accept", "application/json")
	GetSecurityPolicyAPI().ServeHTTP(w, policyReq)
	if _, ok := decodeBody(t, w)["policy"]; !ok {
		t.Error("policy field missing")
	}
	w = httptest.NewRecorder()
	thanksReq := httptest.NewRequest(http.MethodGet, "/api/v1/server/security/thanks", nil)
	thanksReq.Header.Set("Accept", "application/json")
	GetSecurityThanksAPI().ServeHTTP(w, thanksReq)
	if _, ok := decodeBody(t, w)["entries"]; !ok {
		t.Error("entries field missing")
	}
}

func TestServeTrackingScript(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.Tracking = config.TrackingConfig{Type: "google", ID: "G-TEST1"}
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(nil) })

	consent := `%7B%22essential%22%3Atrue%2C%22analytics%22%3Atrue%7D`
	cases := []struct {
		name   string
		host   string
		cookie string
		want   int
	}{
		{"no consent", "wthr.example.com", "", http.StatusNotFound},
		{"analytics declined", "wthr.example.com", `%7B%22analytics%22%3Afalse%7D`, http.StatusNotFound},
		{"consent given", "wthr.example.com", consent, http.StatusOK},
		{"onion never", "abcdefghijklmnop.onion", consent, http.StatusNotFound},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/server/tracking.js", nil)
		r.Host = c.host
		if c.cookie != "" {
			r.AddCookie(&http.Cookie{Name: "cookie_consent", Value: c.cookie})
		}
		w := httptest.NewRecorder()
		ServeTrackingScript(w, r)
		if w.Code != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, w.Code, c.want)
		}
		if c.want == http.StatusOK {
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
				t.Errorf("content type = %q", ct)
			}
			if w.Header().Get("Cache-Control") != "no-cache" {
				t.Errorf("cache control = %q", w.Header().Get("Cache-Control"))
			}
			if !strings.Contains(w.Body.String(), `gtag('config',"G-TEST1")`) {
				t.Errorf("body = %q", w.Body.String())
			}
		}
	}

	if !middleware.TrackingAllowed(func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: "cookie_consent", Value: consent})
		return r
	}()) {
		t.Error("TrackingAllowed must be true with consent on a clearnet host")
	}
}

func TestContactAPIRequiresCaptcha(t *testing.T) {
	cfg := &config.AppConfig{}
	post := func(body map[string]string) *httptest.ResponseRecorder {
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/contact", body)
		HandleContactFormSubmission(nil, cfg).ServeHTTP(w, r)
		return w
	}

	w := post(map[string]string{"name": "A", "email": "a@example.com", "subject": "S", "message": "M"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing captcha status = %d, want 400", w.Code)
	}
	body := decodeBody(t, w)
	if body["ok"] != false || body["error"] != "CAPTCHA_FAILED" {
		t.Errorf("error body = %v", body)
	}

	w = post(map[string]string{"name": "", "email": "bad", "subject": "S", "message": "M"})
	if got := decodeBody(t, w)["error"]; got != "VALIDATION_FAILED" {
		t.Errorf("invalid form error = %v", got)
	}
}
