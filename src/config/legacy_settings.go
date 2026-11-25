package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type legacySettingsReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// MigrateLegacyDatabaseSettings imports old SQLite server_config rows once;
// production reads and writes after migration use only the configuration files.
func MigrateLegacyDatabaseSettings(db legacySettingsReader) error {
	if db == nil || (reflect.ValueOf(db).Kind() == reflect.Pointer && reflect.ValueOf(db).IsNil()) {
		return nil
	}
	var table string
	ctx := context.Background()
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'server_config'").Scan(&table); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check legacy settings table: %w", err)
	}
	rows, err := db.QueryContext(ctx, "SELECT key, value, type FROM server_config ORDER BY key")
	if err != nil {
		return fmt.Errorf("read legacy settings: %w", err)
	}
	type legacySetting struct{ key, value, settingType string }
	legacySettings := make([]legacySetting, 0)
	for rows.Next() {
		var setting legacySetting
		if err := rows.Scan(&setting.key, &setting.value, &setting.settingType); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read legacy setting row: %w", err)
		}
		legacySettings = append(legacySettings, setting)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate legacy settings: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy settings: %w", err)
	}
	for _, setting := range legacySettings {
		key, value, settingType := setting.key, setting.value, setting.settingType
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
	return nil
}
