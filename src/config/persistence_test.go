package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCanonicalConfigOwner(t *testing.T) {
	tests := map[string]string{
		"server.port":                                "server.yml",
		"server.database.host":                       "server.yml",
		"server.security.encryption_key":             "secrets.yml",
		"server.branding.title":                      "yaml.d/branding.yml",
		"web.cors":                                   "yaml.d/web.yml",
		"server.pages.contact.captcha_secret_key":    "secrets.yml",
		"server.contact.admin.webhooks.url":          "secrets.yml",
		"server.logs.level":                          "yaml.d/logs.yml",
		"server.auth.oidc.providers.0.client_secret": "secrets.yml",
		"server.metrics.auth.tokens.prometheus":      "secrets.yml",
		"project.custom.setting":                     "yaml.d/app.yml",
	}
	for key, want := range tests {
		if got := CanonicalConfigOwner(key); got != want {
			t.Errorf("CanonicalConfigOwner(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestSaveConfigFilesPartitionsAndPreservesAppSettings(t *testing.T) {
	configDir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "server.yml")
	areaDir := DropinDir(configPath)
	if err := os.MkdirAll(areaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(areaDir, "app.yml"), []byte("project:\n  custom_setting: kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	input := []byte(`server:
  address: "[::]"
  token: operator-secret-value
  branding:
    title: Weather
  auth:
    oidc:
      providers:
        - name: login
          client_secret: oidc-secret-value
weather:
  forecast_days: 7
project:
  active: true
`)
	if err := saveConfigFiles(configPath, input); err != nil {
		t.Fatalf("saveConfigFiles() error = %v", err)
	}

	serverData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serverData), "operator-secret-value") || strings.Contains(string(serverData), "oidc-secret-value") {
		t.Fatal("server.yml contains a credential")
	}
	if !strings.Contains(string(serverData), "address: '[::]'") && !strings.Contains(string(serverData), `address: "[::]"`) {
		t.Fatal("server.yml did not retain the node-local address")
	}
	if runtime.GOOS != "windows" {
		configInfo, err := os.Stat(configDir)
		if err != nil {
			t.Fatal(err)
		}
		if configInfo.Mode().Perm() != 0o700 {
			t.Fatalf("config directory mode = %04o; want 0700", configInfo.Mode().Perm())
		}
		areaInfo, err := os.Stat(areaDir)
		if err != nil {
			t.Fatal(err)
		}
		if areaInfo.Mode().Perm() != 0o700 {
			t.Fatalf("yaml.d mode = %04o; want 0700", areaInfo.Mode().Perm())
		}
		brandingInfo, err := os.Stat(filepath.Join(areaDir, "branding.yml"))
		if err != nil {
			t.Fatal(err)
		}
		if brandingInfo.Mode().Perm() != 0o600 {
			t.Fatalf("branding.yml mode = %04o; want 0600", brandingInfo.Mode().Perm())
		}
		serverInfo, err := os.Stat(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if serverInfo.Mode().Perm() != 0o600 {
			t.Fatalf("server.yml mode = %04o; want 0600", serverInfo.Mode().Perm())
		}
		secretsInfo, err := os.Stat(filepath.Join(configDir, "secrets.yml"))
		if err != nil {
			t.Fatal(err)
		}
		if secretsInfo.Mode().Perm() != 0o600 {
			t.Fatalf("secrets.yml mode = %04o; want 0600", secretsInfo.Mode().Perm())
		}
	}

	brandingData, err := os.ReadFile(filepath.Join(areaDir, "branding.yml"))
	if err != nil || !strings.Contains(string(brandingData), "title: Weather") {
		t.Fatalf("branding.yml missing branding setting: err=%v data=%s", err, brandingData)
	}
	appData, err := os.ReadFile(filepath.Join(areaDir, "app.yml"))
	if err != nil || !strings.Contains(string(appData), "custom_setting: kept") || !strings.Contains(string(appData), "active: true") {
		t.Fatalf("app.yml did not preserve and update project settings: err=%v data=%s", err, appData)
	}
	secretsData, err := os.ReadFile(filepath.Join(configDir, "secrets.yml"))
	if err != nil || !strings.Contains(string(secretsData), "operator-secret-value") || !strings.Contains(string(secretsData), "oidc-secret-value") {
		t.Fatalf("secrets.yml missing extracted credentials: err=%v data=%s", err, secretsData)
	}
}

func TestMergeSecretsIntoConfigMergesProviderFieldsByIndex(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	secretData := []byte("server:\n  token: operator-secret\n  auth:\n    oidc:\n      providers:\n        - client_secret: oidc-secret\n")
	if err := os.WriteFile(filepath.Join(configDir, "secrets.yml"), secretData, 0o600); err != nil {
		t.Fatal(err)
	}
	var merged yaml.Node
	if err := yaml.Unmarshal([]byte("server:\n  auth:\n    oidc:\n      providers:\n        - name: login\n          client_id: public-id\n          client_secret: ''\n"), &merged); err != nil {
		t.Fatal(err)
	}
	root, err := rootMapping(&merged)
	if err != nil {
		t.Fatal(err)
	}
	if err := mergeSecretsIntoConfig(root); err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := root.Decode(&got); err != nil {
		t.Fatal(err)
	}
	server := got["server"].(map[string]interface{})
	provider := server["auth"].(map[string]interface{})["oidc"].(map[string]interface{})["providers"].([]interface{})[0].(map[string]interface{})
	if provider["name"] != "login" || provider["client_id"] != "public-id" || provider["client_secret"] != "oidc-secret" {
		t.Fatalf("provider after secret merge = %#v", provider)
	}
	if server["token"] != "operator-secret" {
		t.Fatal("operator token was not restored from secrets.yml")
	}
}

func TestMigrateDropinSecretsPreservesWinnerAndRemovesSourceValues(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	areaDir := filepath.Join(configDir, DropinDirName)
	if err := os.Mkdir(areaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "secrets.yml"), []byte("server:\n  token: existing-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"10-early.yml": "server:\n  token: early-token\n  notifications:\n    email:\n      smtp:\n        password: smtp-secret\n",
		"20-late.yml":  "server:\n  token: late-token\n  logs:\n    level: info\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(areaDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateDropinSecrets(filepath.Join(configDir, "server.yml")); err != nil {
		t.Fatalf("migrateDropinSecrets() error = %v", err)
	}
	secrets, err := readSecretsFile()
	if err != nil {
		t.Fatal(err)
	}
	if got := secrets["server"].(map[string]interface{})["token"]; got != "existing-secret" {
		t.Fatalf("secrets token = %v, want existing winner", got)
	}
	if got := secrets["server"].(map[string]interface{})["notifications"].(map[string]interface{})["email"].(map[string]interface{})["smtp"].(map[string]interface{})["password"]; got != "smtp-secret" {
		t.Fatalf("smtp password = %v, want migrated value", got)
	}
	for _, name := range []string{"10-early.yml", "20-late.yml"} {
		data, err := os.ReadFile(filepath.Join(areaDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "token:") || strings.Contains(string(data), "password:") {
			t.Fatalf("%s still contains a credential: %s", name, data)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(areaDir, "10-early.yml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("migrated drop-in mode = %04o; want 0600", info.Mode().Perm())
		}
	}
}

func TestMigrateConfigAreasRedactsCredentialsFromBackup(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	configPath := filepath.Join(configDir, "server.yml")
	original := []byte("server:\n  port: 8080\n  token: plaintext-token\n  branding:\n    title: Weather\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateConfigAreas(configPath); err != nil {
		t.Fatalf("migrateConfigAreas() error = %v", err)
	}
	backup, err := os.ReadFile(configPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(backup), "plaintext-token") || strings.Contains(string(backup), "token:") {
		t.Fatalf("redacted backup contains a credential path/value: %s", backup)
	}
	if !strings.Contains(string(backup), "port: 8080") {
		t.Fatalf("backup lost nonsecret recovery data: %s", backup)
	}
	serverData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serverData), "plaintext-token") || strings.Contains(string(serverData), "token:") {
		t.Fatalf("server.yml retained credential path/value: %s", serverData)
	}
}
