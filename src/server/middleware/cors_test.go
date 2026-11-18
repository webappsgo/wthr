package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func withWebCORS(t *testing.T, value string) {
	t.Helper()
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Web.CORS = value
	config.SetGlobalConfig(cfg)
	t.Setenv("DOMAIN", "")
	ResetLearnedCORSHosts()
	t.Cleanup(func() {
		config.SetGlobalConfig(previous)
		ResetLearnedCORSHosts()
	})
}

func TestResolveCORSPolicyOrder(t *testing.T) {
	t.Setenv("DOMAIN", "")
	ResetLearnedCORSHosts()
	t.Cleanup(ResetLearnedCORSHosts)

	if !ResolveCORSPolicy("*").AllowAll {
		t.Error("the shipped default with no other source must allow all")
	}
	if !ResolveCORSPolicy("").Disabled {
		t.Error("an empty web.cors must disable CORS entirely")
	}

	explicit := ResolveCORSPolicy("https://a.example.com, https://B.example.com/")
	want := []string{"https://a.example.com", "https://b.example.com"}
	if !reflect.DeepEqual(explicit.Origins, want) || explicit.AllowAll {
		t.Errorf("explicit entries = %+v, want %v", explicit, want)
	}

	t.Setenv("DOMAIN", "weather.example.com, other.example.com")
	fromDomain := ResolveCORSPolicy("*")
	if fromDomain.AllowAll || len(fromDomain.Origins) != 2 || fromDomain.Origins[0] != "https://weather.example.com" {
		t.Errorf("DOMAIN must replace the * fallback, got %+v", fromDomain)
	}

	t.Setenv("DOMAIN", "")
	LearnCORSHost("proxy.example.com")
	learned := ResolveCORSPolicy("*")
	if learned.AllowAll || len(learned.Origins) != 1 || learned.Origins[0] != "https://proxy.example.com" {
		t.Errorf("a learned host must be appended, got %+v", learned)
	}

	combined := ResolveCORSPolicy("https://a.example.com")
	if len(combined.Origins) != 2 {
		t.Errorf("explicit config and learned hosts combine, got %+v", combined)
	}
}

func TestLearnCORSHostRejectsGarbage(t *testing.T) {
	ResetLearnedCORSHosts()
	t.Cleanup(ResetLearnedCORSHosts)
	for _, bad := range []string{"", "  ", "a b", "x/y", "user@host", `back\slash`} {
		LearnCORSHost(bad)
	}
	if got := LearnedCORSOrigins(); len(got) != 0 {
		t.Errorf("invalid hosts must not be learned, got %v", got)
	}
}

func TestCORSAllowAllSendsNoCredentials(t *testing.T) {
	withWebCORS(t, "*")
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Origin", "https://anywhere.example.org")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("credentials must never be sent with *")
	}
}

func TestCORSExplicitOriginAllowsCredentials(t *testing.T) {
	withWebCORS(t, "https://app.example.com")
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("a listed origin must get its own value and credentials, got %v", w.Header())
	}

	other := httptest.NewRequest(http.MethodGet, "/x", nil)
	other.Header.Set("Origin", "https://evil.example.net")
	wOther := httptest.NewRecorder()
	handler.ServeHTTP(wOther, other)
	if wOther.Header().Get("Access-Control-Allow-Origin") != "" || wOther.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("an unlisted origin must get no CORS grant, got %v", wOther.Header())
	}
}

func TestCORSDisabledSendsNothing(t *testing.T) {
	withWebCORS(t, "")
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	for name := range w.Header() {
		if len(name) > 14 && name[:14] == "Access-Control" {
			t.Errorf("a disabled policy must send no CORS header, got %s", name)
		}
	}
}

func TestCORSPreflightReturns204WithoutReachingTheHandler(t *testing.T) {
	withWebCORS(t, "*")
	reached := false
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/x", nil)
	req.Header.Set("Origin", "https://anywhere.example.org")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent || reached {
		t.Errorf("preflight = %d, reached handler %v; want 204 and not reached", w.Code, reached)
	}
	if w.Header().Get("Access-Control-Max-Age") != "86400" || w.Header().Get("Access-Control-Allow-Methods") != "POST" {
		t.Errorf("preflight headers missing: %v", w.Header())
	}
}

