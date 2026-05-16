package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// Username validation rules per AI.md PART 22
const (
	MinUsernameLength = 3
	MaxUsernameLength = 32
)

// Username regex per AI.md PART 22:
// - Must start with a-z (lowercase letter)
// - Can contain a-z, 0-9, _, - (lowercase only)
// - Must end with a-z or 0-9 (not _ or -)
// - Length 3-32 characters
var usernameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,30}[a-z0-9]$`)

// ValidateUsername validates a username according to AI.md PART 22:
// - Length between 3 and 32 characters
// - Only lowercase letters (a-z), numbers (0-9), underscore (_), hyphen (-)
// - Must start with a letter (a-z)
// - Cannot end with underscore or hyphen
// - No consecutive __, --, _-, -_
// - Not on the blocklist
func ValidateUsername(username string) error {
	// Trim spaces
	username = strings.TrimSpace(username)

	// Convert to lowercase per AI.md PART 22 (case-insensitive)
	username = strings.ToLower(username)

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
		if !regexp.MustCompile(`^[a-z]`).MatchString(username) {
			return fmt.Errorf("username must start with a lowercase letter (a-z)")
		}
		if regexp.MustCompile(`[_-]$`).MatchString(username) {
			return fmt.Errorf("username cannot end with underscore or hyphen")
		}
		if regexp.MustCompile(`[^a-z0-9_-]`).MatchString(username) {
			return fmt.Errorf("username can only contain lowercase letters (a-z), numbers (0-9), underscore (_), and hyphen (-)")
		}
		return fmt.Errorf("username format is invalid")
	}

	// Check for consecutive special characters per AI.md PART 22
	if strings.Contains(username, "__") {
		return fmt.Errorf("username cannot contain consecutive underscores (__)")
	}
	if strings.Contains(username, "--") {
		return fmt.Errorf("username cannot contain consecutive hyphens (--)")
	}
	if strings.Contains(username, "_-") || strings.Contains(username, "-_") {
		return fmt.Errorf("username cannot contain consecutive underscore and hyphen (_- or -_)")
	}

	// Check against blocklist
	// Create a fake email to use the blocklist checker
	fakeEmail := username + "@example.com"
	if IsUsernameBlocked(fakeEmail) {
		return fmt.Errorf("this username is reserved and cannot be used")
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
