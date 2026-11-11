package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withGlobalConfig sets the global config for the duration of the test and
// restores whatever was there before, so tests never leak state into each
// other (the package uses a single package-level var).
func withGlobalConfig(t *testing.T, cfg *AppConfig) {
	t.Helper()
	prev := GetGlobalConfig()
	SetGlobalConfig(cfg)
	t.Cleanup(func() {
		SetGlobalConfig(prev)
	})
}

func TestRandomPort(t *testing.T) {
	for i := 0; i < 50; i++ {
		p := randomPort()
		if p < 64000 || p > 64999 {
			t.Fatalf("randomPort() = %d, want value in [64000, 64999]", p)
		}
	}
}

func TestGetDefaultFQDN(t *testing.T) {
	got := getDefaultFQDN()
	if got == "" {
		t.Error("getDefaultFQDN() returned empty string")
	}
}

func TestDefaultEmailAddressForHost(t *testing.T) {
	tests := []struct {
		name      string
		localPart string
		host      string
		want      string
	}{
		{"empty host falls back", "admin", "", "admin@wthr.top"},
		{"localhost falls back", "admin", "localhost", "admin@wthr.top"},
		{"IP address falls back", "admin", "192.168.1.1", "admin@wthr.top"},
		{"no dot falls back", "admin", "myhost", "admin@wthr.top"},
		{"valid FQDN kept", "admin", "example.com", "admin@example.com"},
		{"host with port strips port", "admin", "example.com:8080", "admin@example.com"},
		{"uppercase host lowercased", "admin", "EXAMPLE.COM", "admin@example.com"},
		{"empty local part defaults to admin", "", "example.com", "admin@example.com"},
		{"custom local part kept", "support", "example.com", "support@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultEmailAddressForHost(tt.localPart, tt.host)
			if got != tt.want {
				t.Errorf("defaultEmailAddressForHost(%q, %q) = %q, want %q", tt.localPart, tt.host, got, tt.want)
			}
		})
	}
}

func TestDefaultEmailAddress(t *testing.T) {
	t.Run("nil config falls back to wthr.top", func(t *testing.T) {
		got := DefaultEmailAddress("admin", nil)
		if got != "admin@wthr.top" {
			t.Errorf("DefaultEmailAddress() = %q, want admin@wthr.top", got)
		}
	})

	t.Run("uses config FQDN", func(t *testing.T) {
		cfg := &AppConfig{Server: ServerConfig{FQDN: "weather.example.com"}}
		got := DefaultEmailAddress("admin", cfg)
		if got != "admin@weather.example.com" {
			t.Errorf("DefaultEmailAddress() = %q, want admin@weather.example.com", got)
		}
	})

	t.Run("config with localhost FQDN falls back", func(t *testing.T) {
		cfg := &AppConfig{Server: ServerConfig{FQDN: "localhost"}}
		got := DefaultEmailAddress("admin", cfg)
		if got != "admin@wthr.top" {
			t.Errorf("DefaultEmailAddress() = %q, want admin@wthr.top", got)
		}
	})
}

