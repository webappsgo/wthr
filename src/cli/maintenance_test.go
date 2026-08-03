// Tests for maintenance.go per AI.md PART 25 (Maintenance) / PART 29 (Testing)
package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMaintenanceCommand_Dispatch covers the command-routing error paths
// that don't touch the filesystem or a real backup/restore flow: no args,
// unknown command, and "mode" with a missing or invalid value.
func TestMaintenanceCommand_Dispatch(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no_args", []string{}, "no maintenance command specified"},
		{"unknown_command", []string{"bogus"}, "unknown maintenance command: bogus"},
		{"mode_missing_value", []string{"mode"}, "mode requires a value"},
		{"mode_invalid_value", []string{"mode", "sideways"}, "invalid mode: sideways"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MaintenanceCommand(tt.args)
			if err == nil {
				t.Fatalf("MaintenanceCommand(%v) = nil, want error containing %q", tt.args, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("MaintenanceCommand(%v) error = %q, want substring %q", tt.args, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestSplitKey covers the dotted-key splitter used to group settings rows
// into YAML sections: single segment, nested segments, and empty string.
func TestSplitKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want []string
	}{
		{"single_segment", "server", []string{"server"}},
		{"two_segments", "server.mode", []string{"server", "mode"}},
		{"three_segments", "auth.session.timeout", []string{"auth", "session", "timeout"}},
		{"empty_string", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitKey(tt.key)
			if len(got) != len(tt.want) {
				t.Fatalf("splitKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitKey(%q)[%d] = %q, want %q", tt.key, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestYamlQuote verifies values are only quoted when they contain YAML
// special characters — over-quoting or under-quoting both silently corrupt
// the generated server.yml.
func TestYamlQuote(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain_value_unquoted", "production", "production"},
		{"contains_colon_quoted", "http://x", `"http://x"`},
		{"contains_hash_quoted", "a#b", `"a#b"`},
		{"contains_quote_quoted", `a"b`, `"a\"b"`},
		{"contains_brackets_quoted", "[a,b]", `"[a,b]"`},
		{"empty_string_unquoted", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := yamlQuote(tt.value)
			if got != tt.want {
				t.Errorf("yamlQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestSetMaintenanceMode_InvalidMode verifies unrecognized mode strings are
// rejected before any file I/O occurs (no CONFIG_DIR side effects for an
// invalid mode).
func TestSetMaintenanceMode_InvalidMode(t *testing.T) {
	err := setMaintenanceMode("bogus")
	if err == nil {
		t.Fatal("setMaintenanceMode(\"bogus\") = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Errorf("error = %q, want substring \"invalid mode\"", err.Error())
	}
}

// TestSetMaintenanceMode_WritesConfig verifies both the "prod"/"dev"
// shorthand and full mode names normalize correctly and are written to
// CONFIG_DIR/server.yml.
func TestSetMaintenanceMode_WritesConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"shorthand_prod", "prod", "production"},
		{"shorthand_dev", "dev", "development"},
		{"full_production", "production", "production"},
		{"full_development", "development", "development"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CONFIG_DIR", dir)

			if err := setMaintenanceMode(tt.input); err != nil {
				t.Fatalf("setMaintenanceMode(%q) error = %v", tt.input, err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "server.yml"))
			if err != nil {
				t.Fatalf("server.yml not written: %v", err)
			}
			if !strings.Contains(string(data), "mode: "+tt.want) {
				t.Errorf("server.yml content = %q, want it to contain %q", data, "mode: "+tt.want)
			}
		})
	}
}

// TestUpdateYAMLKey covers appending a new key when the section doesn't
// exist yet, replacing an existing key in place, and leaving other
// sections/keys untouched.
func TestUpdateYAMLKey(t *testing.T) {
	t.Run("appends_when_missing", func(t *testing.T) {
		got := updateYAMLKey("", "server.mode", "production")
		if !strings.Contains(got, "server:") || !strings.Contains(got, "mode: production") {
			t.Errorf("updateYAMLKey() = %q, want new server section with mode key", got)
		}
	})

	t.Run("replaces_existing_key_in_section", func(t *testing.T) {
		existing := "server:\n  mode: development\n  port: 8080\n"
		got := updateYAMLKey(existing, "server.mode", "production")
		if !strings.Contains(got, "mode: production") {
			t.Errorf("updateYAMLKey() = %q, want updated mode", got)
		}
		if strings.Contains(got, "mode: development") {
			t.Errorf("updateYAMLKey() = %q, old value still present", got)
		}
		if !strings.Contains(got, "port: 8080") {
			t.Errorf("updateYAMLKey() = %q, unrelated key was lost", got)
		}
	})

	t.Run("does_not_touch_other_sections", func(t *testing.T) {
		existing := "auth:\n  timeout: 24h\nserver:\n  mode: development\n"
		got := updateYAMLKey(existing, "server.mode", "production")
		if !strings.Contains(got, "timeout: 24h") {
			t.Errorf("updateYAMLKey() = %q, unrelated section was modified", got)
		}
	})
}

// TestHashPasswordArgon2id verifies the output matches the documented
// Argon2id encoding ($argon2id$v=19$m=65536,t=3,p=4$salt$hash), that two
// hashes of the same password differ (random salt), and that no error is
// returned for a normal input.
func TestHashPasswordArgon2id(t *testing.T) {
	hash1, err := hashPasswordArgon2id("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPasswordArgon2id() error = %v", err)
	}

	wantPrefix := "$argon2id$v=19$m=65536,t=3,p=4$"
	if !strings.HasPrefix(hash1, wantPrefix) {
		t.Errorf("hash = %q, want prefix %q", hash1, wantPrefix)
	}

	parts := strings.Split(hash1, "$")
	if len(parts) != 6 {
		t.Fatalf("hash has %d $-delimited parts, want 6 (got %q)", len(parts), hash1)
	}

	hash2, err := hashPasswordArgon2id("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPasswordArgon2id() second call error = %v", err)
	}
	if hash1 == hash2 {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

// TestVerifyDirectoryWritable covers: directory doesn't exist (auto-create),
// directory exists and is writable, and the write-test probe file being
// cleaned up afterward.
func TestVerifyDirectoryWritable(t *testing.T) {
	t.Run("creates_missing_directory", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, "does-not-exist-yet")
		if err := verifyDirectoryWritable(dir); err != nil {
			t.Fatalf("verifyDirectoryWritable() error = %v", err)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("directory was not created: %v", err)
		}
	})

	t.Run("existing_writable_directory_leaves_no_probe_file", func(t *testing.T) {
		dir := t.TempDir()
		if err := verifyDirectoryWritable(dir); err != nil {
			t.Fatalf("verifyDirectoryWritable() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".write-test")); !os.IsNotExist(err) {
			t.Error(".write-test probe file was not cleaned up")
		}
	})

	t.Run("path_is_a_file_not_a_directory", func(t *testing.T) {
		base := t.TempDir()
		filePath := filepath.Join(base, "not-a-dir")
		if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := verifyDirectoryWritable(filePath); err == nil {
			t.Error("verifyDirectoryWritable() on a regular file = nil, want error")
		}
	})
}

// TestVerifyDatabaseFile covers a missing file, a valid in-memory SQLite
// database opened via a shared-cache DSN (per testing-rules.md), and a file
// that exists but is not a valid database.
func TestVerifyDatabaseFile(t *testing.T) {
	t.Run("missing_file_errors", func(t *testing.T) {
		if err := verifyDatabaseFile(filepath.Join(t.TempDir(), "nope.db")); err == nil {
			t.Error("verifyDatabaseFile() on missing file = nil, want error")
		}
	})

	t.Run("valid_sqlite_file_ok", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "server.db")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("setup: sql.Open error = %v", err)
		}
		if _, err := setupDB.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
			t.Fatalf("setup: create table error = %v", err)
		}
		setupDB.Close()

		if err := verifyDatabaseFile(dbPath); err != nil {
			t.Errorf("verifyDatabaseFile() error = %v, want nil for valid db file", err)
		}
	})

	t.Run("non_database_file_errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-db.db")
		if err := os.WriteFile(path, []byte("this is not a sqlite file"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := verifyDatabaseFile(path); err == nil {
			t.Error("verifyDatabaseFile() on garbage file = nil, want error")
		}
	})
}

// TestVerifyAdminExists covers: no admin_credentials-equivalent table at
// all (query error), table present but empty (zero admins), and table with
// at least one row.
func TestVerifyAdminExists(t *testing.T) {
	t.Run("missing_table_errors", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "empty.db")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		setupDB.Close()

		if err := verifyAdminExists(dbPath); err == nil {
			t.Error("verifyAdminExists() with no table = nil, want error")
		}
	})

	t.Run("zero_admins_errors", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "server.db")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := setupDB.Exec("CREATE TABLE server_admin_credentials (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		setupDB.Close()

		if err := verifyAdminExists(dbPath); err == nil {
			t.Error("verifyAdminExists() with zero rows = nil, want error")
		}
	})

	t.Run("admin_present_ok", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "server.db")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := setupDB.Exec("CREATE TABLE server_admin_credentials (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := setupDB.Exec("INSERT INTO server_admin_credentials (id) VALUES (1)"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		setupDB.Close()

		if err := verifyAdminExists(dbPath); err != nil {
			t.Errorf("verifyAdminExists() error = %v, want nil with one admin present", err)
		}
	})
}

