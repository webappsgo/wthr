package service

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestTorKeys_NewTorKeyManager covers construction of the manager with the
// data dir stored verbatim (happy path + empty string boundary).
func TestTorKeys_NewTorKeyManager(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
	}{
		{"non-empty dir", "/var/lib/webappsgo/wthr"},
		{"empty dir", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := NewTorKeyManager(tt.dataDir)
			if km == nil {
				t.Fatal("expected non-nil manager")
			}
			if km.dataDir != tt.dataDir {
				t.Errorf("dataDir = %q, want %q", km.dataDir, tt.dataDir)
			}
		})
	}
}

// TestTorKeys_ImportExportRoundTrip covers the happy path: import a freshly
// generated ed25519 keypair, then export it back out and verify the
// resulting bytes match what was imported (idempotency of the on-disk
// encoding round trip).
func TestTorKeys_ImportExportRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	km := NewTorKeyManager(t.TempDir())

	if err := km.ImportKeys(pub, priv); err != nil {
		t.Fatalf("ImportKeys() error = %v", err)
	}

	gotPub, gotPriv, err := km.ExportKeys()
	if err != nil {
		t.Fatalf("ExportKeys() error = %v", err)
	}

	if string(gotPub) != string(pub) {
		t.Errorf("exported public key mismatch: got %x want %x", gotPub, pub)
	}

	// ImportKeys stores the 32-byte seed, and the private key passed in was
	// already a full 64-byte expanded key: the seed is the first 32 bytes.
	wantSeed := priv.Seed()
	if string(gotPriv) != string(wantSeed) {
		t.Errorf("exported private key mismatch: got %x want %x", gotPriv, wantSeed)
	}

	// Idempotency: importing the same keys again should succeed and produce
	// identical exported bytes.
	if err := km.ImportKeys(pub, priv); err != nil {
		t.Fatalf("second ImportKeys() error = %v", err)
	}
	gotPub2, gotPriv2, err := km.ExportKeys()
	if err != nil {
		t.Fatalf("second ExportKeys() error = %v", err)
	}
	if string(gotPub2) != string(gotPub) || string(gotPriv2) != string(gotPriv) {
		t.Error("re-importing identical keys produced different exported bytes")
	}
}

// TestTorKeys_ImportKeys_Seed covers importing with a 32-byte seed instead
// of a full 64-byte expanded private key.
func TestTorKeys_ImportKeys_Seed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	seed := priv.Seed()

	km := NewTorKeyManager(t.TempDir())
	if err := km.ImportKeys(pub, seed); err != nil {
		t.Fatalf("ImportKeys() with seed error = %v", err)
	}

	gotPub, _, err := km.ExportKeys()
	if err != nil {
		t.Fatalf("ExportKeys() error = %v", err)
	}
	if string(gotPub) != string(pub) {
		t.Errorf("exported public key mismatch: got %x want %x", gotPub, pub)
	}
}

