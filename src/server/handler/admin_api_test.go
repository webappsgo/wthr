package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// adminAPIContextWithDB wires a gin context with a real *sql.DB stashed in
// the "db" context key, mirroring how the production router injects it
// (see src/main.go db middleware).
func adminAPIContextWithDB(t *testing.T, method, target string, body interface{}) (*sql.DB, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	db := newTestServerDB(t)
	// SettingsModel and several admin_api.go helpers read through the
	// package-level database.GetServerDB() accessor rather than the
	// struct's injected DB field, so the global dual-DB must be wired for
	// these tests to hit the same in-memory database the context carries.
	setGlobalTestDualDB(t, db, db)
	var c *gin.Context
	var w *httptest.ResponseRecorder
	if body != nil {
		c, w = newTestContextJSON(t, method, target, body)
	} else {
		c, w = newTestContext(method, target)
	}
	c.Set("db", db)
	return db, c, w
}

func TestSaveWebSettings(t *testing.T) {
	t.Run("saves mixed-type settings", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/web", map[string]interface{}{
			"site.title":   "My Weather",
			"site.port":    float64(8080),
			"site.enabled": true,
			"site.extra":   map[string]interface{}{"a": 1},
		})
		SaveWebSettings(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/web", "{bad")
		SaveWebSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/web", map[string]interface{}{"a": "b"})
		SaveWebSettings(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("db error returns 500", func(t *testing.T) {
		db, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/web", map[string]interface{}{"a": "b"})
		if _, err := db.Exec("DROP TABLE server_config"); err != nil {
			t.Fatalf("drop table: %v", err)
		}
		SaveWebSettings(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestSaveSecuritySettings(t *testing.T) {
	t.Run("saves settings", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/security", map[string]interface{}{
			"security.mfa": true,
		})
		SaveSecuritySettings(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/security", "{bad")
		SaveSecuritySettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/security", map[string]interface{}{"a": "b"})
		SaveSecuritySettings(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestTestDatabaseConnection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodGet, "/admin/db/test", nil)
		TestDatabaseConnection(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/admin/db/test")
		TestDatabaseConnection(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("closed db fails ping", func(t *testing.T) {
		db, c, w := adminAPIContextWithDB(t, http.MethodGet, "/admin/db/test", nil)
		db.Close()
		TestDatabaseConnection(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestOptimizeDatabase(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/db/optimize", nil)
		OptimizeDatabase(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/admin/db/optimize")
		OptimizeDatabase(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("closed db returns 500", func(t *testing.T) {
		db, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/db/optimize", nil)
		db.Close()
		OptimizeDatabase(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestVacuumDatabase(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/db/vacuum", nil)
		VacuumDatabase(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/admin/db/vacuum")
		VacuumDatabase(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("closed db returns 500", func(t *testing.T) {
		db, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/db/vacuum", nil)
		db.Close()
		VacuumDatabase(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestClearCache(t *testing.T) {
	t.Run("no cache in context still succeeds", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/admin/cache/clear")
		ClearCache(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong type in context still succeeds (not enabled)", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/admin/cache/clear")
		c.Set("cache", "not-a-cache-manager")
		ClearCache(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestCreateBackup(t *testing.T) {
	t.Run("success with empty dirs", func(t *testing.T) {
		configDir := t.TempDir()
		dataDir := t.TempDir()
		t.Setenv("CONFIG_DIR", configDir)
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodPost, "/admin/backup/create")
		CreateBackup(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestRestoreBackup(t *testing.T) {
	t.Run("no file returns 400", func(t *testing.T) {
		c, w := newTestContext(http.MethodPost, "/admin/backup/restore")
		RestoreBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestListBackups(t *testing.T) {
	t.Run("empty dir returns empty list", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodGet, "/admin/backup/list")
		ListBackups(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("lists only .gz files", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		backupDir := filepath.Join(dataDir, "backups")
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(backupDir, "one.tar.gz"), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.WriteFile(filepath.Join(backupDir, "ignore.txt"), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		c, w := newTestContext(http.MethodGet, "/admin/backup/list")
		ListBackups(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var backups []BackupFile
		if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(backups) != 1 || backups[0].Filename != "one.tar.gz" {
			t.Fatalf("unexpected backups: %+v", backups)
		}
	})
}

func TestDownloadBackup(t *testing.T) {
	t.Run("not found returns 404", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodGet, "/admin/backup/download/missing.tar.gz")
		c.Params = []gin.Param{{Key: "filename", Value: "missing.tar.gz"}}
		DownloadBackup(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodGet, "/admin/backup/download/..%2F..%2Fetc%2Fpasswd")
		c.Params = []gin.Param{{Key: "filename", Value: "../../etc/passwd"}}
		DownloadBackup(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("existing file downloads", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		backupDir := filepath.Join(dataDir, "backups")
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(backupDir, "one.tar.gz"), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		c, w := newTestContext(http.MethodGet, "/admin/backup/download/one.tar.gz")
		c.Params = []gin.Param{{Key: "filename", Value: "one.tar.gz"}}
		DownloadBackup(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestDeleteBackup(t *testing.T) {
	t.Run("not found returns 404", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodDelete, "/admin/backup/missing.tar.gz")
		c.Params = []gin.Param{{Key: "filename", Value: "missing.tar.gz"}}
		DeleteBackup(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)

		c, w := newTestContext(http.MethodDelete, "/admin/backup/traverse")
		c.Params = []gin.Param{{Key: "filename", Value: "../../etc/passwd"}}
		DeleteBackup(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("existing file deletes", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		backupDir := filepath.Join(dataDir, "backups")
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		target := filepath.Join(backupDir, "one.tar.gz")
		if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		c, w := newTestContext(http.MethodDelete, "/admin/backup/one.tar.gz")
		c.Params = []gin.Param{{Key: "filename", Value: "one.tar.gz"}}
		DeleteBackup(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("expected file removed, stat err = %v", err)
		}
	})
}

func TestSaveDatabaseSettings(t *testing.T) {
	t.Run("invalid JSON returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/database", "{bad")
		SaveDatabaseSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing db context returns 500", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "sqlite",
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid driver returns 400", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "oracle",
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid port returns 400", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "postgres",
			"database.port":   float64(70000),
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("remote db missing required fields returns 400", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "postgres",
			"database.port":   float64(5432),
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("sqlite driver saves without remote validation", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "sqlite",
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid remote config saves", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/settings/database", map[string]interface{}{
			"database.driver": "postgres",
			"database.port":   float64(5432),
			"database.host":   "localhost",
			"database.name":   "wthr",
		})
		SaveDatabaseSettings(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestTestDatabaseConfigConnection(t *testing.T) {
	t.Run("invalid JSON returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", "{bad")
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("sqlite driver always valid", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", map[string]interface{}{
			"driver": "sqlite",
		})
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing host returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", map[string]interface{}{
			"driver": "postgres",
		})
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid port returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", map[string]interface{}{
			"driver": "postgres",
			"host":   "localhost",
			"port":   99999,
		})
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", map[string]interface{}{
			"driver": "postgres",
			"host":   "localhost",
			"port":   5432,
		})
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid remote config", func(t *testing.T) {
		c, w := newTestContextJSON(t, http.MethodPost, "/admin/db/test-config", map[string]interface{}{
			"driver": "postgres",
			"host":   "localhost",
			"port":   5432,
			"name":   "wthr",
		})
		TestDatabaseConfigConnection(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}
