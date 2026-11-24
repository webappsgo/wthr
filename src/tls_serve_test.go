package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/util"
)

// configSSL returns SSL settings with Let's Encrypt switched on or off.
func configSSL(letsEncrypt bool) config.SSLConfig {
	return config.SSLConfig{
		MinVersion:  "TLS1.2",
		LetsEncrypt: config.LetsEncryptConfig{Enabled: letsEncrypt, Email: "admin@example.com", Challenge: "http-01"},
	}
}

func TestPlanListeners(t *testing.T) {
	cases := []struct {
		name                string
		http, https         int
		ssl                 bool
		wantPlain, wantSecu int
	}{
		{"single http", 8080, 0, false, 8080, 0},
		{"single 443 is https only", 443, 0, false, 0, 443},
		{"ssl.enabled turns a single port into https", 8080, 0, true, 0, 8080},
		{"dual", 80, 443, false, 80, 443},
		{"dual high ports", 8080, 8443, false, 8080, 8443},
		{"dual ignores ssl.enabled", 8080, 8443, true, 8080, 8443},
	}
	for _, c := range cases {
		plain, secure := planListeners(c.http, c.https, c.ssl)
		if plain != c.wantPlain || secure != c.wantSecu {
			t.Errorf("%s: got %d/%d, want %d/%d", c.name, plain, secure, c.wantPlain, c.wantSecu)
		}
	}
}

func TestRedirectToHTTPS(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})

	cases := []struct {
		name     string
		host     string
		target   string
		port     int
		wantCode int
		wantLoc  string
	}{
		{"clearnet to 443", "example.com", "/weather/paris?x=1", 443, http.StatusPermanentRedirect, "https://example.com/weather/paris?x=1"},
		{"clearnet strips the plain port", "example.com:8080", "/", 8443, http.StatusPermanentRedirect, "https://example.com:8443/"},
		{"ipv6 literal", "[2001:db8::1]:8080", "/", 443, http.StatusPermanentRedirect, "https://[2001:db8::1]/"},
		{"acme challenge stays plain", "example.com", "/.well-known/acme-challenge/tok", 443, http.StatusNoContent, ""},
		{"onion stays plain", "abcdefghijklmnop.onion", "/", 443, http.StatusNoContent, ""},
		{"i2p stays plain", "abc.b32.i2p", "/", 443, http.StatusNoContent, ""},
		{"localhost health check stays plain", "localhost:8080", "/server/healthz", 443, http.StatusNoContent, ""},
		{"loopback ip stays plain", "127.0.0.1:8080", "/server/healthz", 443, http.StatusNoContent, ""},
		{"hostile host is refused", "evil.com/@x", "/", 443, http.StatusBadRequest, ""},
	}
	for _, c := range cases {
		reached = false
		req := httptest.NewRequest(http.MethodGet, c.target, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		redirectToHTTPS(next, c.port).ServeHTTP(rec, req)
		if rec.Code != c.wantCode {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.wantCode)
		}
		if c.wantLoc != "" && rec.Header().Get("Location") != c.wantLoc {
			t.Errorf("%s: Location %q, want %q", c.name, rec.Header().Get("Location"), c.wantLoc)
		}
		if (c.wantCode == http.StatusNoContent) != reached {
			t.Errorf("%s: handler reached = %v", c.name, reached)
		}
	}
}

