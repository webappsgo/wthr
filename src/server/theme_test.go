package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"dark", true},
		{"light", true},
		{"auto", true},
		{"solarized", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := IsValidTheme(tt.mode); got != tt.want {
				t.Errorf("IsValidTheme(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestResolveTheme(t *testing.T) {
	t.Run("no cookie defaults to dark", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := ResolveTheme(req); got != DefaultTheme {
			t.Errorf("ResolveTheme() = %q, want %q", got, DefaultTheme)
		}
	})

	t.Run("valid cookie is honored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: ThemeCookieName, Value: "light"})
		if got := ResolveTheme(req); got != "light" {
			t.Errorf("ResolveTheme() = %q, want %q", got, "light")
		}
	})

	t.Run("invalid cookie value falls back to dark", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: ThemeCookieName, Value: "not-a-theme"})
		if got := ResolveTheme(req); got != DefaultTheme {
			t.Errorf("ResolveTheme() = %q, want %q", got, DefaultTheme)
		}
	})
}

func TestSetThemeCookie(t *testing.T) {
	t.Run("valid mode is persisted as-is", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		SetThemeCookie(rec, req, "light")

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value != "light" {
			t.Fatalf("SetThemeCookie() cookies = %+v, want one cookie with value %q", cookies, "light")
		}
		if cookies[0].Secure {
			t.Error("SetThemeCookie() Secure = true for a plain HTTP request, want false")
		}
	})

	t.Run("invalid mode falls back to default", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		SetThemeCookie(rec, req, "bogus")

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value != DefaultTheme {
			t.Fatalf("SetThemeCookie() cookies = %+v, want one cookie with value %q", cookies, DefaultTheme)
		}
	})

	t.Run("TLS request marks the cookie secure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.TLS = &tls.ConnectionState{}
		SetThemeCookie(rec, req, "dark")

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure {
			t.Fatalf("SetThemeCookie() cookies = %+v, want Secure=true over TLS", cookies)
		}
	})

	t.Run("forwarded-proto https marks the cookie secure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		SetThemeCookie(rec, req, "dark")

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure {
			t.Fatalf("SetThemeCookie() cookies = %+v, want Secure=true behind an https-terminating proxy", cookies)
		}
	})
}

func TestThemeClass(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"dark", "theme-dark"},
		{"light", "theme-light"},
		{"auto", "theme-auto"},
		{"bogus", "theme-" + DefaultTheme},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := ThemeClass(tt.mode); got != tt.want {
				t.Errorf("ThemeClass(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSetThemeHandler(t *testing.T) {
	t.Run("invalid theme is rejected", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/theme?theme=bogus", nil)
		SetThemeHandler(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("SetThemeHandler() status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("valid theme sets cookie and redirects to root by default", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/theme?theme=light", nil)
		SetThemeHandler(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("SetThemeHandler() status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("SetThemeHandler() Location = %q, want %q", loc, "/")
		}
		if len(rec.Result().Cookies()) != 1 {
			t.Fatalf("SetThemeHandler() cookies = %+v, want exactly one", rec.Result().Cookies())
		}
	})

	t.Run("same-site path redirect is honored", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/theme?theme=light&redirect=/settings", nil)
		SetThemeHandler(rec, req)

		if loc := rec.Header().Get("Location"); loc != "/settings" {
			t.Fatalf("SetThemeHandler() Location = %q, want %q", loc, "/settings")
		}
	})

	t.Run("protocol-relative redirect is rejected as an open-redirect vector", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/theme?theme=light&redirect=//evil.com", nil)
		SetThemeHandler(rec, req)

		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("SetThemeHandler() Location = %q, want %q (open redirect rejected)", loc, "/")
		}
	})

	t.Run("backslash redirect is rejected as an open-redirect vector", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, `/theme?theme=light&redirect=/\evil.com`, nil)
		SetThemeHandler(rec, req)

		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("SetThemeHandler() Location = %q, want %q (open redirect rejected)", loc, "/")
		}
	})
}