func TestAppConfig_GetAdminPath(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AppConfig
		want string
	}{
		{"nil receiver defaults", nil, "administration"},
		{"empty admin path defaults", &AppConfig{}, "administration"},
		{"custom admin path kept", &AppConfig{Server: ServerConfig{AdminPath: "backoffice"}}, "backoffice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetAdminPath(); got != tt.want {
				t.Errorf("GetAdminPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppConfig_GetAPIVersion(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AppConfig
		want string
	}{
		{"nil receiver defaults", nil, "v1"},
		{"empty api version defaults", &AppConfig{}, "v1"},
		{"custom api version kept", &AppConfig{Server: ServerConfig{APIVersion: "v2"}}, "v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetAPIVersion(); got != tt.want {
				t.Errorf("GetAPIVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppConfig_GetAPIPath(t *testing.T) {
	cfg := &AppConfig{Server: ServerConfig{APIVersion: "v3"}}
	if got := cfg.GetAPIPath(); got != "/api/v3" {
		t.Errorf("GetAPIPath() = %q, want /api/v3", got)
	}

	cfg = &AppConfig{}
	if got := cfg.GetAPIPath(); got != "/api/v1" {
		t.Errorf("GetAPIPath() = %q, want /api/v1 (default)", got)
	}
}

func TestAppConfig_GetAdminAPIPath(t *testing.T) {
	cfg := &AppConfig{Server: ServerConfig{APIVersion: "v1", AdminPath: "backoffice"}}
	if got := cfg.GetAdminAPIPath(); got != "/api/v1/server/backoffice" {
		t.Errorf("GetAdminAPIPath() = %q, want /api/v1/server/backoffice", got)
	}

	cfg = &AppConfig{}
	if got := cfg.GetAdminAPIPath(); got != "/api/v1/server/administration" {
		t.Errorf("GetAdminAPIPath() = %q, want /api/v1/server/administration (defaults)", got)
	}
}

func TestValidateServerUpdate(t *testing.T) {
	t.Run("normalizes legacy channels", func(t *testing.T) {
		cfg := &AppConfig{Server: ServerConfig{Update: UpdateConfig{Branch: "daily"}}}
		validateServerUpdate(cfg)
		if cfg.Server.Update.Branch != "devel" {
			t.Fatalf("branch = %q, want devel", cfg.Server.Update.Branch)
		}
	})
	t.Run("resets invalid values", func(t *testing.T) {
		cfg := &AppConfig{Server: ServerConfig{Update: UpdateConfig{Branch: "unknown", DeferDays: 366}}}
		validateServerUpdate(cfg)
		if cfg.Server.Update.Branch != "stable" || cfg.Server.Update.DeferDays != 0 {
			t.Fatalf("update config = %#v, want stable with zero defer days", cfg.Server.Update)
		}
	})
}

func TestGetConfigPath(t *testing.T) {
	got := getConfigPath()
	if os.Geteuid() == 0 {
		if got != "/etc/webappsgo/wthr/server.yml" {
			t.Errorf("getConfigPath() as root = %q, want /etc/webappsgo/wthr/server.yml", got)
		}
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		if got != "server.yml" {
			t.Errorf("getConfigPath() without home = %q, want server.yml", got)
		}
		return
	}

	want := filepath.Join(home, ".config", "webappsgo", "wthr", "server.yml")
	if got != want {
		t.Errorf("getConfigPath() = %q, want %q", got, want)
	}
}

func TestFindConfigFile(t *testing.T) {
	t.Run("finds file via CONFIG_DIR", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		if err := os.WriteFile(path, []byte("server:\n  fqdn: example.com\n"), 0644); err != nil {
			t.Fatalf("failed to write test config: %v", err)
		}

		oldConfigDir := os.Getenv("CONFIG_DIR")
		os.Setenv("CONFIG_DIR", dir)
		t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

		got := findConfigFile()
		if got != path {
			t.Errorf("findConfigFile() = %q, want %q", got, path)
		}
	})

	t.Run("CONFIG_DIR set but file missing does not use it", func(t *testing.T) {
		dir := t.TempDir()

		oldConfigDir := os.Getenv("CONFIG_DIR")
		os.Setenv("CONFIG_DIR", dir)
		t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

		got := findConfigFile()
		missing := filepath.Join(dir, "server.yml")
		if got == missing {
			t.Errorf("findConfigFile() = %q, should not return nonexistent path", got)
		}
	})

	t.Run("migrates server.yaml to server.yml via standard path", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: standard path is /etc/webappsgo/wthr, skipping to avoid host side effects")
		}

		dir := t.TempDir()
		stdDir := filepath.Join(dir, ".config", "webappsgo", "wthr")
		if err := os.MkdirAll(stdDir, 0755); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		yamlPath := filepath.Join(stdDir, "server.yaml")
		if err := os.WriteFile(yamlPath, []byte("server:\n  fqdn: legacy.example.com\n"), 0644); err != nil {
			t.Fatalf("failed to write legacy config: %v", err)
		}

		oldHome := os.Getenv("HOME")
		oldConfigDir := os.Getenv("CONFIG_DIR")
		os.Setenv("HOME", dir)
		os.Unsetenv("CONFIG_DIR")
		t.Cleanup(func() {
			os.Setenv("HOME", oldHome)
			os.Setenv("CONFIG_DIR", oldConfigDir)
		})

		got := findConfigFile()
		want := filepath.Join(stdDir, "server.yml")
		if got != want {
			t.Errorf("findConfigFile() = %q, want %q (migrated)", got, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected server.yml to exist after migration: %v", err)
		}
		if _, err := os.Stat(yamlPath); !os.IsNotExist(err) {
			t.Errorf("expected server.yaml to be removed after migration, stat err = %v", err)
		}
	})
}

func TestCreateDefaultConfig(t *testing.T) {
	t.Run("creates config file with header", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "server.yml")

		cfg := &AppConfig{Server: ServerConfig{FQDN: "example.com", Mode: "production"}}
		if err := createDefaultConfig(cfg, path); err != nil {
			t.Fatalf("createDefaultConfig() unexpected error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read created config: %v", err)
		}
		if len(data) == 0 {
			t.Error("created config file is empty")
		}
		if !containsBytes(data, []byte("Weather Configuration")) {
			t.Error("created config file missing header comment")
		}
		if !containsBytes(data, []byte("example.com")) {
			t.Error("created config file missing marshalled fqdn value")
		}
	})

	t.Run("fails when path is unwritable directory", func(t *testing.T) {
		dir := t.TempDir()
		// Create a file where a directory is expected so MkdirAll fails.
		blocker := filepath.Join(dir, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		badPath := filepath.Join(blocker, "sub", "server.yml")

		cfg := &AppConfig{}
		if err := createDefaultConfig(cfg, badPath); err == nil {
			t.Error("createDefaultConfig() expected error for unwritable path, got nil")
		}
	})
}

func containsBytes(haystack, needle []byte) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexBytes(haystack, needle) >= 0)
}

