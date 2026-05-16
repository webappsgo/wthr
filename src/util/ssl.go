package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// SSLManager handles SSL certificate management
type SSLManager struct {
	db           *sql.DB
	dataDir      string
	certPath     string
	keyPath      string
	domain       string
	httpsPort    int
	httpsEnabled bool
	certExpiry   time.Time
	certIssuer   string
}

// NewSSLManager creates a new SSL manager
func NewSSLManager(db *sql.DB, dataDir string) *SSLManager {
	return &SSLManager{
		db:           db,
		dataDir:      dataDir,
		httpsEnabled: false,
	}
}

// CheckExistingCerts checks for existing Let's Encrypt certificates
func (sm *SSLManager) CheckExistingCerts(domain string) (bool, error) {
	// Check /etc/letsencrypt/live/{domain}/
	letsencryptPath := filepath.Join("/etc/letsencrypt/live", domain)
	certPath := filepath.Join(letsencryptPath, "fullchain.pem")
	keyPath := filepath.Join(letsencryptPath, "privkey.pem")

	// Check if both cert and key exist
	certExists := fileExists(certPath)
	keyExists := fileExists(keyPath)

	if certExists && keyExists {
		// Load and verify the certificate
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return false, fmt.Errorf("found certs but failed to load: %w", err)
		}

		// Parse certificate to check expiry
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return false, fmt.Errorf("failed to parse certificate: %w", err)
		}

		// Check if certificate is expired
		if time.Now().After(x509Cert.NotAfter) {
			return false, fmt.Errorf("certificate expired on %s", x509Cert.NotAfter)
		}

		// Store certificate info
		sm.certPath = certPath
		sm.keyPath = keyPath
		sm.domain = domain
		sm.httpsEnabled = true
		sm.certExpiry = x509Cert.NotAfter
		sm.certIssuer = x509Cert.Issuer.String()

		return true, nil
	}

	return false, nil
}

// GenerateSelfSignedCert creates a self-signed certificate
func (sm *SSLManager) GenerateSelfSignedCert(domain string) error {
	// Create certs directory in data dir
	// AI.md PART 7: Permissions - root: 0755, user: 0700
	dirPerm := os.FileMode(0700)
	if os.Geteuid() == 0 {
		dirPerm = 0755
	}
	certsDir := filepath.Join(sm.dataDir, "certs")
	if err := os.MkdirAll(certsDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create certs directory: %w", err)
	}

	certPath := filepath.Join(certsDir, "server.crt")
	keyPath := filepath.Join(certsDir, "server.key")

	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Weather Service"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		// 1 year
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate to file
	certFile, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write cert: %w", err)
	}

	// Write private key to file
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	// Store certificate info
	sm.certPath = certPath
	sm.keyPath = keyPath
	sm.domain = domain
	sm.httpsEnabled = true
	sm.certExpiry = template.NotAfter
	sm.certIssuer = "Self-Signed"

	return nil
}

// GetCertInfo returns certificate information for health check
func (sm *SSLManager) GetCertInfo() map[string]interface{} {
	if !sm.httpsEnabled {
		return map[string]interface{}{
			"enabled":        false,
			"status":         "none",
			"expires_at":     nil,
			"days_remaining": 0,
			"issuer":         "Unknown",
		}
	}

	daysRemaining := int(time.Until(sm.certExpiry).Hours() / 24)
	status := "valid"
	if daysRemaining < 30 {
		status = "expiring_soon"
	}
	if daysRemaining < 0 {
		status = "expired"
	}

	return map[string]interface{}{
		"enabled":        true,
		"status":         status,
		"expires_at":     sm.certExpiry.Format(time.RFC3339),
		"days_remaining": daysRemaining,
		"issuer":         sm.certIssuer,
	}
}

// GetTLSConfig returns TLS configuration
func (sm *SSLManager) GetTLSConfig() (*tls.Config, error) {
	if !sm.httpsEnabled {
		return nil, fmt.Errorf("HTTPS not enabled")
	}

	cert, err := tls.LoadX509KeyPair(sm.certPath, sm.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// IsHTTPSEnabled returns whether HTTPS is enabled
func (sm *SSLManager) IsHTTPSEnabled() bool {
	return sm.httpsEnabled
}

// GetCertPaths returns certificate and key paths
func (sm *SSLManager) GetCertPaths() (string, string) {
	return sm.certPath, sm.keyPath
}

// CheckRenewal checks if certificate needs renewal (< 30 days)
func (sm *SSLManager) CheckRenewal() bool {
	if !sm.httpsEnabled {
		return false
	}

	daysRemaining := int(time.Until(sm.certExpiry).Hours() / 24)
	return daysRemaining < 30
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
