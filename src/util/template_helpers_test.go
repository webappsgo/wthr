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
}

func newTemplateDataRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "http://example.com/London", nil)
}

// withValue returns a copy of r whose context carries key/value, mirroring
// the gin.Context.Set semantics the tests previously exercised.
func withValue(r *http.Request, key string, value interface{}) *http.Request {
	return r.WithContext(reqctx.Set(r.Context(), key, value))
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
