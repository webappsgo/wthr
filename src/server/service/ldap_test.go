package service

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// setupLDAPServerDB opens a fresh in-memory SQLite database and an isolated
// config directory for file-backed LDAP settings.
func setupLDAPServerDB(t *testing.T) *sql.DB {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_ldap?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return db
}

// wireLDAPGlobalDB wires database.SetGlobalDualDB since SettingsModel reads
// via database.GetServerDB() rather than its injected DB field, and
// restores nil afterward.
func wireLDAPGlobalDB(t *testing.T, serverDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// TestLDAP_Config_Defaults covers the boundary of an empty config file: every
// field must fall back to its documented Go-level default.
func TestLDAP_Config_Defaults(t *testing.T) {
	db := setupLDAPServerDB(t)
	wireLDAPGlobalDB(t, db)

	svc := NewLDAPService()
	cfg := svc.Config()

	if cfg.Enabled {
		t.Error("Enabled default = true, want false")
	}
	if cfg.Server != "" {
		t.Errorf("Server default = %q, want empty", cfg.Server)
	}
	if cfg.Port != 389 {
		t.Errorf("Port default = %d, want 389", cfg.Port)
	}
	if cfg.BindDN != "" {
		t.Errorf("BindDN default = %q, want empty", cfg.BindDN)
	}
	if cfg.BindPass != "" {
		t.Errorf("BindPass default = %q, want empty", cfg.BindPass)
	}
	if cfg.BaseDN != "" {
		t.Errorf("BaseDN default = %q, want empty", cfg.BaseDN)
	}
	if cfg.UserFilter != "(uid=%s)" {
		t.Errorf("UserFilter default = %q, want %q", cfg.UserFilter, "(uid=%s)")
	}
}

// TestLDAP_Config_SeededValues covers the happy path: values in the config
// files are read back verbatim, overriding every default.
func TestLDAP_Config_SeededValues(t *testing.T) {
	db := setupLDAPServerDB(t)
	wireLDAPGlobalDB(t, db)
	sm := &models.SettingsModel{Store: serviceStoreForDB(db)}

	seed := map[string]string{
		"server.auth.ldap.enabled":     "true",
		"server.auth.ldap.server":      "ldap.example.com",
		"server.auth.ldap.port":        "636",
		"server.auth.ldap.bind_dn":     "cn=admin,dc=example,dc=com",
		"server.auth.ldap.base_dn":     "dc=example,dc=com",
		"server.auth.ldap.user_filter": "(sAMAccountName=%s)",
	}
	for k, v := range seed {
		if err := sm.SetString(k, v); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
	if err := config.WriteSecret("server.auth.ldap.bind_password", "secret-file-value"); err != nil {
		t.Fatal(err)
	}

	svc := NewLDAPService()
	cfg := svc.Config()

	if !cfg.Enabled {
		t.Error("expected Enabled true")
	}
	if cfg.Server != "ldap.example.com" {
		t.Errorf("Server = %q, want %q", cfg.Server, "ldap.example.com")
	}
	if cfg.Port != 636 {
		t.Errorf("Port = %d, want 636", cfg.Port)
	}
	if cfg.BindDN != "cn=admin,dc=example,dc=com" {
		t.Errorf("BindDN = %q, want %q", cfg.BindDN, "cn=admin,dc=example,dc=com")
	}
	if cfg.BindPass != "secret-file-value" {
		t.Errorf("BindPass = %q, want value from secrets.yml", cfg.BindPass)
	}
	if cfg.BaseDN != "dc=example,dc=com" {
		t.Errorf("BaseDN = %q, want %q", cfg.BaseDN, "dc=example,dc=com")
	}
	if cfg.UserFilter != "(sAMAccountName=%s)" {
		t.Errorf("UserFilter = %q, want %q", cfg.UserFilter, "(sAMAccountName=%s)")
	}
}

func TestLDAP_ConfigDoesNotUsePlaintextDatabasePassword(t *testing.T) {
	db := setupLDAPServerDB(t)
	wireLDAPGlobalDB(t, db)
	settings := &models.SettingsModel{Store: serviceStoreForDB(db)}
	if err := settings.SetString("server.auth.ldap.bind_password", "legacy-plaintext"); err != nil {
		t.Fatal(err)
	}
	if got := NewLDAPService().Config().BindPass; got != "" {
		t.Errorf("BindPass = %q, want empty without secrets.yml value", got)
	}
}

// TestLDAP_Authenticate_GuardClauses is table-driven over the three PURE
// validation guards at the top of Authenticate, which run before any
// network I/O. These are the only parts of Authenticate that can be tested
// safely without a real LDAP server (see coverage-gap note below).
//
// COVERAGE GAP: the remainder of Authenticate (DialURL, StartTLS, Bind,
// Search, re-Bind, and displayName fallback logic) requires a live LDAP
// connection or protocol-level mock and has no injectable seam in this
// codebase (ldap.DialURL is called directly, not through an interface).
// Per project instructions this path is intentionally left untested rather
// than faked with a stub network connection.
func TestLDAP_Authenticate_GuardClauses(t *testing.T) {
	tests := []struct {
		name          string
		seed          map[string]string
		username      string
		password      string
		wantErrSubstr string
	}{
		{
			name:          "LDAP not enabled",
			seed:          map[string]string{},
			username:      "alice",
			password:      "hunter2",
			wantErrSubstr: "LDAP authentication is not enabled",
		},
		{
			name: "enabled but server unconfigured",
			seed: map[string]string{
				"server.auth.ldap.enabled": "true",
			},
			username:      "alice",
			password:      "hunter2",
			wantErrSubstr: "LDAP is not fully configured",
		},
		{
			name: "enabled but base_dn unconfigured",
			seed: map[string]string{
				"server.auth.ldap.enabled": "true",
				"server.auth.ldap.server":  "ldap.example.com",
			},
			username:      "alice",
			password:      "hunter2",
			wantErrSubstr: "LDAP is not fully configured",
		},
		{
			name: "empty username",
			seed: map[string]string{
				"server.auth.ldap.enabled": "true",
				"server.auth.ldap.server":  "ldap.example.com",
				"server.auth.ldap.base_dn": "dc=example,dc=com",
			},
			username:      "",
			password:      "hunter2",
			wantErrSubstr: "username and password are required",
		},
		{
			name: "empty password",
			seed: map[string]string{
				"server.auth.ldap.enabled": "true",
				"server.auth.ldap.server":  "ldap.example.com",
				"server.auth.ldap.base_dn": "dc=example,dc=com",
			},
			username:      "alice",
			password:      "",
			wantErrSubstr: "username and password are required",
		},
		{
			name: "both empty",
			seed: map[string]string{
				"server.auth.ldap.enabled": "true",
				"server.auth.ldap.server":  "ldap.example.com",
				"server.auth.ldap.base_dn": "dc=example,dc=com",
			},
			username:      "",
			password:      "",
			wantErrSubstr: "username and password are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupLDAPServerDB(t)
			wireLDAPGlobalDB(t, db)
			sm := &models.SettingsModel{Store: serviceStoreForDB(db)}
			for k, v := range tt.seed {
				if err := sm.SetString(k, v); err != nil {
					t.Fatalf("seed %s: %v", k, err)
				}
			}

			svc := NewLDAPService()
			email, displayName, err := svc.Authenticate(tt.username, tt.password)

			if err == nil {
				t.Fatal("expected error from guard clause, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
			}
			if email != "" || displayName != "" {
				t.Errorf("expected empty email/displayName on guard failure, got (%q, %q)", email, displayName)
			}
		})
	}
}
