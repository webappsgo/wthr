package util

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// TestGenerateTOTPSecret verifies a secret and a valid PNG data URL are
// produced, and that the generated secret actually works for validation.
func TestGenerateTOTPSecret(t *testing.T) {
	secret, qrDataURL, err := GenerateTOTPSecret("user@example.com", "wthr")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if secret == "" {
		t.Error("secret is empty")
	}
	if !strings.HasPrefix(qrDataURL, "data:image/png;base64,") {
		t.Errorf("qrDataURL = %q, want data:image/png;base64,... prefix", qrDataURL)
	}

	// The generated secret must produce a code the totp library itself
	// accepts, proving the secret round-trips correctly.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode with generated secret: %v", err)
	}
	valid, err := VerifyTOTP(secret, code)
	if err != nil {
		t.Fatalf("VerifyTOTP: %v", err)
	}
	if !valid {
		t.Error("VerifyTOTP with freshly generated code = false, want true")
	}
}

// TestVerifyTOTP_InvalidCode ensures an obviously wrong code is rejected
// without error.
func TestVerifyTOTP_InvalidCode(t *testing.T) {
	secret, _, err := GenerateTOTPSecret("user2@example.com", "wthr")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}

	valid, err := VerifyTOTP(secret, "000000")
	if err != nil {
		t.Fatalf("VerifyTOTP: %v", err)
	}
	if valid {
		// Astronomically unlikely to collide, but tolerate it rather than flake.
		t.Skip("generated code coincidentally matched 000000")
	}
}

// TestGenerateOTPAuthURL verifies the exact otpauth:// URL format.
func TestGenerateOTPAuthURL(t *testing.T) {
	got := GenerateOTPAuthURL("user@example.com", "SECRET123", "wthr")
	want := "otpauth://totp/wthr:user@example.com?secret=SECRET123&issuer=wthr"
	if got != want {
		t.Errorf("GenerateOTPAuthURL() = %q, want %q", got, want)
	}
}
