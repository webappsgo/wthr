// Package utils provides email validation per AI.md PART 33
package utils

import (
	"errors"
	"regexp"
	"strings"
)

// Email validation errors
var (
	ErrEmailTooLong           = errors.New("email too long (max 254 characters)")
	ErrEmailInvalidFormat     = errors.New("invalid email format")
	ErrEmailLocalPartLength   = errors.New("invalid local part length (1-64 characters)")
	ErrEmailLocalPartDot      = errors.New("local part cannot start or end with dot")
	ErrEmailConsecutiveDots   = errors.New("local part cannot have consecutive dots")
	ErrEmailDomainLength      = errors.New("invalid domain length")
	ErrEmailDomainNoTLD       = errors.New("domain must have valid TLD")
	ErrEmailLocalPartChars    = errors.New("invalid characters in local part")
	ErrEmailDomainFormat      = errors.New("invalid domain format")
	ErrEmailDisposableDomain  = errors.New("disposable email domains are not allowed")
)

// localPartRegex validates allowed characters in local part
// Per AI.md PART 33: a-z, 0-9, ., +, _, -
var localPartRegex = regexp.MustCompile(`^[a-z0-9.+_-]+$`)

// domainRegex validates domain format
// Per AI.md PART 33: Must have valid TLD
var domainRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]\.[a-z]{2,}$`)

// singleCharDomainRegex for single character domains before TLD
var singleCharDomainRegex = regexp.MustCompile(`^[a-z0-9]\.[a-z]{2,}$`)

// EmailDomainBlocklist contains known disposable email domains
// Per AI.md PART 33: Optional blocklist
var EmailDomainBlocklist = []string{
	// Common disposable email domains
	"tempmail.com",
	"throwaway.email",
	"guerrillamail.com",
	"mailinator.com",
	"10minutemail.com",
	"trashmail.com",
	"fakeinbox.com",
	"sharklasers.com",
	"guerrillamail.info",
	"grr.la",
	"spam4.me",
	"temp-mail.org",
	"disposablemail.com",
	"yopmail.com",
	"maildrop.cc",
	"getairmail.com",
	"mohmal.com",
	"tempail.com",
	"discard.email",
	"mailnesia.com",
}

// ValidateEmail validates an email address per AI.md PART 33 (RFC 5322 compliant)
// Returns nil if valid, error if invalid
func ValidateEmail(email string) error {
	email = strings.TrimSpace(strings.ToLower(email))

	// Length checks
	// Per AI.md PART 33: Max total length 254 characters (RFC 5321)
	if len(email) > 254 {
		return ErrEmailTooLong
	}

	// Split into local and domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ErrEmailInvalidFormat
	}

	local, domain := parts[0], parts[1]

	// Local part checks
	// Per AI.md PART 33: Min 1 char, Max 64 chars (RFC 5321)
	if len(local) == 0 || len(local) > 64 {
		return ErrEmailLocalPartLength
	}

	// Per AI.md PART 33: Cannot start or end with dot
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return ErrEmailLocalPartDot
	}

	// Per AI.md PART 33: No consecutive dots
	if strings.Contains(local, "..") {
		return ErrEmailConsecutiveDots
	}

	// Domain checks
	// Per AI.md PART 33: Max 255 characters (RFC 5321)
	if len(domain) == 0 || len(domain) > 255 {
		return ErrEmailDomainLength
	}

	// Per AI.md PART 33: Must have valid TLD
	if !strings.Contains(domain, ".") {
		return ErrEmailDomainNoTLD
	}

	// Per AI.md PART 33: Allowed chars in local part (a-z, 0-9, ., +, _, -)
	if !localPartRegex.MatchString(local) {
		return ErrEmailLocalPartChars
	}

	// Per AI.md PART 33: Valid domain format
	if !domainRegex.MatchString(domain) && !singleCharDomainRegex.MatchString(domain) {
		return ErrEmailDomainFormat
	}

	return nil
}

// ValidateEmailWithBlocklist validates email and checks against disposable domain blocklist
func ValidateEmailWithBlocklist(email string) error {
	// First validate format
	if err := ValidateEmail(email); err != nil {
		return err
	}

	// Check blocklist
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ErrEmailInvalidFormat
	}

	domain := parts[1]

	for _, blocked := range EmailDomainBlocklist {
		if domain == blocked {
			return ErrEmailDisposableDomain
		}
	}

	return nil
}

// NormalizeEmail normalizes an email address (lowercase, trimmed)
// Per AI.md PART 33: Case-insensitive, stored lowercase
func NormalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

// MaskEmail returns a masked version of email for display
// Per AI.md PART 33: j***n@e***.com format for admin view
func MaskEmail(email string) string {
	email = NormalizeEmail(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}

	local, domain := parts[0], parts[1]
	domainParts := strings.Split(domain, ".")

	// Mask local part (keep first and last char if long enough)
	var maskedLocal string
	if len(local) <= 2 {
		maskedLocal = strings.Repeat("*", len(local))
	} else {
		maskedLocal = string(local[0]) + strings.Repeat("*", len(local)-2) + string(local[len(local)-1])
	}

	// Mask domain (keep first char and TLD)
	var maskedDomain string
	if len(domainParts) >= 2 {
		firstPart := domainParts[0]
		tld := domainParts[len(domainParts)-1]
		if len(firstPart) <= 1 {
			maskedDomain = firstPart + "." + tld
		} else {
			maskedDomain = string(firstPart[0]) + strings.Repeat("*", len(firstPart)-1) + "." + tld
		}
	} else {
		maskedDomain = strings.Repeat("*", len(domain))
	}

	return maskedLocal + "@" + maskedDomain
}
