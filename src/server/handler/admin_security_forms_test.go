package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/model"
)

func TestSaveSecuritySettingsForm(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"security.csp", "security.cors_enabled", "security.cors_origins", "security.cors_methods", "security.cors_headers", "security.cors_credentials", "security.cors_max_age", "rate_limit.enabled", "rate_limit.global", "rate_limit.per_ip", "rate_limit.api", "rate_limit.admin", "security.session_timeout", "security.max_login_attempts", "security.lockout_duration"} {
		if err := (&model.SettingsModel{}).SetString(key, "old"); err != nil {
			t.Fatal(err)
		}
	}
	form := url.Values{
		"section": {"cors"}, "security.cors_enabled": {"on"}, "security.cors_origins": {"https://example.com"},
		"security.cors_methods": {"GET, POST"}, "security.cors_headers": {"Content-Type"}, "security.cors_max_age": {"3600"},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/administration/security/settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	SaveSecuritySettingsForm(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", w.Code, w.Body.String())
	}
	enabledValue, _, err := config.ReadSetting("security.cors_enabled")
	if err != nil {
		t.Fatal(err)
	}
	originsValue, _, err := config.ReadSetting("security.cors_origins")
	if err != nil {
		t.Fatal(err)
	}
	enabled, origins := fmt.Sprint(enabledValue), fmt.Sprint(originsValue)
	if enabled != "true" || origins != "https://example.com" {
		t.Fatalf("saved CORS settings = (%q, %q)", enabled, origins)
	}
}

func TestSaveSecuritySettingsFormRejectsInvalidSection(t *testing.T) {
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, newTestUsersDB(t))
	r := httptest.NewRequest(http.MethodPost, "/server/administration/security/settings", strings.NewReader("section=invalid"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	SaveSecuritySettingsForm(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
}
