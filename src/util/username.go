package util

import (
	"fmt"
	"regexp"
	"strings"
)

// Username validation rules per AI.md PART 22
const (
	MinUsernameLength = 2
	MaxUsernameLength = 39
)

// Username regex per AI.md PART 22:
// - Must start with a lowercase letter or digit
// - Can contain lowercase letters, digits, and hyphens
// - Must end with a lowercase letter or digit (not a hyphen)
// - Length 2-39 characters
var usernameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// ValidateUsername validates a regular-user username according to AI.md PART 34:
// the shared syntax rules of validateUsernameSyntax, then the reserved-name list
// (PART 16) and the username blocklist. Rejects uppercase letters and
// surrounding whitespace (does NOT normalize).
func ValidateUsername(username string) error {
	if err := validateUsernameSyntax(username); err != nil {
		return err
	}

	if IsReservedName(username) {
		return fmt.Errorf("this username is reserved and cannot be used")
	}

	// Create a fake email to use the blocklist checker
	fakeEmail := username + "@example.com"
	if IsUsernameBlocked(fakeEmail) {
		return fmt.Errorf("this username is reserved and cannot be used")
	}

	return nil
}

// ValidateAdminUsername validates a Server Admin username (AI.md PART 17 and
// PART 34): the same syntax and length rules as any username, exempt from the
// public username blocklist, but never equal to an admin section name because
// /server/{admin_path}/{name}/* would then be ambiguous.
func ValidateAdminUsername(username string) error {
	if err := validateUsernameSyntax(username); err != nil {
		return err
	}

	if IsAdminSectionName(username) {
		return fmt.Errorf("this username is reserved and cannot be used")
	}

	return nil
}

// validateUsernameSyntax checks whitespace, case, length, allowed characters
// and consecutive hyphens shared by user and admin usernames.
func validateUsernameSyntax(username string) error {
	// Check for surrounding whitespace (must not be present)
	if username != strings.TrimSpace(username) {
		return fmt.Errorf("username cannot contain leading or trailing whitespace")
	}

	// Check for uppercase letters (must be rejected, not normalized)
	if regexp.MustCompile(`[A-Z]`).MatchString(username) {
		return fmt.Errorf("username must contain only lowercase letters")
	}

	// Check length
	if len(username) < MinUsernameLength {
		return fmt.Errorf("username must be at least %d characters long", MinUsernameLength)
	}

	if len(username) > MaxUsernameLength {
		return fmt.Errorf("username must be no more than %d characters long", MaxUsernameLength)
	}

	// Check format with regex
	if !usernameRegex.MatchString(username) {
		// Provide more specific error messages
		if !regexp.MustCompile(`^[a-z0-9]`).MatchString(username) {
			return fmt.Errorf("username must start with a lowercase letter or digit")
		}
		if regexp.MustCompile(`-$`).MatchString(username) {
			return fmt.Errorf("username cannot end with hyphen")
		}
		if regexp.MustCompile(`[^a-z0-9-]`).MatchString(username) {
			return fmt.Errorf("username can only contain lowercase letters (a-z), numbers (0-9), and hyphen (-)")
		}
		return fmt.Errorf("username format is invalid")
	}

	// Check for consecutive special characters
	if strings.Contains(username, "--") {
		return fmt.Errorf("username cannot contain consecutive hyphens (--)")
	}

	return nil
}

// NormalizeUsername converts username to lowercase and trims spaces
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidatePhone validates a phone number
// Basic validation - can be enhanced with libphonenumber later
func ValidatePhone(phone string) error {
	if phone == "" {
		// Phone is optional
		return nil
	}

	// Remove common formatting characters
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '+' {
			return r
		}
		return -1
	}, phone)

	// Check length (basic validation)
	if len(cleaned) < 10 || len(cleaned) > 15 {
		return fmt.Errorf("phone number must be between 10 and 15 digits")
	}

	return nil
}

// NormalizePhone removes formatting from phone number
func NormalizePhone(phone string) string {
	if phone == "" {
		return ""
	}

	// Keep only digits and leading +
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '+' {
			return r
		}
		return -1
	}, phone)

	return cleaned
}
