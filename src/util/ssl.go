package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// acmeTLSALPNProtocol is the ALPN protocol of the TLS-ALPN-01 challenge (RFC 8737).
const acmeTLSALPNProtocol = "acme-tls/1"

// appCertRenewalDays is how many days before expiry an app-managed certificate is renewed
// (AI.md PART 15 renewal rules).
const appCertRenewalDays = 7

// systemLetsEncryptDir is where certbot keeps its certificates; the app reads them and
// never renews them (AI.md PART 15 certificate ownership). A variable so tests can point
// it at a temporary directory.
var systemLetsEncryptDir = "/etc/letsencrypt/live"

// CertOwner says who manages a certificate and therefore who renews it.
type CertOwner string

const (
	// CertOwnerSystem is a certbot-managed certificate under /etc/letsencrypt/live: never renewed by the app
	CertOwnerSystem CertOwner = "system"
	// CertOwnerApp is an app-managed Let's Encrypt certificate: renewed 7 days before expiry
	CertOwnerApp CertOwner = "app"
	// CertOwnerUser is a self-signed or user-provided certificate: never renewed by the app
	CertOwnerUser CertOwner = "user"
)

// CertLocation names a certificate and key pair on disk and who manages it.
type CertLocation struct {
	CertPath string
	KeyPath  string
	Owner    CertOwner
}

// CertDirName turns a host name into the directory name used under ssl/letsencrypt and
// ssl/local. Anything outside a hostname alphabet becomes '_' so a hostile value can never
// introduce a path separator or a traversal sequence.
func CertDirName(fqdn string) string {
	fqdn = strings.ReplaceAll(strings.TrimSpace(fqdn), "*", "_wildcard_")
	var b strings.Builder
	for _, r := range fqdn {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" || name == "." || name == ".." || strings.Contains(name, "..") {
		return "_invalid_"
	}
	return name
}

// AppCertDir is the app-managed Let's Encrypt directory for a host: {ssl_dir}/letsencrypt/{fqdn}.
func AppCertDir(sslDir, fqdn string) string {
	return filepath.Join(sslDir, "letsencrypt", CertDirName(fqdn))
}

// LocalCertDir is the self-signed or user-provided directory for a host: {ssl_dir}/local/{fqdn}.
func LocalCertDir(sslDir, fqdn string) string {
	return filepath.Join(sslDir, "local", CertDirName(fqdn))
}

// certCandidates lists the places a certificate for fqdn may live, in AI.md PART 15 lookup order.
func certCandidates(sslDir, fqdn string) []CertLocation {
	name := CertDirName(fqdn)
	live := func(dir string) CertLocation {
		return CertLocation{
			CertPath: filepath.Join(systemLetsEncryptDir, dir, "fullchain.pem"),
			KeyPath:  filepath.Join(systemLetsEncryptDir, dir, "privkey.pem"),
			Owner:    CertOwnerSystem,
		}
	}
	appDir := AppCertDir(sslDir, fqdn)
	localDir := LocalCertDir(sslDir, fqdn)
	return []CertLocation{
		live("domain"),
		live(name),
		{CertPath: filepath.Join(appDir, "fullchain.pem"), KeyPath: filepath.Join(appDir, "privkey.pem"), Owner: CertOwnerApp},
		{CertPath: filepath.Join(localDir, "cert.pem"), KeyPath: filepath.Join(localDir, "key.pem"), Owner: CertOwnerUser},
	}
}

// loadCertificate reads and checks a certificate: both files readable, the leaf valid now
// and naming fqdn in its CN or SAN (AI.md PART 15 certificate validation).
func loadCertificate(location CertLocation, fqdn string, now time.Time) (*tls.Certificate, *x509.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(location.CertPath, location.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the certificate and key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("cannot parse the certificate: %w", err)
	}
	if now.Before(leaf.NotBefore) {
		return nil, nil, fmt.Errorf("the certificate is not valid before %s", leaf.NotBefore.Format(time.RFC3339))
	}
	if now.After(leaf.NotAfter) {
		return nil, nil, fmt.Errorf("the certificate expired on %s", leaf.NotAfter.Format(time.RFC3339))
	}
	if fqdn != "" {
		if err := leaf.VerifyHostname(fqdn); err != nil {
			return nil, nil, fmt.Errorf("the certificate does not match %s", fqdn)
		}
	}
	pair.Leaf = leaf
	return &pair, leaf, nil
}

