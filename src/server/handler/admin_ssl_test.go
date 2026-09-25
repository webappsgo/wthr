package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewSSLHandler_ExplicitHTTPSAddr verifies a non-empty httpsAddr is kept
// as passed, along with the other fields.
func TestNewSSLHandler_ExplicitHTTPSAddr(t *testing.T) {
	db := newTestServerDB(t)
	h := NewSSLHandler("/tmp/example/certs", db, "192.0.2.1:8443")
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.certsDir != "/tmp/example/certs" {
		t.Errorf("certsDir = %q, want %q", h.certsDir, "/tmp/example/certs")
	}
	if h.db != db {
		t.Error("expected db field to be the passed *sql.DB")
	}
	if h.httpsAddr != "192.0.2.1:8443" {
		t.Errorf("httpsAddr = %q, want %q", h.httpsAddr, "192.0.2.1:8443")
	}
}

// TestNewSSLHandler_DefaultsHTTPSAddr verifies an empty httpsAddr defaults
// to "127.0.0.1:443" for local cert checking.
func TestNewSSLHandler_DefaultsHTTPSAddr(t *testing.T) {
	h := NewSSLHandler("/tmp/example/certs", nil, "")
	if h.httpsAddr != "127.0.0.1:443" {
		t.Errorf("httpsAddr = %q, want default %q", h.httpsAddr, "127.0.0.1:443")
	}
}

// TestCalculateNextRenewal_Future verifies a certificate expiring well in
// the future returns a formatted renewal date 30 days before expiry.
func TestCalculateNextRenewal_Future(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	notAfter := time.Now().Add(60 * 24 * time.Hour)
	got := calculateNextRenewal(req, notAfter)

	want := notAfter.Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04")
	if got != want {
		t.Errorf("calculateNextRenewal() = %q, want %q", got, want)
	}
}

// TestCalculateNextRenewal_PastRenewalWindow verifies a certificate whose
// 30-day-before-expiry renewal window has already passed returns the
// translated "Now" value.
func TestCalculateNextRenewal_PastRenewalWindow(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Expires in 10 days: the renewal date (expiry-30d) is already in the past.
	notAfter := time.Now().Add(10 * 24 * time.Hour)
	got := calculateNextRenewal(req, notAfter)

	want := Translate(req, "admin.ssl.status.now")
	if got != want {
		t.Errorf("calculateNextRenewal() = %q, want %q", got, want)
	}
}

// TestSSLHandlerUpdateSettingsFormEncoded verifies the SSL settings form
// binds through the content-type-aware decoder so the admin panel works with
// JavaScript disabled, per AI.md PART 16.
func TestSSLHandlerUpdateSettingsFormEncoded(t *testing.T) {
	h := NewSSLHandler("/tmp/example/certs", newTestServerDB(t), "")
	form := strings.NewReader("autoRenewal=yes&renewalDays=30&emailNotifications=no")
	req := httptest.NewRequest(http.MethodPost, "/server/admin/config/ssl", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.UpdateSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settings struct {
			AutoRenewal        bool `json:"autoRenewal"`
			RenewalDays        int  `json:"renewalDays"`
			EmailNotifications bool `json:"emailNotifications"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !body.Settings.AutoRenewal {
		t.Error("AutoRenewal = false, want true for the form value \"yes\"")
	}
	if body.Settings.RenewalDays != 30 {
		t.Errorf("RenewalDays = %d, want 30", body.Settings.RenewalDays)
	}
	if body.Settings.EmailNotifications {
		t.Error("EmailNotifications = true, want false for the form value \"no\"")
	}
}

// TestSSLHandlerUpdateSettingsFormOutOfRange verifies a form submission whose
// renewal window falls outside the supported 1-60 day range is rejected.
func TestSSLHandlerUpdateSettingsFormOutOfRange(t *testing.T) {
	h := NewSSLHandler("/tmp/example/certs", newTestServerDB(t), "")
	form := strings.NewReader("autoRenewal=yes&renewalDays=99")
	req := httptest.NewRequest(http.MethodPost, "/server/admin/config/ssl", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.UpdateSettings(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
