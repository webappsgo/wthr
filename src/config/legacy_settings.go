package config

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// MigrateLegacyDatabaseSettings imports old SQLite server_config rows once;
// production reads and writes after migration use only the configuration files.
func MigrateLegacyDatabaseSettings(db *sql.DB) error {
	if db == nil {
		return nil
	}
	var table string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'server_config'").Scan(&table); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check legacy settings table: %w", err)
	}
	rows, err := db.Query("SELECT key, value, type FROM server_config ORDER BY key")
	if err != nil {
		return fmt.Errorf("read legacy settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, value, settingType string
		if err := rows.Scan(&key, &value, &settingType); err != nil {
			return fmt.Errorf("read legacy setting row: %w", err)
		}
		if _, exists, err := ReadSetting(key); err != nil {
			return fmt.Errorf("check migrated setting %s: %w", key, err)
		} else if exists {
			continue
		}
		if settingType == "" {
			settingType = "string"
		}
		if CanonicalConfigOwner(key) == "secrets.yml" && value == "" {
			continue
		}
		var decoded any = value
		switch settingType {
		case "boolean", "bool":
			parsed, err := ParseBool(value, false)
			if err != nil {
				return fmt.Errorf("migrate legacy boolean setting %s: %w", key, err)
			}
			decoded = parsed
		case "number":
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				if integer, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil {
					decoded = integer
				} else if decimal, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
					decoded = decimal
				} else {
					return fmt.Errorf("migrate legacy numeric setting %s: %w", key, err)
				}
			}
		case "json":
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				return fmt.Errorf("migrate legacy JSON setting %s: %w", key, err)
			}
		}
		if err := WriteSetting(key, decoded); err != nil {
			return fmt.Errorf("migrate legacy setting %s: %w", key, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy settings: %w", err)
	}
	return nil
}
