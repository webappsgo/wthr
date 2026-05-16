package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret generates a new TOTP secret for a user
// Returns the secret key and a QR code data URL
func GenerateTOTPSecret(email, issuer string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Generate QR code
	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	if err := png.Encode(&buf, img); err != nil {
		return "", "", fmt.Errorf("failed to encode QR code: %w", err)
	}

	// Convert to data URL
	qrDataURL := fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()))

	return key.Secret(), qrDataURL, nil
}

// VerifyTOTP verifies a TOTP code against a secret
func VerifyTOTP(secret, code string) (bool, error) {
	valid := totp.Validate(code, secret)
	return valid, nil
}

// GenerateOTPAuthURL generates an otpauth:// URL for manual entry
// This is useful if users can't scan the QR code
func GenerateOTPAuthURL(email, secret, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer, email, secret, issuer)
}
