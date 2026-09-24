package util

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/server/reqctx"
)

// setSafeConfigDir points config.LoadConfig() at a pre-existing minimal
// server.yml inside t.TempDir() so it never writes to the real
// /etc/webappsgo or ~/.config paths — LoadConfig() creates a default config
// file on disk when none is found, which would otherwise escape the
// sandboxed test directory.
func setSafeConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  admin_path: admin\n"), 0644); err != nil {
		t.Fatalf("WriteFile server.yml: %v", err)
	}
	t.Setenv("CONFIG_DIR", dir)

	// GetHostFromRequest falls back to GetFQDN() (never raw r.Host) once no
	// forwarded-host header is present, so pin DOMAIN to the same host the
	// test requests use — otherwise GetFQDN() resolves the sandbox's actual
	// os.Hostname(), which varies by container/run and breaks the fixed
	// "http://example.com/..." expectations below.
	t.Setenv("DOMAIN", "example.com")
}

func newTemplateDataRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "http://example.com/London", nil)
}

// withValue returns a copy of r whose context carries key/value, mirroring
// the gin.Context.Set semantics the tests previously exercised.
func withValue(r *http.Request, key string, value interface{}) *http.Request {
	return r.WithContext(reqctx.SetValue(r.Context(), key, value))
}

type fakeLangInfoProvider struct {
	infos []LanguageInfo
}

func (f fakeLangInfoProvider) GetLanguageInfos() []LanguageInfo {
	return f.infos
}

// TestTemplateData_Defaults verifies the fallback values when no server,
// user, csrf_token, lang, or i18n keys are set on the request context.
func TestTemplateData_Defaults(t *testing.T) {
	setSafeConfigDir(t)
	r := newTemplateDataRequest()

	got := TemplateData(r, map[string]interface{}{})

	server, ok := got["server"].(map[string]string)
	if !ok {
		t.Fatalf("server = %T, want map[string]string", got["server"])
	}
	if server["Title"] != "wthr" {
		t.Errorf("server[Title] = %q, want wthr", server["Title"])
	}

	user, ok := got["user"].(map[string]string)
	if !ok {
		t.Fatalf("user = %T, want map[string]string", got["user"])
	}
	if user["Role"] != "guest" {
		t.Errorf("user[Role] = %q, want guest", user["Role"])
	}

	if got["csrf_token"] != "" {
		t.Errorf("csrf_token = %v, want empty string", got["csrf_token"])
	}
	if got["lang"] != "en" {
		t.Errorf("lang = %v, want en", got["lang"])
	}
	if got["current_url"] != "http://example.com/London" {
		t.Errorf("current_url = %v, want http://example.com/London", got["current_url"])
	}
	if got["admin_path"] == "" || got["api_path"] == "" || got["admin_api_path"] == "" {
		t.Error("admin_path/api_path/admin_api_path should be non-empty")
	}
	if got["available_languages"] != nil {
		if langs, ok := got["available_languages"].([]LanguageInfo); ok && len(langs) != 0 {
			t.Errorf("available_languages = %v, want empty/nil without an i18n provider", langs)
		}
	}
}

// TestTemplateData_ContextOverrides verifies server/user/csrf_token/lang
// values already present on the request context are used instead of defaults.
func TestTemplateData_ContextOverrides(t *testing.T) {
	setSafeConfigDir(t)
	r := newTemplateDataRequest()

	r = withValue(r, "server", map[string]string{"Title": "custom-title"})
	r = withValue(r, "user", map[string]string{"Email": "a@b.com", "Role": "admin"})
	r = withValue(r, "csrf_token", "tok-123")
	r = withValue(r, "lang", "es")

	got := TemplateData(r, map[string]interface{}{})

	server := got["server"].(map[string]string)
	if server["Title"] != "custom-title" {
		t.Errorf("server[Title] = %q, want custom-title", server["Title"])
	}
	user := got["user"].(map[string]string)
	if user["Role"] != "admin" {
		t.Errorf("user[Role] = %q, want admin", user["Role"])
	}
	if got["csrf_token"] != "tok-123" {
		t.Errorf("csrf_token = %v, want tok-123", got["csrf_token"])
	}
	if got["lang"] != "es" {
		t.Errorf("lang = %v, want es", got["lang"])
	}
}

