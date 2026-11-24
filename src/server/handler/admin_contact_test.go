package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
)

func TestSaveAdminContactNotificationsStoresAndRemovesWebhookSecrets(t *testing.T) {
	withTestGlobalConfig(t)
	cfg := config.DefaultAppConfig()
	config.SetGlobalConfig(cfg)

	post := func(values url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/server/administration/notify", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		SaveAdminContactNotifications(rec, req)
		return rec
	}

	values := url.Values{"role": {"general"}, "email": {"general@example.test"}, "webhook_generic": {"https://hooks.example.test/contact"}}
	if rec := post(values); rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	urlKey := "server.contact.general.webhooks.generic"
	secretKey := urlKey + "_secret"
	if got, err := config.ReadSecret(urlKey); err != nil || got != "https://hooks.example.test/contact" {
		t.Fatalf("saved URL secret = %q, %v", got, err)
	}
	secret, err := config.ReadSecret(secretKey)
	if err != nil || secret == "" {
		t.Fatalf("generated signing secret = %q, %v", secret, err)
	}

	if rec := post(url.Values{"role": {"general"}, "email": {"general@example.test"}, "webhook_generic": {""}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("remove status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{urlKey, secretKey} {
		if got, err := config.ReadSecret(key); err != nil || got != "" {
			t.Errorf("removed secret %q = %q, %v; want empty", key, got, err)
		}
	}
}

func TestShowAdminContactNotificationsDoesNotExposeSigningSecrets(t *testing.T) {
	withTestGlobalConfig(t)
	config.SetGlobalConfig(config.DefaultAppConfig())
	previousRenderer := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, _ *http.Request, status int, _ string, data map[string]interface{}) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("rendered"))
		if _, ok := data["roles"].([]contactWebhookFormRole); !ok {
			t.Errorf("render data roles type = %T", data["roles"])
		}
	}
	t.Cleanup(func() { middleware.RenderHTML = previousRenderer })
	configDir := os.Getenv("CONFIG_DIR")
	if err := config.WriteSecret("server.contact.admin.webhooks.generic_secret", "signing-secret-value"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/server/administration/notify", nil)
	rec := httptest.NewRecorder()
	ShowAdminContactNotifications(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("show status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "signing-secret-value") {
		t.Fatal("Notify page exposed a webhook signing secret")
	}
}

func TestAdminContactWebhooksHandlesMissingConfigurationAndRole(t *testing.T) {
	withTestGlobalConfig(t)
	config.SetGlobalConfig(nil)
	for _, role := range []string{"unknown", "general"} {
		req := httptest.NewRequest(http.MethodPost, "/server/administration/notify/test", strings.NewReader("role="+role))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		TestAdminContactWebhooks(rec, req)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/server/administration/notify" {
			t.Errorf("role %q response = %d %q", role, rec.Code, rec.Header().Get("Location"))
		}
	}
	config.SetGlobalConfig(config.DefaultAppConfig())
	req := httptest.NewRequest(http.MethodPost, "/server/administration/notify/test", strings.NewReader("role=general"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TestAdminContactWebhooks(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/server/administration/notify" {
		t.Fatalf("unconfigured webhook response = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