func TestHealthCheckTargets(t *testing.T) {
	got := healthCheckTargets("8080, 8443")
	want := []string{
		"http://localhost:8080/server/healthz",
		"https://localhost:8080/server/healthz",
		"http://localhost:8443/server/healthz",
		"https://localhost:8443/server/healthz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRunHealthCheckReachesPlainAndTLSServers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/server/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	plain := httptest.NewServer(handler)
	defer plain.Close()
	secure := httptest.NewTLSServer(handler)
	defer secure.Close()

	portOf := func(url string) string {
		return url[strings.LastIndex(url, ":")+1:]
	}
	if !runHealthCheck(portOf(plain.URL)) {
		t.Error("a plain HTTP server must pass the health check")
	}
	if !runHealthCheck(portOf(secure.URL)) {
		t.Error("an HTTPS server with a self-signed certificate must pass the health check")
	}
	if runHealthCheck("1") {
		t.Error("nothing listens on port 1")
	}
}

func TestIsACMEEligibleHost(t *testing.T) {
	for _, host := range []string{"example.com", "api.example.org"} {
		if !isACMEEligibleHost(host) {
			t.Errorf("%s is a public domain", host)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1", "2001:db8::1", "myhost", "app.local", "x.onion"} {
		if isACMEEligibleHost(host) {
			t.Errorf("%s can never get a public certificate", host)
		}
	}
}

func TestProvisionAndRenewDoNothingWhenThereIsNothingToDo(t *testing.T) {
	manager := util.NewSSLManager(nil, t.TempDir())
	if err := manager.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatal(err)
	}
	disabled := newTLSRuntime(manager, configSSL(false), "example.com")
	if err := disabled.Provision(); err != nil {
		t.Errorf("Let's Encrypt off: Provision = %v", err)
	}
	if err := disabled.Renew(); err != nil {
		t.Errorf("a user certificate must never be renewed: %v", err)
	}

	ineligible := newTLSRuntime(manager, configSSL(true), "localhost")
	if err := ineligible.Provision(); err != nil {
		t.Errorf("an ineligible host must be skipped quietly: %v", err)
	}
}

func TestPrepareTLSGeneratesAndReusesASelfSignedCertificate(t *testing.T) {
	ssl := t.TempDir()
	first := util.NewSSLManager(nil, ssl)
	tlsConfig, err := prepareTLS(first, configSSL(false), "example.com")
	if err != nil || tlsConfig == nil {
		t.Fatalf("prepareTLS: %v", err)
	}
	if first.Owner() != util.CertOwnerUser {
		t.Errorf("owner = %q, want the user-owned self-signed certificate", first.Owner())
	}

	second := util.NewSSLManager(nil, ssl)
	if _, err := prepareTLS(second, configSSL(false), "example.com"); err != nil {
		t.Fatalf("second start: %v", err)
	}
	firstCert, _ := first.GetCertPaths()
	secondCert, _ := second.GetCertPaths()
	if firstCert != secondCert {
		t.Errorf("the second start must reuse %s, got %s", firstCert, secondCert)
	}
}

func TestPrepareTLSManualCertificate(t *testing.T) {
	seed := util.NewSSLManager(nil, t.TempDir())
	if err := seed.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := seed.GetCertPaths()

	manual := configSSL(false)
	manual.Cert, manual.Key = certPath, keyPath
	if _, err := prepareTLS(util.NewSSLManager(nil, t.TempDir()), manual, "another.example.net"); err != nil {
		t.Fatalf("a manual certificate is used as given: %v", err)
	}

	manual.Cert = "relative/cert.pem"
	if _, err := prepareTLS(util.NewSSLManager(nil, t.TempDir()), manual, "example.com"); err == nil {
		t.Error("a relative manual path must be refused")
	}
	manual.Cert = certPath
	manual.Key = keyPath + ".missing"
	if _, err := prepareTLS(util.NewSSLManager(nil, t.TempDir()), manual, "example.com"); err == nil {
		t.Error("a missing manual key must be an error, not a silent fallback")
	}
}

func TestNeedsPortBindingIsOffForStatus(t *testing.T) {
	t.Setenv("CLI_STATUS_FLAG", "1")
	if needsPortBinding() {
		t.Error("--status must not bind ports: the server it reports on holds them")
	}
	t.Setenv("CLI_STATUS_FLAG", "")
	if !needsPortBinding() {
		t.Error("a normal start binds its ports")
	}
}

func TestProvisionWaitsForADNSProviderOnDNS01(t *testing.T) {
	manager := util.NewSSLManager(nil, t.TempDir())
	if err := manager.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatal(err)
	}
	settings := configSSL(true)
	settings.LetsEncrypt.Challenge = "dns-01"
	if err := newTLSRuntime(manager, settings, "example.com").Provision(); err != nil {
		t.Errorf("dns-01 with no provider is skipped quietly, got %v", err)
	}
}

func TestLetsEncryptAndDNSRuntimeHelpers(t *testing.T) {
	manager := util.NewSSLManager(nil, t.TempDir())
	settings := configSSL(true).LetsEncrypt
	settings.Email = "  "
	runtime := newTLSRuntime(manager, config.SSLConfig{LetsEncrypt: settings}, "example.com")
	if _, err := runtime.letsEncrypt(); err == nil || !strings.Contains(err.Error(), "email is empty") {
		t.Fatalf("letsEncrypt() with no email = %v, want an email configuration error", err)
	}

	local := configSSL(false).LetsEncrypt
	runtime = newTLSRuntime(manager, config.SSLConfig{LetsEncrypt: local}, "example.com")
	if got := runtime.currentLetsEncryptSettings(); got.Email != local.Email {
		t.Fatalf("current settings without global config = %#v, want %#v", got, local)
	}
	previous := config.GetGlobalConfig()
	live := &config.AppConfig{Server: config.ServerConfig{SSL: config.SSLConfig{LetsEncrypt: config.LetsEncryptConfig{Email: "live@example.test", DNSProvider: "cloudflare"}}}}
	config.SetGlobalConfig(live)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	if got := runtime.currentLetsEncryptSettings(); got.Email != "live@example.test" {
		t.Fatalf("current settings with global config = %#v", got)
	}

	config.SetGlobalConfig(nil)
	runtime.settings.LetsEncrypt.Challenge = "http-01"
	runtime.recordDNSValidated()
	runtime.settings.LetsEncrypt.Challenge = "dns-01"
	runtime.settings.LetsEncrypt.DNSProvider = ""
	runtime.recordDNSValidated()
}

func TestPrepareChallengeFailsOnBadDNSCredentials(t *testing.T) {
	previous := config.GetGlobalConfig()
	cfg := &config.AppConfig{}
	cfg.Server.Security.EncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	cfg.Server.SSL.LetsEncrypt = config.LetsEncryptConfig{Enabled: true, Challenge: "dns-01", DNSProvider: "cloudflare", DNSCredentials: map[string]string{}}
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	runtime := newTLSRuntime(util.NewSSLManager(nil, t.TempDir()), cfg.Server.SSL, "example.com")
	err := runtime.prepareChallenge(nil)
	if err == nil || !strings.Contains(err.Error(), "CLOUDFLARE_DNS_API_TOKEN") {
		t.Errorf("the request must fail naming the variable, got %v", err)
	}
}