// FindCertificate walks the AI.md PART 15 lookup order and returns the first usable
// certificate for fqdn, or nil when there is none. skipped explains each candidate that
// exists but was rejected, so the caller can log why.
func FindCertificate(sslDir, fqdn string, now time.Time) (found *CertLocation, skipped []string) {
	for _, candidate := range certCandidates(sslDir, fqdn) {
		if !fileExists(candidate.CertPath) && !fileExists(candidate.KeyPath) {
			continue
		}
		if _, _, err := loadCertificate(candidate, fqdn, now); err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", candidate.CertPath, err))
			continue
		}
		location := candidate
		return &location, skipped
	}
	return nil, skipped
}

// ParseTLSMinVersion maps a ssl.min_version value ("TLS1.2", "TLS1.3") to the crypto/tls
// constant. Anything else falls back to TLS 1.2, the PART 11 transport floor.
func ParseTLSMinVersion(value string) uint16 {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", "")) {
	case "TLS1.3", "1.3", "TLSV1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

// SSLManager serves the active certificate and keeps it replaceable at runtime, so a
// renewal is picked up by the next handshake without a restart.
type SSLManager struct {
	sslDir string

	mu        sync.RWMutex
	cert      *tls.Certificate
	leaf      *x509.Certificate
	location  CertLocation
	fqdn      string
	challenge map[string]*tls.Certificate
}

// NewSSLManager creates a manager rooted at {config_dir}/ssl.
func NewSSLManager(sslDir string) *SSLManager {
	return &SSLManager{sslDir: sslDir, challenge: map[string]*tls.Certificate{}}
}

// SSLDir returns the {config_dir}/ssl directory the manager reads and writes.
func (sm *SSLManager) SSLDir() string {
	return sm.sslDir
}

// install makes a loaded certificate the one served.
func (sm *SSLManager) install(pair *tls.Certificate, leaf *x509.Certificate, location CertLocation, fqdn string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cert = pair
	sm.leaf = leaf
	sm.location = location
	sm.fqdn = fqdn
}

// CheckExistingCerts looks for a usable certificate in the AI.md PART 15 lookup order and,
// when one is found, makes it the served certificate. A missing certificate is not an error.
func (sm *SSLManager) CheckExistingCerts(fqdn string) (bool, error) {
	location, _ := FindCertificate(sm.sslDir, fqdn, time.Now())
	if location == nil {
		return false, nil
	}
	pair, leaf, err := loadCertificate(*location, fqdn, time.Now())
	if err != nil {
		return false, err
	}
	sm.install(pair, leaf, *location, fqdn)
	return true, nil
}

// LoadManualCertificate serves the certificate and key named by ssl.cert and ssl.key.
// The operator chose these files, so the host name is not checked; the user owns them and
// the app never renews them.
func (sm *SSLManager) LoadManualCertificate(certPath, keyPath string) error {
	if !filepath.IsAbs(certPath) || !filepath.IsAbs(keyPath) {
		return fmt.Errorf("ssl.cert and ssl.key must be absolute paths")
	}
	location := CertLocation{CertPath: filepath.Clean(certPath), KeyPath: filepath.Clean(keyPath), Owner: CertOwnerUser}
	pair, leaf, err := loadCertificate(location, "", time.Now())
	if err != nil {
		return err
	}
	sm.install(pair, leaf, location, "")
	return nil
}

// SkippedCertificates reports the candidates that exist but were rejected for fqdn.
func (sm *SSLManager) SkippedCertificates(fqdn string) []string {
	_, skipped := FindCertificate(sm.sslDir, fqdn, time.Now())
	return skipped
}

// Reload re-reads the served certificate from disk after a renewal or an import.
func (sm *SSLManager) Reload() error {
	sm.mu.RLock()
	location, fqdn := sm.location, sm.fqdn
	sm.mu.RUnlock()
	if location.CertPath == "" {
		return fmt.Errorf("no certificate is loaded")
	}
	pair, leaf, err := loadCertificate(location, fqdn, time.Now())
	if err != nil {
		return err
	}
	sm.install(pair, leaf, location, fqdn)
	return nil
}

// GenerateSelfSignedCert creates a self-signed certificate for fqdn in
// {ssl_dir}/local/{fqdn}/ (cert.pem and key.pem) and serves it. The user owns that
// directory, so the app never renews it; an expired one is regenerated at startup.
func (sm *SSLManager) GenerateSelfSignedCert(fqdn string) error {
	dir := LocalCertDir(sm.sslDir, fqdn)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create certs directory: %w", err)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"Weather"}, CommonName: fqdn},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if ip := net.ParseIP(strings.Trim(fqdn, "[]")); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{fqdn}
	}
	template.DNSNames = append(template.DNSNames, "localhost")
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return fmt.Errorf("failed to write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	location := CertLocation{CertPath: certPath, KeyPath: keyPath, Owner: CertOwnerUser}
	pair, leaf, err := loadCertificate(location, fqdn, time.Now())
	if err != nil {
		return err
	}
	sm.install(pair, leaf, location, fqdn)
	return nil
}

// SetChallengeCertificate stores the certificate a TLS-ALPN-01 validation handshake must
// receive for domain (RFC 8737).
func (sm *SSLManager) SetChallengeCertificate(domain string, cert *tls.Certificate) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.challenge[strings.ToLower(domain)] = cert
}

