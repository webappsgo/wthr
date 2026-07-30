package utils

import (
	"strings"
	"testing"
)

// TestHashPasswordAndVerifyPassword covers the full Argon2id roundtrip: a
// correct password verifies, an incorrect one doesn't, and two hashes of the
// same password differ (random salt).
func TestHashPasswordAndVerifyPassword(t *testing.T) {
	hash1, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash1, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("hash format = %q, want PHC argon2id prefix", hash1)
	}

	hash2, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash1 == hash2 {
		t.Error("two hashes of the same password are identical, want different salts")
	}

	ok, err := VerifyPassword("correct-horse-battery-staple", hash1)
	if err != nil {
		t.Fatalf("VerifyPassword correct: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword with correct password = false, want true")
	}

	ok, err = VerifyPassword("wrong-password", hash1)
	if err != nil {
		t.Fatalf("VerifyPassword wrong: %v", err)
	}
	if ok {
		t.Error("VerifyPassword with wrong password = true, want false")
	}
}

// TestHashPassword_EmptyPassword still produces a valid, verifiable hash.
func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(\"\"): %v", err)
	}
	ok, err := VerifyPassword("", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword(\"\", hash) = false, want true")
	}
}

// TestVerifyPassword_MalformedHash covers the format-error paths.
func TestVerifyPassword_MalformedHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"too_few_parts", "$argon2id$v=19$m=65536,t=3,p=4$saltonly"},
		{"wrong_algorithm", "$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"},
		{"bad_salt_encoding", "$argon2id$v=19$m=65536,t=3,p=4$not!base64$aGFzaA"},
		{"bad_hash_encoding", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$not!base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword("anything", tt.hash)
			if err == nil {
				t.Errorf("VerifyPassword with %q: err = nil, want error", tt.hash)
			}
			if ok {
				t.Errorf("VerifyPassword with malformed hash = true, want false")
			}
		})
	}
}

// TestIsBcryptHash covers all three bcrypt prefixes and non-bcrypt inputs.
func TestIsBcryptHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
		want bool
	}{
		{"prefix_2a", "$2a$10$abcdefghijklmnopqrstuv", true},
		{"prefix_2b", "$2b$10$abcdefghijklmnopqrstuv", true},
		{"prefix_2y", "$2y$10$abcdefghijklmnopqrstuv", true},
		{"argon2id", "$argon2id$v=19$m=65536,t=3,p=4$salt$hash", false},
		{"empty", "", false},
		{"garbage", "not-a-hash", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBcryptHash(tt.hash); got != tt.want {
				t.Errorf("IsBcryptHash(%q) = %v, want %v", tt.hash, got, tt.want)
			}
		})
	}
}

// TestIsArgon2idHash covers the argon2id prefix check.
func TestIsArgon2idHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
		want bool
	}{
		{"argon2id", "$argon2id$v=19$m=65536,t=3,p=4$salt$hash", true},
		{"bcrypt", "$2a$10$abcdefghijklmnopqrstuv", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsArgon2idHash(tt.hash); got != tt.want {
				t.Errorf("IsArgon2idHash(%q) = %v, want %v", tt.hash, got, tt.want)
			}
		})
	}
}

// TestHashAPIToken verifies deterministic SHA-256 hex output and that
// different inputs produce different hashes.
func TestHashAPIToken(t *testing.T) {
	h1 := HashAPIToken("token-abc")
	h2 := HashAPIToken("token-abc")
	h3 := HashAPIToken("token-xyz")

	if h1 != h2 {
		t.Errorf("HashAPIToken not deterministic: %q != %q", h1, h2)
	}
	if h1 == h3 {
		t.Error("HashAPIToken produced same hash for different inputs")
	}
	if len(h1) != 64 {
		t.Errorf("HashAPIToken length = %d, want 64 (SHA-256 hex)", len(h1))
	}
}

// TestGenerateAPIToken verifies the prefix is applied and tokens are unique.
func TestGenerateAPIToken(t *testing.T) {
	token1, err := GenerateAPIToken("adm_")
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if !strings.HasPrefix(token1, "adm_") {
		t.Errorf("token = %q, want adm_ prefix", token1)
	}

	token2, err := GenerateAPIToken("adm_")
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if token1 == token2 {
		t.Error("two generated tokens are identical, want unique random tokens")
	}
}
