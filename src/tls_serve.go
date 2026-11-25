package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// activeTLSRuntime is the certificate lifecycle of the running server; nil while the
// server serves no HTTPS. The ssl_renewal task reads it.
var activeTLSRuntime atomic.Pointer[tlsRuntime]

// prepareTLS loads the certificate the HTTPS listener serves (AI.md PART 15): the
// operator's ssl.cert/ssl.key when set, else the first usable certificate in the lookup
// order, else a new self-signed one in {config_dir}/ssl/local/{fqdn}/.
func prepareTLS(manager *util.SSLManager, settings config.SSLConfig, fqdn string) (*tls.Config, error) {
	switch {
	case strings.TrimSpace(settings.Cert) != "" || strings.TrimSpace(settings.Key) != "":
		if err := manager.LoadManualCertificate(strings.TrimSpace(settings.Cert), strings.TrimSpace(settings.Key)); err != nil {
			return nil, fmt.Errorf("ssl.cert/ssl.key: %w", err)
		}
	default:
		found, err := manager.CheckExistingCerts(fqdn)
		if err != nil {
			return nil, err
		}
		for _, reason := range manager.SkippedCertificates(fqdn) {
			log.Printf("WARNING: certificate skipped: %s", reason)
		}
		if !found {
			if err := manager.GenerateSelfSignedCert(fqdn); err != nil {
				return nil, fmt.Errorf("self-signed certificate for %s: %w", fqdn, err)
			}
			log.Printf("INFO: no certificate found for %s, serving a self-signed one", fqdn)
		}
	}
	return manager.GetTLSConfig(settings.MinVersion)
}

// planListeners decides which listen ports serve plain HTTP and which serve HTTPS
// (AI.md PART 5 port rules): a dual-port setting is HTTP then HTTPS; a single port is HTTP
// unless it is 443 (HTTPS-only) or ssl.enabled is on, which turns that port into HTTPS.
// A zero result means that kind of listener does not exist.
func planListeners(httpPort, httpsPort int, sslEnabled bool) (plain, secure int) {
	if httpsPort > 0 {
		return httpPort, httpsPort
	}
	if httpPort == 443 || sslEnabled {
		return 0, httpPort
	}
	return httpPort, 0
}

// isLoopbackHost reports whether a Host header names this machine itself.
func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectToHTTPS sends clearnet plain-HTTP requests to the HTTPS port (AI.md PART 12:
// clearnet keeps the redirect). Three kinds of request are never redirected: the ACME
// HTTP-01 challenge (the CA validates over plain HTTP), overlay requests, because a
// .onion/.i2p address has no HTTPS endpoint, and requests addressed to this machine
// itself, which is how the container health check reaches the server.
func redirectToHTTPS(next http.Handler, securePort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") || util.OverlayHostFromRequest(r) != "" {
			next.ServeHTTP(w, r)
			return
		}

		host := r.Host
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			host = splitHost
		}
		if isLoopbackHost(host) {
			next.ServeHTTP(w, r)
			return
		}
		if host == "" || strings.ContainsAny(host, "/\\@ \t\r\n") {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		target := host
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			target = "[" + host + "]"
		}
		if securePort != 443 {
			target = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(securePort))
		}
		http.Redirect(w, r, "https://"+target+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}

// healthCheckTargets lists the URLs the container health check tries for a port setting
// ("80", "8080,8443"): plain HTTP and HTTPS on each port, because the check cannot know
// which one serves TLS.
func healthCheckTargets(spec string) []string {
	var targets []string
	for _, part := range strings.Split(spec, ",") {
		port := strings.TrimSpace(part)
		if port == "" {
			continue
		}
		targets = append(targets,
			"http://localhost:"+port+"/server/healthz",
			"https://localhost:"+port+"/server/healthz",
		)
	}
	return targets
}

// runHealthCheck reports whether any health target answers 200. The certificate is not
// verified: the check talks to the local process, and a self-signed certificate is normal.
func runHealthCheck(spec string) bool {
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, target := range healthCheckTargets(spec) {
		resp, err := client.Get(target)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}

// isACMEEligibleHost reports whether a public CA can issue a certificate for host: it
// must be a real domain name, never an IP address, localhost or a reserved dev TLD.
func isACMEEligibleHost(host string) bool {
	lower := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if strings.HasSuffix(lower, ".onion") || strings.HasSuffix(lower, ".i2p") {
		return false
	}
	return util.IsValidHost(host, false, "wthr")
}

// tlsRuntime owns the certificate lifecycle of the running server: it requests a
// Let's Encrypt certificate when none exists and renews an app-managed one 7 days
// before expiry (AI.md PART 15).
type tlsRuntime struct {
	manager  *util.SSLManager
	settings config.SSLConfig
	fqdn     string

	// serverDB holds the app_secrets row that records the last DNS-backed issuance
	serverDB *database.Store

	mu sync.Mutex
	le *service.LetsEncryptService
}

// SetServerDB lets the runtime record dns_validated_at after a DNS-backed issuance.
func (t *tlsRuntime) SetServerDB(db *database.Store) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.serverDB = db
}

// currentLetsEncryptSettings returns the live Let's Encrypt settings, so a credential the admin
// saved after startup is used by the next request.
func (t *tlsRuntime) currentLetsEncryptSettings() config.LetsEncryptConfig {
	if live := config.GetGlobalConfig(); live != nil {
		return live.Server.SSL.LetsEncrypt
	}
	return t.settings.LetsEncrypt
}

