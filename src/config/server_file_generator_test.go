package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDefaultServerYMLUsesTypedNodeDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateDefaultServerYML(dir, "80,443"); err != nil {
		t.Fatalf("GenerateDefaultServerYML() error = %v", err)
	}
	path := filepath.Join(dir, "server.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		"port: \"80,443\"",
		"# HTTP port, or a comma-separated HTTP and HTTPS port pair.",
		"# Database connection used by this node.",
		"# Cache connection used by this node.",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("server.yml does not contain %q", expected)
		}
	}
	for _, excluded := range []string{"branding:", "notifications:", "rate_limit:", "token:", "password:"} {
		if strings.Contains(content, excluded) {
			t.Errorf("server.yml contains shared or secret setting %q", excluded)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("server.yml mode = %04o, want 0600", info.Mode().Perm())
	}
}
