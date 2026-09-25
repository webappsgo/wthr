package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsFormSubmission(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"urlencoded form", "application/x-www-form-urlencoded", true},
		{"urlencoded with charset", "application/x-www-form-urlencoded; charset=UTF-8", true},
		{"json", "application/json", false},
		{"empty", "", false},
		{"multipart", "multipart/form-data; boundary=abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/server/admin/config/channels", nil)
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}
			if got := wantsFormSubmission(r); got != tt.want {
				t.Fatalf("wantsFormSubmission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdminConfigPagePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		page string
		want string
	}{
		{"default admin path", "/server/admin/config/channels", "/config/channels", "/server/admin/config/channels"},
		{"custom admin path", "/server/console/config/email/templates", "/config/email/templates", "/server/console/config/email/templates"},
		{"no config segment", "/server/admin/dashboard", "/config/channels", "/config/channels"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tt.path, nil)
			if got := adminConfigPagePath(r, tt.page); got != tt.want {
				t.Fatalf("adminConfigPagePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// flashCookieValue returns the value of the flash cookie set on the response,
// or false if no flash cookie was set.
func flashCookieValue(t *testing.T, w *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == FlashCookieName {
			return c.Value, true
		}
	}
	return "", false
}

func TestRedirectAdminForm(t *testing.T) {
	t.Run("success sets success flash and redirects", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/channels", nil)
		redirectAdminForm(w, r, "/server/admin/config/channels", "flash_channel_updated", "flash_channel_update_failed", nil)

		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
		}
		if loc := w.Header().Get("Location"); loc != "/server/admin/config/channels" {
			t.Fatalf("Location = %q, want %q", loc, "/server/admin/config/channels")
		}
		val, ok := flashCookieValue(t, w)
		if !ok {
			t.Fatal("expected a flash cookie")
		}
		if val != "success:flash_channel_updated" {
			t.Fatalf("flash = %q, want %q", val, "success:flash_channel_updated")
		}
	})

	t.Run("failure sets error flash and redirects", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/channels", nil)
		redirectAdminForm(w, r, "/server/admin/config/channels", "flash_channel_updated", "flash_channel_update_failed", errors.New("write failed"))

		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
		}
		val, ok := flashCookieValue(t, w)
		if !ok {
			t.Fatal("expected a flash cookie")
		}
		if val != "error:flash_channel_update_failed" {
			t.Fatalf("flash = %q, want %q", val, "error:flash_channel_update_failed")
		}
	})

	t.Run("unlisted key sets no cookie but still redirects", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/channels", nil)
		redirectAdminForm(w, r, "/server/admin/config/channels", "flash_not_a_real_key", "flash_channel_update_failed", nil)

		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
		}
		if _, ok := flashCookieValue(t, w); ok {
			t.Fatal("expected no flash cookie for an unlisted key")
		}
	})
}

func TestFlashRoundTrip(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
	SetFlash(w, r, "success", "flash_channel_updated")

	val, ok := flashCookieValue(t, w)
	if !ok {
		t.Fatal("expected SetFlash to set a cookie")
	}

	// Feed the set cookie into the next request and take it back.
	r2 := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
	r2.AddCookie(&http.Cookie{Name: FlashCookieName, Value: val})
	w2 := httptest.NewRecorder()
	flash := TakeFlash(w2, r2)
	if flash == nil {
		t.Fatal("expected a pending flash")
	}
	if flash.Kind != "success" || flash.Key != "flash_channel_updated" {
		t.Fatalf("flash = %+v, want success/flash_channel_updated", flash)
	}

	// TakeFlash must clear the cookie (MaxAge -1).
	var cleared bool
	for _, c := range w2.Result().Cookies() {
		if c.Name == FlashCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("expected TakeFlash to clear the cookie with MaxAge -1")
	}

	// A second take on the same (now-cleared) request yields nothing.
	r3 := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
	if f := TakeFlash(httptest.NewRecorder(), r3); f != nil {
		t.Fatalf("expected no flash on empty request, got %+v", f)
	}
}

func TestFlashRejectsTamperedValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"unknown key", "success:flash_not_a_real_key"},
		{"unknown kind", "bogus:flash_channel_updated"},
		{"no separator", "flash_channel_updated"},
		{"markup injection", "success:<script>alert(1)</script>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
			r.AddCookie(&http.Cookie{Name: FlashCookieName, Value: tt.value})
			if f := TakeFlash(httptest.NewRecorder(), r); f != nil {
				t.Fatalf("expected nil flash for tampered value %q, got %+v", tt.value, f)
			}
		})
	}
}

func TestSetFlashSecureFlag(t *testing.T) {
	t.Run("plain http is not secure", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
		SetFlash(w, r, "success", "flash_channel_updated")
		for _, c := range w.Result().Cookies() {
			if c.Name == FlashCookieName && c.Secure {
				t.Fatal("expected Secure=false on plain http")
			}
		}
	})

	t.Run("forwarded https sets secure", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/server/admin/config/channels", nil)
		r.Header.Set("X-Forwarded-Proto", "https")
		SetFlash(w, r, "success", "flash_channel_updated")

		var found bool
		for _, c := range w.Result().Cookies() {
			if c.Name == FlashCookieName {
				found = true
				if !c.Secure {
					t.Fatal("expected Secure=true when X-Forwarded-Proto is https")
				}
			}
		}
		if !found {
			t.Fatal("expected a flash cookie")
		}
	})
}

func TestFlashKeysStayAllowListed(t *testing.T) {
	required := []string{
		"flash_settings_saved",
		"flash_settings_save_failed",
		"flash_channel_updated",
		"flash_channel_update_failed",
		"flash_channel_enabled",
		"flash_channel_disabled",
		"flash_channel_test_sent",
		"flash_channel_test_failed",
		"flash_email_template_saved",
		"flash_email_template_save_failed",
		"flash_test_email_sent",
		"flash_test_email_failed",
	}
	for _, key := range required {
		if !flashKeys[key] {
			t.Fatalf("expected %q to stay in the flash allow-list", key)
		}
	}

	// Every allow-listed key must carry the flash_ prefix convention.
	for key := range flashKeys {
		if !strings.HasPrefix(key, "flash_") {
			t.Fatalf("allow-listed key %q does not use the flash_ prefix", key)
		}
	}
}
