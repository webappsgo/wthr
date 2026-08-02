package handler

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// newNotificationsTestConfigFile writes a minimal valid server.yml under
// Go's per-test temp dir (t.TempDir(), auto-cleaned) and returns its path
// for AdminNotificationsHandler.ConfigPath, matching the pattern used
// throughout this package's other handler tests.
func newNotificationsTestConfigFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  notifications:\n    webhook:\n      enabled: false\n"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// TestAdminNotificationsHandler_UpdateNotificationSettings_Success verifies
// a valid JSON body is applied to the on-disk YAML config and a 200 ok
// response is sent.
func TestAdminNotificationsHandler_UpdateNotificationSettings_Success(t *testing.T) {
	h := &AdminNotificationsHandler{ConfigPath: newNotificationsTestConfigFile(t)}

	body := []byte(`{"email_startup":true,"email_shutdown":true,"email_backup_complete":true,"email_backup_failed":true,"email_cert_renewal":true,"webhook_enabled":true,"webhook_url":"https://hooks.example.com/x","webhook_events":["backup_failed"],"webui_position":"top-right","webui_duration":5000,"webui_max_stored":100,"webui_retention_days":30}`)

	c, w := newAPITestContext("/admin/config/notifications")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationSettings(c)

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
	if !bytes.Contains(updated, []byte("top-right")) {
		t.Errorf("expected config to contain webui position 'top-right', got:\n%s", updated)
	}
}

// TestAdminNotificationsHandler_UpdateNotificationSettings_InvalidJSON
// verifies malformed request bodies are rejected with 400 and never reach
// the config writer.
func TestAdminNotificationsHandler_UpdateNotificationSettings_InvalidJSON(t *testing.T) {
	configPath := newNotificationsTestConfigFile(t)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read baseline config: %v", err)
	}

	h := &AdminNotificationsHandler{ConfigPath: configPath}

	c, w := newAPITestContext("/admin/config/notifications")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte("{not valid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationSettings(c)

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

// TestAdminNotificationsHandler_UpdateNotificationSettings_ConfigWriteError
// verifies a missing/unreadable config file surfaces as 500, not a panic.
func TestAdminNotificationsHandler_UpdateNotificationSettings_ConfigWriteError(t *testing.T) {
	h := &AdminNotificationsHandler{ConfigPath: filepath.Join(t.TempDir(), "does-not-exist", "server.yml")}

	c, w := newAPITestContext("/admin/config/notifications")
	c.Request.Method = http.MethodPost
	c.Request.Body = io.NopCloser(bytes.NewReader([]byte(`{"email_startup":true}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationSettings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}