// ClearChallengeCertificate removes the TLS-ALPN-01 certificate for domain.
func (sm *SSLManager) ClearChallengeCertificate(domain string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.challenge, strings.ToLower(domain))
}

// GetCertificate is the tls.Config callback: a TLS-ALPN-01 validation handshake gets the
// challenge certificate, everything else gets the current served certificate.
func (sm *SSLManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, proto := range hello.SupportedProtos {
		if proto == acmeTLSALPNProtocol {
			if cert, ok := sm.challenge[strings.ToLower(hello.ServerName)]; ok {
				return cert, nil
			}
			return nil, fmt.Errorf("no TLS-ALPN-01 challenge is pending for %q", hello.ServerName)
		}
	}
	if sm.cert == nil {
		return nil, fmt.Errorf("no certificate is loaded")
	}
	return sm.cert, nil
}

// GetCertInfo returns certificate information for the health check.
func (sm *SSLManager) GetCertInfo() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.leaf == nil {
		return map[string]interface{}{
			"enabled":        false,
			"status":         "none",
			"expires_at":     nil,
			"days_remaining": 0,
			"issuer":         "Unknown",
		}
	}

	daysRemaining := int(time.Until(sm.leaf.NotAfter).Hours() / 24)
	status := "valid"
	if daysRemaining < 30 {
		status = "expiring_soon"
	}
	if daysRemaining < 0 {
		status = "expired"
	}
	issuer := sm.leaf.Issuer.CommonName
	if sm.location.Owner == CertOwnerUser && sm.leaf.Issuer.String() == sm.leaf.Subject.String() {
		issuer = "Self-Signed"
	}
	return map[string]interface{}{
		"enabled":        true,
		"status":         status,
		"expires_at":     sm.leaf.NotAfter.Format(time.RFC3339),
		"days_remaining": daysRemaining,
		"issuer":         issuer,
	}
}

// GetTLSConfig returns the server TLS configuration. The certificate is fetched per
// handshake, so a renewal needs no restart.
func (sm *SSLManager) GetTLSConfig(minVersion string) (*tls.Config, error) {
	if !sm.IsHTTPSEnabled() {
		return nil, fmt.Errorf("HTTPS not enabled")
	}
	return &tls.Config{
		GetCertificate: sm.GetCertificate,
		MinVersion:     ParseTLSMinVersion(minVersion),
		NextProtos:     []string{"h2", "http/1.1", acmeTLSALPNProtocol},
	}, nil
}

// IsHTTPSEnabled reports whether a certificate is loaded.
func (sm *SSLManager) IsHTTPSEnabled() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.cert != nil
}

// GetCertPaths returns the served certificate and key paths.
func (sm *SSLManager) GetCertPaths() (string, string) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.location.CertPath, sm.location.KeyPath
}

// Owner reports who manages the served certificate; empty when none is loaded.
func (sm *SSLManager) Owner() CertOwner {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.location.Owner
}

// CheckRenewal reports whether the served certificate is app-managed and within 7 days of
// expiry (AI.md PART 15). A system or user certificate is never renewed by the app.
func (sm *SSLManager) CheckRenewal() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.leaf == nil || sm.location.Owner != CertOwnerApp {
		return false
	}
	return time.Until(sm.leaf.NotAfter) <= appCertRenewalDays*24*time.Hour
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
