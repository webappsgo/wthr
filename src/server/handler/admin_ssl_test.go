package handler

import (
	"net/http"
	"net/http/httptest"
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
