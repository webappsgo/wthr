package config

import "testing"

func TestDefaultLimitsConfig(t *testing.T) {
	got := DefaultLimitsConfig()
	want := LimitsConfig{
		MaxBodySize:  "10MB",
		ReadTimeout:  "30s",
		WriteTimeout: "30s",
		IdleTimeout:  "120s",
	}
	if got != want {
		t.Errorf("DefaultLimitsConfig() = %+v, want %+v", got, want)
	}
}

func TestDefaultCompressionConfig(t *testing.T) {
	got := DefaultCompressionConfig()
	if !got.Enabled {
		t.Error("compression must be enabled by default")
	}
	if got.Level != 5 {
		t.Errorf("level = %d, want 5", got.Level)
	}
	want := []string{"text/html", "text/css", "text/javascript", "application/json", "application/xml"}
	if len(got.Types) != len(want) {
		t.Fatalf("types = %v, want %v", got.Types, want)
	}
	for i := range want {
		if got.Types[i] != want[i] {
			t.Errorf("types[%d] = %q, want %q", i, got.Types[i], want[i])
		}
	}
}

func TestDefaultSessionConfig(t *testing.T) {
	got := DefaultSessionConfig()

	if got.Admin.MaxAge != "30d" {
		t.Errorf("admin.max_age = %q, want 30d", got.Admin.MaxAge)
	}
	if got.User.MaxAge != "7d" {
		t.Errorf("user.max_age = %q, want 7d", got.User.MaxAge)
	}
	if got.Admin.IdleTimeout != "24h" || got.User.IdleTimeout != "24h" {
		t.Errorf("idle timeouts = %q/%q, want 24h/24h", got.Admin.IdleTimeout, got.User.IdleTimeout)
	}
	if got.SameSite != "strict" {
		t.Errorf("same_site = %q, want strict", got.SameSite)
	}
	if got.Secure != "auto" {
		t.Errorf("secure = %q, want auto", got.Secure)
	}
	if !got.HTTPOnly {
		t.Error("http_only must default to true")
	}
}

func TestDefaultI18nConfig(t *testing.T) {
	got := DefaultI18nConfig()
	if got.DefaultLanguage != "en" {
		t.Errorf("default_language = %q, want en", got.DefaultLanguage)
	}
	if len(got.Supported) == 0 || got.Supported[0] != "en" {
		t.Errorf("supported = %v, want en included", got.Supported)
	}
}

func TestDefaultRateLimitConfigMatchesSpec(t *testing.T) {
	got := DefaultRateLimitConfig()

	tests := []struct {
		name   string
		bucket RateLimitBucket
		want   RateLimitBucket
	}{
		{"read", got.Read, RateLimitBucket{Requests: 120, Window: 60}},
		{"write", got.Write, RateLimitBucket{Requests: 10, Window: 60}},
		{"health", got.Health, RateLimitBucket{Requests: 120, Window: 60}},
		{"login", got.Auth.Login, RateLimitBucket{Requests: 5, Window: 900}},
		{"password_reset", got.Auth.PasswordReset, RateLimitBucket{Requests: 3, Window: 3600}},
		{"registration", got.Auth.Registration, RateLimitBucket{Requests: 5, Window: 3600}},
	}

	for _, tt := range tests {
		if tt.bucket != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.name, tt.bucket, tt.want)
		}
	}

	if got.GlobalBurst != 240 {
		t.Errorf("global_burst = %d, want 240", got.GlobalBurst)
	}
}

func TestDefaultCacheConfigIsMemory(t *testing.T) {
	got := DefaultCacheConfig()
	if got.Type != "memory" {
		t.Errorf("type = %q, want memory - memory must work standalone", got.Type)
	}
	if got.Port != 6379 {
		t.Errorf("port = %d, want 6379", got.Port)
	}
	if got.Prefix == "" {
		t.Error("prefix must be set so keys are unique per app")
	}
}

