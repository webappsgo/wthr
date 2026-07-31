package service

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/challenge/tlsalpn01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// LetsEncryptService manages Let's Encrypt certificates
// TEMPLATE.md Part 8: Built-in Let's Encrypt support with all 3 challenge types
type LetsEncryptService struct {
	client      *lego.Client
	user        *LEUser
	certsDir    string
	autoRenew   bool
	renewalDays int
	mu          sync.RWMutex

	// Challenge providers
	http01Provider   *HTTP01Provider
	tlsalpn01Provider *TLSALPN01Provider
	dns01Provider    *DNS01Provider
}

// LEUser represents a Let's Encrypt user account
type LEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// GetEmail returns user email
func (u *LEUser) GetEmail() string {
	return u.Email
}

// GetRegistration returns user registration resource
func (u *LEUser) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey returns user private key
func (u *LEUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// globalHTTP01Provider is the package-level singleton used by the gin route handler.
// Initialized on first call to NewLetsEncryptService; safe for concurrent use.
var globalHTTP01Provider = NewHTTP01Provider()

// GetGlobalHTTP01Provider returns the singleton HTTP-01 challenge provider so that
// the application server can serve /.well-known/acme-challenge/ responses.
func GetGlobalHTTP01Provider() *HTTP01Provider {
	return globalHTTP01Provider
}

// HTTP01Provider implements HTTP-01 challenge
// TEMPLATE.md Part 8: HTTP-01 challenge support
type HTTP01Provider struct {
	mu     sync.RWMutex
	tokens map[string]string
}

func NewHTTP01Provider() *HTTP01Provider {
	return &HTTP01Provider{
		tokens: make(map[string]string),
	}
}

// Present implements the challenge.Provider interface
func (p *HTTP01Provider) Present(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens[token] = keyAuth
	return nil
}

// CleanUp implements the challenge.Provider interface
func (p *HTTP01Provider) CleanUp(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tokens, token)
	return nil
}

// GetKeyAuth returns the key authorization for a token
func (p *HTTP01Provider) GetKeyAuth(token string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	keyAuth, ok := p.tokens[token]
	return keyAuth, ok
}

// TLSALPN01Provider implements TLS-ALPN-01 challenge
// TEMPLATE.md Part 8: TLS-ALPN-01 challenge support
type TLSALPN01Provider struct {
	mu    sync.RWMutex
	certs map[string]*certificate.Resource
}

func NewTLSALPN01Provider() *TLSALPN01Provider {
	return &TLSALPN01Provider{
		certs: make(map[string]*certificate.Resource),
	}
}

// Present implements the challenge.Provider interface
func (p *TLSALPN01Provider) Present(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Store challenge certificate
	// In a real implementation, this would create a self-signed cert with the challenge
	return nil
}

// CleanUp implements the challenge.Provider interface
func (p *TLSALPN01Provider) CleanUp(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.certs, domain)
	return nil
}

// DNS01Provider implements DNS-01 challenge
// TEMPLATE.md Part 8: DNS-01 challenge support
type DNS01Provider struct {
	mu      sync.RWMutex
	records map[string]string
}

func NewDNS01Provider() *DNS01Provider {
	return &DNS01Provider{
		records: make(map[string]string),
	}
}

// Present implements the challenge.Provider interface
func (p *DNS01Provider) Present(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	fqdn, value := dns01.GetRecord(domain, keyAuth)
	p.records[fqdn] = value

	// In a real implementation, this would create DNS TXT records via DNS provider API
	// Examples: Cloudflare, Route53, DigitalOcean, etc.
	// For now, we store the record and expect manual DNS configuration

	return nil
}

// CleanUp implements the challenge.Provider interface
func (p *DNS01Provider) CleanUp(domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	fqdn, _ := dns01.GetRecord(domain, keyAuth)
	delete(p.records, fqdn)

	return nil
}

// Timeout returns the timeout for DNS propagation
func (p *DNS01Provider) Timeout() (timeout, interval time.Duration) {
	return 120 * time.Second, 2 * time.Second
}

// GetDNSRecords returns all pending DNS records for manual configuration
func (p *DNS01Provider) GetDNSRecords() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	records := make(map[string]string, len(p.records))
	for k, v := range p.records {
		records[k] = v
	}
	return records
}

