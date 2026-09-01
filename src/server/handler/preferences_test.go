package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/server/middleware"
)

// setPreferencesTestRenderer installs a stub RenderHTML so the HTML branch of
// NegotiateResponse is exercised instead of panicking on a nil func value.
func setPreferencesTestRenderer(t *testing.T) {
	t.Helper()
	orig := middleware.RenderHTML
	middleware.RenderHTML = func(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]interface{}) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(name))
	}
	t.Cleanup(func() { middleware.RenderHTML = orig })
}

// newPreferencesBrowserRequest builds a request that content negotiation
// classifies as a browser, so the HTML branch is taken.
func newPreferencesBrowserRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func TestShowPreferencesRendersPage(t *testing.T) {
	setPreferencesTestRenderer(t)

	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodGet, "/server/preferences", "")
	rec := httptest.NewRecorder()

	h.ShowPreferences(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ShowPreferences() status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "page/preferences.tmpl" {
		t.Fatalf("ShowPreferences() rendered %q, want %q", got, "page/preferences.tmpl")
	}
}

func TestSavePreferencesSetsCookieAndRedirects(t *testing.T) {
	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", "theme=light")
	req.Header.Set("Referer", "http://example.test/moon")
	req.Host = "example.test"
	rec := httptest.NewRecorder()

	h.SavePreferences(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SavePreferences() status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/moon" {
		t.Fatalf("SavePreferences() Location = %q, want %q", loc, "/moon")
	}

	var themeCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "theme" {
			themeCookie = c
		}
	}
	if themeCookie == nil {
		t.Fatal("SavePreferences() did not set the theme cookie")
	}
	if themeCookie.Value != "light" {
		t.Fatalf("theme cookie = %q, want %q", themeCookie.Value, "light")
	}
}

func TestSavePreferencesRejectsInvalidTheme(t *testing.T) {
	h := NewPreferencesHandler(nil)
	req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", "theme=sepia")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	h.SavePreferences(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("SavePreferences() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "theme" {
			t.Fatalf("invalid theme must not set the theme cookie (got %q)", c.Value)
		}
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("invalid theme must not redirect (got Location %q)", loc)
	}
}

func TestPreferencesRedirectTarget(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		referer string
		want    string
	}{
		{"explicit redirect field wins", "theme=dark&redirect=/earthquake", "http://example.test/moon", "/earthquake"},
		{"referer path is used when no field", "theme=dark", "http://example.test/moon?zoom=3", "/moon?zoom=3"},
		{"no referer falls back to the preferences page", "theme=dark", "", "/server/preferences"},
		{"cross-host referer is rejected", "theme=dark", "http://evil.example/moon", "/server/preferences"},
		{"protocol-relative redirect field is rejected", "theme=dark&redirect=//evil.example", "", "/server/preferences"},
		{"backslash redirect field is rejected", "theme=dark&redirect=/\\evil.example", "", "/server/preferences"},
		{"non-rooted redirect field is rejected", "theme=dark&redirect=evil.example", "", "/server/preferences"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newPreferencesBrowserRequest(http.MethodPost, "/server/preferences", tt.body)
			req.Host = "example.test"
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			if got := preferencesRedirectTarget(req); got != tt.want {
				t.Fatalf("preferencesRedirectTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
