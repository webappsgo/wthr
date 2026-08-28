package config

import (
	"os"
	"testing"
)

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name       string
		configMode string
		envVars    map[string]string
		want       ModeString
	}{
		{
			name:       "config development",
			configMode: "development",
			want:       ModeDevelopment,
		},
		{
			name:       "config dev",
			configMode: "dev",
			want:       ModeDevelopment,
		},
		{
			name:       "config production",
			configMode: "production",
			want:       ModeProduction,
		},
		{
			name:       "config prod",
			configMode: "prod",
			want:       ModeProduction,
		},
		{
			name:       "empty defaults to production",
			configMode: "",
			want:       ModeProduction,
		},
		{
			name:       "env MODE development",
			configMode: "",
			envVars:    map[string]string{"MODE": "development"},
			want:       ModeDevelopment,
		},
		{
			name:       "env APP_MODE production",
			configMode: "",
			envVars:    map[string]string{"APP_MODE": "production"},
			want:       ModeProduction,
		},
		{
			name:       "env takes precedence over config",
			configMode: "production",
			envVars:    map[string]string{"MODE": "development"},
			want:       ModeDevelopment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars
			os.Unsetenv("MODE")
			os.Unsetenv("APP_MODE")
			os.Unsetenv("ENVIRONMENT")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got := DetectMode(tt.configMode)
			if got != tt.want {
				t.Errorf("DetectMode() = %v, want %v", got, tt.want)
			}

			// Cleanup
			for k := range tt.envVars {
				os.Unsetenv(k)
			}
		})
	}
}

