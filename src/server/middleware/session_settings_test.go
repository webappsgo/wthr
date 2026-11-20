package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

// TestSessionSettingsForDefaults verifies the AI.md PART 12 defaults: separate cookie names
// and lifetimes for admin and user sessions.
func TestSessionSettingsForDefaults(t *testing.T) {
	admin := SessionSettingsFor(SessionAccountAdmin)
	if admin.CookieName != "admin_session" || admin.MaxAge != 30*24*time.Hour || admin.IdleTimeout != 24*time.Hour {
		t.Errorf("admin settings = %+v", admin)
	}

	user := SessionSettingsFor(SessionAccountUser)
	if user.CookieName != "user_session" || user.MaxAge != 7*24*time.Hour || user.IdleTimeout != 24*time.Hour {
		t.Errorf("user settings = %+v", user)
	}

	if UserSessionCookieName() != "user_session" || AdminSessionCookieName() != "admin_session" {
		t.Error("cookie name helpers must match the settings")
	}
}

// TestSessionSameSiteAndSecure verifies the SameSite mapping and the Secure resolution.
func TestSessionSameSiteAndSecure(t *testing.T) {
	cases := map[string]http.SameSite{
		"strict": http.SameSiteStrictMode,
		"STRICT": http.SameSiteStrictMode,
		"lax":    http.SameSiteLaxMode,
		"none":   http.SameSiteNoneMode,
		"bogus":  http.SameSiteStrictMode,
		"":       http.SameSiteStrictMode,
	}
	for value, want := range cases {
		if got := sessionSameSite(value); got != want {
			t.Errorf("sessionSameSite(%q) = %v, want %v", value, got, want)
		}
	}

	plain := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if sessionSecure(plain, "auto") {
		t.Error("auto over plain HTTP must not be Secure")
	}
	if !sessionSecure(plain, "true") || sessionSecure(plain, "false") {
		t.Error("explicit true and false must win over the scheme")
	}
}

// TestNewSessionCookie verifies the cookie carries the configured shape and caps the lifetime.
func TestNewSessionCookie(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)

	cookie := NewSessionCookie(request, SessionAccountUser, "token", time.Hour)
	if cookie.Name != "user_session" || cookie.Value != "token" || cookie.Path != "/" {
		t.Errorf("cookie = %+v", cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 3600 {
		t.Errorf("cookie attributes = HttpOnly %v SameSite %v MaxAge %d", cookie.HttpOnly, cookie.SameSite, cookie.MaxAge)
	}

	capped := NewSessionCookie(request, SessionAccountUser, "token", 365*24*time.Hour)
	if capped.MaxAge != int((7 * 24 * time.Hour).Seconds()) {
		t.Errorf("a lifetime above the maximum must be capped, MaxAge = %d", capped.MaxAge)
	}

	unset := NewSessionCookie(request, SessionAccountAdmin, "token", 0)
	if unset.MaxAge != int((30 * 24 * time.Hour).Seconds()) {
		t.Errorf("no lifetime must use the maximum, MaxAge = %d", unset.MaxAge)
	}

	cleared := ClearSessionCookie(request, SessionAccountAdmin)
	if cleared.Value != "" || cleared.MaxAge != -1 || cleared.Name != "admin_session" {
		t.Errorf("cleared cookie = %+v", cleared)
	}
}

// TestSessionSettingsFromConfig verifies configured names and lifetimes are used and an
// unparsable lifetime falls back to the default.
func TestSessionSettingsFromConfig(t *testing.T) {
	previous := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previous)

	cfg := &config.AppConfig{}
	cfg.Server.Session = config.DefaultSessionConfig()
	cfg.Server.Session.User.CookieName = "custom_user"
	cfg.Server.Session.User.MaxAge = "2d"
	cfg.Server.Session.Admin.MaxAge = "not-a-duration"
	cfg.Server.Session.SameSite = "lax"
	config.SetGlobalConfig(cfg)

	user := SessionSettingsFor(SessionAccountUser)
	if user.CookieName != "custom_user" || user.MaxAge != 48*time.Hour {
		t.Errorf("user settings = %+v", user)
	}
	if got := SessionSettingsFor(SessionAccountAdmin).MaxAge; got != 30*24*time.Hour {
		t.Errorf("an unparsable max_age must fall back to the default, got %v", got)
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if got := NewSessionCookie(request, SessionAccountUser, "t", time.Hour).SameSite; got != http.SameSiteLaxMode {
		t.Errorf("configured same_site lax = %v", got)
	}
}