func indexBytes(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestLoadConfig_ExistingValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	yamlContent := `
server:
  fqdn: weather.example.com
  mode: production
users:
  enabled: true
  registration:
    mode: open
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("CONFIG_DIR", dir)
	t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}
	if cfg.Server.FQDN != "weather.example.com" {
		t.Errorf("LoadConfig() FQDN = %q, want weather.example.com", cfg.Server.FQDN)
	}
	if cfg.Users.Registration.Mode != "open" {
		t.Errorf("LoadConfig() registration mode = %q, want open", cfg.Users.Registration.Mode)
	}
}

// AI.md PART 12: an unknown key is not a startup failure - the known keys
// still apply and the rest of the config falls back to its defaults.
func TestLoadConfig_UnknownFieldIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	yamlContent := `
server:
  fqdn: weather.example.com
not_a_real_field: true
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("CONFIG_DIR", dir)
	t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}
	if cfg.Server.FQDN != "weather.example.com" {
		t.Errorf("LoadConfig() FQDN = %q, want weather.example.com", cfg.Server.FQDN)
	}
}

// AI.md PART 12: "Never crash on config - server must start with sane
// defaults." A file that cannot be parsed warns and yields the defaults.
func TestLoadConfig_MalformedYAMLFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	// Invalid YAML: mismatched indentation / broken mapping.
	yamlContent := "server:\n  fqdn: [unterminated\n"
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("CONFIG_DIR", dir)
	t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() must not fail on malformed YAML, got: %v", err)
	}
	if cfg.Server.AdminPath != "administration" {
		t.Errorf("LoadConfig() admin_path = %q, want the default administration", cfg.Server.AdminPath)
	}
	if cfg.Server.Cache.Type != "memory" {
		t.Errorf("LoadConfig() cache.type = %q, want the default memory", cfg.Server.Cache.Type)
	}
}

func TestLoadConfig_DefaultsWhenNoFileFound(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: LoadConfig() would write to /etc/webappsgo/wthr, skipping to avoid host side effects")
	}

	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("HOME", dir)
	os.Unsetenv("CONFIG_DIR")
	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("CONFIG_DIR", oldConfigDir)
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() unexpected error: %v", err)
	}
	if cfg.Server.AdminPath != "administration" {
		t.Errorf("LoadConfig() default AdminPath = %q, want administration", cfg.Server.AdminPath)
	}
	if cfg.Users.Registration.Mode != "open" {
		t.Errorf("LoadConfig() default registration mode = %q, want open", cfg.Users.Registration.Mode)
	}

	created := filepath.Join(dir, ".config", "webappsgo", "wthr", "server.yml")
	if _, err := os.Stat(created); err != nil {
		t.Errorf("expected default config to be created at %q: %v", created, err)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  fqdn: placeholder.example.com\n"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("CONFIG_DIR", dir)
	t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

	cfg := &AppConfig{Server: ServerConfig{FQDN: "saved.example.com"}}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if !containsBytes(data, []byte("saved.example.com")) {
		t.Error("SaveConfig() did not persist the fqdn value")
	}
}

