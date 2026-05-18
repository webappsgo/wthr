package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CLIConfig represents the CLI client configuration
// Per AI.md PART 33 lines 45267-45326: Nested YAML structure
type CLIConfig struct {
	// Server connection settings
	Server ServerConfig `yaml:"server,omitempty"`
	// Authentication
	Auth AuthConfig `yaml:"auth,omitempty"`
	// Output preferences
	Output OutputConfig `yaml:"output,omitempty"`
	// TUI preferences
	TUI TUIConfig `yaml:"tui,omitempty"`
	// Logging
	Logging LoggingConfig `yaml:"logging,omitempty"`
	// Cache
	Cache CacheConfig `yaml:"cache,omitempty"`
	// Debug mode
	Debug bool `yaml:"debug,omitempty"`
	// Default location for weather queries
	Location string `yaml:"location,omitempty"`
	// User/org context (--user flag value)
	User string `yaml:"user,omitempty"`
}

// ServerConfig holds server connection settings
// Per AI.md line 45269-45279
type ServerConfig struct {
	Primary    string   `yaml:"primary,omitempty"`
	Cluster    []string `yaml:"cluster,omitempty"`
	APIVersion string   `yaml:"api_version,omitempty"`
	AdminPath  string   `yaml:"admin_path,omitempty"`
	Timeout    string   `yaml:"timeout,omitempty"`
	Retry      int      `yaml:"retry,omitempty"`
	RetryDelay string   `yaml:"retry_delay,omitempty"`
}

// AuthConfig holds authentication settings
// Per AI.md line 45282-45285
type AuthConfig struct {
	Token     string `yaml:"token,omitempty"`
	TokenFile string `yaml:"token_file,omitempty"`
}

// OutputConfig holds output preferences
// Per AI.md line 45288-45294
type OutputConfig struct {
	Format  string `yaml:"format,omitempty"`
	Color   string `yaml:"color,omitempty"`
	Pager   string `yaml:"pager,omitempty"`
	Quiet   bool   `yaml:"quiet,omitempty"`
	Verbose bool   `yaml:"verbose,omitempty"`
}

// TUIConfig holds TUI preferences
// Per AI.md line 45297-45302
type TUIConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Theme   string `yaml:"theme,omitempty"`
	Mouse   bool   `yaml:"mouse,omitempty"`
	Unicode bool   `yaml:"unicode,omitempty"`
}

// LoggingConfig holds logging settings
// Per AI.md line 45305-45310
type LoggingConfig struct {
	Level    string `yaml:"level,omitempty"`
	File     string `yaml:"file,omitempty"`
	MaxSize  string `yaml:"max_size,omitempty"`
	MaxFiles int    `yaml:"max_files,omitempty"`
}

// CacheConfig holds cache settings
// Per AI.md line 45313-45316
type CacheConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	TTL     string `yaml:"ttl,omitempty"`
	MaxSize string `yaml:"max_size,omitempty"`
}

// GetAPIPath returns the API base path (default: /api/v1)
func (c *CLIConfig) GetAPIPath() string {
	version := c.Server.APIVersion
	if version == "" {
		version = "v1"
	}
	return "/api/" + version
}

// GetPrimaryServer returns the primary server URL
// Per AI.md PART 33 line 45376-45395: Priority order for server resolution
func (c *CLIConfig) GetPrimaryServer() string {
	if c.Server.Primary != "" {
		return c.Server.Primary
	}
	return OfficialSite
}

// GetAllServers returns all available servers (primary + cluster) for failover
// Per AI.md PART 33: CLI cluster failover support
func (c *CLIConfig) GetAllServers() []string {
	servers := []string{}

	primary := c.GetPrimaryServer()
	if primary != "" {
		servers = append(servers, primary)
	}

	for _, node := range c.Server.Cluster {
		if node != "" && node != primary {
			servers = append(servers, node)
		}
	}

	return servers
}

// GetDefaultLocation returns the default location from config or environment variables
// Priority: 1. config location, 2. MYLOCATION_NAME, 3. MYLOCATION_ZIP
func (c *CLIConfig) GetDefaultLocation() string {
	if c.Location != "" {
		return c.Location
	}
	if loc := os.Getenv("MYLOCATION_NAME"); loc != "" {
		return loc
	}
	if zip := os.Getenv("MYLOCATION_ZIP"); zip != "" {
		return zip
	}
	return ""
}

