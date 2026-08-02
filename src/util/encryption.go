package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// ErrEncryptionKeyInvalid is returned when the configured
// server.security.encryption_key does not decode to a 32-byte AES-256 key.
var ErrEncryptionKeyInvalid = errors.New("encryption key must be a base64-encoded 32-byte value")

// EncryptAtRest encrypts plaintext with AES-256-GCM using the project-wide
// server.security.encryption_key (base64-encoded 32-byte key) per AI.md
// PART 11 ("Cryptographic Keys" -> "Server Encryption Key"). Used for all
// at-rest sensitive server data: 2FA/TOTP secrets, security report bodies,
// and any future at-rest encrypted data. The returned string is
// base64-encoded (nonce || ciphertext || tag).
func EncryptAtRest(base64Key, plaintext string) (string, error) {
	block, err := newAESCipher(base64Key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptAtRest reverses EncryptAtRest using the same
// server.security.encryption_key.
func DecryptAtRest(base64Key, encoded string) (string, error) {
	block, err := newAESCipher(base64Key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// IsEncryptedAtRest reports whether a stored value looks like output from
// EncryptAtRest (base64, decodes to at least a GCM nonce + tag) rather than
// legacy plaintext. Used by the one-time migration path to tell already
// re-encrypted secrets apart from pre-existing plaintext ones without a
// separate format-version marker column.
func IsEncryptedAtRest(base64Key, value string) bool {
	if value == "" {
		return false
	}
	_, err := DecryptAtRest(base64Key, value)
	return err == nil
}

// newAESCipher decodes a base64-encoded 32-byte key and builds the AES block
// cipher used by EncryptAtRest/DecryptAtRest.
func newAESCipher(base64Key string) (cipher.Block, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, ErrEncryptionKeyInvalid
	}
	if len(key) != 32 {
		return nil, ErrEncryptionKeyInvalid
	}
	return aes.NewCipher(key)
}
