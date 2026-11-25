package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setSettingsTestConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestSettingsModel_GetSetFamily covers the server_config-backed Get/Set
// methods and their typed convenience wrappers, including the
// defaultValue fallback path for a missing key.
func TestSettingsModel_GetSetFamily(t *testing.T) {
	setSettingsTestConfig(t)
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, nil)
	model := &SettingsModel{testDB: serverDB}

	t.Run("Get missing key errors", func(t *testing.T) {
		if _, err := model.GetSetting("missing.key"); err == nil {
			t.Error("Get() expected error for missing key")
		}
	})

	t.Run("Set and Get round trip", func(t *testing.T) {
		if err := model.SetSetting("server.title", "Weather App", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		got, err := model.GetSetting("server.title")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Value != "Weather App" {
			t.Errorf("Get() value = %q, want %q", got.Value, "Weather App")
		}
	})

	t.Run("Set upserts existing key", func(t *testing.T) {
		if err := model.SetSetting("server.title", "First", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := model.SetSetting("server.title", "Second", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		got, err := model.GetSetting("server.title")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Value != "Second" {
			t.Errorf("Get() value = %q, want %q", got.Value, "Second")
		}
	})

	t.Run("SetWithDescription writes the setting value", func(t *testing.T) {
		if err := model.SetWithDescription("server.tagline", "Tag", "string", "A tagline"); err != nil {
			t.Fatalf("SetWithDescription() error = %v", err)
		}
		got := model.GetString("server.tagline", "")
		if got != "Tag" {
			t.Errorf("setting value = %q, want Tag", got)
		}
	})

	t.Run("GetString", func(t *testing.T) {
		if err := model.SetString("k.str", "hello"); err != nil {
			t.Fatalf("SetString() error = %v", err)
		}
		if got := model.GetString("k.str", "fallback"); got != "hello" {
			t.Errorf("GetString() = %q, want %q", got, "hello")
		}
		if got := model.GetString("k.missing", "fallback"); got != "fallback" {
			t.Errorf("GetString() missing = %q, want %q", got, "fallback")
		}
	})

	t.Run("GetInt", func(t *testing.T) {
		if err := model.SetInt("k.int", 42); err != nil {
			t.Fatalf("SetInt() error = %v", err)
		}
		if got := model.GetInt("k.int", -1); got != 42 {
			t.Errorf("GetInt() = %d, want 42", got)
		}
		if got := model.GetInt("k.missing", -1); got != -1 {
			t.Errorf("GetInt() missing = %d, want -1", got)
		}
	})

	t.Run("GetInt with unparsable value returns default", func(t *testing.T) {
		if err := model.SetString("k.badint", "not-a-number"); err != nil {
			t.Fatalf("SetString() error = %v", err)
		}
		if got := model.GetInt("k.badint", 99); got != 99 {
			t.Errorf("GetInt() with bad value = %d, want 99", got)
		}
	})

	t.Run("GetBool", func(t *testing.T) {
		if err := model.SetBool("k.bool", true); err != nil {
			t.Fatalf("SetBool() error = %v", err)
		}
		if got := model.GetBool("k.bool", false); !got {
			t.Error("GetBool() = false, want true")
		}
		if err := model.SetBool("k.bool", false); err != nil {
			t.Fatalf("SetBool() error = %v", err)
		}
		if got := model.GetBool("k.bool", true); got {
			t.Error("GetBool() = true, want false")
		}
		if got := model.GetBool("k.missing", true); !got {
			t.Error("GetBool() missing = false, want default true")
		}
	})

	t.Run("GetJSON and SetJSON", func(t *testing.T) {
		type payload struct {
			Name string `json:"name"`
			N    int    `json:"n"`
		}
		want := payload{Name: "test", N: 7}
		if err := model.SetJSON("k.json", want); err != nil {
			t.Fatalf("SetJSON() error = %v", err)
		}
		var got payload
		if err := model.GetJSON("k.json", &got); err != nil {
			t.Fatalf("GetJSON() error = %v", err)
		}
		if got != want {
			t.Errorf("GetJSON() = %+v, want %+v", got, want)
		}
	})

	t.Run("GetJSON missing key errors", func(t *testing.T) {
		var dest map[string]string
		if err := model.GetJSON("k.missing", &dest); err == nil {
			t.Error("GetJSON() expected error for missing key")
		}
	})
}

// TestSettingsModel_ListDeleteFamily covers Delete/List/ListByPrefix against
// the canonical configuration files while ignoring unrelated typed defaults.
func TestSettingsModel_ListDeleteFamily(t *testing.T) {
	setSettingsTestConfig(t)
	db := newModelServerDB(t)
	setModelGlobalDualDB(t, db, nil)
	model := &SettingsModel{testDB: db}

	t.Run("List on empty custom config", func(t *testing.T) {
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		for _, setting := range list {
			if strings.HasPrefix(setting.Key, "app.") || setting.Key == "mail.host" {
				t.Errorf("List() contains unseeded custom setting %q", setting.Key)
			}
		}
	})

	seed := []struct{ key, value, typ string }{
		{"app.name", "Weather", "string"},
		{"app.port", "8080", "number"},
		{"mail.host", "smtp.example.com", "string"},
	}
	for _, s := range seed {
		if err := model.SetSetting(s.key, s.value, s.typ); err != nil {
			t.Fatalf("seed setting: %v", err)
		}
	}

	t.Run("List returns all ordered by key", func(t *testing.T) {
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		positions := map[string]int{}
		for i, setting := range list {
			if setting.Key == "app.name" || setting.Key == "app.port" || setting.Key == "mail.host" {
				positions[setting.Key] = i
			}
		}
		if len(positions) != 3 {
			t.Fatalf("List() did not return all custom settings: %+v", positions)
		}
		if positions["app.name"] >= positions["app.port"] || positions["app.port"] >= positions["mail.host"] {
			t.Errorf("List() custom settings are not ordered by key: %+v", positions)
		}
	})

	t.Run("ListByPrefix", func(t *testing.T) {
		list, err := model.ListByPrefix("app.")
		if err != nil {
			t.Fatalf("ListByPrefix() error = %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ListByPrefix() = %d settings, want 2", len(list))
		}
	})

	t.Run("ListByPrefix no matches", func(t *testing.T) {
		list, err := model.ListByPrefix("nonexistent.")
		if err != nil {
			t.Fatalf("ListByPrefix() error = %v", err)
		}
		if len(list) != 0 {
			t.Errorf("ListByPrefix() = %d settings, want 0", len(list))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := model.DeleteSetting("mail.host"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		for _, setting := range list {
			if setting.Key == "mail.host" {
				t.Errorf("List() after delete still contains mail.host")
			}
		}
	})

	t.Run("Delete non-existent is a no-op", func(t *testing.T) {
		if err := model.DeleteSetting("does.not.exist"); err != nil {
			t.Errorf("Delete() of missing key should not error, got %v", err)
		}
	})
}

// TestSettingsModel_NilDBUsesFiles proves the compatibility DB field is not required.
func TestSettingsModel_NilDBFallsBackToGlobal(t *testing.T) {
	setSettingsTestConfig(t)
	global := newModelServerDB(t)
	setModelGlobalDualDB(t, global, nil)
	model := &SettingsModel{}

	if err := model.SetString("server.title", "Fallback Title"); err != nil {
		t.Fatalf("SetString() error = %v", err)
	}
	if got := model.GetString("server.title", "Weather"); got != "Fallback Title" {
		t.Errorf("GetString() = %q, want %q", got, "Fallback Title")
	}
	if got := model.GetString("missing.key", "fallback"); got != "fallback" {
		t.Errorf("GetString() default = %q, want %q", got, "fallback")
	}
	if _, err := model.List(); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if _, err := model.ListByPrefix("server."); err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if err := model.DeleteSetting("server.title"); err != nil {
		t.Fatalf("DeleteSetting() error = %v", err)
	}
}