// projectName is defined in paths.go

// DefaultConfig returns the default configuration
// Per AI.md line 45267-45326
func DefaultConfig() *CLIConfig {
	return &CLIConfig{
		Server: ServerConfig{
			Primary:    OfficialSite,
			APIVersion: "v1",
			AdminPath:  "admin",
			Timeout:    "30s",
			Retry:      3,
			RetryDelay: "1s",
		},
		Output: OutputConfig{
			Format: "table",
			Color:  "auto",
			Pager:  "auto",
		},
		TUI: TUIConfig{
			Enabled: true,
			Theme:   "dark",
			Mouse:   true,
			Unicode: true,
		},
		Logging: LoggingConfig{
			Level:    "warn",
			MaxSize:  "10MB",
			MaxFiles: 5,
		},
		Cache: CacheConfig{
			Enabled: true,
			TTL:     "5m",
			MaxSize: "100MB",
		},
	}
}

// ConfigDir returns the config directory path
func ConfigDir() (string, error) {
	return CLIConfigDir(), nil
}

// ConfigPath returns the path to the config file
func ConfigPath() (string, error) {
	return CLIConfigFile(), nil
}

// TokenPath returns the path to the token file
func TokenPath() (string, error) {
	return CLITokenFile(), nil
}

// checkSensitiveFilePerms checks that a file containing credentials is not
// world- or group-readable. Per AI.md PART 33: if perms are too loose, warn
// on stderr and return an error so the caller skips using the token.
// On Windows the mode bits are not meaningful; ACL inheritance handles access.
func checkSensitiveFilePerms(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: %s has insecure permissions (%v); "+
			"token will not be loaded. Run: chmod 0600 %s\n",
			path, info.Mode().Perm(), path)
		return fmt.Errorf("insecure file permissions on %s", path)
	}
	return nil
}

// LoadConfig loads the configuration from the default config file
func LoadConfig() (*CLIConfig, error) {
	return LoadConfigFromProfile("")
}

// LoadConfigFromProfile loads configuration from a named profile or default
// Per AI.md PART 33 line 45181-45195: --config NAME resolves to {config_dir}/{name}.yml
func LoadConfigFromProfile(profile string) (*CLIConfig, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("failed to get config path: %v", err))
	}

	// If profile specified, use profile-specific config
	if profile != "" {
		configDir := filepath.Dir(configPath)
		// Handle extension per line 45193-45195
		if !strings.HasSuffix(profile, ".yml") && !strings.HasSuffix(profile, ".yaml") {
			profile = profile + ".yml"
		}
		configPath = filepath.Join(configDir, profile)
	}

	// Start with defaults
	config := DefaultConfig()

	// If config exists, load it — but check permissions first per AI.md PART 33
	if _, err := os.Stat(configPath); err == nil {
		if permErr := checkSensitiveFilePerms(configPath); permErr != nil {
			// Permissions too loose: skip loading token from config, warn already printed
			config.Auth.Token = ""
		} else {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, NewConfigError(fmt.Sprintf("failed to read config: %v", err))
			}

			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, NewConfigError(fmt.Sprintf("failed to parse config: %v", err))
			}
		}
	}

	// Apply environment variable overrides per AI.md PART 33
	// Pattern: WTHR_{SECTION}_{KEY}
	if v := os.Getenv("WTHR_SERVER_PRIMARY"); v != "" {
		config.Server.Primary = v
	}
	if v := os.Getenv("WTHR_TOKEN"); v != "" {
		config.Auth.Token = v
	}
	if v := os.Getenv("WTHR_OUTPUT_FORMAT"); v != "" {
		config.Output.Format = v
	}
	if os.Getenv("WTHR_DEBUG") != "" {
		config.Debug = true
	}

	return config, nil
}

