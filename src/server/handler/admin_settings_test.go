package handler

import (
	"database/sql"
	"net/http"
	"testing"
)

// newAdminSettingsTestHandler wires an AdminSettingsHandler against a fresh
// in-memory ServerSchema database, registered as the package-level global
// DB since AdminSettingsHandler methods call database.GetServerDB()
// directly rather than reading h.DB.
func newAdminSettingsTestHandler(t *testing.T) (*AdminSettingsHandler, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, serverDB, usersDB)
	return &AdminSettingsHandler{DB: serverDB}, serverDB
}

func seedServerConfigRow(t *testing.T, db *sql.DB, key, value, typ string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO server_config (key, value, type, description) VALUES (?, ?, ?, '')`,
		key, value, typ,
	); err != nil {
		t.Fatalf("seed server_config: %v", err)
	}
}

// TestAdminSettingsHandler_GetAllSettings covers the success path (rows
// returned, grouped into categories) and the DB-error path (query against a
// closed connection).
func TestAdminSettingsHandler_GetAllSettings(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, db := newAdminSettingsTestHandler(t)
		seedServerConfigRow(t, db, "smtp.host", "smtp.example.com", "string")
		seedServerConfigRow(t, db, "smtp.enabled", "true", "boolean")

		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings")
		h.GetAllSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("db error", func(t *testing.T) {
		h, db := newAdminSettingsTestHandler(t)
		db.Close()

		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings")
		h.GetAllSettings(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminSettingsHandler_UpdateSettings covers a successfully applied
// existing key, an unknown key reported as "failed" (not an HTTP error),
// and malformed JSON.
func TestAdminSettingsHandler_UpdateSettings(t *testing.T) {
	t.Run("success and unknown key", func(t *testing.T) {
		h, db := newAdminSettingsTestHandler(t)
		seedServerConfigRow(t, db, "smtp.host", "old.example.com", "string")

		body := map[string]interface{}{
			"settings": map[string]interface{}{
				"smtp.host":       "new.example.com",
				"does.not.exist":  "x",
			},
		}
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/settings", body)
		h.UpdateSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var got string
		if err := db.QueryRow("SELECT value FROM server_config WHERE key = ?", "smtp.host").Scan(&got); err != nil {
			t.Fatalf("query updated value: %v", err)
		}
		if got != "new.example.com" {
			t.Errorf("smtp.host = %q, want %q", got, "new.example.com")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		h, _ := newAdminSettingsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/server/admin/settings", "not json")
		h.UpdateSettings(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminSettingsHandler_ResetSettings covers the success path: all rows
// cleared then re-seeded with InitializeDefaults.
func TestAdminSettingsHandler_ResetSettings(t *testing.T) {
	h, db := newAdminSettingsTestHandler(t)
	seedServerConfigRow(t, db, "custom.key", "custom.value", "string")

	c, w := newTestContext(http.MethodPost, "/api/v1/server/admin/settings/reset")
	h.ResetSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM server_config WHERE key = ?", "custom.key").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("custom.key still present after reset, want removed")
	}
}

// TestAdminSettingsHandler_ExportSettings covers the success path,
// including the Content-Disposition header used for the downloadable file.
func TestAdminSettingsHandler_ExportSettings(t *testing.T) {
	h, db := newAdminSettingsTestHandler(t)
	seedServerConfigRow(t, db, "smtp.host", "smtp.example.com", "string")

	c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/settings/export")
	h.ExportSettings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Disposition"); got == "" {
		t.Errorf("Content-Disposition header not set")
	}
}

// TestAdminSettingsHandler_ImportSettings covers a successful import of an
// existing key and the empty-map edge case (imports zero, still 200).
func TestAdminSettingsHandler_ImportSettings(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, db := newAdminSettingsTestHandler(t)
		seedServerConfigRow(t, db, "smtp.host", "old.example.com", "string")

		body := map[string]interface{}{
			"settings": map[string]string{"smtp.host": "imported.example.com"},
		}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/settings/import", body)
		h.ImportSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("empty settings map", func(t *testing.T) {
		h, _ := newAdminSettingsTestHandler(t)
		body := map[string]interface{}{"settings": map[string]string{}}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/server/admin/settings/import", body)
		h.ImportSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminSettingsHandler_ReloadConfig covers the trivial always-200
// acknowledgement path.
func TestAdminSettingsHandler_ReloadConfig(t *testing.T) {
	h, _ := newAdminSettingsTestHandler(t)
	c, w := newTestContext(http.MethodPost, "/api/v1/server/admin/settings/reload")
	h.ReloadConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}