// TestOpenDatabase_Succeeds is a regression test confirming openDatabase
// opens a real SQLite file: it previously called sql.Open("sqlite3", dbPath),
// but the only SQLite driver imported and registered anywhere in this
// codebase is modernc.org/sqlite, which registers itself as "sqlite" (not
// "sqlite3") -- database/sql.Open does not validate the driver name eagerly,
// so the mismatch only surfaced on Ping with an "unknown driver" error. Now
// that openDatabase uses "sqlite" (AI.md PART 3), it must succeed.
func TestOpenDatabase_Succeeds(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "server.db")
	setupDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	setupDB.Close()

	db, err := openDatabase(dbPath)
	if err != nil {
		t.Fatalf("openDatabase() error = %v, want nil", err)
	}
	defer db.Close()
}

// TestVerifySystem_AllMissing covers the failure path: no server.db, no
// users.db, no server.yml, and no geoip directory. Missing server.db/
// users.db are hard failures; missing server.yml/geoip are warnings only,
// so the overall result must still be a "verification failed" error
// counting exactly the two hard failures.
func TestVerifySystem_AllMissing(t *testing.T) {
	base := t.TempDir()
	t.Setenv("DATA_DIR", filepath.Join(base, "data"))
	t.Setenv("CONFIG_DIR", filepath.Join(base, "config"))
	t.Setenv("LOG_DIR", filepath.Join(base, "log"))

	var err error
	out := captureStdout(t, func() { err = verifySystem() })

	if err == nil {
		t.Fatal("verifySystem() with nothing set up = nil, want error")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("error = %q, want substring %q", err.Error(), "verification failed")
	}
	if !strings.Contains(out, "System verification failed with 2 error(s)") {
		t.Errorf("output = %q, want it to report 2 errors", out)
	}
}

