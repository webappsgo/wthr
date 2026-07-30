package utils

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// setSafeConfigDir points config.LoadConfig() at a pre-existing minimal
// server.yml inside t.TempDir() so it never writes to the real
// /etc/casapps or ~/.config paths — LoadConfig() creates a default config
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

func newTemplateDataContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.com/London", nil)
	return c, w
}

type fakeLangInfoProvider struct {
	infos []LanguageInfo
}

func (f fakeLangInfoProvider) GetLanguageInfos() []LanguageInfo {
	return f.infos
}

// TestTemplateData_Defaults verifies the fallback values when no server,
// user, csrf_token, lang, or i18n keys are set on the gin context.
func TestTemplateData_Defaults(t *testing.T) {
	setSafeConfigDir(t)
	c, _ := newTemplateDataContext()

	got := TemplateData(c, gin.H{})

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
// values already present on the gin context are used instead of defaults.
func TestTemplateData_ContextOverrides(t *testing.T) {
	setSafeConfigDir(t)
	c, _ := newTemplateDataContext()

	c.Set("server", map[string]string{"Title": "custom-title"})
	c.Set("user", map[string]string{"Email": "a@b.com", "Role": "admin"})
	c.Set("csrf_token", "tok-123")
	c.Set("lang", "es")

	got := TemplateData(c, gin.H{})

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
	c, _ := newTemplateDataContext()

	want := []LanguageInfo{{Code: "en", Name: "English", NativeName: "English", Direction: "ltr"}}
	c.Set("i18n", fakeLangInfoProvider{infos: want})

	got := TemplateData(c, gin.H{})
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
	c, _ := newTemplateDataContext()
	c.Set("i18n", "not-a-provider")

	got := TemplateData(c, gin.H{})
	if langs, ok := got["available_languages"].([]LanguageInfo); ok && len(langs) != 0 {
		t.Errorf("available_languages = %v, want empty for a non-conforming i18n value", langs)
	}
}

// TestTemplateData_MergesUserData verifies caller-supplied data overrides
// the enriched defaults (e.g. a caller can override "lang").
func TestTemplateData_MergesUserData(t *testing.T) {
	setSafeConfigDir(t)
	c, _ := newTemplateDataContext()

	got := TemplateData(c, gin.H{"lang": "fr", "extra": "value"})
	if got["lang"] != "fr" {
		t.Errorf("lang = %v, want fr (caller override)", got["lang"])
	}
	if got["extra"] != "value" {
		t.Errorf("extra = %v, want value", got["extra"])
	}
}

// TestTemplateData_HTTPSScheme verifies current_url uses https:// when
// c.Request.TLS is set.
func TestTemplateData_HTTPSScheme(t *testing.T) {
	setSafeConfigDir(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/path", nil)
	req.TLS = &tls.ConnectionState{}
	c.Request = req

	got := TemplateData(c, gin.H{})
	if got["current_url"] != "https://example.com/path" {
		t.Errorf("current_url = %v, want https://example.com/path", got["current_url"])
	}
}
