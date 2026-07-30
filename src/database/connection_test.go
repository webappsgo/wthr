package database

import (
	"fmt"
	"testing"
	"time"
)

func TestInitDBWithConfig_SQLiteSuccess(t *testing.T) {
	dsn := fmt.Sprintf("file:cfg_sqlite_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := InitDBWithConfig(&DatabaseConfig{Type: "sqlite", Database: dsn})
	if err != nil {
		t.Fatalf("InitDBWithConfig(sqlite): %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, SchemaVersion)
	}

	// The pool config should have applied DefaultPoolConfig since none was given.
	stats := db.Stats()
	want := DefaultPoolConfig()
	if stats.MaxOpenConnections != want.MaxOpen {
		t.Errorf("MaxOpenConnections = %d, want %d (DefaultPoolConfig)", stats.MaxOpenConnections, want.MaxOpen)
	}
}

func TestInitDBWithConfig_SQLiteCustomPool(t *testing.T) {
	dsn := fmt.Sprintf("file:cfg_sqlite_pool_%d?mode=memory&cache=shared", time.Now().UnixNano())
	cfg := &DatabaseConfig{
		Type:     "sqlite",
		Database: dsn,
		Pool:     PoolConfig{MaxOpen: 7, MaxIdle: 2, MaxLifetime: time.Minute, MaxIdleTime: time.Minute},
	}
	db, err := InitDBWithConfig(cfg)
	if err != nil {
		t.Fatalf("InitDBWithConfig: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 7 {
		t.Errorf("MaxOpenConnections = %d, want 7 (custom pool config must be honored, not overridden by default)", got)
	}
}

func TestInitDBWithConfig_SQLiteEmptyDatabase(t *testing.T) {
	_, err := InitDBWithConfig(&DatabaseConfig{Type: "sqlite", Database: ""})
	if err == nil {
		t.Fatal("InitDBWithConfig with empty Database path = nil error, want error")
	}
	if !contains(err.Error(), "database path required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "database path required")
	}
}

func TestInitDBWithConfig_MongoDBUnsupported(t *testing.T) {
	tests := []string{"mongodb", "mongo", "MongoDB", "MONGO"}
	for _, typ := range tests {
		t.Run(typ, func(t *testing.T) {
			_, err := InitDBWithConfig(&DatabaseConfig{Type: typ, Database: "irrelevant"})
			if err == nil {
				t.Fatalf("InitDBWithConfig(type=%s) = nil error, want error", typ)
			}
			if !contains(err.Error(), "not supported") {
				t.Errorf("error = %q, want substring %q", err.Error(), "not supported")
			}
		})
	}
}

func TestInitDBWithConfig_UnsupportedType(t *testing.T) {
	_, err := InitDBWithConfig(&DatabaseConfig{Type: "oracle", Database: "irrelevant"})
	if err == nil {
		t.Fatal("InitDBWithConfig(type=oracle) = nil error, want error")
	}
	if !contains(err.Error(), "unsupported database type") {
		t.Errorf("error = %q, want substring %q", err.Error(), "unsupported database type")
	}
}

// TestInitDBWithConfig_TypeCaseInsensitive verifies the type switch lowercases
// config.Type before matching, so "SQLite" behaves the same as "sqlite".
func TestInitDBWithConfig_TypeCaseInsensitive(t *testing.T) {
	dsn := fmt.Sprintf("file:cfg_case_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := InitDBWithConfig(&DatabaseConfig{Type: "SQLite", Database: dsn})
	if err != nil {
		t.Fatalf("InitDBWithConfig(type=SQLite): %v", err)
	}
	db.Close()
}

func TestParseConnectionString(t *testing.T) {
	tests := []struct {
		name       string
		connString string
		wantType   string
		wantDB     string
		wantErr    bool
		wantErrSub string
	}{
		{"sqlite double-slash prefix", "sqlite:///var/lib/wthr/server.db", "sqlite", "/var/lib/wthr/server.db", false, ""},
		{"sqlite short prefix", "sqlite:/var/lib/wthr/server.db", "sqlite", "/var/lib/wthr/server.db", false, ""},
		{"sqlite relative path prefix", "sqlite://./data/server.db", "sqlite", "./data/server.db", false, ""},
		{"postgres", "postgres://user:pass@localhost:5432/wthr", "", "", true, "PostgreSQL connection string format is not supported"},
		{"postgresql alt scheme", "postgresql://user:pass@localhost:5432/wthr", "", "", true, "PostgreSQL connection string format is not supported"},
		{"mysql", "mysql://user:pass@localhost:3306/wthr", "", "", true, "MySQL connection string format is not supported"},
		{"sqlserver", "sqlserver://user:pass@localhost:1433/wthr", "", "", true, "MSSQL connection string format is not supported"},
		{"mssql alt scheme", "mssql://user:pass@localhost:1433/wthr", "", "", true, "MSSQL connection string format is not supported"},
		{"mongodb", "mongodb://localhost:27017/wthr", "", "", true, "MongoDB is not supported"},
		{"mongo alt scheme", "mongo://localhost:27017/wthr", "", "", true, "MongoDB is not supported"},
		{"raw path fallback", "/var/lib/wthr/server.db", "sqlite", "/var/lib/wthr/server.db", false, ""},
		{"empty string fallback to sqlite", "", "sqlite", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConnectionString(tt.connString)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseConnectionString(%q) = nil error, want error", tt.connString)
				}
				if !contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConnectionString(%q): %v", tt.connString, err)
			}
			if cfg.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", cfg.Type, tt.wantType)
			}
			if cfg.Database != tt.wantDB {
				t.Errorf("Database = %q, want %q", cfg.Database, tt.wantDB)
			}
		})
	}
}

// TestInitDBFromConnectionString_SQLite exercises the full ParseConnectionString
// -> InitDBWithConfig pipeline end to end for the sqlite happy path.
func TestInitDBFromConnectionString_SQLite(t *testing.T) {
	dsn := fmt.Sprintf("file:connstr_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := InitDBFromConnectionString("sqlite://" + dsn)
	if err != nil {
		t.Fatalf("InitDBFromConnectionString: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, SchemaVersion)
	}
}

func TestInitDBFromConnectionString_UnsupportedScheme(t *testing.T) {
	_, err := InitDBFromConnectionString("mongodb://localhost:27017/wthr")
	if err == nil {
		t.Fatal("InitDBFromConnectionString(mongodb) = nil error, want error")
	}
}