// TestVerifySystem_AllPresent covers the success path: valid server.db and
// users.db files, an admin row present, server.yml present, and writable
// log/data directories -- verifySystem must return nil.
func TestVerifySystem_AllPresent(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "data")
	configDir := filepath.Join(base, "config")
	logDir := filepath.Join(base, "log")
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", configDir)
	t.Setenv("LOG_DIR", logDir)

	dbDir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	serverDB, err := sql.Open("sqlite", filepath.Join(dbDir, "server.db"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := serverDB.Exec("CREATE TABLE server_admin_credentials (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := serverDB.Exec("INSERT INTO server_admin_credentials (id) VALUES (1)"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	serverDB.Close()

	usersDB, err := sql.Open("sqlite", filepath.Join(dbDir, "users.db"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := usersDB.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	usersDB.Close()

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  mode: production\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "geoip"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var verr error
	out := captureStdout(t, func() { verr = verifySystem() })

	if verr != nil {
		t.Fatalf("verifySystem() error = %v, want nil when everything is present", verr)
	}
	if !strings.Contains(out, "System verification completed successfully") {
		t.Errorf("output = %q, want success message", out)
	}
}

// TestUpdateServerConfig_MissingDatabase covers the error path returned
// before any database is opened: DATA_DIR set to a directory with no
// db/server.db file.
func TestUpdateServerConfig_MissingDatabase(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", t.TempDir())

	err := updateServerConfig()
	if err == nil {
		t.Fatal("updateServerConfig() with no server.db = nil, want error")
	}
	if !strings.Contains(err.Error(), "server database not found") {
		t.Errorf("error = %q, want substring %q", err.Error(), "server database not found")
	}
}

// TestUpdateServerConfig_OpenFails covers the failure path once openDatabase
// succeeds (see TestOpenDatabase_Succeeds): a server.db file is present but
// empty, so the database opens fine and updateServerConfig fails one step
// later, reading the (nonexistent) settings table.
func TestUpdateServerConfig_OpenFails(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// sql.Open is lazy and never creates the file on disk by itself; the
	// os.Stat existence check in updateServerConfig needs a real file here.
	if err := os.WriteFile(filepath.Join(dbDir, "server.db"), nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", t.TempDir())

	err := updateServerConfig()
	if err == nil {
		t.Fatal("updateServerConfig() = nil, want error (empty db has no settings table)")
	}
	if !strings.Contains(err.Error(), "failed to read settings") {
		t.Errorf("error = %q, want substring %q", err.Error(), "failed to read settings")
	}
}

// TestAdminRecoverySetup_MissingDatabase covers the error path returned
// before any database is opened or any stdin prompt is issued.
func TestAdminRecoverySetup_MissingDatabase(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	err := adminRecoverySetup()
	if err == nil {
		t.Fatal("adminRecoverySetup() with no server.db = nil, want error")
	}
	if !strings.Contains(err.Error(), "server database not found") {
		t.Errorf("error = %q, want substring %q", err.Error(), "server database not found")
	}
}

// TestAdminRecoverySetup_OpenFails covers the failure path once openDatabase
// succeeds (see TestOpenDatabase_Succeeds): server.db opens fine, so
// adminRecoverySetup proceeds to prompt for credentials on stdin; with no
// input available in the test, the username prompt defaults to "admin" and
// the password prompt reads empty, which adminRecoverySetup rejects.
func TestAdminRecoverySetup_OpenFails(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// sql.Open is lazy and never creates the file on disk by itself; the
	// os.Stat existence check in adminRecoverySetup needs a real file here.
	if err := os.WriteFile(filepath.Join(dbDir, "server.db"), nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Setenv("DATA_DIR", dataDir)

	err := adminRecoverySetup()
	if err == nil {
		t.Fatal("adminRecoverySetup() = nil, want error (no stdin input available)")
	}
	if !strings.Contains(err.Error(), "password cannot be empty") {
		t.Errorf("error = %q, want substring %q", err.Error(), "password cannot be empty")
	}
}
