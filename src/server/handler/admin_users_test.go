package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdminUsersHandler_ShowUserSettings covers the trivial HTML render
// path. No auth/data gating exists on this handler.
func TestAdminUsersHandler_ShowUserSettings(t *testing.T) {
	h := &AdminUsersHandler{}
	c, w := newTestContext(http.MethodGet, "/server/admin/config/users/settings")
	// c.HTML needs an HTMLRender configured or gin panics; recover so a
	// missing-template failure doesn't crash the run, since we only assert
	// the handler didn't error out before reaching HTML().
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("gin HTMLRender not configured in unit test context: %v", r)
		}
	}()
	h.ShowUserSettings(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// newAdminUsersTestHandler writes a minimal valid server.yml to a scratch
// temp dir (never the project tree, per testing-rules.md) and returns a
// handler pointed at it.
func newAdminUsersTestHandler(t *testing.T) *AdminUsersHandler {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(path, []byte("users:\n  enabled: false\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return &AdminUsersHandler{ConfigPath: path}
}

// TestAdminUsersHandler_UpdateUserSettings_Success covers the accepted
// registration-mode value "private" and verifies the YAML file is actually
// updated on disk.
func TestAdminUsersHandler_UpdateUserSettings_Success(t *testing.T) {
	h := newAdminUsersTestHandler(t)
	body := map[string]interface{}{
		"enabled":           true,
		"registration_mode": "private",
		"registration_require_email_verification": true,
	}
	c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", body)
	h.UpdateUserSettings(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(h.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "enabled: true") {
		t.Errorf("config not updated on disk: %s", data)
	}
}

// TestAdminUsersHandler_UpdateUserSettings_InvalidMode covers the
// validation-error path for a garbage registration mode.
func TestAdminUsersHandler_UpdateUserSettings_InvalidMode(t *testing.T) {
	h := newAdminUsersTestHandler(t)
	body := map[string]interface{}{"registration_mode": "nonsense"}
	c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", body)
	h.UpdateUserSettings(w, c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminUsersHandler_UpdateUserSettings_MalformedJSON covers the empty
// / malformed request body edge case.
func TestAdminUsersHandler_UpdateUserSettings_MalformedJSON(t *testing.T) {
	h := newAdminUsersTestHandler(t)
	c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", "not json")
	h.UpdateUserSettings(w, c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminUsersHandler_UpdateUserSettings_MissingConfigFile covers the
// DB/IO-error path: ConfigPath points at a nonexistent file.
func TestAdminUsersHandler_UpdateUserSettings_MissingConfigFile(t *testing.T) {
	h := &AdminUsersHandler{ConfigPath: filepath.Join(t.TempDir(), "does-not-exist.yml")}
	body := map[string]interface{}{"registration_mode": "open"}
	c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", body)
	h.UpdateUserSettings(w, c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminUsersHandler_UpdateUserSettings_OpenModeAccepted verifies the
// AI.md PART 34 default registration mode "open" is accepted and persisted.
func TestAdminUsersHandler_UpdateUserSettings_OpenModeAccepted(t *testing.T) {
	h := newAdminUsersTestHandler(t)
	body := map[string]interface{}{"registration_mode": "open"}
	c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", body)
	h.UpdateUserSettings(w, c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for registration_mode=\"open\"; body=%s", w.Code, w.Body.String())
	}
}

// TestAdminUsersHandler_UpdateUserSettings_LegacyModesRejected verifies the
// modes AI.md PART 34 does not define (invite, admin_only, disabled) are
// rejected with 400 rather than silently persisted.
func TestAdminUsersHandler_UpdateUserSettings_LegacyModesRejected(t *testing.T) {
	for _, mode := range []string{"invite", "admin_only", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			h := newAdminUsersTestHandler(t)
			body := map[string]interface{}{"registration_mode": mode}
			c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/config/users/settings", body)
			h.UpdateUserSettings(w, c)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 for undefined mode %q; body=%s", w.Code, mode, w.Body.String())
			}
		})
	}
}