// TestTemplateData_AvailableLanguages verifies the i18n context value is
// type-asserted to langInfoProvider and its GetLanguageInfos() result used.
func TestTemplateData_AvailableLanguages(t *testing.T) {
	setSafeConfigDir(t)
	r := newTemplateDataRequest()

	want := []LanguageInfo{{Code: "en", Name: "English", NativeName: "English", Direction: "ltr"}}
	r = withValue(r, "i18n", fakeLangInfoProvider{infos: want})

	got := TemplateData(r, map[string]interface{}{})
	langs, ok := got["available_languages"].([]LanguageInfo)
	if !ok {
		t.Fatalf("available_languages = %T, want []LanguageInfo", got["available_languages"])
	}
	if len(langs) != 1 || langs[0].Code != "en" {
		t.Errorf("available_languages = %v, want %v", langs, want)
	}
}

// TestTemplateData_AvailableLanguages_WrongType verifies a non-conforming
// i18n context value is silently ignored rather than panicking.
func TestTemplateData_AvailableLanguages_WrongType(t *testing.T) {
	setSafeConfigDir(t)
	r := newTemplateDataRequest()
	r = withValue(r, "i18n", "not-a-provider")

	got := TemplateData(r, map[string]interface{}{})
	if langs, ok := got["available_languages"].([]LanguageInfo); ok && len(langs) != 0 {
		t.Errorf("available_languages = %v, want empty for a non-conforming i18n value", langs)
	}
}

// TestTemplateData_MergesUserData verifies caller-supplied data overrides
// the enriched defaults (e.g. a caller can override "lang").
func TestTemplateData_MergesUserData(t *testing.T) {
	setSafeConfigDir(t)
	r := newTemplateDataRequest()

	got := TemplateData(r, map[string]interface{}{"lang": "fr", "extra": "value"})
	if got["lang"] != "fr" {
		t.Errorf("lang = %v, want fr (caller override)", got["lang"])
	}
	if got["extra"] != "value" {
		t.Errorf("extra = %v, want value", got["extra"])
	}
}

// TestTemplateData_HTTPSScheme verifies current_url uses https:// when
// r.TLS is set.
func TestTemplateData_HTTPSScheme(t *testing.T) {
	setSafeConfigDir(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/path", nil)
	req.TLS = &tls.ConnectionState{}

	got := TemplateData(req, map[string]interface{}{})
	if got["current_url"] != "https://example.com/path" {
		t.Errorf("current_url = %v, want https://example.com/path", got["current_url"])
	}
}

// TestSafeRedirectPath verifies open-redirect protection for form redirects.
func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		want      string
	}{
		// Valid same-site redirects
		{"root_only", "/", "/"},
		{"simple_path", "/settings", "/settings"},
		{"nested_path", "/users/profile", "/users/profile"},
		{"with_query", "/settings?tab=privacy", "/settings?tab=privacy"},
		{"with_fragment", "/users#top", "/users#top"},

		// Invalid: Open redirect attempts
		{"empty", "", ""},
		{"protocol_relative", "//evil.com", ""},
		{"backslash_relative", "/\\evil.com", ""},
		{"double_slash", "//", ""},
		{"no_leading_slash", "settings", ""},
		{"external_url", "https://evil.com", ""},
		{"mailto", "mailto:test@example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeRedirectPath(tt.candidate)
			if got != tt.want {
				t.Errorf("SafeRedirectPath(%q) = %q, want %q", tt.candidate, got, tt.want)
			}
		})
	}
}

// TestSetI2PLinkProvider verifies the I2P provider registration.
func TestSetI2PLinkProvider(t *testing.T) {
	// Test setting a provider
	provider := func() (string, bool) {
		return "abc123.b32.i2p", true
	}
	SetI2PLinkProvider(provider)

	// Test clearing the provider
	SetI2PLinkProvider(nil)

	// Verify i2pTemplateInfo reflects cleared provider
	info := i2pTemplateInfo()
	if info == nil {
		t.Fatal("i2pTemplateInfo should not return nil")
	}
	if show, ok := info["Show"].(bool); !ok || show {
		t.Errorf("i2pTemplateInfo()[\"Show\"] = %v, want false when provider is nil", show)
	}

	// Re-register to verify it works with a real provider
	SetI2PLinkProvider(provider)
	info = i2pTemplateInfo()
	if show, ok := info["Show"].(bool); !ok || !show {
		t.Errorf("i2pTemplateInfo()[\"Show\"] = %v, want true when provider exists", show)
	}
	if addr, ok := info["Address"].(string); !ok || addr != "abc123.b32.i2p" {
		t.Errorf("i2pTemplateInfo()[\"Address\"] = %v, want abc123.b32.i2p", addr)
	}

	// Cleanup
	SetI2PLinkProvider(nil)
}
