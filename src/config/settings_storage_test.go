package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsStorageUsesCanonicalOwnerFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	configPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(configPath, []byte("server:\n  address: '[::]'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteSetting("server.logs.level", "debug"); err != nil {
		t.Fatalf("WriteSetting() error = %v", err)
	}
	areaPath := filepath.Join(dir, DropinDirName, "logs.yml")
	areaInfo, err := os.Stat(areaPath)
	if err != nil {
		t.Fatalf("stat owner file: %v", err)
	}
	if areaInfo.Mode().Perm() != 0o600 {
		t.Fatalf("owner file mode = %04o, want 0600", areaInfo.Mode().Perm())
	}
	value, exists, err := ReadSetting("server.logs.level")
	if err != nil || !exists || value != "debug" {
		t.Fatalf("ReadSetting() = (%v, %t, %v), want (debug, true, nil)", value, exists, err)
	}

	if err := WriteSetting("server.token", "operator-secret"); err != nil {
		t.Fatalf("WriteSetting(secret) error = %v", err)
	}
	secret, exists, err := ReadSetting("server.token")
	if err != nil || !exists || secret != "operator-secret" {
		t.Fatalf("ReadSetting(secret) = (%v, %t, %v), want (operator-secret, true, nil)", secret, exists, err)
	}
	all, err := ReadAllSettings()
	if err != nil {
		t.Fatalf("ReadAllSettings() error = %v", err)
	}
	if _, exists := all["server.token"]; exists {
		t.Fatal("ReadAllSettings() exposed a secret")
	}
	if err := WriteSetting("smtp.password", "smtp-secret"); err != nil {
		t.Fatalf("WriteSetting(legacy SMTP password) error = %v", err)
	}
	smtpPassword, exists, err := ReadSetting("smtp.password")
	if err != nil || !exists || smtpPassword != "smtp-secret" {
		t.Fatalf("ReadSetting(legacy SMTP password) = (%v, %t, %v)", smtpPassword, exists, err)
	}
	secretFile, err := os.ReadFile(filepath.Join(dir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secretFile), "notifications:") || !strings.Contains(string(secretFile), "smtp-secret") {
		t.Fatal("legacy SMTP password did not use its canonical secrets.yml path")
	}
	if err := WriteSetting("server.cache.url", "redis://operator:cache-secret@cache.example"); err != nil {
		t.Fatalf("WriteSetting(credential URL) error = %v", err)
	}
	serverFile, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	secretFile, err = os.ReadFile(filepath.Join(dir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serverFile), "cache-secret") || !strings.Contains(string(secretFile), "cache-secret") {
		t.Fatal("credential-bearing cache URL was not kept in secrets.yml")
	}

	if err := DeleteSetting("server.logs.level"); err != nil {
		t.Fatalf("DeleteSetting() error = %v", err)
	}
	if _, exists, err := ReadSetting("server.logs.level"); err != nil || exists {
		t.Fatalf("ReadSetting() after delete = (%t, %v), want (false, nil)", exists, err)
	}
}