// TestTorKeys_ImportKeys_Errors covers boundary/error conditions: wrong
// private key length, wrong public key length, and a mismatched keypair.
func TestTorKeys_ImportKeys_Errors(t *testing.T) {
	pub1, priv1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair 1: %v", err)
	}
	pub2, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair 2: %v", err)
	}

	tests := []struct {
		name    string
		pub     []byte
		priv    []byte
		wantErr bool
	}{
		{"nil private key", pub1, nil, true},
		{"empty private key", pub1, []byte{}, true},
		{"short private key", pub1, priv1[:10], true},
		{"wrong public key length short", []byte{1, 2, 3}, priv1, true},
		{"wrong public key length long", append(append([]byte{}, pub1...), 0xFF), priv1, true},
		{"nil public key", nil, priv1, true},
		{"mismatched keypair", pub2, priv1, true},
		{"valid keypair", pub1, priv1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := NewTorKeyManager(t.TempDir())
			err := km.ImportKeys(tt.pub, tt.priv)
			if (err != nil) != tt.wantErr {
				t.Errorf("ImportKeys() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTorKeys_ExportKeys_Errors covers missing and malformed key files.
func TestTorKeys_ExportKeys_Errors(t *testing.T) {
	t.Run("missing keys directory", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		if _, _, err := km.ExportKeys(); err == nil {
			t.Error("expected error for missing keys directory, got nil")
		}
	})

	t.Run("missing public key only", func(t *testing.T) {
		dataDir := t.TempDir()
		km := NewTorKeyManager(dataDir)
		siteDir := filepath.Join(dataDir, "site")
		if err := os.MkdirAll(siteDir, 0700); err != nil {
			t.Fatalf("failed to create site dir: %v", err)
		}
		if err := km.writePrivateKey(filepath.Join(siteDir, "hs_ed25519_secret_key"), make([]byte, 32)); err != nil {
			t.Fatalf("failed to write private key: %v", err)
		}
		if _, _, err := km.ExportKeys(); err == nil {
			t.Error("expected error for missing public key, got nil")
		}
	})

	t.Run("truncated private key file", func(t *testing.T) {
		dataDir := t.TempDir()
		km := NewTorKeyManager(dataDir)
		siteDir := filepath.Join(dataDir, "site")
		if err := os.MkdirAll(siteDir, 0700); err != nil {
			t.Fatalf("failed to create site dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(siteDir, "hs_ed25519_secret_key"), []byte("too short"), 0600); err != nil {
			t.Fatalf("failed to write private key: %v", err)
		}
		if err := km.writePublicKey(filepath.Join(siteDir, "hs_ed25519_public_key"), make([]byte, 32)); err != nil {
			t.Fatalf("failed to write public key: %v", err)
		}
		if _, _, err := km.ExportKeys(); err == nil {
			t.Error("expected error for truncated private key file, got nil")
		}
	})
}

// TestTorKeys_ImportFromFile covers the raw 64-byte Tor key file format, the
// base64-encoded private key format, PEM (unsupported), and malformed input.
func TestTorKeys_ImportFromFile(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	t.Run("raw 64-byte private key file", func(t *testing.T) {
		dataDir := t.TempDir()
		km := NewTorKeyManager(dataDir)

		header := make([]byte, 32)
		copy(header, "== ed25519v1-secret: type0 ==")
		data := append(append([]byte{}, header...), priv.Seed()...)

		filePath := filepath.Join(t.TempDir(), "raw_key")
		if err := os.WriteFile(filePath, data, 0600); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}

		if err := km.ImportFromFile(filePath); err != nil {
			t.Fatalf("ImportFromFile() error = %v", err)
		}

		gotPub, _, err := km.ExportKeys()
		if err != nil {
			t.Fatalf("ExportKeys() error = %v", err)
		}
		if string(gotPub) != string(pub) {
			t.Errorf("exported public key mismatch: got %x want %x", gotPub, pub)
		}
	})

	t.Run("raw 64-byte public key file alone", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())

		header := make([]byte, 32)
		copy(header, "== ed25519v1-public: type0 ==")
		data := append(append([]byte{}, header...), pub...)

		filePath := filepath.Join(t.TempDir(), "raw_pub")
		if err := os.WriteFile(filePath, data, 0600); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}

		if err := km.ImportFromFile(filePath); err == nil {
			t.Error("expected error importing a public-key-only file, got nil")
		}
	})

	t.Run("base64 encoded private key", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())

		encoded := base64.StdEncoding.EncodeToString(priv)
		filePath := filepath.Join(t.TempDir(), "b64_key")
		if err := os.WriteFile(filePath, []byte(encoded), 0600); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}

		if err := km.ImportFromFile(filePath); err != nil {
			t.Fatalf("ImportFromFile() error = %v", err)
		}

		gotPub, _, err := km.ExportKeys()
		if err != nil {
			t.Fatalf("ExportKeys() error = %v", err)
		}
		if string(gotPub) != string(pub) {
			t.Errorf("exported public key mismatch: got %x want %x", gotPub, pub)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		if err := km.ImportFromFile(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
			t.Error("expected error for missing file, got nil")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		filePath := filepath.Join(t.TempDir(), "empty")
		if err := os.WriteFile(filePath, []byte{}, 0600); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}
		if err := km.ImportFromFile(filePath); err == nil {
			t.Error("expected error for empty/unsupported file, got nil")
		}
	})

	t.Run("garbage file", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		filePath := filepath.Join(t.TempDir(), "garbage")
		if err := os.WriteFile(filePath, []byte("not a key file at all, just text"), 0600); err != nil {
			t.Fatalf("failed to write fixture file: %v", err)
		}
		if err := km.ImportFromFile(filePath); err == nil {
			t.Error("expected error for garbage file, got nil")
		}
	})
}

