package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Per AI.md PART 33 line 45267-45326: Nested YAML structure
	if config.Server.Primary != OfficialSite {
		t.Errorf("Expected default server to be %s, got %s", OfficialSite, config.Server.Primary)
	}

	if config.Auth.Token != "" {
		t.Errorf("Expected default token to be empty, got %s", config.Auth.Token)
	}

	if config.Output.Format != "table" {
		t.Errorf("Expected default output format to be table, got %s", config.Output.Format)
	}

	if config.Output.Color != "auto" {
		t.Errorf("Expected default output color to be auto, got %s", config.Output.Color)
	}

	if config.Server.Timeout != "30s" {
		t.Errorf("Expected default timeout to be 30s, got %s", config.Server.Timeout)
	}
}

func TestConfigPath(t *testing.T) {
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() failed: %v", err)
	}

	if !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got %s", path)
	}

	if filepath.Base(path) != "cli.yml" {
		t.Errorf("Expected filename to be cli.yml, got %s", filepath.Base(path))
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
	// Set a custom config path that doesn't exist
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Should return default config with OfficialSite
	if config.Server.Primary != OfficialSite {
		t.Errorf("Expected default server %s, got %s", OfficialSite, config.Server.Primary)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Set up temporary directory for config
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Create config with nested structure per AI.md PART 33
	config := &CLIConfig{
		Server: ServerConfig{
			Primary:    "http://test.example.com",
			APIVersion: "v1",
			Timeout:    "60s",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
		Output: OutputConfig{
			Format: "json",
			Color:  "no",
		},
	}

	// Save config
	if err := SaveConfig(config); err != nil {
		t.Fatalf("SaveConfig() failed: %v", err)
	}

	// Load config
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Verify all fields
	if loaded.Server.Primary != config.Server.Primary {
		t.Errorf("Expected server %s, got %s", config.Server.Primary, loaded.Server.Primary)
	}
	if loaded.Auth.Token != config.Auth.Token {
		t.Errorf("Expected token %s, got %s", config.Auth.Token, loaded.Auth.Token)
	}
	if loaded.Output.Format != config.Output.Format {
		t.Errorf("Expected output format %s, got %s", config.Output.Format, loaded.Output.Format)
	}
	if loaded.Output.Color != config.Output.Color {
		t.Errorf("Expected output color %s, got %s", config.Output.Color, loaded.Output.Color)
	}
}

func TestInitConfig(t *testing.T) {
	// Set up temporary directory for config
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Initialize config
	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig() failed: %v", err)
	}

	// Verify config file exists
	configPath, _ := ConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("CLIConfig file was not created")
	}

	// Try to init again (should fail)
	err := InitConfig()
	if err == nil {
		t.Error("Expected error when initializing existing config")
	}
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitConfigError {
			t.Errorf("Expected ExitConfigError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type")
	}
}

func TestGetConfigValue(t *testing.T) {
	// Set up temporary directory for config
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Save test config with nested structure
	config := &CLIConfig{
		Server: ServerConfig{
			Primary:    "http://test.example.com",
			APIVersion: "v1",
			Timeout:    "45s",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
		Output: OutputConfig{
			Format: "json",
			Color:  "no",
		},
		Debug: true,
	}
	SaveConfig(config)

	tests := []struct {
		key      string
		expected string
	}{
		{"server.primary", "http://test.example.com"},
		{"auth.token", "test-token"},
		{"output.format", "json"},
		{"output.color", "no"},
		{"debug", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			value, err := GetConfigValue(tt.key)
			if err != nil {
				t.Fatalf("GetConfigValue(%s) failed: %v", tt.key, err)
			}
			if value != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, value)
			}
		})
	}

	// Test invalid key
	_, err := GetConfigValue("invalid_key")
	if err == nil {
		t.Error("Expected error for invalid key")
	}
}

func TestSetConfigValue(t *testing.T) {
	// Set up temporary directory for config
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Initialize config
	InitConfig()

	tests := []struct {
		key   string
		value string
	}{
		{"server.primary", "http://new.example.com"},
		{"auth.token", "new-token"},
		{"output.format", "json"},
		{"output.color", "no"},
		{"debug", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if err := SetConfigValue(tt.key, tt.value); err != nil {
				t.Fatalf("SetConfigValue(%s, %s) failed: %v", tt.key, tt.value, err)
			}

			// Verify the value was set
			value, err := GetConfigValue(tt.key)
			if err != nil {
				t.Fatalf("GetConfigValue(%s) failed: %v", tt.key, err)
			}
			if value != tt.value {
				t.Errorf("Expected %s, got %s", tt.value, value)
			}
		})
	}
}

func TestSetConfigValueValidation(t *testing.T) {
	// Set up temporary directory for config
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Initialize config
	InitConfig()

	// Test invalid output format
	err := SetConfigValue("output.format", "invalid")
	if err == nil {
		t.Error("Expected error for invalid output format")
	}

	// Test invalid output color
	err = SetConfigValue("output.color", "invalid")
	if err == nil {
		t.Error("Expected error for invalid output color")
	}

	// Test invalid tui theme
	err = SetConfigValue("tui.theme", "invalid")
	if err == nil {
		t.Error("Expected error for invalid tui theme")
	}

	// Test invalid key
	err = SetConfigValue("invalid_key", "value")
	if err == nil {
		t.Error("Expected error for invalid key")
	}
}
