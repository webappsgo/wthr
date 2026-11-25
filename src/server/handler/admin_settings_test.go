package handler

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/model"
)

func newAdminSettingsTestHandler(t *testing.T) (*AdminSettingsHandler, *sql.DB) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &AdminSettingsHandler{DB: storeForDB(serverDB)}, serverDB
}

func TestAdminSettingsHandler_GetAllSettings(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	if err := (&model.SettingsModel{}).SetString("smtp.host", "smtp.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := (&model.SettingsModel{}).SetBool("smtp.enabled", true); err != nil {
		t.Fatal(err)
	}
	c, w := newTestContext("GET", "/api/v1/server/administration/settings")
	h.GetAllSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminSettingsHandler_UpdateSettings(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	body := map[string]any{"settings": map[string]any{"smtp.host": "new.example.com", "new.setting": true}}
	c, w := newTestContextJSON(t, "PUT", "/api/v1/server/administration/settings", body)
	h.UpdateSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := (&model.SettingsModel{}).GetString("smtp.host", ""); got != "new.example.com" {
		t.Errorf("smtp.host = %q, want new value", got)
	}
	if got := (&model.SettingsModel{}).GetBool("new.setting", false); !got {
		t.Error("new.setting was not written as a boolean")
	}
}

func TestAdminSettingsHandler_UpdateSettingsRejectsMalformedJSON(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	c, w := newTestContextJSON(t, "PUT", "/api/v1/server/administration/settings", "not json")
	h.UpdateSettings(w, c)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminSettingsHandler_ResetSettings(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	settings := &model.SettingsModel{}
	if err := settings.SetString("custom.key", "custom.value"); err != nil {
		t.Fatal(err)
	}
	c, w := newTestContext("POST", "/api/v1/server/administration/settings/reset")
	h.ResetSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if _, err := settings.GetSetting("custom.key"); err == nil {
		t.Error("custom setting remains after reset")
	}
}

func TestAdminSettingsHandler_ExportSettings(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	if err := (&model.SettingsModel{}).SetString("smtp.host", "smtp.example.com"); err != nil {
		t.Fatal(err)
	}
	c, w := newTestContext("GET", "/api/v1/server/administration/settings/export")
	h.ExportSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Disposition") == "" {
		t.Error("Content-Disposition header not set")
	}
}

func TestAdminSettingsHandler_ImportSettings(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	body := map[string]any{"settings": map[string]any{"smtp.host": "imported.example.com", "smtp.enabled": true}}
	c, w := newTestContextJSON(t, "POST", "/api/v1/server/administration/settings/import", body)
	h.ImportSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := (&model.SettingsModel{}).GetString("smtp.host", ""); got != "imported.example.com" {
		t.Errorf("smtp.host = %q, want imported value", got)
	}
}

func TestAdminSettingsHandler_ReloadConfig(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	c, w := newTestContext("POST", "/api/v1/server/administration/settings/reload")
	h.ReloadConfig(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminSettingsHandler_ResetUsesTypedDefaults(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	settings := &model.SettingsModel{}
	if err := settings.SetString("server.branding.title", "custom"); err != nil {
		t.Fatal(err)
	}
	c, w := newTestContext("POST", "/api/v1/server/administration/settings/reset")
	h.ResetSettings(w, c)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	cfg, err := config.CurrentConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Branding.Title == "custom" {
		t.Error("reset retained a custom branding title")
	}
}
