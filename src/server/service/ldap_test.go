package service

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// setupLDAPServerDB opens a fresh in-memory SQLite database with the real
// production ServerSchema applied (server_config lives there), uniquely
// named per test via t.Name().
func setupLDAPServerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_ldap?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

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

// TestLDAP_Config_Defaults covers the boundary of an empty server_config
// table: every field must fall back to its documented Go-level default.
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

// TestLDAP_Config_SeededValues covers the happy path: values stored in
// server_config are read back verbatim, overriding every default.
func TestLDAP_Config_SeededValues(t *testing.T) {
	db := setupLDAPServerDB(t)
	wireLDAPGlobalDB(t, db)
	sm := &models.SettingsModel{DB: db}

	seed := map[string]string{
		"server.auth.ldap.enabled":       "true",
		"server.auth.ldap.server":        "ldap.example.com",
		"server.auth.ldap.port":          "636",
		"server.auth.ldap.bind_dn":       "cn=admin,dc=example,dc=com",
		"server.auth.ldap.bind_password": "secret",
		"server.auth.ldap.base_dn":       "dc=example,dc=com",
		"server.auth.ldap.user_filter":   "(sAMAccountName=%s)",
	}
	for k, v := range seed {
		if err := sm.SetString(k, v); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
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
	if cfg.BaseDN != "dc=example,dc=com" {
		t.Errorf("BaseDN = %q, want %q", cfg.BaseDN, "dc=example,dc=com")
	}
	if cfg.UserFilter != "(sAMAccountName=%s)" {
		t.Errorf("UserFilter = %q, want %q", cfg.UserFilter, "(sAMAccountName=%s)")
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
			sm := &models.SettingsModel{DB: db}
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
