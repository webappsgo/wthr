package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestNewSSLHandler_ExplicitHTTPSAddr verifies a non-empty httpsAddr is kept
// as passed, along with the other fields.
func TestNewSSLHandler_ExplicitHTTPSAddr(t *testing.T) {
	db := newTestServerDB(t)
	h := NewSSLHandler("/tmp/example/certs", storeForDB(db), "192.0.2.1:8443")
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.certsDir != "/tmp/example/certs" {
		t.Errorf("certsDir = %q, want %q", h.certsDir, "/tmp/example/certs")
	}
	if h.db.DB() != db {
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

func TestSSLAdminActionsUsePostRedirectGetForForms(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		form   url.Values
		action func(http.ResponseWriter, *http.Request)
	}{
		{
			name: "certificate revocation",
			path: "/server/console/ssl/revoke",
			form: url.Values{"domain": {"example.com"}},
			action: func(w http.ResponseWriter, r *http.Request) {
				NewSSLHandler(t.TempDir(), nil, "").RevokeCertificate(w, r)
			},
		},
		{
			name: "automatic renewal",
			path: "/server/console/ssl/auto-renew",
			form: url.Values{"domains": {"example.com, api.example.com"}, "challengeType": {"http-01"}},
			action: func(w http.ResponseWriter, r *http.Request) {
				NewSSLHandler(t.TempDir(), nil, "").StartAutoRenewal(w, r)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			test.action(response, request)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body.String())
			}
			if got, want := response.Header().Get("Location"), "/server/console/ssl"; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			if len(response.Result().Cookies()) == 0 {
				t.Error("form action did not set a flash cookie for the redirected page")
			}
		})
	}
}

func TestSplitSSLFormDomains(t *testing.T) {
	got := splitSSLFormDomains(" example.com,\napi.example.com \t, ")
	want := []string{"example.com", "api.example.com"}
	if len(got) != len(want) {
		t.Fatalf("domains = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("domains = %v, want %v", got, want)
			break
		}
	}
	for _, challenge := range []string{"http-01", "tls-alpn-01", "dns-01"} {
		if !isSSLChallengeType(challenge) {
			t.Errorf("isSSLChallengeType(%q) = false, want true", challenge)
		}
	}
	if isSSLChallengeType("invalid") {
		t.Error("isSSLChallengeType(invalid) = true, want false")
	}
}
