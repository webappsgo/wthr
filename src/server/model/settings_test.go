package model

import (
	"testing"
)

// TestSettingsModel_GetSetFamily covers the server_config-backed Get/Set
// methods and their typed convenience wrappers, including the
// defaultValue fallback path for a missing key.
func TestSettingsModel_GetSetFamily(t *testing.T) {
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, nil)
	model := &SettingsModel{DB: serverDB}

	t.Run("Get missing key errors", func(t *testing.T) {
		if _, err := model.Get("missing.key"); err == nil {
			t.Error("Get() expected error for missing key")
		}
	})

	t.Run("Set and Get round trip", func(t *testing.T) {
		if err := model.Set("server.title", "Weather App", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		got, err := model.Get("server.title")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Value != "Weather App" {
			t.Errorf("Get() value = %q, want %q", got.Value, "Weather App")
		}
	})

	t.Run("Set upserts existing key", func(t *testing.T) {
		if err := model.Set("server.title", "First", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if err := model.Set("server.title", "Second", "string"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		got, err := model.Get("server.title")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Value != "Second" {
			t.Errorf("Get() value = %q, want %q", got.Value, "Second")
		}
	})

	t.Run("SetWithDescription persists description", func(t *testing.T) {
		if err := model.SetWithDescription("server.tagline", "Tag", "string", "A tagline"); err != nil {
			t.Fatalf("SetWithDescription() error = %v", err)
		}
		var description string
		if err := serverDB.QueryRow("SELECT description FROM server_config WHERE key = ?", "server.tagline").Scan(&description); err != nil {
			t.Fatalf("query description: %v", err)
		}
		if description != "A tagline" {
			t.Errorf("description = %q, want %q", description, "A tagline")
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
// the real server_config table from the production ServerSchema, reached
// through the global server-DB accessor.
func TestSettingsModel_ListDeleteFamily(t *testing.T) {
	db := newModelServerDB(t)
	setModelGlobalDualDB(t, db, nil)
	model := &SettingsModel{DB: db}

	t.Run("List on empty table", func(t *testing.T) {
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 0 {
			t.Errorf("List() = %d settings, want 0", len(list))
		}
	})

	seed := []struct{ key, value, typ string }{
		{"app.name", "Weather", "string"},
		{"app.port", "8080", "number"},
		{"mail.host", "smtp.example.com", "string"},
	}
	for _, s := range seed {
		if _, err := db.Exec("INSERT INTO server_config (key, value, type) VALUES (?, ?, ?)", s.key, s.value, s.typ); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}

	t.Run("List returns all ordered by key", func(t *testing.T) {
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("List() = %d settings, want 3", len(list))
		}
		if list[0].Key != "app.name" || list[1].Key != "app.port" {
			t.Errorf("List() not ordered by key: %+v", list)
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
		if err := model.Delete("mail.host"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		list, err := model.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 2 {
			t.Errorf("List() after delete = %d, want 2", len(list))
		}
	})

	t.Run("Delete non-existent is a no-op", func(t *testing.T) {
		if err := model.Delete("does.not.exist"); err != nil {
			t.Errorf("Delete() of missing key should not error, got %v", err)
		}
	})
}

// TestSettingsModel_InitializeDefaults verifies defaults are populated on
// first run, that a custom backup path is honored, and that re-running is
// idempotent (never overwrites a key a user already changed).
func TestSettingsModel_InitializeDefaults(t *testing.T) {
	serverDB := newModelServerDB(t)
	setModelGlobalDualDB(t, serverDB, nil)
	model := &SettingsModel{DB: serverDB}

	if err := model.InitializeDefaults("/custom/backups"); err != nil {
		t.Fatalf("InitializeDefaults() error = %v", err)
	}

	got, err := model.Get("backup.location")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Value != "/custom/backups" {
		t.Errorf("backup.location = %q, want %q", got.Value, "/custom/backups")
	}

	title, err := model.Get("server.title")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if title.Value != "Weather" {
		t.Errorf("server.title = %q, want %q", title.Value, "Weather")
	}

	t.Run("does not overwrite user-modified values", func(t *testing.T) {
		if err := model.SetString("server.title", "My Custom Title"); err != nil {
			t.Fatalf("SetString() error = %v", err)
		}
		if err := model.InitializeDefaults("/custom/backups"); err != nil {
			t.Fatalf("InitializeDefaults() second run error = %v", err)
		}
		got, err := model.Get("server.title")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.Value != "My Custom Title" {
			t.Errorf("server.title = %q, want unchanged %q", got.Value, "My Custom Title")
		}
	})
}
