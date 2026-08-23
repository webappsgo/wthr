package handler

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// newAuthSettingsTestConfigFile writes a minimal valid server.yml under
// Go's per-test temp dir (t.TempDir(), auto-cleaned) and returns its path
// for AdminAuthSettingsHandler.ConfigPath, matching the pattern used
// throughout this package's other handler tests.
func newAuthSettingsTestConfigFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  auth:\n    oidc:\n      enabled: false\n"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// TestAdminAuthSettingsHandler_ShowAuthSettings covers the trivial HTML
// render path. No auth/data gating exists on this handler.
func TestAdminAuthSettingsHandler_ShowAuthSettings(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	r, w := newAPITestContext("/admin/config/auth")
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

	body := `{"oidc_enabled":true,"ldap_enabled":true,"ldap_server":"ldap.example.com","ldap_port":636,"ldap_bind_dn":"cn=admin,dc=example,dc=com","ldap_bind_password":"secret","ldap_base_dn":"dc=example,dc=com","ldap_user_filter":"(uid=%s)","totp_enabled":true,"totp_issuer":"wthr","totp_digits":6,"totp_period":30,"passkeys_enabled":true,"passkeys_rp_id":"example.com","passkeys_rp_name":"wthr"}`

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/config/auth", body)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("ldap.example.com")) {
		t.Errorf("expected config to contain ldap server 'ldap.example.com', got:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte("enabled: true")) {
		t.Errorf("expected config to contain 'enabled: true', got:\n%s", updated)
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_PersistsOIDCProviders
// verifies the oidc_providers list is actually written to server.yml, not
// silently dropped (regression: the updates map used to omit this field).
func TestAdminAuthSettingsHandler_UpdateAuthSettings_PersistsOIDCProviders(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}

	body := `{"oidc_enabled":true,"oidc_providers":[{"name":"okta","client_id":"abc123","client_secret":"shh","issuer_url":"https://okta.example.com","redirect_url":"https://wthr.example.com/callback"}]}`

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/config/auth", body)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("client_id: abc123")) {
		t.Errorf("expected config to contain persisted OIDC provider client_id 'abc123', got:\n%s", updated)
	}
	if !bytes.Contains(updated, []byte("name: okta")) {
		t.Errorf("expected config to contain persisted OIDC provider name 'okta', got:\n%s", updated)
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

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/config/auth", "{not valid json")

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

	r, w := newTestContextJSON(t, http.MethodPost, "/admin/config/auth", `{"oidc_enabled":true}`)

	h.UpdateAuthSettings(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