func TestSetGlobalConfig_GetGlobalConfig(t *testing.T) {
	cfg := &AppConfig{Server: ServerConfig{FQDN: "global.example.com"}}
	withGlobalConfig(t, cfg)

	got := GetGlobalConfig()
	if got != cfg {
		t.Error("GetGlobalConfig() did not return the config set by SetGlobalConfig()")
	}
}

func TestUpdateWebRobotsTxt(t *testing.T) {
	t.Run("errors when global config not initialized", func(t *testing.T) {
		withGlobalConfig(t, nil)
		if err := UpdateWebRobotsTxt("User-agent: *"); err == nil {
			t.Error("UpdateWebRobotsTxt() expected error when global config is nil")
		}
	})

	t.Run("updates and persists robots.txt", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		if err := os.WriteFile(path, []byte("server:\n  fqdn: example.com\n"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		oldConfigDir := os.Getenv("CONFIG_DIR")
		os.Setenv("CONFIG_DIR", dir)
		t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

		cfg := &AppConfig{}
		withGlobalConfig(t, cfg)

		if err := UpdateWebRobotsTxt("User-agent: *\nDisallow:\n"); err != nil {
			t.Fatalf("UpdateWebRobotsTxt() unexpected error: %v", err)
		}
		if cfg.Web.RobotsTxt != "User-agent: *\nDisallow:\n" {
			t.Errorf("Web.RobotsTxt = %q, not updated", cfg.Web.RobotsTxt)
		}
	})
}

func TestUpdateWebSecurityTxt(t *testing.T) {
	t.Run("errors when global config not initialized", func(t *testing.T) {
		withGlobalConfig(t, nil)
		if err := UpdateWebSecurityTxt("Contact: mailto:security@example.com"); err == nil {
			t.Error("UpdateWebSecurityTxt() expected error when global config is nil")
		}
	})

	t.Run("updates and persists security.txt", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "server.yml")
		if err := os.WriteFile(path, []byte("server:\n  fqdn: example.com\n"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		oldConfigDir := os.Getenv("CONFIG_DIR")
		os.Setenv("CONFIG_DIR", dir)
		t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })

		cfg := &AppConfig{}
		withGlobalConfig(t, cfg)

		content := "Contact: mailto:security@example.com"
		if err := UpdateWebSecurityTxt(content); err != nil {
			t.Fatalf("UpdateWebSecurityTxt() unexpected error: %v", err)
		}
		if cfg.Web.SecurityTxt != content {
			t.Errorf("Web.SecurityTxt = %q, not updated", cfg.Web.SecurityTxt)
		}
	})
}

func TestIsMultiUserEnabled(t *testing.T) {
	t.Run("nil global config defaults true", func(t *testing.T) {
		withGlobalConfig(t, nil)
		if !IsMultiUserEnabled() {
			t.Error("IsMultiUserEnabled() = false, want true when global config is nil")
		}
	})

	t.Run("respects configured value true", func(t *testing.T) {
		withGlobalConfig(t, &AppConfig{Users: UsersConfig{Enabled: true}})
		if !IsMultiUserEnabled() {
			t.Error("IsMultiUserEnabled() = false, want true")
		}
	})

	t.Run("respects configured value false", func(t *testing.T) {
		withGlobalConfig(t, &AppConfig{Users: UsersConfig{Enabled: false}})
		if IsMultiUserEnabled() {
			t.Error("IsMultiUserEnabled() = true, want false")
		}
	})
}

func TestGetRegistrationMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AppConfig
		want string
	}{
		{"nil config defaults to open", nil, "open"},
		{"unset mode defaults to open", &AppConfig{}, "open"},
		{"unknown value defaults to open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "bogus"}}}, "open"},
		{"canonical open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "open"}}}, "open"},
		{"canonical private", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "private"}}}, "private"},
		{"undefined invite falls back to open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "invite"}}}, "open"},
		{"undefined disabled falls back to open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "disabled"}}}, "open"},
		{"undefined admin_only falls back to open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "admin_only"}}}, "open"},
		{"legacy public falls back to open", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: "public"}}}, "open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGlobalConfig(t, tt.cfg)
			if got := GetRegistrationMode(); got != tt.want {
				t.Errorf("GetRegistrationMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetUserInviteExpirationDays(t *testing.T) {
	tests := []struct {
		name string
		cfg  *AppConfig
		want int
	}{
		{"nil config defaults to 7", nil, 7},
		{"zero value defaults to 7", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{InviteExpirationDays: 0}}}, 7},
		{"negative value defaults to 7", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{InviteExpirationDays: -5}}}, 7},
		{"positive value kept", &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{InviteExpirationDays: 14}}}, 14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGlobalConfig(t, tt.cfg)
			if got := GetUserInviteExpirationDays(); got != tt.want {
				t.Errorf("GetUserInviteExpirationDays() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRegistrationModeQueries(t *testing.T) {
	tests := []struct {
		mode        string
		wantOpen    bool
		wantPrivate bool
	}{
		{"open", true, false},
		{"private", false, true},
		{"invite", true, false},
		{"admin_only", true, false},
		{"disabled", true, false},
		{"public", true, false},
		{"", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			withGlobalConfig(t, &AppConfig{Users: UsersConfig{Registration: RegistrationConfig{Mode: tt.mode}}})

			if got := IsRegistrationOpen(); got != tt.wantOpen {
				t.Errorf("IsRegistrationOpen() = %v, want %v", got, tt.wantOpen)
			}
			if got := IsRegistrationPrivate(); got != tt.wantPrivate {
				t.Errorf("IsRegistrationPrivate() = %v, want %v", got, tt.wantPrivate)
			}
		})
	}
}

// TestDefaultI2PConfig verifies the opt-in defaults per AI.md PART 32.2
func TestDefaultI2PConfig(t *testing.T) {
	cfg := DefaultI2PConfig()
	if cfg.Enabled {
		t.Error("DefaultI2PConfig().Enabled must be false: I2P is strictly opt-in")
	}
	if cfg.SAMAddress != "127.0.0.1:7656" {
		t.Errorf("SAMAddress = %q, want 127.0.0.1:7656", cfg.SAMAddress)
	}
	if cfg.VirtualPort != 80 || cfg.SignatureType != 7 || cfg.BootstrapTimeout != 300 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.InboundLength != 3 || cfg.OutboundLength != 3 {
		t.Errorf("tunnel lengths = %d/%d, want 3/3", cfg.InboundLength, cfg.OutboundLength)
	}
	if cfg.InboundQuantity != 5 || cfg.OutboundQuantity != 5 {
		t.Errorf("tunnel quantities = %d/%d, want 5/5", cfg.InboundQuantity, cfg.OutboundQuantity)
	}
	if errs := ValidateI2PConfig(&cfg); len(errs) != 0 {
		t.Errorf("ValidateI2PConfig(defaults) = %v, want no errors", errs)
	}
}