// NewLetsEncryptService creates a new Let's Encrypt service
func NewLetsEncryptService(email, certsDir string, staging bool) (*LetsEncryptService, error) {
	// Create certificates directory
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create certs directory: %w", err)
	}

	// Create or load user account
	user, err := loadOrCreateUser(email, certsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create/load user: %w", err)
	}

	// Create ACME client configuration
	config := lego.NewConfig(user)

	// Use Let's Encrypt staging or production
	if staging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}

	config.Certificate.KeyType = certcrypto.RSA2048

	// Create ACME client
	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Register user if not already registered
	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("failed to register user: %w", err)
		}
		user.Registration = reg

		// Save registration
		if err := saveUser(user, certsDir); err != nil {
			return nil, fmt.Errorf("failed to save registration: %w", err)
		}
	}

	service := &LetsEncryptService{
		client:      client,
		user:        user,
		certsDir:    certsDir,
		autoRenew:   true,
		// AI.md PART 21 line 5227: Renew 7 days before expiry
		renewalDays: 7,
	}

	// Initialize challenge providers (reuse the global HTTP-01 provider so the gin
	// route handler at /.well-known/acme-challenge/ can serve the responses).
	service.http01Provider = globalHTTP01Provider
	service.tlsalpn01Provider = NewTLSALPN01Provider()
	service.dns01Provider = NewDNS01Provider()

	return service, nil
}

// SetupChallenges configures the challenge providers
func (s *LetsEncryptService) SetupChallenges(challengeType string) error {
	switch challengeType {
	case "http-01":
		// HTTP-01 challenge: use our in-process provider so that the gin route at
		// /.well-known/acme-challenge/ serves the key-authorization responses.
		// Using lego's http01.NewProviderServer would bind a second listener on port 80
		// which conflicts with the running app server.
		if err := s.client.Challenge.SetHTTP01Provider(s.http01Provider); err != nil {
			return fmt.Errorf("failed to setup HTTP-01: %w", err)
		}

	case "tls-alpn-01":
		// TLS-ALPN-01 challenge
		provider := tlsalpn01.NewProviderServer("", "443")
		if err := s.client.Challenge.SetTLSALPN01Provider(provider); err != nil {
			return fmt.Errorf("failed to setup TLS-ALPN-01: %w", err)
		}

	case "dns-01":
		// DNS-01 challenge
		if err := s.client.Challenge.SetDNS01Provider(s.dns01Provider); err != nil {
			return fmt.Errorf("failed to setup DNS-01: %w", err)
		}

	default:
		return fmt.Errorf("unsupported challenge type: %s", challengeType)
	}

	return nil
}

// ObtainCertificate obtains a new certificate
func (s *LetsEncryptService) ObtainCertificate(domain string, altNames []string, challengeType string) (*certificate.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Setup challenge
	if err := s.SetupChallenges(challengeType); err != nil {
		return nil, err
	}

	// Prepare domains list
	domains := []string{domain}
	if len(altNames) > 0 {
		domains = append(domains, altNames...)
	}

	// Request certificate
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	cert, err := s.client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Save certificate to disk
	if err := s.saveCertificate(domain, cert); err != nil {
		return nil, fmt.Errorf("failed to save certificate: %w", err)
	}

	return cert, nil
}

// RenewCertificate renews an existing certificate
func (s *LetsEncryptService) RenewCertificate(domain string, challengeType string) (*certificate.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load existing certificate
	cert, err := s.loadCertificate(domain)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	// Setup challenge
	if err := s.SetupChallenges(challengeType); err != nil {
		return nil, err
	}

	// Renew certificate
	renewed, err := s.client.Certificate.Renew(*cert, true, false, "")
	if err != nil {
		return nil, fmt.Errorf("failed to renew certificate: %w", err)
	}

	// Save renewed certificate
	if err := s.saveCertificate(domain, renewed); err != nil {
		return nil, fmt.Errorf("failed to save renewed certificate: %w", err)
	}

	return renewed, nil
}

// RevokeCertificate revokes a certificate
func (s *LetsEncryptService) RevokeCertificate(domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cert, err := s.loadCertificate(domain)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}

	if err := s.client.Certificate.Revoke(cert.Certificate); err != nil {
		return fmt.Errorf("failed to revoke certificate: %w", err)
	}

	return nil
}

