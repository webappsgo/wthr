package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// newTorTestHandler wires a TorAdminHandler against a fresh in-memory
// server DB (for settings) and a temp data dir (for Tor keys/state). The
// underlying tor binary is not expected to be present in the test/CI
// environment, so Start()-dependent paths (Enable/Regenerate/etc.) exercise
// their error branches rather than a real Tor process.
func newTorTestHandler(t *testing.T) *TorAdminHandler {
	t.Helper()
	serverDB := newTestServerDB(t)
	// SettingsModel reads/writes through the package-level
	// database.GetServerDB() accessor rather than its injected DB field, so
	// the global dual-DB must be wired for SetBool/Get calls inside
	// Enable/Disable/UpdateSettings to hit this in-memory database.
	setGlobalTestDualDB(t, serverDB, serverDB)
	dataDir := t.TempDir()
	torService := service.NewTorService(&database.DB{DB: serverDB}, dataDir)
	settingsModel := &models.SettingsModel{DB: serverDB}
	return NewTorAdminHandler(torService, settingsModel, dataDir)
}

func TestTorAdminHandler_GetStatus(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/admin/tor/status")
	h.GetStatus(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["status"]; !ok {
		t.Fatalf("missing status key: %+v", resp)
	}
}

func TestTorAdminHandler_GetHealth(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/admin/tor/health")
	h.GetHealth(w, c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["health"]; !ok {
		t.Fatalf("missing health key: %+v", resp)
	}
}

// TestTorAdminHandler_Enable exercises the settings-write success path and
// (since the tor binary is unavailable in this environment) the
// TOR_START_FAILED branch.
func TestTorAdminHandler_Enable(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/admin/tor/enable")
	h.Enable(w, c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (no tor binary in test env); body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"] != "TOR_START_FAILED" {
		t.Fatalf("unexpected error payload: %+v", resp)
	}
}

func TestTorAdminHandler_Enable_SettingsFailure(t *testing.T) {
	h := newTorTestHandler(t)
	// Drop the settings table so SetBool fails before Start() is reached.
	if _, err := h.settingsModel.DB.Exec("DROP TABLE server_config"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	c, w := newTestContext(http.MethodPost, "/admin/tor/enable")
	h.Enable(w, c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"] != "DATABASE_ERROR" {
		t.Fatalf("unexpected error payload: %+v", resp)
	}
}

func TestTorAdminHandler_Disable(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/admin/tor/disable")
	h.Disable(w, c)
	// Stop() is a no-op success when the service was never started.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestTorAdminHandler_Disable_SettingsFailure(t *testing.T) {
	h := newTorTestHandler(t)
	if _, err := h.settingsModel.DB.Exec("DROP TABLE server_config"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	c, w := newTestContext(http.MethodPost, "/admin/tor/disable")
	h.Disable(w, c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

func TestTorAdminHandler_UpdateSettings(t *testing.T) {
	t.Run("invalid body returns 400", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/admin/tor", "{bad")
		h.UpdateSettings(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("no enabled field returns 400", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/admin/tor", map[string]interface{}{})
		h.UpdateSettings(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("enabled true delegates to Enable (fails without tor binary)", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/admin/tor", map[string]interface{}{"enabled": true})
		h.UpdateSettings(w, c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("enabled false delegates to Disable", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/admin/tor", map[string]interface{}{"enabled": false})
		h.UpdateSettings(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestTorAdminHandler_Regenerate(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/admin/tor/regenerate")
	h.Regenerate(w, c)
	// Stop() succeeds (never started), then Start() fails (no tor binary).
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

func TestTorAdminHandler_GenerateVanity(t *testing.T) {
	t.Run("missing prefix returns 400", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/tor/vanity/generate", map[string]interface{}{})
		h.GenerateVanity(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid prefix chars returns 400", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/tor/vanity/generate", map[string]interface{}{"prefix": "AB!"})
		h.GenerateVanity(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid prefix starts generation", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/tor/vanity/generate", map[string]interface{}{"prefix": "ab"})
		h.GenerateVanity(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		// Cancel immediately so the background goroutine doesn't outlive the test.
		_ = h.vanityGenerator.Cancel()
	})
}

func TestTorAdminHandler_GetVanityStatus(t *testing.T) {
	t.Run("not started returns running=false", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/admin/tor/vanity/status")
		h.GetVanityStatus(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["running"] != false {
			t.Fatalf("unexpected resp: %+v", resp)
		}
	})

	t.Run("started returns full status fields", func(t *testing.T) {
		h := newTorTestHandler(t)
		if err := h.vanityGenerator.Start("ab"); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer h.vanityGenerator.Cancel()

		c, w := newTestContext(http.MethodGet, "/admin/tor/vanity/status")
		h.GetVanityStatus(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := resp["prefix"]; !ok {
			t.Fatalf("missing prefix field: %+v", resp)
		}
	})
}

func TestTorAdminHandler_CancelVanity(t *testing.T) {
	t.Run("nothing running returns 400", func(t *testing.T) {
		h := newTorTestHandler(t)
		c, w := newTestContext(http.MethodPost, "/admin/tor/vanity/cancel")
		h.CancelVanity(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("running generation cancels successfully", func(t *testing.T) {
		h := newTorTestHandler(t)
		if err := h.vanityGenerator.Start("ab"); err != nil {
			t.Fatalf("start: %v", err)
		}
		c, w := newTestContext(http.MethodPost, "/admin/tor/vanity/cancel")
		h.CancelVanity(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestTorAdminHandler_ApplyVanity(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/admin/tor/vanity/apply")
	h.ApplyVanity(w, c)
	// No keys have ever been generated, so GetKeys() fails -> NO_KEYS.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"] != "NO_KEYS" {
		t.Fatalf("unexpected error payload: %+v", resp)
	}
}

func TestTorAdminHandler_ImportKeys(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/admin/tor/keys/import")
	h.ImportKeys(w, c)
	// No multipart file attached.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"] != "NO_FILE" {
		t.Fatalf("unexpected error payload: %+v", resp)
	}
}

func TestTorAdminHandler_ExportKeys(t *testing.T) {
	h := newTorTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/admin/tor/keys/export")
	h.ExportKeys(w, c)
	// No key files on disk yet.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok || errObj["code"] != "EXPORT_FAILED" {
		t.Fatalf("unexpected error payload: %+v", resp)
	}
}

// TestTorAdminHandler_NonHandlerMethods covers the plain (non-gin) wrapper
// methods used by non-HTTP callers (e.g. WebSocket admin push).
func TestTorAdminHandler_NonHandlerMethods(t *testing.T) {
	h := newTorTestHandler(t)

	if status := h.GetServiceStatus(); status == nil {
		t.Fatal("GetServiceStatus returned nil")
	}
	if health := h.GetServiceHealth(); health == nil {
		t.Fatal("GetServiceHealth returned nil")
	}
	if _, err := h.EnableService(8080); err == nil {
		t.Fatal("EnableService: expected error (no tor binary)")
	}
	if err := h.DisableService(); err != nil {
		t.Fatalf("DisableService: unexpected error: %v", err)
	}
	// RegenerateService's Start() short-circuits as a no-op success when
	// tor.enabled is false, so re-enable before expecting the no-tor-binary
	// failure path.
	if err := h.settingsModel.SetBool("tor.enabled", true); err != nil {
		t.Fatalf("failed to re-enable tor setting: %v", err)
	}
	if _, err := h.RegenerateService(8080); err == nil {
		t.Fatal("RegenerateService: expected error (no tor binary)")
	}
	if status := h.GetVanityGenerationStatus(); status != nil {
		t.Fatalf("expected nil status before Start, got %+v", status)
	}
	if err := h.StartVanityGeneration("ab"); err != nil {
		t.Fatalf("StartVanityGeneration: unexpected error: %v", err)
	}
	if err := h.CancelVanityGeneration(); err != nil {
		t.Fatalf("CancelVanityGeneration: unexpected error: %v", err)
	}
	if _, err := h.ApplyVanityKeys(8080); err == nil {
		t.Fatal("ApplyVanityKeys: expected error (no keys)")
	}
	if _, _, err := h.ExportTorKeys(); err == nil {
		t.Fatal("ExportTorKeys: expected error (no keys on disk)")
	}
	if _, err := h.ImportTorKeys([]byte("pub"), make([]byte, 32), 8080); err == nil {
		t.Fatal("ImportTorKeys: expected error (invalid key material propagates to RegenerateAddress failure)")
	}
}