// TestValidateI2PConfig covers every documented boundary per AI.md PART 32.2
func TestValidateI2PConfig(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*I2PConfig)
		wantField string
	}{
		{"virtual port zero", func(c *I2PConfig) { c.VirtualPort = 0 }, "virtual_port"},
		{"virtual port too high", func(c *I2PConfig) { c.VirtualPort = 65536 }, "virtual_port"},
		{"inbound length too high", func(c *I2PConfig) { c.InboundLength = 8 }, "inbound_length"},
		{"inbound length negative", func(c *I2PConfig) { c.InboundLength = -1 }, "inbound_length"},
		{"outbound length too high", func(c *I2PConfig) { c.OutboundLength = 8 }, "outbound_length"},
		{"inbound quantity zero", func(c *I2PConfig) { c.InboundQuantity = 0 }, "inbound_quantity"},
		{"outbound quantity too high", func(c *I2PConfig) { c.OutboundQuantity = 17 }, "outbound_quantity"},
		{"signature type unsupported", func(c *I2PConfig) { c.SignatureType = 3 }, "signature_type"},
		{"bootstrap timeout too low", func(c *I2PConfig) { c.BootstrapTimeout = 29 }, "bootstrap_timeout"},
		{"bootstrap timeout too high", func(c *I2PConfig) { c.BootstrapTimeout = 601 }, "bootstrap_timeout"},
		{"sam address missing port", func(c *I2PConfig) { c.SAMAddress = "127.0.0.1" }, "sam_address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultI2PConfig()
			tt.mutate(&cfg)
			errs := ValidateI2PConfig(&cfg)
			if len(errs) == 0 {
				t.Fatalf("ValidateI2PConfig() accepted an invalid %s", tt.wantField)
			}
			found := false
			for _, e := range errs {
				if e.Field == tt.wantField {
					found = true
				}
				if e.Error() == "" {
					t.Error("ValidationError.Error() returned an empty string")
				}
			}
			if !found {
				t.Errorf("ValidateI2PConfig() = %v, want an error on field %q", errs, tt.wantField)
			}
		})
	}

	// Signature type 0 is the documented legacy alternative and must pass
	legacy := DefaultI2PConfig()
	legacy.SignatureType = 0
	if errs := ValidateI2PConfig(&legacy); len(errs) != 0 {
		t.Errorf("ValidateI2PConfig(signature_type=0) = %v, want no errors", errs)
	}

	if errs := ValidateI2PConfig(nil); len(errs) == 0 {
		t.Error("ValidateI2PConfig(nil) must report an error")
	}
}

// TestNormalizeI2PConfig verifies AI.md PART 5's warn-and-default behavior:
// a bad value is replaced, never a startup failure
func TestNormalizeI2PConfig(t *testing.T) {
	cfg := I2PConfig{
		Enabled:          true,
		SAMAddress:       "   ",
		VirtualPort:      0,
		InboundLength:    99,
		OutboundLength:   -4,
		InboundQuantity:  0,
		OutboundQuantity: 900,
		SignatureType:    42,
		BootstrapTimeout: 5,
	}
	NormalizeI2PConfig(&cfg)

	defaults := DefaultI2PConfig()
	if !cfg.Enabled {
		t.Error("NormalizeI2PConfig() must not clear an explicitly enabled flag")
	}
	if cfg.SAMAddress != defaults.SAMAddress {
		t.Errorf("SAMAddress = %q, want %q", cfg.SAMAddress, defaults.SAMAddress)
	}
	if errs := ValidateI2PConfig(&cfg); len(errs) != 0 {
		t.Errorf("normalized config still invalid: %v", errs)
	}

	// A nil pointer must be a no-op rather than a panic
	NormalizeI2PConfig(nil)
}

// TestUpdateI2PConfigRejectsInvalid verifies invalid updates never reach disk
func TestUpdateI2PConfigRejectsInvalid(t *testing.T) {
	seeded := &AppConfig{}
	seeded.Server.I2P = DefaultI2PConfig()
	withGlobalConfig(t, seeded)
	want := DefaultI2PConfig().VirtualPort
	bad := DefaultI2PConfig()
	bad.VirtualPort = 0
	errs := UpdateI2PConfig(bad)
	if len(errs) == 0 {
		t.Fatal("UpdateI2PConfig() accepted an invalid virtual_port")
	}
	if GetGlobalConfig().Server.I2P.VirtualPort != want {
		t.Error("UpdateI2PConfig() mutated the global config despite rejecting the update")
	}
}

func TestApplyEnvOverridesI2P(t *testing.T) {
	cfg := &AppConfig{}
	t.Setenv("I2P_ENABLED", "yes")
	applyEnvOverrides(cfg)
	if !cfg.Server.I2P.Enabled || !cfg.Server.Features.I2P {
		t.Error("I2P_ENABLED=yes must enable the eepsite")
	}
	t.Setenv("I2P_ENABLED", "off")
	applyEnvOverrides(cfg)
	if cfg.Server.I2P.Enabled {
		t.Error("I2P_ENABLED=off must disable the eepsite")
	}
	t.Setenv("I2P_ENABLED", "")
	cfg.Server.I2P.Enabled = true
	applyEnvOverrides(cfg)
	if !cfg.Server.I2P.Enabled {
		t.Error("an unset I2P_ENABLED must leave server.i2p.enabled alone")
	}
}
