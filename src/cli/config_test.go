// Tests for config.go per AI.md PART 5 (Config) / PART 29 (Testing).
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateServerYML covers a normal write (file created, contains the
// expected top-level sections and default values), overwrite of an
// existing file, and the error path when the target directory does not
// exist.
func TestGenerateServerYML(t *testing.T) {
	t.Run("writes_default_config", func(t *testing.T) {
		dir := t.TempDir()

		if err := GenerateServerYML(dir); err != nil {
			t.Fatalf("GenerateServerYML() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "server.yml"))
		if err != nil {
			t.Fatalf("server.yml not written: %v", err)
		}
		content := string(data)

		for _, want := range []string{
			"server:", "admin:", "database:", "ssl:", "auth:", "rate_limit:",
			"log:", "backup:", "smtp:", "notifications:", "weather:",
			"alerts:", "geoip:", "schedule:", "cors:", "security:",
			"features:", "metrics:",
			`driver: sqlite`, "port: 80", "mode: production",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("server.yml missing %q", want)
			}
		}
	})

	t.Run("overwrites_existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		if err := os.WriteFile(path, []byte("stale: true"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := GenerateServerYML(dir); err != nil {
			t.Fatalf("GenerateServerYML() error = %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("server.yml missing after regenerate: %v", err)
		}
		if strings.Contains(string(data), "stale: true") {
			t.Error("server.yml still contains stale content after regenerate")
		}
	})

	t.Run("missing_directory_errors", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does", "not", "exist")
		if err := GenerateServerYML(dir); err == nil {
			t.Error("GenerateServerYML() with missing directory = nil, want error")
		}
	})
}
