package utils

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateYAMLConfig updates a YAML configuration file
func UpdateYAMLConfig(filePath string, updates map[string]interface{}) error {
	// Read existing config
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Parse YAML
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Apply updates using dot notation (e.g., "server.users.enabled")
	for key, value := range updates {
		if err := setNestedValue(config, key, value); err != nil {
			return err
		}
	}

	// Marshal back to YAML
	newData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Write back to file
	if err := os.WriteFile(filePath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// setNestedValue sets a value in a nested map using dot notation
func setNestedValue(m map[string]interface{}, path string, value interface{}) error {
	keys := splitPath(path)
	if len(keys) == 0 {
		return fmt.Errorf("empty path")
	}

	// Navigate to the parent
	current := m
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		if _, ok := current[key]; !ok {
			current[key] = make(map[string]interface{})
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return fmt.Errorf("path %s is not a map", key)
		}
	}

	// Set the final value
	current[keys[len(keys)-1]] = value
	return nil
}

// splitPath splits a dot-notation path into keys
func splitPath(path string) []string {
	var keys []string
	var current string
	for _, char := range path {
		if char == '.' {
			if current != "" {
				keys = append(keys, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		keys = append(keys, current)
	}
	return keys
}
