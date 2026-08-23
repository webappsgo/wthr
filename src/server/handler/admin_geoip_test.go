package handler

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// newGeoIPTestConfigFile writes a minimal valid server.yml under Go's
// per-test temp dir (t.TempDir(), auto-cleaned) and returns its path for
// AdminGeoIPHandler.ConfigPath, matching the pattern used throughout this
// package's other handler tests (e.g. admin_web_test.go, admin_api_test.go).
func newGeoIPTestConfigFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  geoip:\n    enabled: false\n"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// TestAdminGeoIPHandler_ShowGeoIPSettings covers the trivial HTML render
// path. No auth/data gating exists on this handler.
func TestAdminGeoIPHandler_ShowGeoIPSettings(t *testing.T) {
	h := &AdminGeoIPHandler{ConfigPath: newGeoIPTestConfigFile(t)}
	c, w := newAPITestContext("/server/admin/config/geoip")
	defer htmlRenderGuard(t)
	h.ShowGeoIPSettings(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// TestAdminGeoIPHandler_UpdateGeoIPSettings_Success verifies a valid JSON
// body is applied to the on-disk YAML config and a 200 ok response is sent.
func TestAdminGeoIPHandler_UpdateGeoIPSettings_Success(t *testing.T) {
	h := &AdminGeoIPHandler{ConfigPath: newGeoIPTestConfigFile(t)}

	body := `{"enabled":true,"dir":"/data/geoip","update_frequency":7,"deny_countries":["CN","RU"],"database_asn":true,"database_country":true,"database_city":false}`

	c, w := newTestContextJSON(t, http.MethodPost, "/admin/config/geoip", body)

	h.UpdateGeoIPSettings(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if !bytes.Contains(updated, []byte("enabled: true")) {
		t.Errorf("expected config to contain 'enabled: true', got:\n%s", updated)
	}
}

// TestAdminGeoIPHandler_UpdateGeoIPSettings_InvalidJSON verifies malformed
// request bodies are rejected with 400 and never reach the config writer.
func TestAdminGeoIPHandler_UpdateGeoIPSettings_InvalidJSON(t *testing.T) {
	configPath := newGeoIPTestConfigFile(t)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read baseline config: %v", err)
	}

	h := &AdminGeoIPHandler{ConfigPath: configPath}

	c, w := newTestContextJSON(t, http.MethodPost, "/admin/config/geoip", "{not valid json")

	h.UpdateGeoIPSettings(w, c)

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

// TestAdminGeoIPHandler_UpdateGeoIPSettings_ConfigWriteError verifies a
// missing/unreadable config file surfaces as 500, not a panic.
func TestAdminGeoIPHandler_UpdateGeoIPSettings_ConfigWriteError(t *testing.T) {
	h := &AdminGeoIPHandler{ConfigPath: filepath.Join(t.TempDir(), "does-not-exist", "server.yml")}

	c, w := newTestContextJSON(t, http.MethodPost, "/admin/config/geoip", `{"enabled":true}`)

	h.UpdateGeoIPSettings(w, c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
