package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadSecretUsesPrivateYAMLFile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)

	if err := WriteSecret("server.backup.encryption_password", "enc:v1:sealed-value"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecret("server.security.encryption_key", "key-material"); err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"server.backup.encryption_password": "enc:v1:sealed-value",
		"server.security.encryption_key":    "key-material",
	} {
		got, err := ReadSecret(key)
		if err != nil {
			t.Fatalf("ReadSecret(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("ReadSecret(%q) = %q, want %q", key, got, want)
		}
	}

	info, err := os.Stat(filepath.Join(configDir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("secrets.yml mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestReadSecretRejectsNonRegularSecretsFile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	if err := os.Mkdir(filepath.Join(configDir, "secrets.yml"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret("server.backup.encryption_password"); err == nil {
		t.Fatal("ReadSecret should reject a secrets.yml directory")
	}
}
