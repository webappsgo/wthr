package database

import (
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// DatabaseConfig holds database connection configuration
// AI.md PART 10: Connection pooling required
type DatabaseConfig struct {
	// sqlite, postgres, mysql, mssql, mongodb
	Type     string
	Host     string
	Port     int
	Database string
	Username string
	Password string
	// For PostgreSQL
	SSLMode string
	Options map[string]string
	// Connection pool settings per AI.md PART 10
	Pool PoolConfig
}

// ParseConnectionString parses a database connection string and returns config
func ParseConnectionString(connString string) (*DatabaseConfig, error) {
	config := &DatabaseConfig{
		Options: make(map[string]string),
	}

	if strings.HasPrefix(connString, "sqlite://") || strings.HasPrefix(connString, "sqlite:") {
		config.Type = "sqlite"
		config.Database = strings.TrimPrefix(connString, "sqlite://")
		config.Database = strings.TrimPrefix(config.Database, "sqlite:")
		config.Database = strings.TrimPrefix(config.Database, "//")
		return config, nil
	}

	if strings.HasPrefix(connString, "postgres://") || strings.HasPrefix(connString, "postgresql://") {
		config.Type = "postgres"
		// Parse postgres://user:pass@host:port/dbname?sslmode=disable
		// This is a simplified parser - for production use url.Parse
		return nil, fmt.Errorf("PostgreSQL connection string format is not supported — set DB_TYPE, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD instead")
	}

	if strings.HasPrefix(connString, "mysql://") {
		config.Type = "mysql"
		return nil, fmt.Errorf("MySQL connection string format is not supported — set DB_TYPE, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD instead")
	}

	if strings.HasPrefix(connString, "sqlserver://") || strings.HasPrefix(connString, "mssql://") {
		config.Type = "mssql"
		return nil, fmt.Errorf("MSSQL connection string format is not supported — set DB_TYPE, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD instead")
	}

	if strings.HasPrefix(connString, "mongodb://") || strings.HasPrefix(connString, "mongo://") {
		return nil, fmt.Errorf("MongoDB is not supported: weather service requires SQL database for relational queries")
	}

	// Assume raw SQLite path
	config.Type = "sqlite"
	config.Database = connString
	return config, nil
}
