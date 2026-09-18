package config

import "testing"

// validConfig returns an AppConfig whose PART 12 trees are all at their defaults
func validConfig() *AppConfig {
	cfg := &AppConfig{}
	cfg.Server.Port = 8080
	cfg.Server.BaseURL = "/"
	cfg.Server.Mode = "production"
	cfg.Server.AdminPath = "admin"
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
	if cfg.Server.AdminPath != "admin" {
		t.Errorf("admin_path = %q, want admin", cfg.Server.AdminPath)
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
}

func TestValidateConfigI18nFallsBackToEnglish(t *testing.T) {
	cfg := validConfig()
	cfg.Server.I18n.DefaultLanguage = "klingon"
	cfg.Server.I18n.Supported = []string{"es", "fr"}

	validateConfig(cfg)

	if cfg.Server.I18n.DefaultLanguage != "en" {
		t.Errorf("default_language = %q, want en", cfg.Server.I18n.DefaultLanguage)
	}

	found := false
	for _, lang := range cfg.Server.I18n.Supported {
		if lang == "en" {
			found = true
		}
	}
	if !found {
		t.Error("en must be added to i18n.supported when it becomes the default")
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
