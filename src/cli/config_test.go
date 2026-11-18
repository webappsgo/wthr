// Tests for config.go per AI.md PART 5 (Config) / PART 29 (Testing).
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateServerYML covers a normal write (file created, contains the
// expected top-level sections and default values), preservation of an
// existing file, and the error path when the target directory does not
// exist.
func TestGenerateServerYML(t *testing.T) {
	t.Run("writes_default_config", func(t *testing.T) {
		dir := t.TempDir()

		if err := GenerateServerYML(dir, "64123"); err != nil {
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
			`driver: sqlite`, "port: 64123", "mode: production",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("server.yml missing %q", want)
			}
		}
	})

	t.Run("is_private_and_saves_a_dual_port", func(t *testing.T) {
		dir := t.TempDir()
		if err := GenerateServerYML(dir, "80,443"); err != nil {
			t.Fatalf("GenerateServerYML() error = %v", err)
		}
		info, err := os.Stat(filepath.Join(dir, "server.yml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("server.yml mode = %o, want 600", info.Mode().Perm())
		}
		data, _ := os.ReadFile(filepath.Join(dir, "server.yml"))
		if !strings.Contains(string(data), `port: "80,443"`) {
			t.Errorf("a dual port must be saved as a quoted string")
		}
	})

	t.Run("preserves_existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		if err := os.WriteFile(path, []byte("stale: true"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := GenerateServerYML(dir, "64123"); err != nil {
			t.Fatalf("GenerateServerYML() error = %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("server.yml missing after generate: %v", err)
		}
		if string(data) != "stale: true" {
			t.Errorf("server.yml was overwritten, got %q, want %q", string(data), "stale: true")
		}
	})

	t.Run("missing_directory_errors", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does", "not", "exist")
		if err := GenerateServerYML(dir, "64123"); err == nil {
			t.Error("GenerateServerYML() with missing directory = nil, want error")
		}
	})
}
