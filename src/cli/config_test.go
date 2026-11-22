// Tests for config.go per AI.md PART 5 (Config) / PART 29 (Testing).
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateServerYML covers a normal write (file created, contains the
// expected typed default values), and preservation of an existing file.
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
			"server:", "database:", "cache:", "ssl:",
			"port: 64123", "mode: production", "address: \"[::]\"",
			"# This node only", "credentials are in secrets.yml.",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("server.yml missing %q", want)
			}
		}
		for _, stale := range []string{"auth:", "smtp:", "branding:", "notifications:", "rate_limit:", "# Set via DOMAIN", "driver: sqlite"} {
			if strings.Contains(content, stale) {
				t.Errorf("server.yml contains stale or shared setting %q", stale)
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

	t.Run("creates_private_config_directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does", "not", "exist")
		if err := GenerateServerYML(dir, "64123"); err != nil {
			t.Fatalf("GenerateServerYML() with missing directory: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "server.yml")); err != nil {
			t.Errorf("server.yml not created: %v", err)
		}
	})
}