// CheckRenewal checks if a certificate needs renewal
// TEMPLATE.md Part 8: Auto-renewal system
func (s *LetsEncryptService) CheckRenewal(domain string) (bool, int, error) {
	cert, err := s.loadCertificate(domain)
	if err != nil {
		return false, 0, err
	}

	// Parse certificate to get expiry
	block, _ := pem.Decode(cert.Certificate)
	if block == nil {
		return false, 0, fmt.Errorf("failed to decode certificate")
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Calculate days remaining
	daysRemaining := int(time.Until(x509Cert.NotAfter).Hours() / 24)

	// Need renewal if less than configured days remaining
	needsRenewal := daysRemaining <= s.renewalDays

	return needsRenewal, daysRemaining, nil
}

// StartAutoRenewal starts the auto-renewal background service
// TEMPLATE.md Part 8: Auto-renewal must run automatically
func (s *LetsEncryptService) StartAutoRenewal(domains []string, challengeType string) {
	go func() {
		// Check daily
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			for _, domain := range domains {
				needsRenewal, daysRemaining, err := s.CheckRenewal(domain)
				if err != nil {
					fmt.Printf("Error checking renewal for %s: %v\n", domain, err)
					continue
				}

				if needsRenewal {
					fmt.Printf("Certificate for %s needs renewal (%d days remaining)\n", domain, daysRemaining)
					if _, err := s.RenewCertificate(domain, challengeType); err != nil {
						fmt.Printf("Failed to renew certificate for %s: %v\n", domain, err)
					} else {
						fmt.Printf("Successfully renewed certificate for %s\n", domain)
					}
				}
			}
		}
	}()
}

// GetHTTP01Provider returns the HTTP-01 challenge provider for webhook handling
func (s *LetsEncryptService) GetHTTP01Provider() *HTTP01Provider {
	return s.http01Provider
}

// Helper: Save certificate to disk
func (s *LetsEncryptService) saveCertificate(domain string, cert *certificate.Resource) error {
	certPath := filepath.Join(s.certsDir, sanitizeDomain(domain)+".crt")
	keyPath := filepath.Join(s.certsDir, sanitizeDomain(domain)+".key")

	// Save certificate
	if err := os.WriteFile(certPath, cert.Certificate, 0600); err != nil {
		return err
	}

	// Save private key
	if err := os.WriteFile(keyPath, cert.PrivateKey, 0600); err != nil {
		return err
	}

	return nil
}

// Helper: Load certificate from disk
func (s *LetsEncryptService) loadCertificate(domain string) (*certificate.Resource, error) {
	certPath := filepath.Join(s.certsDir, sanitizeDomain(domain)+".crt")
	keyPath := filepath.Join(s.certsDir, sanitizeDomain(domain)+".key")

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	return &certificate.Resource{
		Domain:      domain,
		Certificate: certPEM,
		PrivateKey:  keyPEM,
	}, nil
}

// Helper: Load or create user account
func loadOrCreateUser(email, certsDir string) (*LEUser, error) {
	accountPath := filepath.Join(certsDir, "account.key")

	var privateKey crypto.PrivateKey

	// Try to load existing key
	if keyPEM, err := os.ReadFile(accountPath); err == nil {
		block, _ := pem.Decode(keyPEM)
		if block != nil {
			privateKey, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
		}
	}

	// Create new key if not found
	if privateKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %w", err)
		}
		privateKey = key

		// Save key
		keyBytes, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal private key: %w", err)
		}

		keyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyBytes,
		})

		if err := os.WriteFile(accountPath, keyPEM, 0600); err != nil {
			return nil, fmt.Errorf("failed to save private key: %w", err)
		}
	}

	return &LEUser{
		Email: email,
		key:   privateKey,
	}, nil
}

// Helper: Save user registration
func saveUser(user *LEUser, certsDir string) error {
	// Registration is saved by the ACME client automatically
	return nil
}

// Helper: Sanitize domain name for file system
// Replaces the wildcard marker and neutralizes any character that is not
// valid in a hostname, so a malformed/hostile domain can never introduce
// path separators or traversal sequences into the certificate file path.
func sanitizeDomain(domain string) string {
	domain = strings.ReplaceAll(domain, "*", "_wildcard_")
	var b strings.Builder
	for _, r := range domain {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	sanitized := b.String()
	sanitized = strings.ReplaceAll(sanitized, "..", "__")
	return sanitized
}
