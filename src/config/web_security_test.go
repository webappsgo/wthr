package config

import "testing"

func TestIsValidCORSSetting(t *testing.T) {
	for _, ok := range []string{"", "*", " * ", "https://example.com", "https://a.example.com, http://localhost:3000", "https://a.b.c/"} {
		if !IsValidCORSSetting(ok) {
			t.Errorf("%q must be valid", ok)
		}
	}
	for _, bad := range []string{"example.com", "ftp://example.com", "https://example.com/path", "https://u:p@example.com", "https://example.com?x=1", "https://a.example.com,*", "https://a.example.com,"} {
		if IsValidCORSSetting(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}

func TestValidateWebSecurityRepairsValues(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Web.CORS = "not an origin"
	cfg.Web.CSRF = CSRFConfig{
		Enabled:     true,
		TokenLength: 4,
		CookieName:  "bad name",
		HeaderName:  "",
		Secure:      "maybe",
		ExemptPaths: []string{"/webhook/*", "no-slash", " /ok "},
	}
	validateWebSecurity(cfg)

	if cfg.Web.CORS != DefaultCORSSetting {
		t.Errorf("web.cors = %q", cfg.Web.CORS)
	}
	defaults := DefaultCSRFConfig()
	csrf := cfg.Web.CSRF
	if csrf.TokenLength != defaults.TokenLength || csrf.CookieName != defaults.CookieName ||
		csrf.HeaderName != defaults.HeaderName || csrf.Secure != defaults.Secure {
		t.Errorf("invalid csrf values must fall back to defaults, got %+v", csrf)
	}
	if len(csrf.ExemptPaths) != 2 || csrf.ExemptPaths[0] != "/webhook/*" || csrf.ExemptPaths[1] != "/ok" {
		t.Errorf("exempt_paths = %v", csrf.ExemptPaths)
	}
	if !csrf.Enabled {
		t.Error("enabled must be left alone")
	}
}

func TestValidateWebSecurityKeepsValidValues(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Web.CORS = ""
	cfg.Web.CSRF = CSRFConfig{Enabled: false, TokenLength: 64, CookieName: "tok", HeaderName: "X-Tok", Secure: "TRUE"}
	validateWebSecurity(cfg)
	if cfg.Web.CORS != "" || cfg.Web.CSRF.TokenLength != 64 || cfg.Web.CSRF.Secure != "true" || cfg.Web.CSRF.Enabled {
		t.Errorf("valid values must be kept, got cors=%q csrf=%+v", cfg.Web.CORS, cfg.Web.CSRF)
	}
}

func TestUpdateWebCORS(t *testing.T) {
	withIsolatedSchedulerConfig(t)

	if errs := UpdateWebCORS("https://example.com/path"); len(errs) != 1 || errs[0].Message != "errors.admin.web.cors_invalid" {
		t.Errorf("an invalid value must be rejected with a key, got %v", errs)
	}
	if errs := UpdateWebCORS(" https://a.example.com "); len(errs) != 0 {
		t.Fatalf("valid value rejected: %v", errs)
	}
	if got := GetGlobalConfig().Web.CORS; got != "https://a.example.com" {
		t.Errorf("web.cors = %q", got)
	}

	previous := GetGlobalConfig()
	SetGlobalConfig(nil)
	defer SetGlobalConfig(previous)
	if errs := UpdateWebCORS("*"); len(errs) != 1 {
		t.Errorf("without a global config expected one error, got %v", errs)
	}
}