// prepareChallenge applies the dns-01 solver before a request. It fails the request, naming the
// variable, when the provider cannot be built; no other challenge is tried in its place.
func (t *tlsRuntime) prepareChallenge(client *service.LetsEncryptService) error {
	if t.challenge() != "dns-01" {
		return nil
	}
	key := ""
	if live := config.GetGlobalConfig(); live != nil {
		key = live.Server.Security.EncryptionKey
	}
	solver, err := service.ConfiguredDNSSolver(t.currentLetsEncryptSettings(), key)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return err
	}
	client.SetDNSSolver(solver)
	return nil
}

// recordDNSValidated stores when a DNS-backed issuance or renewal succeeded.
func (t *tlsRuntime) recordDNSValidated() {
	if t.challenge() != "dns-01" || strings.TrimSpace(t.currentLetsEncryptSettings().DNSProvider) == "" {
		return
	}
	t.mu.Lock()
	db := t.serverDB
	t.mu.Unlock()
	if err := service.MarkDNSValidated(db); err != nil {
		log.Printf("WARNING: dns_validated_at was not recorded: %v", err)
	}
}

// newTLSRuntime creates the runtime for the host the server answers to.
func newTLSRuntime(manager *util.SSLManager, settings config.SSLConfig, fqdn string) *tlsRuntime {
	return &tlsRuntime{manager: manager, settings: settings, fqdn: fqdn}
}

// letsEncrypt returns the shared ACME client, creating it on first use and wiring the
// TLS-ALPN-01 challenge to the HTTPS listener.
func (t *tlsRuntime) letsEncrypt() (*service.LetsEncryptService, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.le != nil {
		return t.le, nil
	}
	email := strings.TrimSpace(t.settings.LetsEncrypt.Email)
	if email == "" {
		return nil, errors.New("ssl.letsencrypt.email is empty")
	}
	client, err := service.NewLetsEncryptService(email, filepath.Join(t.manager.SSLDir(), "letsencrypt"), t.settings.LetsEncrypt.Staging)
	if err != nil {
		return nil, err
	}
	client.SetChallengeCertSink(t.manager)
	t.le = client
	return client, nil
}

// challenge returns the configured ACME challenge type, defaulting to http-01.
func (t *tlsRuntime) challenge() string {
	if value := strings.TrimSpace(t.settings.LetsEncrypt.Challenge); value != "" {
		return value
	}
	return "http-01"
}

// Provision requests a Let's Encrypt certificate when Let's Encrypt is enabled and the
// server has no certificate managed by certbot or by the app (AI.md PART 15 "not found
// anywhere"). It saves to {config_dir}/ssl/letsencrypt/{fqdn}/ and serves the new
// certificate at once. It does nothing when there is nothing to request.
func (t *tlsRuntime) Provision() error {
	if !t.settings.LetsEncrypt.Enabled {
		return nil
	}
	if owner := t.manager.Owner(); owner == util.CertOwnerSystem || owner == util.CertOwnerApp {
		return nil
	}
	if !isACMEEligibleHost(t.fqdn) {
		log.Printf("INFO: Let's Encrypt skipped: %q is not a public domain name", t.fqdn)
		return nil
	}
	if t.challenge() == "dns-01" && strings.TrimSpace(t.currentLetsEncryptSettings().DNSProvider) == "" {
		log.Printf("INFO: Let's Encrypt dns-01 needs a DNS provider; the certificate is requested from the admin SSL page")
		return nil
	}

	client, err := t.letsEncrypt()
	if err != nil {
		return fmt.Errorf("acme client setup failed: %w", err)
	}
	if err := t.prepareChallenge(client); err != nil {
		return fmt.Errorf("certificate request for %s failed: %w", t.fqdn, err)
	}
	if _, err := client.ObtainCertificate(t.fqdn, nil, t.challenge()); err != nil {
		return fmt.Errorf("certificate request for %s failed: %w", t.fqdn, err)
	}
	if found, err := t.manager.CheckExistingCerts(t.fqdn); err != nil || !found {
		return fmt.Errorf("the new certificate for %s could not be loaded: %v", t.fqdn, err)
	}
	t.recordDNSValidated()
	log.Printf("INFO: Let's Encrypt certificate for %s installed", t.fqdn)
	return nil
}

// Renew is the ssl_renewal task. An app-managed certificate within 7 days of expiry is
// renewed and reloaded; with no certificate from certbot or the app it asks Let's Encrypt
// for one. System and user certificates are never touched.
func (t *tlsRuntime) Renew() error {
	if t.manager.CheckRenewal() {
		client, err := t.letsEncrypt()
		if err != nil {
			return fmt.Errorf("acme client setup failed: %w", err)
		}
		if err := t.prepareChallenge(client); err != nil {
			return fmt.Errorf("renewal for %s failed: %w", t.fqdn, err)
		}
		if _, err := client.RenewCertificate(t.fqdn, t.challenge()); err != nil {
			return fmt.Errorf("renewal for %s failed: %w", t.fqdn, err)
		}
		if err := t.manager.Reload(); err != nil {
			return fmt.Errorf("the renewed certificate for %s could not be loaded: %w", t.fqdn, err)
		}
		t.recordDNSValidated()
		log.Printf("INFO: Let's Encrypt certificate for %s renewed", t.fqdn)
		return nil
	}
	return t.Provision()
}