func TestIsValidFQDN(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		// Valid FQDNs
		{"example.com", true},
		{"www.example.com", true},
		{"api.weather.example.com", true},
		{"test-server.example.com", true},
		{"server1.example.co.uk", true},

		// Invalid: localhost
		{"localhost", false},
		{"localhost.localdomain", false},

		// Invalid: IP addresses
		{"127.0.0.1", false},
		{"192.168.1.1", false},
		{"::1", false},
		{"2001:db8::1", false},

		// Invalid: no TLD
		{"example", false},
		{"server", false},

		// Invalid: format issues
		{"", false},
		{".example.com", false},
		{"example.com.", false},
		{"exam ple.com", false},
		{"example..com", false},
		{"-example.com", false},
		{"example-.com", false},

		// Invalid: numeric TLD
		{"example.123", false},

		// Edge cases
		{"a.b", true},
		{"very-long-subdomain-name-that-is-still-valid.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isValidFQDN(tt.host)
			if got != tt.want {
				t.Errorf("isValidFQDN(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"Localhost", true},
		{"localhost.localdomain", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"127.1.2.3", true},
		{"::1", true},
		{"example.com", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isLocalhost(tt.host)
			if got != tt.want {
				t.Errorf("isLocalhost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestIsIPAddress(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"localhost", false},
		{"example.com", false},
		{"not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isIPAddress(tt.host)
			if got != tt.want {
				t.Errorf("isIPAddress(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestNewModeConfig_Production(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		wantError bool
	}{
		{
			name:      "valid FQDN",
			host:      "weather.example.com",
			port:      8080,
			wantError: false,
		},
		{
			name:      "localhost rejected in production",
			host:      "localhost",
			port:      8080,
			wantError: true,
		},
		{
			name:      "IP address rejected in production",
			host:      "192.168.1.1",
			port:      8080,
			wantError: true,
		},
		{
			name:      "invalid FQDN rejected",
			host:      "invalid_host",
			port:      8080,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc, err := NewModeConfig(ModeProduction, tt.host, tt.port)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewModeConfig() expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewModeConfig() unexpected error: %v", err)
				}
				if mc == nil {
					t.Errorf("NewModeConfig() returned nil config")
					return
				}
				if !mc.IsProduction {
					t.Errorf("IsProduction = false, want true")
				}
				if mc.RequireFQDN != true {
					t.Errorf("RequireFQDN = false, want true")
				}
			}
		})
	}
}

func TestNewModeConfig_Development(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
	}{
		{
			name: "localhost allowed in development",
			host: "localhost",
			port: 8080,
		},
		{
			name: "IP allowed in development",
			host: "127.0.0.1",
			port: 8080,
		},
		{
			name: "valid FQDN also works",
			host: "weather.example.com",
			port: 8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc, err := NewModeConfig(ModeDevelopment, tt.host, tt.port)
			if err != nil {
				t.Errorf("NewModeConfig() unexpected error: %v", err)
			}
			if mc == nil {
				t.Errorf("NewModeConfig() returned nil config")
				return
			}
			if !mc.IsDevelopment {
				t.Errorf("IsDevelopment = false, want true")
			}
			if mc.RequireFQDN != false {
				t.Errorf("RequireFQDN = true, want false")
			}
			if mc.AllowInsecure != true {
				t.Errorf("AllowInsecure = false, want true")
			}
		})
	}
}

func TestModeConfig_GetLogLevel(t *testing.T) {
	prodConfig, _ := NewModeConfig(ModeProduction, "example.com", 8080)
	devConfig, _ := NewModeConfig(ModeDevelopment, "localhost", 8080)

	if prodConfig.GetLogLevel() != "warn" {
		t.Errorf("Production log level = %s, want warn", prodConfig.GetLogLevel())
	}

	if devConfig.GetLogLevel() != "debug" {
		t.Errorf("Development log level = %s, want debug", devConfig.GetLogLevel())
	}
}

func TestModeConfig_SecuritySettings(t *testing.T) {
	prodConfig, _ := NewModeConfig(ModeProduction, "example.com", 8080)
	devConfig, _ := NewModeConfig(ModeDevelopment, "localhost", 8080)

	// Production should have strict security
	if !prodConfig.GetSecurityHeaders() {
		t.Error("Production should enforce security headers")
	}
	if !prodConfig.GetRateLimitEnabled() {
		t.Error("Production should enable rate limiting")
	}
	if !prodConfig.ShouldValidateCSRF() {
		t.Error("Production should validate CSRF")
	}
	if !prodConfig.ShouldRequireHTTPS() {
		t.Error("Production should require HTTPS")
	}

	// Development should have relaxed security
	if devConfig.GetSecurityHeaders() {
		t.Error("Development should not enforce strict security headers")
	}
	if devConfig.GetRateLimitEnabled() {
		t.Error("Development should not enable rate limiting")
	}
	if devConfig.ShouldValidateCSRF() {
		t.Error("Development should not validate CSRF")
	}
	if devConfig.ShouldRequireHTTPS() {
		t.Error("Development should not require HTTPS")
	}
}

func TestModeStringString(t *testing.T) {
	if got := ModeDevelopment.String(); got != "development" {
		t.Errorf("ModeDevelopment.String() = %q, want %q", got, "development")
	}
	if got := ModeProduction.String(); got != "production" {
		t.Errorf("ModeProduction.String() = %q, want %q", got, "production")
	}
}

func TestModeStringValidate(t *testing.T) {
	if err := ModeDevelopment.Validate(); err != nil {
		t.Errorf("ModeDevelopment.Validate() error = %v, want nil", err)
	}
	if err := ModeProduction.Validate(); err != nil {
		t.Errorf("ModeProduction.Validate() error = %v, want nil", err)
	}
	if err := ModeString("bogus").Validate(); err == nil {
		t.Error("ModeString(\"bogus\").Validate() error = nil, want error")
	}
}

func TestGetCORSPolicy(t *testing.T) {
	prodConfig, err := NewModeConfig(ModeProduction, "example.com", 8080)
	if err != nil {
		t.Fatalf("NewModeConfig(production) error = %v", err)
	}
	devConfig, err := NewModeConfig(ModeDevelopment, "localhost", 8080)
	if err != nil {
		t.Fatalf("NewModeConfig(development) error = %v", err)
	}

	if got := prodConfig.GetCORSPolicy(); got != "strict" {
		t.Errorf("production GetCORSPolicy() = %q, want %q", got, "strict")
	}
	if got := devConfig.GetCORSPolicy(); got != "*" {
		t.Errorf("development GetCORSPolicy() = %q, want %q", got, "*")
	}
}

// TestPrintConfig only verifies PrintConfig runs without panicking - its
// output goes to stdout for startup logs/--status and has no return value
// to assert against.
func TestPrintConfig(t *testing.T) {
	prodConfig, err := NewModeConfig(ModeProduction, "example.com", 8080)
	if err != nil {
		t.Fatalf("NewModeConfig(production) error = %v", err)
	}
	prodConfig.PrintConfig()
}

func TestWarningMessage(t *testing.T) {
	prodConfig, err := NewModeConfig(ModeProduction, "example.com", 8080)
	if err != nil {
		t.Fatalf("NewModeConfig(production) error = %v", err)
	}
	if got := prodConfig.WarningMessage(); got != "" {
		t.Errorf("production WarningMessage() = %q, want empty", got)
	}

	devConfig, err := NewModeConfig(ModeDevelopment, "localhost", 8080)
	if err != nil {
		t.Fatalf("NewModeConfig(development) error = %v", err)
	}
	if got := devConfig.WarningMessage(); got == "" {
		t.Error("development WarningMessage() = empty, want a warning")
	}
}
