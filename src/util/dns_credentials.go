package util

import (
	"errors"
	"fmt"
	"strings"

	"github.com/webappsgo/wthr/src/config"
)

// DNSCredentialPrefix marks a dns_credentials value that is AES-256-GCM ciphertext (AI.md PART 15).
const DNSCredentialPrefix = "enc:v1:"

// DNSCredentialMask is what every view, API response and bundle shows instead of a value.
const DNSCredentialMask = "********"

// SealDNSCredential returns value as enc:v1:{base64 of nonce + ciphertext} under key. A value that
// is already sealed is returned unchanged.
func SealDNSCredential(key, value string) (string, error) {
	if strings.HasPrefix(value, DNSCredentialPrefix) {
		return value, nil
	}
	sealed, err := EncryptAtRest(key, value)
	if err != nil {
		return "", err
	}
	return DNSCredentialPrefix + sealed, nil
}

// OpenDNSCredential returns the plaintext of a stored value. A value without the enc:v1: prefix is
// plaintext an operator has just written and is returned as is. usedPrevious reports that only the
// previous key opened it.
func OpenDNSCredential(key, stored string) (plaintext string, usedPrevious bool, err error) {
	if !strings.HasPrefix(stored, DNSCredentialPrefix) {
		return stored, false, nil
	}
	return DecryptAtRestGrace(key, strings.TrimPrefix(stored, DNSCredentialPrefix))
}

// SealDNSCredentials seals every plaintext value of the dns-01 credential map in place and reports
// whether anything changed, so the caller rewrites server.yml.
func SealDNSCredentials(key string, credentials map[string]string) (changed bool, err error) {
	for name, value := range credentials {
		if value == "" || strings.HasPrefix(value, DNSCredentialPrefix) {
			continue
		}
		sealed, err := SealDNSCredential(key, value)
		if err != nil {
			return changed, fmt.Errorf("dns_credentials %s: %w", name, err)
		}
		credentials[name] = sealed
		changed = true
	}
	return changed, nil
}

// OpenDNSCredentials returns the plaintext credential map for one provider call. The result lives
// in memory only for that call.
func OpenDNSCredentials(key string, credentials map[string]string) (map[string]string, error) {
	opened := make(map[string]string, len(credentials))
	for name, stored := range credentials {
		plaintext, _, err := OpenDNSCredential(key, stored)
		if err != nil {
			return nil, fmt.Errorf("dns_credentials %s cannot be decrypted", name)
		}
		opened[name] = plaintext
	}
	return opened, nil
}

// MaskDNSCredentials returns the credential names with every value replaced by the mask.
func MaskDNSCredentials(credentials map[string]string) map[string]string {
	masked := make(map[string]string, len(credentials))
	for name := range credentials {
		masked[name] = DNSCredentialMask
	}
	return masked
}

// SealConfigDNSCredentials seals the plaintext dns-01 credentials of cfg and, when any value was
// sealed, rewrites server.yml atomically with mode 0600. It runs at start and after every reload.
func SealConfigDNSCredentials(cfg *config.AppConfig) error {
	if cfg == nil || len(cfg.Server.SSL.LetsEncrypt.DNSCredentials) == 0 {
		return nil
	}
	key := cfg.Server.Security.EncryptionKey
	if key == "" {
		return errors.New("no encryption key is configured")
	}
	changed, err := SealDNSCredentials(key, cfg.Server.SSL.LetsEncrypt.DNSCredentials)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return config.SaveConfig(cfg)
}

// RebaseDNSCredentials re-encrypts every sealed value from oldKey to newKey and reports whether
// anything changed. A value that already opens under newKey, or under neither key, is left alone.
func RebaseDNSCredentials(oldKey, newKey string, credentials map[string]string) (changed bool, err error) {
	for name, stored := range credentials {
		if !strings.HasPrefix(stored, DNSCredentialPrefix) {
			continue
		}
		body := strings.TrimPrefix(stored, DNSCredentialPrefix)
		if _, openErr := DecryptAtRest(newKey, body); openErr == nil {
			continue
		}
		plaintext, openErr := DecryptAtRest(oldKey, body)
		if openErr != nil {
			continue
		}
		sealed, sealErr := SealDNSCredential(newKey, plaintext)
		if sealErr != nil {
			return changed, sealErr
		}
		credentials[name] = sealed
		changed = true
	}
	return changed, nil
}
