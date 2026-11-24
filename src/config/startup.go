package config

import (
	"path/filepath"
	"strconv"
	"strings"
)

// StartupSettings contains only the server settings that must be known before
// binding listeners or dropping privileges. Secrets and unrelated settings are
// not decoded into this structure.
type StartupSettings struct {
	Port      string
	Address   string
	Daemonize bool
}

// LoadStartupSettings reads the early listener settings from server.yml and
// yaml.d using the same ordering and merge rules as the full configuration
// loader. Invalid layers are skipped here and reported by the full load later.
func LoadStartupSettings(configDir string) StartupSettings {
	configPath := filepath.Join(configDir, "server.yml")
	merged, _, err := loadLayers(configPath, false)
	if err != nil {
		return StartupSettings{}
	}
	var early struct {
		Server struct {
			Port      interface{} `yaml:"port"`
			Address   *string     `yaml:"address"`
			Daemonize bool        `yaml:"daemonize"`
		} `yaml:"server"`
	}
	if err := merged.Decode(&early); err != nil {
		return StartupSettings{}
	}
	port := ""
	switch value := early.Server.Port.(type) {
	case int:
		port = strconv.Itoa(value)
	case int64:
		port = strconv.FormatInt(value, 10)
	case string:
		port = strings.TrimSpace(value)
	}
	address := ""
	if early.Server.Address != nil {
		address = strings.TrimSpace(*early.Server.Address)
		if !validServerAddress(address) {
			address = "[::]"
		}
	}
	return StartupSettings{Port: port, Address: address, Daemonize: early.Server.Daemonize}
}
