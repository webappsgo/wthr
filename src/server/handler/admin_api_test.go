package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// newBackupFormContext builds a gin context carrying a form-encoded body, the
// shape the admin backup page posts to CreateBackup and RestoreBackup.
func newBackupFormContext(method, target string, fields url.Values) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(fields.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c, w
}

// writeTestBackup creates a backup directory entry under dataDir/backups and
// returns its absolute path.
func writeTestBackup(t *testing.T, dataDir, name string) string {
	t.Helper()
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(backupDir, name)
	if err := os.WriteFile(target, []byte("x"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return target
}

func TestCreateBackup(t *testing.T) {
	t.Run("success with empty dirs", func(t *testing.T) {
		configDir := t.TempDir()
		dataDir := t.TempDir()
		t.Setenv("CONFIG_DIR", configDir)
		t.Setenv("DATA_DIR", dataDir)

		c, w := newBackupFormContext(http.MethodPost, "/admin/config/backup", url.Values{})
		CreateBackup(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestRestoreBackup(t *testing.T) {
	t.Run("no file and no filename returns 400", func(t *testing.T) {
		c, w := newBackupFormContext(http.MethodPost, "/admin/config/backup/restore", url.Values{})
		RestoreBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("filename outside the backup directory returns 400", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newBackupFormContext(http.MethodPost, "/admin/config/backup/restore", url.Values{
			"filename": {"../../etc/passwd.tar.gz"},
		})
		RestoreBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown filename returns 404", func(t *testing.T) {
		t.Setenv("CONFIG_DIR", t.TempDir())
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newBackupFormContext(http.MethodPost, "/admin/config/backup/restore", url.Values{
			"filename": {"missing.tar.gz"},
		})
		RestoreBackup(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestListBackups(t *testing.T) {
	t.Run("missing dir returns empty list", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newTestContext(http.MethodGet, "/admin/config/backup")
		ListBackups(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var backups []BackupFile
		if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(backups) != 0 {
			t.Fatalf("expected no backups, got %+v", backups)
		}
	})

	t.Run("lists archives including encrypted ones", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		writeTestBackup(t, dataDir, "one.tar.gz")
		writeTestBackup(t, dataDir, "two.tar.gz.enc")
		writeTestBackup(t, dataDir, "ignore.txt")

		c, w := newTestContext(http.MethodGet, "/admin/config/backup")
		ListBackups(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var backups []BackupFile
		if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(backups) != 2 {
			t.Fatalf("expected 2 archives, got %+v", backups)
		}
		listed := map[string]bool{}
		for _, item := range backups {
			listed[item.Filename] = true
		}
		if !listed["one.tar.gz"] || !listed["two.tar.gz.enc"] {
			t.Fatalf("unexpected backups: %+v", backups)
		}
	})
}

func TestBackupStats(t *testing.T) {
	t.Run("counts archives and their total size", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		writeTestBackup(t, dataDir, "one.tar.gz")
		writeTestBackup(t, dataDir, "two.tar.gz.enc")

		c, w := newTestContext(http.MethodGet, "/admin/config/backup/stats")
		BackupStats(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var stats struct {
			Count     int    `json:"count"`
			TotalSize int64  `json:"total_size"`
			Directory string `json:"directory"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if stats.Count != 2 || stats.TotalSize != 2 {
			t.Fatalf("unexpected stats: %+v", stats)
		}
		if stats.Directory != filepath.Join(dataDir, "backups") {
			t.Fatalf("directory = %q, want the data-dir backups path", stats.Directory)
		}
	})
}

func TestDownloadBackup(t *testing.T) {
	t.Run("not found returns 404", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newTestContext(http.MethodGet, "/admin/config/backup/missing.tar.gz/download")
		c.Params = []gin.Param{{Key: "filename", Value: "missing.tar.gz"}}
		DownloadBackup(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newTestContext(http.MethodGet, "/admin/config/backup/traverse/download")
		c.Params = []gin.Param{{Key: "filename", Value: "../../etc/passwd.tar.gz"}}
		DownloadBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("non-archive name rejected", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		writeTestBackup(t, dataDir, "server.yml")

		c, w := newTestContext(http.MethodGet, "/admin/config/backup/server.yml/download")
		c.Params = []gin.Param{{Key: "filename", Value: "server.yml"}}
		DownloadBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("existing file downloads", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		writeTestBackup(t, dataDir, "one.tar.gz")

		c, w := newTestContext(http.MethodGet, "/admin/config/backup/one.tar.gz/download")
		c.Params = []gin.Param{{Key: "filename", Value: "one.tar.gz"}}
		DownloadBackup(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestDeleteBackup(t *testing.T) {
	t.Run("not found returns 404", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newTestContext(http.MethodDelete, "/admin/config/backup/missing.tar.gz")
		c.Params = []gin.Param{{Key: "filename", Value: "missing.tar.gz"}}
		DeleteBackup(c)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		t.Setenv("DATA_DIR", t.TempDir())

		c, w := newTestContext(http.MethodDelete, "/admin/config/backup/traverse")
		c.Params = []gin.Param{{Key: "filename", Value: "../../etc/passwd.tar.gz"}}
		DeleteBackup(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("existing file deletes", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		target := writeTestBackup(t, dataDir, "one.tar.gz")

		c, w := newTestContext(http.MethodDelete, "/admin/config/backup/one.tar.gz")
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

func TestBackupSchedule(t *testing.T) {
	t.Run("returns defaults when nothing is stored", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodGet, "/admin/config/backup/schedule", nil)
		GetBackupSchedule(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var schedule struct {
			Enabled   bool `json:"enabled"`
			Interval  int  `json:"interval"`
			Retention struct {
				MaxBackups   int    `json:"max_backups"`
				KeepWeekly   int    `json:"keep_weekly"`
				KeepMonthly  int    `json:"keep_monthly"`
				KeepYearly   int    `json:"keep_yearly"`
				MaxTotalSize string `json:"max_total_size"`
			} `json:"retention"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &schedule); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !schedule.Enabled || schedule.Interval != 6 || schedule.Retention.MaxBackups != 1 {
			t.Fatalf("unexpected defaults: %+v", schedule)
		}
	})

	t.Run("saves and reads back", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/config/backup/schedule", map[string]interface{}{
			"enabled":        false,
			"interval":       12,
			"max_backups":    5,
			"keep_weekly":    2,
			"keep_monthly":   3,
			"keep_yearly":    1,
			"max_total_size": "20%",
		})
		SaveBackupSchedule(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		readCtx, readRec := newTestContext(http.MethodGet, "/admin/config/backup/schedule")
		readCtx.Set("db", c.MustGet("db"))
		GetBackupSchedule(readCtx)
		var schedule struct {
			Enabled   bool `json:"enabled"`
			Interval  int  `json:"interval"`
			Retention struct {
				MaxBackups   int    `json:"max_backups"`
				KeepWeekly   int    `json:"keep_weekly"`
				KeepMonthly  int    `json:"keep_monthly"`
				KeepYearly   int    `json:"keep_yearly"`
				MaxTotalSize string `json:"max_total_size"`
			} `json:"retention"`
		}
		if err := json.Unmarshal(readRec.Body.Bytes(), &schedule); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if schedule.Enabled || schedule.Interval != 12 || schedule.Retention.MaxBackups != 5 ||
			schedule.Retention.KeepWeekly != 2 || schedule.Retention.KeepMonthly != 3 ||
			schedule.Retention.KeepYearly != 1 || schedule.Retention.MaxTotalSize != "20%" {
			t.Fatalf("unexpected saved schedule: %+v", schedule)
		}
	})

	t.Run("rejects out-of-range interval", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/config/backup/schedule", map[string]interface{}{
			"interval": 0,
		})
		SaveBackupSchedule(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects out-of-range max_backups", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/config/backup/schedule", map[string]interface{}{
			"max_backups": 0,
		})
		SaveBackupSchedule(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects negative keep_weekly", func(t *testing.T) {
		_, c, w := adminAPIContextWithDB(t, http.MethodPost, "/admin/config/backup/schedule", map[string]interface{}{
			"keep_weekly": -1,
		})
		SaveBackupSchedule(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
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