func TestDefaultContactConfigUsesFQDN(t *testing.T) {
	got := DefaultContactConfig("example.com")

	if got.Admin.Email != "admin@example.com" {
		t.Errorf("admin.email = %q, want admin@example.com", got.Admin.Email)
	}
	if got.Security.Email != "security@example.com" {
		t.Errorf("security.email = %q, want security@example.com", got.Security.Email)
	}
	// AI.md PART 12: abuse@ must be opt-in, never defaulted
	if got.Abuse.Email != "" {
		t.Errorf("abuse.email = %q, want empty - the operator must opt in", got.Abuse.Email)
	}
	if got.General.Email != "" {
		t.Errorf("general.email = %q, want empty", got.General.Email)
	}
}

func TestResolveContactEmailFallbackChain(t *testing.T) {
	cfg := ContactConfig{
		Admin: ContactRoleConfig{Email: "admin@example.com"},
	}

	tests := []struct {
		role string
		want string
	}{
		{"admin", "admin@example.com"},
		{"security", "admin@example.com"},
		{"abuse", "admin@example.com"},
		{"general", "admin@example.com"},
		{"unknown", "admin@example.com"},
	}

	for _, tt := range tests {
		if got := cfg.ResolveContactEmail(tt.role); got != tt.want {
			t.Errorf("ResolveContactEmail(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}

	cfg.General.Email = "hello@example.com"
	if got := cfg.ResolveContactEmail("abuse"); got != "hello@example.com" {
		t.Errorf("abuse should fall back to general, got %q", got)
	}

	cfg.Abuse.Email = "abuse@example.com"
	if got := cfg.ResolveContactEmail("abuse"); got != "abuse@example.com" {
		t.Errorf("abuse should use its own address, got %q", got)
	}

	cfg.Security.Email = "security@example.com"
	if got := cfg.ResolveContactEmail("security"); got != "security@example.com" {
		t.Errorf("security should use its own address, got %q", got)
	}
}

func TestResolveContactWebhookFallbackChain(t *testing.T) {
	cfg := ContactConfig{
		Admin: ContactRoleConfig{Webhooks: map[string]string{"slack": "https://admin.example.com/hook"}},
	}

	if got := cfg.ResolveContactWebhook("security", "slack"); got != "https://admin.example.com/hook" {
		t.Errorf("security slack webhook = %q, want the admin fallback", got)
	}

	cfg.Security = ContactRoleConfig{Webhooks: map[string]string{"slack": "https://sec.example.com/hook"}}
	if got := cfg.ResolveContactWebhook("security", "slack"); got != "https://sec.example.com/hook" {
		t.Errorf("security slack webhook = %q, want its own hook", got)
	}

	if got := cfg.ResolveContactWebhook("security", "discord"); got != "" {
		t.Errorf("unset transport = %q, want empty", got)
	}
}

func TestDefaultPrivacyConfig(t *testing.T) {
	got := DefaultPrivacyConfig()

	if got.Data.Sold {
		t.Error("privacy.data.sold must default to false")
	}
	if !got.Consent.DefaultEnabled {
		t.Error("privacy.consent.default_enabled must default to true")
	}
	if len(got.Data.Sharing) == 0 {
		t.Error("privacy.data.sharing must document the disclosure rows")
	}
	if got.Content.DataCollection == "" || got.Content.DataSecurity == "" {
		t.Error("privacy content prose must have working defaults")
	}
}

func TestTrackingTypesRequiringURL(t *testing.T) {
	for _, kind := range []string{"matomo", "piwik", "owa", "umami"} {
		if !trackingTypesRequiringURL[kind] {
			t.Errorf("%s must require tracking.url", kind)
		}
	}
	for _, kind := range []string{"google", "plausible", "fathom", "simple", "cloudflare"} {
		if trackingTypesRequiringURL[kind] {
			t.Errorf("%s must not require tracking.url", kind)
		}
	}
}
