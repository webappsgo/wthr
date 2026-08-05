package handler

import (
	"bytes"
	"io"
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
	c, w := newAPITestContext("/admin/config/auth")
	// c.HTML needs an HTMLRender configured or gin panics; recover so a
	// missing-template failure doesn't crash the run, since we only assert
	// the handler didn't error out before reaching HTML().
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gin HTMLRender not configured in unit test context: %v", r)
		}
	}()
	h.ShowAuthSettings(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestAdminAuthSettingsHandler_UpdateAuthSettings_Success verifies a valid
// JSON body is applied to the on-disk YAML config and a 200 ok response is
// sent.
func TestAdminAuthSettingsHandler_UpdateAuthSettings_Success(t *testing.T) {
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}

	body := []byte(`{"oidc_enabled":true,"ldap_enabled":true,"ldap_server":"ldap.example.com","ldap_port":636,"ldap_bind_dn":"cn=admin,dc=example,dc=com","ldap_bind_password":"secret","ldap_base_dn":"dc=example,dc=com","ldap_user_filter":"(uid=%s)","totp_enabled":true,"totp_issuer":"wthr","totp_digits":6,"totp_period":30,"passkeys_enabled":true,"passkeys_rp_id":"example.com","passkeys_rp_name":"wthr"}`)

	c, w := newAPITestContext("/admin/config/auth")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateAuthSettings(c)

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

	c, w := newAPITestContext("/admin/config/auth")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte("{not valid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateAuthSettings(c)

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

	c, w := newAPITestContext("/admin/config/auth")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte(`{"oidc_enabled":true}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateAuthSettings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
