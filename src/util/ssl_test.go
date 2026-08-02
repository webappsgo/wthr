package util

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewSSLManager verifies the constructor wires up dataDir and defaults
// httpsEnabled to false until a cert is generated or detected.
func TestNewSSLManager(t *testing.T) {
	dataDir := t.TempDir()
	mgr := NewSSLManager(nil, dataDir)
	if mgr == nil {
		t.Fatal("NewSSLManager returned nil")
	}
	if mgr.IsHTTPSEnabled() {
		t.Error("IsHTTPSEnabled() = true, want false for a fresh manager")
	}
}

// TestCheckExistingCerts_NotPresent verifies the safe, no-error false path
// when no Let's Encrypt cert exists for the domain (which is always the
// case in a test environment — this never touches /etc/letsencrypt for
// writing, only reads via os.Stat).
func TestCheckExistingCerts_NotPresent(t *testing.T) {
	mgr := NewSSLManager(nil, t.TempDir())
	found, err := mgr.CheckExistingCerts("nonexistent-test-domain.invalid")
	if err != nil {
		t.Fatalf("CheckExistingCerts: %v", err)
	}
	if found {
		t.Error("CheckExistingCerts() = true, want false for a domain with no real cert")
	}
}

// TestGenerateSelfSignedCert verifies a self-signed ECDSA cert/key pair is
// written to {dataDir}/certs/ and GetCertInfo reflects a valid, enabled
// state afterward.
func TestGenerateSelfSignedCert(t *testing.T) {
	dataDir := t.TempDir()
	mgr := NewSSLManager(nil, dataDir)

	if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	crtPath, keyPath := mgr.GetCertPaths()
	wantCrt := filepath.Join(dataDir, "certs", "server.crt")
	wantKey := filepath.Join(dataDir, "certs", "server.key")
	if crtPath != wantCrt {
		t.Errorf("cert path = %q, want %q", crtPath, wantCrt)
	}
	if keyPath != wantKey {
		t.Errorf("key path = %q, want %q", keyPath, wantKey)
	}

	if !fileExists(crtPath) {
		t.Errorf("cert file %s does not exist", crtPath)
	}
	if !fileExists(keyPath) {
		t.Errorf("key file %s does not exist", keyPath)
	}

	if !mgr.IsHTTPSEnabled() {
		t.Error("IsHTTPSEnabled() = false after generating a cert, want true")
	}

	info := mgr.GetCertInfo()
	if info["status"] != "valid" {
		t.Errorf("GetCertInfo()[status] = %v, want valid", info["status"])
	}
	if info["enabled"] != true {
		t.Errorf("GetCertInfo()[enabled] = %v, want true", info["enabled"])
	}
	if days, ok := info["days_remaining"].(int); !ok || days <= 0 {
		t.Errorf("GetCertInfo()[days_remaining] = %v, want positive int", info["days_remaining"])
	}
}

// TestGetTLSConfig covers both the disabled-error path and the
// enabled-success path after generating a cert.
func TestGetTLSConfig(t *testing.T) {
	t.Run("disabled_returns_error", func(t *testing.T) {
		mgr := NewSSLManager(nil, t.TempDir())
		if _, err := mgr.GetTLSConfig(); err == nil {
			t.Error("GetTLSConfig() on disabled manager: err = nil, want error")
		}
	})

	t.Run("enabled_after_cert_generation", func(t *testing.T) {
		mgr := NewSSLManager(nil, t.TempDir())
		if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
			t.Fatalf("GenerateSelfSignedCert: %v", err)
		}
		cfg, err := mgr.GetTLSConfig()
		if err != nil {
			t.Fatalf("GetTLSConfig: %v", err)
		}
		if cfg == nil {
			t.Error("GetTLSConfig() returned nil config with no error")
		}
	})
}

// TestCheckRenewal verifies a freshly generated 1-year cert is not yet due
// for renewal (renewal threshold is 30 days).
func TestCheckRenewal(t *testing.T) {
	mgr := NewSSLManager(nil, t.TempDir())
	if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}
	if mgr.CheckRenewal() {
		t.Error("CheckRenewal() = true for a freshly generated 1-year cert, want false")
	}
}

// TestFileExists covers both the present and absent cases.
func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(present, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !fileExists(present) {
		t.Error("fileExists() = false for a present file, want true")
	}
	if fileExists(filepath.Join(dir, "absent.txt")) {
		t.Error("fileExists() = true for an absent file, want false")
	}
}
