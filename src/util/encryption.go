package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"sync/atomic"
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

// previousKeyProvider returns the previous server.security.encryption_key while its 30 day grace
// period lasts, or "" when there is none (AI.md PART 11 "Rotation contract").
var previousKeyProvider atomic.Pointer[func() string]

// SetPreviousEncryptionKeyProvider installs the function that supplies the previous key.
func SetPreviousEncryptionKeyProvider(provider func() string) {
	if provider == nil {
		previousKeyProvider.Store(nil)
		return
	}
	previousKeyProvider.Store(&provider)
}

// DecryptAtRestGrace decrypts with the current key and, when that fails, with the previous key
// during the grace period. usedPrevious is true when only the previous key worked, which tells
// the caller the row still needs re-encrypting under the current key.
func DecryptAtRestGrace(currentKey, encoded string) (plaintext string, usedPrevious bool, err error) {
	plaintext, err = DecryptAtRest(currentKey, encoded)
	if err == nil {
		return plaintext, false, nil
	}
	provider := previousKeyProvider.Load()
	if provider == nil {
		return "", false, err
	}
	previous := (*provider)()
	if previous == "" || previous == currentKey {
		return "", false, err
	}
	if fallback, fallbackErr := DecryptAtRest(previous, encoded); fallbackErr == nil {
		return fallback, true, nil
	}
	return "", false, err
}

// IsEncryptedAtRestGrace reports whether a value opens under the current key or the previous one.
func IsEncryptedAtRestGrace(currentKey, value string) bool {
	if value == "" {
		return false
	}
	_, _, err := DecryptAtRestGrace(currentKey, value)
	return err == nil
}
