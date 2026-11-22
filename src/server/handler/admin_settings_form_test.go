package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/model"
)

func seedSettings(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := newTestServerDB(t)
	setGlobalTestDualDB(t, db, db)
	settings := &model.SettingsModel{DB: db}
	for _, row := range [][]string{
		{"demo.title", "Old", "string"},
		{"demo.count", "5", "number"},
		{"demo.enabled", "false", "boolean"},
		{"demo.api_password", "keepme", "string"},
		{"other.title", "Untouched", "string"},
	} {
		if err := settings.SetSetting(row[0], row[1], row[2]); err != nil {
			t.Fatalf("seed %s: %v", row[0], err)
		}
	}
}

func readSetting(t *testing.T, key string) string {
	t.Helper()
	rows, err := loadSettingsRows(t.Context())
	if err != nil {
		t.Fatalf("load rows: %v", err)
	}
	for _, row := range rows {
		if row.key == key {
			return row.value
		}
	}
	t.Fatalf("setting %s missing", key)
	return ""
}

func postSettings(form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/server/administration/settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	(&AdminHandler{}).SaveSettingsForm(w, r)
	return w
}

func TestSaveSettingsFormAppliesOnlyPostedSection(t *testing.T) {
	seedSettings(t)
	w := postSettings(url.Values{
		"section":           {"demo"},
		"demo.title":        {"  New  "},
		"demo.count":        {"7"},
		"demo.enabled":      {"false", "true"},
		"demo.api_password": {""},
		"other.title":       {"Hijacked"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/administration/settings?section=demo" {
		t.Fatalf("location = %q", loc)
	}
	for key, want := range map[string]string{
		"demo.title":        "New",
		"demo.count":        "7",
		"demo.enabled":      "true",
		"demo.api_password": "keepme",
		"other.title":       "Untouched",
	} {
		if got := readSetting(t, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if value, ok := flashCookieValue(t, w); !ok || value != "success:flash.settings_saved" {
		t.Errorf("flash = %q, %v", value, ok)
	}
}

func TestSaveSettingsFormRejectsInvalidNumber(t *testing.T) {
	seedSettings(t)
	w := postSettings(url.Values{"section": {"demo"}, "demo.count": {"abc"}, "demo.title": {"Changed"}})
	if value, ok := flashCookieValue(t, w); !ok || value != "error:flash.settings_save_failed" {
		t.Errorf("flash = %q, %v", value, ok)
	}
	if got := readSetting(t, "demo.title"); got != "Old" {
		t.Errorf("a rejected save must not apply anything, title = %q", got)
	}
}

func TestSettingsFieldControls(t *testing.T) {
	for _, tt := range []struct {
		row     settingsRow
		control string
	}{
		{settingsRow{key: "a.on", typ: "boolean", value: "true"}, "toggle"},
		{settingsRow{key: "a.n", typ: "number", value: "1"}, "number"},
		{settingsRow{key: "smtp.password", typ: "string", value: "x"}, "password"},
		{settingsRow{key: "server.mode", typ: "string", value: "production"}, "select"},
		{settingsRow{key: "a.j", typ: "json", value: "{}"}, "textarea"},
		{settingsRow{key: "a.t", typ: "string", value: "x"}, "text"},
	} {
		field := settingsFieldFor(tt.row)
		if field.Control != tt.control {
			t.Errorf("%s control = %q, want %q", tt.row.key, field.Control, tt.control)
		}
		if field.Secret && field.Value != "" {
			t.Errorf("%s secret value leaked into the form", tt.row.key)
		}
	}
	if got := humanizeSettingKey("smtp.from_address"); got != "From address" {
		t.Errorf("humanize = %q", got)
	}
}