func TestCORSLearnsForwardedHostOnlyFromTrustedPeers(t *testing.T) {
	withWebCORS(t, "*")
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	untrusted := httptest.NewRequest(http.MethodGet, "/x", nil)
	untrusted.RemoteAddr = "203.0.113.9:4000"
	untrusted.Header.Set("X-Forwarded-Host", "forged.example.com")
	handler.ServeHTTP(httptest.NewRecorder(), untrusted)
	if got := LearnedCORSOrigins(); len(got) != 0 {
		t.Errorf("a forged header from an untrusted peer must not be learned, got %v", got)
	}

	trusted := httptest.NewRequest(http.MethodGet, "/x", nil)
	trusted.RemoteAddr = "127.0.0.1:4000"
	trusted.Header.Set("X-Forwarded-Host", "real.example.com")
	handler.ServeHTTP(httptest.NewRecorder(), trusted)
	if got := LearnedCORSOrigins(); len(got) != 1 || got[0] != "https://real.example.com" {
		t.Errorf("a trusted proxy host must be learned, got %v", got)
	}
}

func TestCORSPreflightListsOnlyTheSpecHeaders(t *testing.T) {
	withWebCORS(t, "*")
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	allowed := httptest.NewRequest(http.MethodOptions, "/api/v1/x", nil)
	allowed.Header.Set("Origin", "https://a.example.org")
	allowed.Header.Set("Access-Control-Request-Method", "POST")
	allowed.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	wAllowed := httptest.NewRecorder()
	handler.ServeHTTP(wAllowed, allowed)
	if got := wAllowed.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Errorf("Authorization and Content-Type must be allowed, got %v", wAllowed.Header())
	}

	denied := httptest.NewRequest(http.MethodOptions, "/api/v1/x", nil)
	denied.Header.Set("Origin", "https://a.example.org")
	denied.Header.Set("Access-Control-Request-Method", "POST")
	denied.Header.Set("Access-Control-Request-Headers", "x-not-in-the-list")
	wDenied := httptest.NewRecorder()
	handler.ServeHTTP(wDenied, denied)
	if wDenied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("a header outside the list must fail the preflight, got %v", wDenied.Header())
	}
}

func TestCORSHeaderPreview(t *testing.T) {
	t.Setenv("DOMAIN", "")
	ResetLearnedCORSHosts()
	t.Cleanup(ResetLearnedCORSHosts)

	all := CORSHeaderPreview("*", "https://x.example.com")
	if all.Get("Access-Control-Allow-Origin") != "*" || all.Get("Access-Control-Allow-Credentials") != "" || all.Get("Access-Control-Max-Age") != "86400" {
		t.Errorf("the preview of * must show * without credentials, got %v", all)
	}
	explicit := CORSHeaderPreview("https://x.example.com", "https://x.example.com")
	if explicit.Get("Access-Control-Allow-Origin") != "https://x.example.com" || explicit.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("the preview of a listed origin must show it with credentials, got %v", explicit)
	}
	if CORSHeaderPreview("", "https://x.example.com").Get("Access-Control-Allow-Origin") != "" {
		t.Error("the preview of a disabled policy must be empty")
	}
}

func TestCSPConnectSrcSharesTheResolvedCORSList(t *testing.T) {
	withWebCORS(t, "https://app.example.com")
	LearnCORSHost("proxy.example.com")

	csp := buildCSP(SecurityHeadersConfig{}, true)
	for _, want := range []string{"https://app.example.com", "https://proxy.example.com"} {
		if !strings.Contains(csp, want) {
			t.Errorf("connect-src must list %s, got %s", want, csp)
		}
	}

	withWebCORS(t, "*")
	if strings.Contains(buildCSP(SecurityHeadersConfig{}, true), "proxy.example.com") && len(LearnedCORSOrigins()) == 0 {
		t.Error("a forgotten learned host must not stay in the policy")
	}
}
