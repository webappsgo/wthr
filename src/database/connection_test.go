package database

import (
	"testing"
)

func TestParseConnectionString(t *testing.T) {
	tests := []struct {
		name       string
		connString string
		wantType   string
		wantDB     string
		wantErr    bool
		wantErrSub string
	}{
		{"sqlite double-slash prefix", "sqlite:///var/lib/webappsgo/wthr/server.db", "sqlite", "/var/lib/webappsgo/wthr/server.db", false, ""},
		{"sqlite short prefix", "sqlite:/var/lib/webappsgo/wthr/server.db", "sqlite", "/var/lib/webappsgo/wthr/server.db", false, ""},
		{"sqlite relative path prefix", "sqlite://./data/server.db", "sqlite", "./data/server.db", false, ""},
		{"postgres", "postgres://user:pass@localhost:5432/wthr", "", "", true, "PostgreSQL connection string format is not supported"},
		{"postgresql alt scheme", "postgresql://user:pass@localhost:5432/wthr", "", "", true, "PostgreSQL connection string format is not supported"},
		{"mysql", "mysql://user:pass@localhost:3306/wthr", "", "", true, "MySQL connection string format is not supported"},
		{"sqlserver", "sqlserver://user:pass@localhost:1433/wthr", "", "", true, "MSSQL connection string format is not supported"},
		{"mssql alt scheme", "mssql://user:pass@localhost:1433/wthr", "", "", true, "MSSQL connection string format is not supported"},
		{"mongodb", "mongodb://localhost:27017/wthr", "", "", true, "MongoDB is not supported"},
		{"mongo alt scheme", "mongo://localhost:27017/wthr", "", "", true, "MongoDB is not supported"},
		{"raw path fallback", "/var/lib/webappsgo/wthr/server.db", "sqlite", "/var/lib/webappsgo/wthr/server.db", false, ""},
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
