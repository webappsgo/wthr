package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

// newAuthSettingsTestConfigFile writes a minimal valid server.yml under
// Go's per-test temp dir (t.TempDir(), auto-cleaned) and returns its path
// for AdminAuthSettingsHandler.ConfigPath, matching the pattern used
// throughout this package's other handler tests.
func newAuthSettingsTestConfigFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  auth:\n    oidc:\n      enabled: false\n"), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	t.Setenv("CONFIG_DIR", filepath.Dir(path))
	return path
}

func authSettingsAreaPath(t *testing.T, key string) string {
	t.Helper()
	return filepath.Join(os.Getenv("CONFIG_DIR"), config.CanonicalConfigOwner(key))
}

// TestAdminAuthSettingsHandler_ShowAuthSettings covers the trivial HTML
// render path. No auth/data gating exists on this handler.
func TestAdminAuthSettingsHandler_ShowAuthSettings(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	r, w := newAPITestContext("/admin/security/auth")
	defer htmlRenderGuard(t)
	h.ShowAuthSettings(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_Success verifies a valid
// JSON body is applied to the on-disk YAML config and a 200 ok response is
// sent.
func TestAdminAuthSettingsHandler_UpdateAuthSettings_Success(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	t.Setenv("CONFIG_DIR", filepath.Dir(h.ConfigPath))

	body := `{"oidc_enabled":true,"ldap_enabled":true,"ldap_server":"ldap.example.com","ldap_port":636,"ldap_bind_dn":"cn=admin,dc=example,dc=com","ldap_bind_password":"secret","ldap_base_dn":"dc=example,dc=com","ldap_user_filter":"(uid=%s)","totp_enabled":true,"totp_issuer":"wthr","totp_digits":6,"totp_period":30,"passkeys_enabled":true,"passkeys_rp_id":"example.com","passkeys_rp_name":"wthr"}`

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/security/auth", body)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(authSettingsAreaPath(t, "server.auth.ldap.server"))
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("ldap.example.com")) {
		t.Errorf("expected config to contain ldap server 'ldap.example.com', got:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte("enabled: true")) {
		t.Errorf("expected config to contain 'enabled: true', got:\n%s", updated)
	}
	if bytes.Contains(updated, []byte("secret")) {
		t.Errorf("config file must not contain the LDAP bind password:\n%s", updated)
	}
	storedSecret, err := config.ReadSecret("server.auth.ldap.bind_password")
	if err != nil || storedSecret != "secret" {
		t.Errorf("LDAP secret = %q, %v; want it stored in secrets.yml", storedSecret, err)
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_PersistsOIDCProviders
// verifies the oidc_providers list is actually written to its owner file, not
// silently dropped (regression: the updates map used to omit this field).
func TestAdminAuthSettingsHandler_UpdateAuthSettings_PersistsOIDCProviders(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	t.Setenv("CONFIG_DIR", filepath.Dir(h.ConfigPath))

	body := `{"oidc_enabled":true,"oidc_providers":[{"name":"okta","client_id":"abc123","client_secret":"shh","issuer_url":"https://okta.example.com","redirect_url":"https://wthr.example.com/callback"}]}`

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/security/auth", body)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(authSettingsAreaPath(t, "server.auth.oidc.providers"))
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("client_id: abc123")) {
		t.Errorf("expected config to contain persisted OIDC provider client_id 'abc123', got:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte("name: okta")) {
		t.Errorf("expected config to contain persisted OIDC provider name 'okta', got:\n%s", updated)
	}
	if bytes.Contains(updated, []byte("shh")) {
		t.Errorf("config file must not contain the OIDC client secret:\n%s", updated)
	}
	storedSecret, err := config.ReadSecret("server.auth.oidc.providers.okta.client_secret")
	if err != nil || storedSecret != "shh" {
		t.Errorf("OIDC secret = %q, %v; want it stored in secrets.yml", storedSecret, err)
	}
}

func TestAdminAuthSettingsHandler_FormPreservesBlankSecrets(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	t.Setenv("CONFIG_DIR", filepath.Dir(h.ConfigPath))
	if err := config.WriteSecret("server.auth.ldap.bind_password", "ldap-existing"); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteSecret("server.auth.oidc.providers.okta.client_secret", "oidc-existing"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.ConfigPath, []byte("server:\n  auth:\n    oidc:\n      enabled: true\n      providers:\n        - name: okta\n          client_id: cid\n          issuer_url: https://issuer.example\n          redirect_url: https://app.example/callback\n    ldap:\n      enabled: true\n      server: ldap.example\n      port: 636\n      bind_dn: cn=service,dc=example\n      base_dn: dc=example\n      user_filter: (uid=%s)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := url.Values{
		"oidc_enabled":                {"true"},
		"oidc_provider_name":          {"okta", ""},
		"oidc_provider_client_id":     {"cid", ""},
		"oidc_provider_client_secret": {"", ""},
		"oidc_provider_issuer_url":    {"https://issuer.example", ""},
		"oidc_provider_redirect_url":  {"https://app.example/callback", ""},
		"ldap_enabled":                {"true"},
		"ldap_server":                 {"ldap.example"},
		"ldap_port":                   {"636"},
		"ldap_bind_dn":                {"cn=service,dc=example"},
		"ldap_bind_password":          {""},
		"ldap_base_dn":                {"dc=example"},
		"ldap_user_filter":            {"(uid=%s)"},
		"totp_digits":                 {"6"},
		"totp_period":                 {"30"},
		"totp_issuer":                 {"Weather"},
		"passkeys_rp_name":            {"Weather"},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/administration/security/auth", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UpdateAuthSettings(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", w.Code, w.Body.String())
	}
	for key, want := range map[string]string{
		"server.auth.ldap.bind_password":                "ldap-existing",
		"server.auth.oidc.providers.okta.client_secret": "oidc-existing",
	} {
		got, err := config.ReadSecret(key)
		if err != nil || got != want {
			t.Errorf("ReadSecret(%q) = %q, %v; want %q", key, got, err, want)
		}
	}
	stored, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("ldap-existing")) || bytes.Contains(stored, []byte("oidc-existing")) {
		t.Errorf("regular YAML contains a secret:\n%s", stored)
	}
}

func TestAdminAuthSettingsHandler_FormAddsOIDCProvider(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	t.Setenv("CONFIG_DIR", filepath.Dir(h.ConfigPath))
	values := url.Values{
		"oidc_enabled":                {"true"},
		"oidc_provider_name":          {"google", ""},
		"oidc_provider_client_id":     {"client-id", ""},
		"oidc_provider_client_secret": {"client-secret", ""},
		"oidc_provider_issuer_url":    {"https://accounts.example.com", ""},
		"oidc_provider_redirect_url":  {"https://app.example.com/callback", ""},
		"ldap_port":                   {"389"},
		"totp_digits":                 {"6"},
		"totp_period":                 {"30"},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/administration/security/auth", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UpdateAuthSettings(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", w.Code, w.Body.String())
	}
	storedSecret, err := config.ReadSecret("server.auth.oidc.providers.google.client_secret")
	if err != nil || storedSecret != "client-secret" {
		t.Errorf("OIDC client secret = %q, %v; want secrets.yml value", storedSecret, err)
	}
	storedConfig, err := os.ReadFile(authSettingsAreaPath(t, "server.auth.oidc.providers"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(storedConfig, []byte("client-secret")) {
		t.Errorf("regular YAML contains the OIDC client secret:\n%s", storedConfig)
	}
	if !bytes.Contains(storedConfig, []byte("name: google")) || !bytes.Contains(storedConfig, []byte("client_id: client-id")) {
		t.Errorf("regular YAML is missing OIDC provider settings:\n%s", storedConfig)
	}
}

func TestAdminAuthSettingsHandler_RejectsInvalidOIDCProviderBeforeSavingSecret(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	body := `{"oidc_enabled":true,"oidc_providers":[{"name":"bad.name","client_id":"cid","client_secret":"must-not-save","issuer_url":"https://issuer.example","redirect_url":"https://app.example/callback"}]}`
	r, w := newTestContextJSON(t, http.MethodPost, "/admin/security/auth", body)
	h.UpdateAuthSettings(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(configDir, "secrets.yml")); !os.IsNotExist(err) {
		t.Fatalf("secrets.yml created for rejected provider: stat error = %v", err)
	}
}

func TestValidOIDCProviderName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{name: "google", want: true},
		{name: "my-provider_2", want: true},
		{name: "", want: false},
		{name: "BadName", want: false},
		{name: "bad.name", want: false},
		{name: "../secret", want: false},
	} {
		if got := validOIDCProviderName(tc.name); got != tc.want {
			t.Errorf("validOIDCProviderName(%q) = %t, want %t", tc.name, got, tc.want)
		}
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_InvalidJSON verifies
// malformed request bodies are rejected with 400 and never reach the
// config writer.
func TestAdminAuthSettingsHandler_UpdateAuthSettings_InvalidJSON(t *testing.T) {
	configPath := newAuthSettingsTestConfigFile(t)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read baseline config: %v", err)
	}

	h := &AdminAuthSettingsHandler{ConfigPath: configPath}

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/security/auth", "{not valid json")

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config after invalid request: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("expected config to be untouched on invalid JSON, before:\n%s\nafter:\n%s", before, after)
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_ConfigWriteError verifies
// a missing/unreadable config file surfaces as 500, not a panic.
func TestAdminAuthSettingsHandler_UpdateAuthSettings_ConfigWriteError(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: filepath.Join(t.TempDir(), "does-not-exist", "server.yml")}
	t.Setenv("CONFIG_DIR", t.TempDir())

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/security/auth", `{"oidc_enabled":true}`)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
