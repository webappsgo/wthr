package util

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// validTestKey returns a fresh base64-encoded random 32-byte AES-256 key.
func validTestKey(t *testing.T) string {
	t.Helper()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestEncryptDecryptAtRestRoundTrip(t *testing.T) {
	key := validTestKey(t)
	plaintext := "a 2FA TOTP secret"

	encrypted, err := EncryptAtRest(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptAtRest() error = %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("EncryptAtRest() returned the plaintext unchanged")
	}

	decrypted, err := DecryptAtRest(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptAtRest() error = %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("DecryptAtRest() = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptAtRestNoncesDiffer(t *testing.T) {
	key := validTestKey(t)

	first, err := EncryptAtRest(key, "same plaintext")
	if err != nil {
		t.Fatalf("EncryptAtRest() error = %v", err)
	}
	second, err := EncryptAtRest(key, "same plaintext")
	if err != nil {
		t.Fatalf("EncryptAtRest() error = %v", err)
	}
	if first == second {
		t.Fatal("EncryptAtRest() produced identical ciphertext for two calls, want a fresh random nonce each time")
	}
}

func TestEncryptAtRestInvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"not base64", "not-valid-base64!!!"},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := EncryptAtRest(tt.key, "plaintext"); err != ErrEncryptionKeyInvalid {
				t.Fatalf("EncryptAtRest() error = %v, want %v", err, ErrEncryptionKeyInvalid)
			}
		})
	}
}

func TestDecryptAtRestInvalidKey(t *testing.T) {
	if _, err := DecryptAtRest("not-valid-base64!!!", "ignored"); err != ErrEncryptionKeyInvalid {
		t.Fatalf("DecryptAtRest() error = %v, want %v", err, ErrEncryptionKeyInvalid)
	}
}

func TestDecryptAtRestMalformedCiphertext(t *testing.T) {
	key := validTestKey(t)

	t.Run("not base64", func(t *testing.T) {
		if _, err := DecryptAtRest(key, "not-valid-base64!!!"); err == nil {
			t.Fatal("DecryptAtRest() error = nil, want a base64 decode error")
		}
	})

	t.Run("too short for a nonce", func(t *testing.T) {
		if _, err := DecryptAtRest(key, base64.StdEncoding.EncodeToString([]byte("x"))); err == nil {
			t.Fatal("DecryptAtRest() error = nil, want a ciphertext-too-short error")
		}
	})

	t.Run("tampered ciphertext fails GCM auth", func(t *testing.T) {
		encrypted, err := EncryptAtRest(key, "plaintext")
		if err != nil {
			t.Fatalf("EncryptAtRest() error = %v", err)
		}
		raw, err := base64.StdEncoding.DecodeString(encrypted)
		if err != nil {
			t.Fatalf("base64 decode error = %v", err)
		}
		raw[len(raw)-1] ^= 0xFF
		tampered := base64.StdEncoding.EncodeToString(raw)

		if _, err := DecryptAtRest(key, tampered); err == nil {
			t.Fatal("DecryptAtRest() error = nil, want a GCM authentication failure")
		}
	})
}

func TestIsEncryptedAtRest(t *testing.T) {
	key := validTestKey(t)

	encrypted, err := EncryptAtRest(key, "a secret value")
	if err != nil {
		t.Fatalf("EncryptAtRest() error = %v", err)
	}

	if !IsEncryptedAtRest(key, encrypted) {
		t.Error("IsEncryptedAtRest() = false for genuinely encrypted value, want true")
	}
	if IsEncryptedAtRest(key, "plain legacy text") {
		t.Error("IsEncryptedAtRest() = true for legacy plaintext, want false")
	}
	if IsEncryptedAtRest(key, "") {
		t.Error("IsEncryptedAtRest() = true for empty string, want false")
	}
}