// TestTorKeys_NormalizeTorPrivateKey covers the seed-size, full-key-size,
// and invalid-length boundary cases of the internal normalizer.
func TestTorKeys_NormalizeTorPrivateKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	seed := priv.Seed()

	tests := []struct {
		name    string
		key     []byte
		wantErr bool
	}{
		{"seed size", seed, false},
		{"full private key size", priv, false},
		{"zero length", nil, true},
		{"too short", seed[:10], true},
		{"too long", append(append([]byte{}, priv...), 0x00), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTorPrivateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeTorPrivateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(got) != ed25519.PrivateKeySize {
				t.Errorf("normalized key length = %d, want %d", len(got), ed25519.PrivateKeySize)
			}
		})
	}
}

// TestTorKeys_GetCurrentAddress covers the happy path (hostname file
// present, matches the address derived from the public key) and the
// missing-file error path.
func TestTorKeys_GetCurrentAddress(t *testing.T) {
	t.Run("missing hostname file", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		if _, err := km.GetCurrentAddress(); err == nil {
			t.Error("expected error for missing hostname file, got nil")
		}
	})

	t.Run("hostname file present after import", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatalf("failed to generate keypair: %v", err)
		}
		km := NewTorKeyManager(t.TempDir())
		if err := km.ImportKeys(pub, priv); err != nil {
			t.Fatalf("ImportKeys() error = %v", err)
		}

		address, err := km.GetCurrentAddress()
		if err != nil {
			t.Fatalf("GetCurrentAddress() error = %v", err)
		}

		want := publicKeyToOnionAddress(pub) + ".onion\n"
		if address != want {
			t.Errorf("GetCurrentAddress() = %q, want %q", address, want)
		}
	})
}

// TestTorKeys_DeleteKeys covers deleting an existing keys directory and the
// idempotent no-op case of deleting a directory that never existed.
func TestTorKeys_DeleteKeys(t *testing.T) {
	t.Run("delete existing keys", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatalf("failed to generate keypair: %v", err)
		}
		dataDir := t.TempDir()
		km := NewTorKeyManager(dataDir)
		if err := km.ImportKeys(pub, priv); err != nil {
			t.Fatalf("ImportKeys() error = %v", err)
		}
		if !km.KeysExist() {
			t.Fatal("expected keys to exist before delete")
		}

		if err := km.DeleteKeys(); err != nil {
			t.Fatalf("DeleteKeys() error = %v", err)
		}
		if km.KeysExist() {
			t.Error("expected keys to not exist after delete")
		}

		// Idempotency: deleting again should not error.
		if err := km.DeleteKeys(); err != nil {
			t.Errorf("second DeleteKeys() error = %v", err)
		}
	})

	t.Run("delete when nothing exists", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		if err := km.DeleteKeys(); err != nil {
			t.Errorf("DeleteKeys() on empty dir error = %v", err)
		}
	})
}

// TestTorKeys_KeysExist covers all four combinations of private/public key
// file presence.
func TestTorKeys_KeysExist(t *testing.T) {
	t.Run("neither present", func(t *testing.T) {
		km := NewTorKeyManager(t.TempDir())
		if km.KeysExist() {
			t.Error("expected KeysExist() = false when nothing on disk")
		}
	})

	t.Run("only private key present", func(t *testing.T) {
		dataDir := t.TempDir()
		km := NewTorKeyManager(dataDir)
		siteDir := filepath.Join(dataDir, "site")
		if err := os.MkdirAll(siteDir, 0700); err != nil {
			t.Fatalf("failed to create site dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(siteDir, "hs_ed25519_secret_key"), []byte("x"), 0600); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
		if km.KeysExist() {
			t.Error("expected KeysExist() = false when only private key present")
		}
	})

	t.Run("both present", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatalf("failed to generate keypair: %v", err)
		}
		km := NewTorKeyManager(t.TempDir())
		if err := km.ImportKeys(pub, priv); err != nil {
			t.Fatalf("ImportKeys() error = %v", err)
		}
		if !km.KeysExist() {
			t.Error("expected KeysExist() = true after import")
		}
	})
}