// GetToken returns the API token using priority order per AI.md line 43367-43372
// Priority: 1. explicit flag, 2. token-file flag, 3. env var, 4. config, 5. token file
func GetToken(flagToken, flagTokenFile string, config *CLIConfig) string {
	// 1. Explicit --token flag
	if flagToken != "" {
		return flagToken
	}

	// 2. --token-file flag
	if flagTokenFile != "" {
		if data, err := os.ReadFile(flagTokenFile); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// 3. Environment variable: WTHR_TOKEN
	if token := os.Getenv("WTHR_TOKEN"); token != "" {
		return token
	}

	// 4. Config file auth.token
	if config.Auth.Token != "" {
		return config.Auth.Token
	}

	// 5. Default token file: {config_dir}/token (check perms per AI.md PART 33)
	tokenPath, err := TokenPath()
	if err == nil {
		if checkSensitiveFilePerms(tokenPath) == nil {
			if data, err := os.ReadFile(tokenPath); err == nil {
				return strings.TrimSpace(string(data))
			}
		}
	}

	return "" // No token (anonymous access if allowed)
}

// SaveConfig saves the configuration to the config file
func SaveConfig(config *CLIConfig) error {
	configPath, err := ConfigPath()
	if err != nil {
		return NewConfigError(fmt.Sprintf("failed to get config path: %v", err))
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return NewConfigError(fmt.Sprintf("failed to create config directory: %v", err))
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return NewConfigError(fmt.Sprintf("failed to marshal config: %v", err))
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return NewConfigError(fmt.Sprintf("failed to write config: %v", err))
	}

	return nil
}

// SaveToken saves the token to the token file
func SaveToken(token string) error {
	tokenPath, err := TokenPath()
	if err != nil {
		return NewConfigError(fmt.Sprintf("failed to get token path: %v", err))
	}

	configDir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return NewConfigError(fmt.Sprintf("failed to create config directory: %v", err))
	}

	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0600); err != nil {
		return NewConfigError(fmt.Sprintf("failed to write token: %v", err))
	}

	return nil
}

// InitConfig initializes a new configuration file
func InitConfig() error {
	configPath, err := ConfigPath()
	if err != nil {
		return NewConfigError(fmt.Sprintf("failed to get config path: %v", err))
	}

	if _, err := os.Stat(configPath); err == nil {
		return NewConfigError("config file already exists")
	}

	config := DefaultConfig()
	if err := SaveConfig(config); err != nil {
		return err
	}

	fmt.Printf("Configuration file created at: %s\n", configPath)
	return nil
}

// GetConfigValue returns a specific configuration value
// Supports dot notation: server.primary, output.format, etc.
func GetConfigValue(key string) (string, error) {
	config, err := LoadConfig()
	if err != nil {
		return "", err
	}

	switch key {
	case "server.primary":
		return config.Server.Primary, nil
	case "server.api_version":
		return config.Server.APIVersion, nil
	case "server.timeout":
		return config.Server.Timeout, nil
	case "auth.token":
		return config.Auth.Token, nil
	case "output.format":
		return config.Output.Format, nil
	case "output.color":
		return config.Output.Color, nil
	case "tui.theme":
		return config.TUI.Theme, nil
	case "location":
		return config.Location, nil
	case "user":
		return config.User, nil
	case "debug":
		return fmt.Sprintf("%t", config.Debug), nil
	default:
		return "", NewConfigError(fmt.Sprintf("unknown config key: %s", key))
	}
}

// SetConfigValue sets a specific configuration value
// Supports dot notation: server.primary, output.format, etc.
func SetConfigValue(key, value string) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}

	switch key {
	case "server.primary":
		config.Server.Primary = value
	case "server.api_version":
		config.Server.APIVersion = value
	case "server.timeout":
		config.Server.Timeout = value
	case "auth.token":
		config.Auth.Token = value
	case "output.format":
		if value != "json" && value != "table" && value != "plain" && value != "yaml" && value != "csv" {
			return NewConfigError("output.format must be json, table, plain, yaml, or csv")
		}
		config.Output.Format = value
	case "output.color":
		if value != "auto" && value != "always" && value != "never" {
			return NewConfigError("output.color must be auto, always, or never")
		}
		config.Output.Color = value
	case "tui.theme":
		if value != "dark" && value != "light" && value != "system" {
			return NewConfigError("tui.theme must be dark, light, or system")
		}
		config.TUI.Theme = value
	case "location":
		config.Location = value
	case "user":
		config.User = value
	case "debug":
		config.Debug = parseBoolValue(value)
	default:
		return NewConfigError(fmt.Sprintf("unknown config key: %s", key))
	}

	return SaveConfig(config)
}

// parseBoolValue parses a boolean string value
// Supports: true/false, yes/no, 1/0, on/off, enable/disable per AI.md PART 5
func parseBoolValue(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "true", "yes", "1", "on", "enable", "enabled", "yep", "yup", "yeah", "aye", "si", "oui", "da", "hai", "affirmative":
		return true
	default:
		return false
	}
}
