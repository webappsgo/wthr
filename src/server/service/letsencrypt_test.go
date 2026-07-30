package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/certificate"
)

// generateTestCertPEM creates a self-signed EC certificate PEM valid until notAfter.
func generateTestCertPEM(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestSanitizeDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{"plain domain", "example.com", "example.com"},
		{"wildcard domain", "*.example.com", "_wildcard_.example.com"},
		{"multiple asterisks", "*.*.example.com", "_wildcard_._wildcard_.example.com"},
		{"no asterisk", "sub.example.com", "sub.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeDomain(tt.domain); got != tt.want {
				t.Errorf("sanitizeDomain(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestHTTP01Provider(t *testing.T) {
	p := NewHTTP01Provider()

	t.Run("present then get", func(t *testing.T) {
		if err := p.Present("example.com", "token1", "keyauth1"); err != nil {
			t.Fatalf("Present returned error: %v", err)
		}
		got, ok := p.GetKeyAuth("token1")
		if !ok || got != "keyauth1" {
			t.Errorf("GetKeyAuth(token1) = (%q,%v), want (keyauth1,true)", got, ok)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		_, ok := p.GetKeyAuth("nonexistent")
		if ok {
			t.Error("expected ok=false for missing token")
		}
	})

	t.Run("cleanup removes token", func(t *testing.T) {
		_ = p.Present("example.com", "token2", "keyauth2")
		if err := p.CleanUp("example.com", "token2", "keyauth2"); err != nil {
			t.Fatalf("CleanUp returned error: %v", err)
		}
		_, ok := p.GetKeyAuth("token2")
		if ok {
			t.Error("expected token to be removed after CleanUp")
		}
	})

	t.Run("cleanup of nonexistent token is a no-op", func(t *testing.T) {
		if err := p.CleanUp("example.com", "never-set", "x"); err != nil {
			t.Errorf("CleanUp on missing token returned error: %v", err)
		}
	})
}

func TestTLSALPN01Provider(t *testing.T) {
	p := NewTLSALPN01Provider()
	if err := p.Present("example.com", "token", "keyauth"); err != nil {
		t.Fatalf("Present returned error: %v", err)
	}
	if err := p.CleanUp("example.com", "token", "keyauth"); err != nil {
		t.Fatalf("CleanUp returned error: %v", err)
	}
}

func TestDNS01Provider(t *testing.T) {
	p := NewDNS01Provider()

	t.Run("present adds a record", func(t *testing.T) {
		if err := p.Present("example.com", "token", "keyauth"); err != nil {
			t.Fatalf("Present returned error: %v", err)
		}
		records := p.GetDNSRecords()
		if len(records) != 1 {
			t.Errorf("len(records) = %d, want 1", len(records))
		}
	})

	t.Run("cleanup removes the record", func(t *testing.T) {
		q := NewDNS01Provider()
		_ = q.Present("example.com", "token", "keyauth")
		if err := q.CleanUp("example.com", "token", "keyauth"); err != nil {
			t.Fatalf("CleanUp returned error: %v", err)
		}
		records := q.GetDNSRecords()
		if len(records) != 0 {
			t.Errorf("len(records) = %d, want 0 after cleanup", len(records))
		}
	})

	t.Run("timeout returns expected values", func(t *testing.T) {
		timeout, interval := p.Timeout()
		if timeout != 120*time.Second || interval != 2*time.Second {
			t.Errorf("Timeout() = (%v,%v), want (120s,2s)", timeout, interval)
		}
	})

	t.Run("GetDNSRecords returns a copy", func(t *testing.T) {
		q := NewDNS01Provider()
		_ = q.Present("example.com", "token", "keyauth")
		records := q.GetDNSRecords()
		for k := range records {
			delete(records, k)
		}
		if len(q.GetDNSRecords()) != 1 {
			t.Error("mutating returned map affected internal state; GetDNSRecords should return a copy")
		}
	})
}

func TestLEUser_Getters(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	user := &LEUser{Email: "admin@example.com", key: priv}

	if got := user.GetEmail(); got != "admin@example.com" {
		t.Errorf("GetEmail() = %q, want admin@example.com", got)
	}
	if got := user.GetRegistration(); got != nil {
		t.Errorf("GetRegistration() = %v, want nil", got)
	}
	if got := user.GetPrivateKey(); got != priv {
		t.Error("GetPrivateKey() did not return the stored key")
	}
}

func TestGetGlobalHTTP01Provider(t *testing.T) {
	p1 := GetGlobalHTTP01Provider()
	p2 := GetGlobalHTTP01Provider()
	if p1 != p2 {
		t.Error("GetGlobalHTTP01Provider() should return the same singleton instance")
	}
	if p1 == nil {
		t.Fatal("GetGlobalHTTP01Provider() returned nil")
	}
}

func TestLetsEncryptService_GetHTTP01Provider(t *testing.T) {
	provider := NewHTTP01Provider()
	s := &LetsEncryptService{http01Provider: provider}
	if got := s.GetHTTP01Provider(); got != provider {
		t.Error("GetHTTP01Provider() did not return the configured provider")
	}
}

func TestLetsEncryptService_SaveAndLoadCertificate(t *testing.T) {
	dir := t.TempDir()
	s := &LetsEncryptService{certsDir: dir}

	cert := &certificate.Resource{
		Domain:      "example.com",
		Certificate: []byte("cert-bytes"),
		PrivateKey:  []byte("key-bytes"),
	}

	if err := s.saveCertificate("example.com", cert); err != nil {
		t.Fatalf("saveCertificate returned error: %v", err)
	}

	// Files should exist on disk with the sanitized domain name.
	if _, err := os.Stat(filepath.Join(dir, "example.com.crt")); err != nil {
		t.Errorf("expected cert file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "example.com.key")); err != nil {
		t.Errorf("expected key file to exist: %v", err)
	}

	loaded, err := s.loadCertificate("example.com")
	if err != nil {
		t.Fatalf("loadCertificate returned error: %v", err)
	}
	if string(loaded.Certificate) != "cert-bytes" {
		t.Errorf("loaded Certificate = %q, want cert-bytes", loaded.Certificate)
	}
	if string(loaded.PrivateKey) != "key-bytes" {
		t.Errorf("loaded PrivateKey = %q, want key-bytes", loaded.PrivateKey)
	}
}

func TestLetsEncryptService_SaveCertificate_WildcardDomain(t *testing.T) {
	dir := t.TempDir()
	s := &LetsEncryptService{certsDir: dir}
	cert := &certificate.Resource{Certificate: []byte("c"), PrivateKey: []byte("k")}

	if err := s.saveCertificate("*.example.com", cert); err != nil {
		t.Fatalf("saveCertificate returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_wildcard_.example.com.crt")); err != nil {
		t.Errorf("expected sanitized wildcard cert file to exist: %v", err)
	}
}

func TestLetsEncryptService_LoadCertificate_Missing(t *testing.T) {
	dir := t.TempDir()
	s := &LetsEncryptService{certsDir: dir}
	if _, err := s.loadCertificate("nonexistent.com"); err == nil {
		t.Fatal("expected error loading a certificate that was never saved")
	}
}

func TestLetsEncryptService_CheckRenewal(t *testing.T) {
	t.Run("needs renewal when expiring soon", func(t *testing.T) {
		dir := t.TempDir()
		s := &LetsEncryptService{certsDir: dir, renewalDays: 30}
		certPEM := generateTestCertPEM(t, time.Now().Add(10*24*time.Hour))
		if err := s.saveCertificate("soon.example.com", &certificate.Resource{Certificate: certPEM, PrivateKey: []byte("k")}); err != nil {
			t.Fatalf("saveCertificate returned error: %v", err)
		}

		needsRenewal, daysRemaining, err := s.CheckRenewal("soon.example.com")
		if err != nil {
			t.Fatalf("CheckRenewal returned error: %v", err)
		}
		if !needsRenewal {
			t.Error("expected needsRenewal=true for a soon-expiring cert")
		}
		if daysRemaining < 8 || daysRemaining > 11 {
			t.Errorf("daysRemaining = %d, want ~10", daysRemaining)
		}
	})

	t.Run("does not need renewal when far from expiry", func(t *testing.T) {
		dir := t.TempDir()
		s := &LetsEncryptService{certsDir: dir, renewalDays: 30}
		certPEM := generateTestCertPEM(t, time.Now().Add(90*24*time.Hour))
		if err := s.saveCertificate("far.example.com", &certificate.Resource{Certificate: certPEM, PrivateKey: []byte("k")}); err != nil {
			t.Fatalf("saveCertificate returned error: %v", err)
		}

		needsRenewal, _, err := s.CheckRenewal("far.example.com")
		if err != nil {
			t.Fatalf("CheckRenewal returned error: %v", err)
		}
		if needsRenewal {
			t.Error("expected needsRenewal=false for a cert far from expiry")
		}
	})

	t.Run("missing certificate errors", func(t *testing.T) {
		dir := t.TempDir()
		s := &LetsEncryptService{certsDir: dir, renewalDays: 30}
		if _, _, err := s.CheckRenewal("nonexistent.example.com"); err == nil {
			t.Fatal("expected error for missing certificate")
		}
	})

	t.Run("corrupt PEM errors", func(t *testing.T) {
		dir := t.TempDir()
		s := &LetsEncryptService{certsDir: dir, renewalDays: 30}
		if err := s.saveCertificate("corrupt.example.com", &certificate.Resource{Certificate: []byte("not pem data"), PrivateKey: []byte("k")}); err != nil {
			t.Fatalf("saveCertificate returned error: %v", err)
		}
		if _, _, err := s.CheckRenewal("corrupt.example.com"); err == nil {
			t.Fatal("expected error for corrupt PEM certificate")
		}
	})

	t.Run("exactly at renewal boundary needs renewal", func(t *testing.T) {
		dir := t.TempDir()
		s := &LetsEncryptService{certsDir: dir, renewalDays: 30}
		certPEM := generateTestCertPEM(t, time.Now().Add(30*24*time.Hour))
		if err := s.saveCertificate("boundary.example.com", &certificate.Resource{Certificate: certPEM, PrivateKey: []byte("k")}); err != nil {
			t.Fatalf("saveCertificate returned error: %v", err)
		}
		needsRenewal, _, err := s.CheckRenewal("boundary.example.com")
		if err != nil {
			t.Fatalf("CheckRenewal returned error: %v", err)
		}
		if !needsRenewal {
			t.Error("expected needsRenewal=true at exactly the renewal boundary (<=)")
		}
	})
}

func TestLoadOrCreateUser(t *testing.T) {
	t.Run("creates a new key when none exists", func(t *testing.T) {
		dir := t.TempDir()
		user, err := loadOrCreateUser("admin@example.com", dir)
		if err != nil {
			t.Fatalf("loadOrCreateUser returned error: %v", err)
		}
		if user.Email != "admin@example.com" {
			t.Errorf("Email = %q, want admin@example.com", user.Email)
		}
		if user.key == nil {
			t.Error("expected a generated private key")
		}
		if _, err := os.Stat(filepath.Join(dir, "account.key")); err != nil {
			t.Errorf("expected account.key to be persisted: %v", err)
		}
	})

	t.Run("loads an existing key idempotently", func(t *testing.T) {
		dir := t.TempDir()
		user1, err := loadOrCreateUser("admin@example.com", dir)
		if err != nil {
			t.Fatalf("first loadOrCreateUser returned error: %v", err)
		}
		user2, err := loadOrCreateUser("admin@example.com", dir)
		if err != nil {
			t.Fatalf("second loadOrCreateUser returned error: %v", err)
		}

		key1, ok1 := user1.key.(*ecdsa.PrivateKey)
		key2, ok2 := user2.key.(*ecdsa.PrivateKey)
		if !ok1 || !ok2 {
			t.Fatal("expected ecdsa.PrivateKey types")
		}
		if key1.D.Cmp(key2.D) != 0 {
			t.Error("expected the same private key to be reloaded, got a different key")
		}
	})

	t.Run("corrupt key file errors", func(t *testing.T) {
		dir := t.TempDir()
		badPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not a real key")})
		if err := os.WriteFile(filepath.Join(dir, "account.key"), badPEM, 0600); err != nil {
			t.Fatalf("failed to write corrupt key: %v", err)
		}
		if _, err := loadOrCreateUser("admin@example.com", dir); err == nil {
			t.Fatal("expected error for corrupt account key")
		}
	})
}

func TestSaveUser(t *testing.T) {
	// saveUser is currently a no-op (registration persistence is handled by the
	// ACME client); this test guards against a future regression that panics
	// or errors unexpectedly.
	user := &LEUser{Email: "admin@example.com"}
	if err := saveUser(user, t.TempDir()); err != nil {
		t.Errorf("saveUser returned error: %v, want nil", err)
	}
}
