package model

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
)

// Setting represents a configuration setting
type Setting struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
	// string, number, boolean, json
	Type string `json:"type"`
	// Human-readable description
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SettingsModel handles settings operations
type SettingsModel struct {
	Store  *database.Store
	testDB *sql.DB
}

// GetSetting retrieves a setting by key
func (m *SettingsModel) GetSetting(key string) (*Setting, error) {
	value, exists, err := config.ReadSetting(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, sql.ErrNoRows
	}
	return settingFromValue(key, value), nil
}

// GetString retrieves a setting value as string
func (m *SettingsModel) GetString(key, defaultValue string) string {
	setting, err := m.GetSetting(key)
	if err != nil {
		return defaultValue
	}
	return setting.Value
}

// GetInt retrieves a setting value as int
func (m *SettingsModel) GetInt(key string, defaultValue int) int {
	setting, err := m.GetSetting(key)
	if err != nil {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(setting.Value, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}

// GetBool retrieves a setting value as bool
func (m *SettingsModel) GetBool(key string, defaultValue bool) bool {
	setting, err := m.GetSetting(key)
	if err != nil {
		return defaultValue
	}

	return config.IsTruthy(setting.Value)
}

// GetJSON retrieves a setting value as JSON
func (m *SettingsModel) GetJSON(key string, dest interface{}) error {
	setting, err := m.GetSetting(key)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(setting.Value), dest)
}

// SetSetting creates or updates a setting
func (m *SettingsModel) SetSetting(key, value, settingType string) error {
	var decoded any = value
	switch settingType {
	case "number", "boolean", "json":
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return fmt.Errorf("invalid %s setting %s: %w", settingType, key, err)
		}
	}
	return config.WriteSetting(key, decoded)
}

// SetWithDescription sets a setting with description
func (m *SettingsModel) SetWithDescription(key, value, settingType, description string) error {
	return m.SetSetting(key, value, settingType)
}

// SetString sets a string setting
func (m *SettingsModel) SetString(key, value string) error {
	return m.SetSetting(key, value, "string")
}

// SetInt sets an integer setting
func (m *SettingsModel) SetInt(key string, value int) error {
	return m.SetSetting(key, fmt.Sprintf("%d", value), "number")
}

// SetBool sets a boolean setting
func (m *SettingsModel) SetBool(key string, value bool) error {
	stringValue := "false"
	if value {
		stringValue = "true"
	}
	return m.SetSetting(key, stringValue, "boolean")
}

// SetJSON sets a JSON setting
func (m *SettingsModel) SetJSON(key string, value interface{}) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.SetSetting(key, string(jsonBytes), "json")
}

// DeleteSetting removes a setting
func (m *SettingsModel) DeleteSetting(key string) error {
	return config.DeleteSetting(key)
}

// List returns all settings
func (m *SettingsModel) List() ([]*Setting, error) {
	values, err := config.ReadAllSettings()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	settings := make([]*Setting, 0, len(keys))
	for _, key := range keys {
		settings = append(settings, settingFromValue(key, values[key]))
	}
	return settings, nil
}

// ListByPrefix returns all settings with a specific prefix
func (m *SettingsModel) ListByPrefix(prefix string) ([]*Setting, error) {
	settings, err := m.List()
	if err != nil {
		return nil, err
	}
	filtered := make([]*Setting, 0)
	for _, setting := range settings {
		if strings.HasPrefix(setting.Key, prefix) {
			filtered = append(filtered, setting)
		}
	}
	return filtered, nil
}

func settingFromValue(key string, value any) *Setting {
	setting := &Setting{Key: key}
	switch typed := value.(type) {
	case string:
		setting.Value = typed
		setting.Type = "string"
	case bool:
		setting.Value = fmt.Sprintf("%t", typed)
		setting.Type = "boolean"
	case int, int64, uint64, float32, float64:
		setting.Value = fmt.Sprint(typed)
		setting.Type = "number"
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			setting.Value = "null"
		} else {
			setting.Value = string(encoded)
		}
		setting.Type = "json"
	}
	if dot := strings.IndexByte(key, '.'); dot >= 0 {
		setting.Category = key[:dot]
	}
	return setting
}

// MigrateLegacyDatabaseSettings moves previously persisted settings into the
// canonical file that owns each key. Existing file values win and the legacy
// table is retained until the complete schema migration can remove it.
func (m *SettingsModel) MigrateLegacyDatabaseSettings() error {
	store := m.Store
	if store == nil && m.testDB != nil {
		store = database.NewStore(m.testDB)
	}
	if store == nil {
		store = database.GetServerStore()
	}
	if store == nil || !store.IsSQLite() {
		return nil
	}
	return config.MigrateLegacyDatabaseSettings(store)
}
