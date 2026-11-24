package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureContactWebhookSecrets(t *testing.T) {
	SetSecretGeneratedAuditHook(nil)
	var generated []string
	SetSecretGeneratedAuditHook(func(key string) { generated = append(generated, key) })
	t.Cleanup(func() { SetSecretGeneratedAuditHook(nil) })
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	cfg := DefaultAppConfig()
	cfg.Server.Contact.General.Webhooks["generic"] = "https://hooks.example.test/contact"

	if err := EnsureContactWebhookSecrets(cfg); err != nil {
		t.Fatalf("EnsureContactWebhookSecrets() error = %v", err)
	}
	key := "server.contact.general.webhooks.generic_secret"
	first, err := ReadSecret(key)
	if err != nil {
		t.Fatalf("ReadSecret() error = %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("webhook secret decoded length = %d, error = %v; want 32 random bytes", len(decoded), err)
	}

	cfg.Server.Contact.General.Webhooks["generic"] = "https://hooks.example.test/changed"
	if err := EnsureContactWebhookSecrets(cfg); err != nil {
		t.Fatalf("EnsureContactWebhookSecrets() second call error = %v", err)
	}
	second, err := ReadSecret(key)
	if err != nil {
		t.Fatalf("ReadSecret() second call error = %v", err)
	}
	if second != first {
		t.Fatal("webhook secret changed when only its URL changed")
	}
	if len(generated) != 1 || generated[0] != key {
		t.Errorf("generated-secret audit keys = %v, want only %q", generated, key)
	}
	info, err := os.Stat(filepath.Join(configDir, "secrets.yml"))
	if err != nil {
		t.Fatalf("stat secrets.yml: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("secrets.yml mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestRemoveWebhookSecretValues(t *testing.T) {
	contact := ContactConfig{General: ContactRoleConfig{Webhooks: map[string]string{
		"generic": "https://hooks.example.test/contact", "generic_secret": "private",
	}}}
	removeWebhookSecretValues(&contact)
	if len(contact.General.Webhooks) != 1 || contact.General.Webhooks["generic"] == "" {
		t.Fatalf("sanitized webhooks = %#v, want only the URL", contact.General.Webhooks)
	}
}

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
