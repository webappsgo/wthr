package config

import (
	"testing"
	"time"
)

// validConfig returns an AppConfig whose PART 12 trees are all at their defaults
func validConfig() *AppConfig {
	cfg := &AppConfig{}
	cfg.Server.Port = 8080
	cfg.Server.BaseURL = "/"
	cfg.Server.Mode = "production"
	cfg.Server.AdminPath = "administration"
	cfg.Server.APIVersion = "v1"
	cfg.Server.Limits = DefaultLimitsConfig()
	cfg.Server.Compression = DefaultCompressionConfig()
	cfg.Server.Session = DefaultSessionConfig()
	cfg.Server.RateLimit = DefaultRateLimitConfig()
	cfg.Server.I18n = DefaultI18nConfig()
	cfg.Server.Cache = DefaultCacheConfig()
	return cfg
}

func TestValidateConfigLeavesValidValuesAlone(t *testing.T) {
	cfg := validConfig()
	validateConfig(cfg)

	if cfg.Server.Port != 8080 {
		t.Errorf("port = %v, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Mode != "production" {
		t.Errorf("mode = %q, want production", cfg.Server.Mode)
	}
	if cfg.Server.Compression.Level != 5 {
		t.Errorf("compression.level = %d, want 5", cfg.Server.Compression.Level)
	}
	if cfg.Server.Cache.Type != "memory" {
		t.Errorf("cache.type = %q, want memory", cfg.Server.Cache.Type)
	}
}

func TestValidateConfigReplacesInvalidPort(t *testing.T) {
	for _, bad := range []interface{}{-1, 70000, "not-a-port", "1,2,3", nil, 3.5} {
		cfg := validConfig()
		cfg.Server.Port = bad

		validateConfig(cfg)

		port, ok := cfg.Server.Port.(int)
		if !ok {
			t.Fatalf("port %v: got %T, want a replacement int", bad, cfg.Server.Port)
		}
		if port < 64000 || port > 64999 {
			t.Errorf("port %v: replacement %d outside 64000-64999", bad, port)
		}
	}
}

// AI.md PART 4: a substituted port must be reported so the caller persists it
func TestValidateConfigReportsPortReplacement(t *testing.T) {
	cfg := validConfig()
	if validateConfig(cfg) {
		t.Error("validateConfig() reported a port replacement for a valid port")
	}

	cfg = validConfig()
	cfg.Server.Port = "nonsense"
	if !validateConfig(cfg) {
		t.Error("validateConfig() did not report the replaced port")
	}
}

func TestValidateConfigAcceptsDualPortString(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Port = "8090,8443"

	validateConfig(cfg)

	if cfg.Server.Port != "8090,8443" {
		t.Errorf("port = %v, want the dual-port string preserved", cfg.Server.Port)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/"},
		{"   ", "/"},
		{"/", "/"},
		{"///", "/"},
		{"/myproject", "/myproject"},
		{"/myproject/", "/myproject"},
		{"myproject", "/myproject"},
		{"/api/v2/", "/api/v2"},
	}

	for _, tt := range tests {
		if got := normalizeBaseURL(tt.in); got != tt.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateConfigReplacesInvalidIdentity(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Mode = "banana"
	cfg.Server.AdminPath = ""
	cfg.Server.APIVersion = "v1/extra"

	validateConfig(cfg)

	if cfg.Server.Mode != "production" {
		t.Errorf("mode = %q, want production", cfg.Server.Mode)
	}
	if cfg.Server.AdminPath != "administration" {
		t.Errorf("admin_path = %q, want administration", cfg.Server.AdminPath)
	}
	if cfg.Server.APIVersion != "v1" {
		t.Errorf("api_version = %q, want v1", cfg.Server.APIVersion)
	}
}

func TestValidateConfigReplacesInvalidLimits(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Limits.ReadTimeout = ""
	cfg.Server.Limits.MaxBodySize = "  "
	cfg.Server.Compression.Level = 99
	cfg.Server.Compression.Types = nil

	validateConfig(cfg)

	defaults := DefaultLimitsConfig()
	if cfg.Server.Limits.ReadTimeout != defaults.ReadTimeout {
		t.Errorf("read_timeout = %q, want %q", cfg.Server.Limits.ReadTimeout, defaults.ReadTimeout)
	}
	if cfg.Server.Limits.MaxBodySize != defaults.MaxBodySize {
		t.Errorf("max_body_size = %q, want %q", cfg.Server.Limits.MaxBodySize, defaults.MaxBodySize)
	}
	if cfg.Server.Compression.Level != 5 {
		t.Errorf("compression.level = %d, want 5", cfg.Server.Compression.Level)
	}
	if len(cfg.Server.Compression.Types) == 0 {
		t.Error("compression.types was not restored to the defaults")
	}
}

func TestValidateConfigSessionAlwaysHTTPOnly(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Session.HTTPOnly = false
	cfg.Server.Session.SameSite = "sometimes"
	cfg.Server.Session.Secure = "maybe"
	cfg.Server.Session.Admin.CookieName = ""
	cfg.Server.Session.User.MaxAge = ""

	validateConfig(cfg)

	defaults := DefaultSessionConfig()
	if !cfg.Server.Session.HTTPOnly {
		t.Error("session.http_only must always be true")
	}
	if cfg.Server.Session.SameSite != defaults.SameSite {
		t.Errorf("session.same_site = %q, want %q", cfg.Server.Session.SameSite, defaults.SameSite)
	}
	if cfg.Server.Session.Secure != defaults.Secure {
		t.Errorf("session.secure = %q, want %q", cfg.Server.Session.Secure, defaults.Secure)
	}
	if cfg.Server.Session.Admin.CookieName != defaults.Admin.CookieName {
		t.Errorf("session.admin.cookie_name = %q, want %q",
			cfg.Server.Session.Admin.CookieName, defaults.Admin.CookieName)
	}
	if cfg.Server.Session.User.MaxAge != defaults.User.MaxAge {
		t.Errorf("session.user.max_age = %q, want %q", cfg.Server.Session.User.MaxAge, defaults.User.MaxAge)
	}
}

func TestValidateConfigReplacesInvalidRateLimits(t *testing.T) {
	cfg := validConfig()
	cfg.Server.RateLimit.Read.Requests = 0
	cfg.Server.RateLimit.Write.Window = -1
	cfg.Server.RateLimit.Auth.Login.Requests = -5
	cfg.Server.RateLimit.GlobalBurst = 0
	maxDurationSeconds := int64((time.Duration(1<<63 - 1)) / time.Second)
	windowOverflowAvailable := int64(int(^uint(0)>>1)) > maxDurationSeconds
	if windowOverflowAvailable {
		cfg.Server.RateLimit.Health.Window = int(maxDurationSeconds + 1)
	}

	validateConfig(cfg)

	defaults := DefaultRateLimitConfig()
	if cfg.Server.RateLimit.Read.Requests != defaults.Read.Requests {
		t.Errorf("read.requests = %d, want %d", cfg.Server.RateLimit.Read.Requests, defaults.Read.Requests)
	}
	if cfg.Server.RateLimit.Write.Window != defaults.Write.Window {
		t.Errorf("write.window = %d, want %d", cfg.Server.RateLimit.Write.Window, defaults.Write.Window)
	}
	if cfg.Server.RateLimit.Auth.Login.Requests != defaults.Auth.Login.Requests {
		t.Errorf("auth.login.requests = %d, want %d",
			cfg.Server.RateLimit.Auth.Login.Requests, defaults.Auth.Login.Requests)
	}
	if cfg.Server.RateLimit.GlobalBurst != defaults.GlobalBurst {
		t.Errorf("global_burst = %d, want %d", cfg.Server.RateLimit.GlobalBurst, defaults.GlobalBurst)
	}
	if windowOverflowAvailable && cfg.Server.RateLimit.Health.Window != defaults.Health.Window {
		t.Errorf("health.window = %d, want %d", cfg.Server.RateLimit.Health.Window, defaults.Health.Window)
	}
}

func TestValidateConfigI18nFallsBackToEnglish(t *testing.T) {
	cfg := validConfig()
	cfg.Server.I18n.DefaultLanguage = "klingon"
	cfg.Server.I18n.AvailableLanguages = []string{"es", "fr"}

	validateConfig(cfg)

	if cfg.Server.I18n.DefaultLanguage != "en" {
		t.Errorf("default_language = %q, want en", cfg.Server.I18n.DefaultLanguage)
	}

	found := false
	for _, lang := range cfg.Server.I18n.AvailableLanguages {
		if lang == "en" {
			found = true
		}
	}
	if !found {
		t.Error("en must be restored in i18n.available_languages (required minimum)")
	}
}

func TestValidateConfigTracking(t *testing.T) {
	tests := []struct {
		name     string
		tracking TrackingConfig
		want     string
	}{
		{"unset stays off", TrackingConfig{}, ""},
		{"none stays off", TrackingConfig{Type: "none"}, ""},
		{"unknown platform disabled", TrackingConfig{Type: "spyware", ID: "x"}, ""},
		{"missing id disabled", TrackingConfig{Type: "plausible"}, ""},
		{"missing url disabled", TrackingConfig{Type: "matomo", ID: "1"}, ""},
		{"valid without url kept", TrackingConfig{Type: "Plausible", ID: "example.com"}, "plausible"},
		{"valid with url kept", TrackingConfig{Type: "matomo", ID: "1", URL: "https://m.example.com"}, "matomo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.Tracking = tt.tracking

			validateConfig(cfg)

			if cfg.Server.Tracking.Type != tt.want {
				t.Errorf("tracking.type = %q, want %q", cfg.Server.Tracking.Type, tt.want)
			}
		})
	}
}

func TestValidateConfigReplacesInvalidCache(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Cache.Type = "memcached"
	cfg.Server.Cache.Port = 0
	cfg.Server.Cache.PoolSize = 0
	cfg.Server.Cache.MinIdle = 500
	cfg.Server.Cache.Timeout = ""
	cfg.Server.Cache.TTL = ""
	cfg.Server.Cache.Prefix = ""

	validateConfig(cfg)

	defaults := DefaultCacheConfig()
	if cfg.Server.Cache.Type != defaults.Type {
		t.Errorf("cache.type = %q, want %q", cfg.Server.Cache.Type, defaults.Type)
	}
	if cfg.Server.Cache.Port != defaults.Port {
		t.Errorf("cache.port = %d, want %d", cfg.Server.Cache.Port, defaults.Port)
	}
	if cfg.Server.Cache.PoolSize != defaults.PoolSize {
		t.Errorf("cache.pool_size = %d, want %d", cfg.Server.Cache.PoolSize, defaults.PoolSize)
	}
	if cfg.Server.Cache.MinIdle != defaults.MinIdle {
		t.Errorf("cache.min_idle = %d, want %d", cfg.Server.Cache.MinIdle, defaults.MinIdle)
	}
	if cfg.Server.Cache.Timeout != defaults.Timeout {
		t.Errorf("cache.timeout = %q, want %q", cfg.Server.Cache.Timeout, defaults.Timeout)
	}
	if cfg.Server.Cache.TTL != defaults.TTL {
		t.Errorf("cache.ttl = %q, want %q", cfg.Server.Cache.TTL, defaults.TTL)
	}
	if cfg.Server.Cache.Prefix != defaults.Prefix {
		t.Errorf("cache.prefix = %q, want %q", cfg.Server.Cache.Prefix, defaults.Prefix)
	}
}

func TestValidateConfigNormalizesCacheTypeCase(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Cache.Type = "  Valkey  "

	validateConfig(cfg)

	if cfg.Server.Cache.Type != "valkey" {
		t.Errorf("cache.type = %q, want valkey", cfg.Server.Cache.Type)
	}
}

func TestValidateConfigAcceptsDisabledCache(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Cache.Type = " NONE "

	validateConfig(cfg)

	if cfg.Server.Cache.Type != "none" {
		t.Fatalf("cache.type = %q, want none", cfg.Server.Cache.Type)
	}
}

func TestValidateConfigReplacesInvalidCacheConnectionSettings(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Cache.Timeout = "-1s"
	cfg.Server.Cache.TTL = "not-a-duration"
	cfg.Server.Cache.URL = "http://cache.example"
	cfg.Server.Cache.DB = -1
	cfg.Server.Cache.Cluster = true
	cfg.Server.Cache.ClusterNodes = []string{"invalid-node"}

	validateConfig(cfg)

	defaults := DefaultCacheConfig()
	if cfg.Server.Cache.Timeout != defaults.Timeout || cfg.Server.Cache.TTL != defaults.TTL {
		t.Fatalf("invalid durations not replaced: timeout=%q ttl=%q", cfg.Server.Cache.Timeout, cfg.Server.Cache.TTL)
	}
	if cfg.Server.Cache.URL != defaults.URL || cfg.Server.Cache.DB != defaults.DB {
		t.Fatalf("invalid URL/db not replaced: url=%q db=%d", cfg.Server.Cache.URL, cfg.Server.Cache.DB)
	}
	if cfg.Server.Cache.Cluster || len(cfg.Server.Cache.ClusterNodes) != 0 {
		t.Fatalf("invalid cluster nodes not replaced: cluster=%v nodes=%v", cfg.Server.Cache.Cluster, cfg.Server.Cache.ClusterNodes)
	}
}

func TestIsValidAdminPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"administration", true},
		{"my-admin", true},
		{"a1", true},
		{"a", false},
		{"-admin-", false},
		{"admin-", false},
		{"Admin", false},
		{"my admin", false},
		{"a/b", false},
		{"api", false},
		{"security", false},
		{"", false},
		{"abcdefghijklmnopqrstuvwxyz0123456789", false},
	}
	for _, c := range cases {
		if got := IsValidAdminPath(c.in); got != c.want {
			t.Errorf("IsValidAdminPath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
