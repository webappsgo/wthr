package util

import (
	"testing"
)

// TestValidateUsername tests username validation per AI.md PART 22
func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
		errMsg   string
	}{
		// Valid usernames
		{"valid_simple", "abc", false, ""},
		// Changed from "user123" which is blocked
		{"valid_with_numbers", "player123", false, ""},
		{"valid_with_hyphen", "player-name", false, ""},
		{"valid_mixed", "player-123-test", false, ""},
		// 39 chars (AI.md PART 22 maximum)
		{"valid_max_length", "abcdefghij1234567890abcdefghij123456789", false, ""},
		// AI.md PART 22 allows a leading digit
		{"valid_starts_with_digit", "1player", false, ""},
		// AI.md PART 22 minimum length is 2
		{"valid_min_length", "ab", false, ""},

		// Invalid: Length
		{"too_short", "a", true, "at least 2 characters"},
		// 40 chars
		{"too_long", "abcdefghij1234567890abcdefghij1234567890", true, "no more than 39 characters"},

		// Invalid: Must start with alphanumeric
		{"starts_with_underscore", "_user", true, "must start with a lowercase letter or digit"},
		{"starts_with_hyphen", "-user", true, "must start with a lowercase letter or digit"},

		// Invalid: Cannot end with hyphen
		{"ends_with_underscore", "user_", true, "can only contain lowercase letters"},
		{"ends_with_hyphen", "user-", true, "cannot end with hyphen"},

		// Invalid: Uppercase letters are rejected, never normalized
		{"uppercase", "User", true, "only lowercase"},
		{"mixed_case", "UsErNaMe", true, "only lowercase"},

		// Invalid: Consecutive special characters
		{"consecutive_hyphens", "user--name", true, "consecutive hyphens"},
		{"consecutive_underscores", "user__name", true, "can only contain lowercase letters"},
		{"consecutive_mixed_1", "user_-name", true, "can only contain lowercase letters"},
		{"consecutive_mixed_2", "user-_name", true, "can only contain lowercase letters"},

		// Invalid: Invalid characters
		{"with_space", "user name", true, "can only contain lowercase letters"},
		{"with_dot", "user.name", true, "can only contain lowercase letters"},
		{"with_at", "user@name", true, "can only contain lowercase letters"},
		{"with_special", "user!name", true, "can only contain lowercase letters"},

		// Invalid: Blocklist - exact matches
		{"blocklist_admin", "admin", true, "reserved and cannot be used"},
		{"blocklist_root", "root", true, "reserved and cannot be used"},
		{"blocklist_system", "system", true, "reserved and cannot be used"},
		{"blocklist_mod", "mod", true, "reserved and cannot be used"},

		// Invalid: Blocklist - critical substring terms
		{"substring_admin", "myadmin", true, "reserved and cannot be used"},
		{"substring_admin_middle", "myadminuser", true, "reserved and cannot be used"},
		{"substring_root", "rootuser", true, "reserved and cannot be used"},
		{"substring_official", "officialname", true, "reserved and cannot be used"},
		{"substring_verified", "verifieduser", true, "reserved and cannot be used"},

		// Invalid: Blocklist - with simple suffixes
		{"blocklist_test123", "test123", true, "reserved and cannot be used"},
		{"blocklist_user_1", "user-1", true, "reserved and cannot be used"},
		{"blocklist_guest_2", "guest-2", true, "reserved and cannot be used"},

		// Valid: Blocklist - complex suffixes (allowed)
		// "test" + "ing" = complex suffix
		{"testing_allowed", "testing", false, ""},
		// "user" + "name" = complex suffix
		{"username_allowed", "username", false, ""},

		// Valid: Not on blocklist
		{"valid_custom", "johndoe", false, ""},
		{"valid_numbers", "player42", false, ""},
		{"valid_complex", "cool-username-123", false, ""},

		// Edge cases
		{"empty", "", true, "at least 2 characters"},
		{"whitespace_only", "   ", true, "leading or trailing whitespace"},
		// Surrounding whitespace is rejected, never trimmed away
		{"with_leading_space", "  user", true, "leading or trailing whitespace"},
		{"with_trailing_space", "user  ", true, "leading or trailing whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUsername(%q) error = %v, wantErr %v", tt.username, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateUsername(%q) error message = %q, want substring %q", tt.username, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestNormalizeUsername tests username normalization
func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     string
	}{
		{"lowercase", "user", "user"},
		{"uppercase", "USER", "user"},
		{"mixed_case", "UsEr", "user"},
		{"with_spaces", "  user  ", "user"},
		{"complex", "  JohnDoe123  ", "johndoe123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUsername(tt.username)
			if got != tt.want {
				t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.username, got, tt.want)
			}
		})
	}
}

// TestValidatePhone tests phone number validation.
func TestValidatePhone(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{"empty_is_optional", "", false},
		{"valid_10_digits", "1234567890", false},
		{"valid_with_formatting", "(123) 456-7890", false},
		{"valid_with_leading_plus", "+12345678901", false},
		{"valid_15_digits", "123456789012345", false},
		{"too_short_9_digits", "123456789", true},
		{"too_long_16_digits", "1234567890123456", true},
		{"letters_stripped_leaves_too_short", "call-me-maybe", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhone(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePhone(%q) error = %v, wantErr %v", tt.phone, err, tt.wantErr)
			}
		})
	}
}

// TestNormalizePhone tests phone number normalization strips everything
// except digits and a leading '+'.
func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		want  string
	}{
		{"empty", "", ""},
		{"already_clean", "1234567890", "1234567890"},
		{"strips_formatting", "(123) 456-7890", "1234567890"},
		{"keeps_plus", "+1 (234) 567-8901", "+12345678901"},
		{"strips_letters", "1-800-FLOWERS", "1800"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePhone(tt.phone)
			if got != tt.want {
				t.Errorf("NormalizePhone(%q) = %q, want %q", tt.phone, got, tt.want)
			}
		})
	}
}

// TestUsernameRegex tests the username regex pattern directly
func TestUsernameRegex(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		// Should match
		{"min_length", "ab", true},
		// 39 chars
		{"max_length", "abcdefghij1234567890abcdefghij123456789", true},
		{"with_numbers", "user123", true},
		{"with_hyphen", "user-name", true},
		{"complex", "user-123-test-456", true},
		{"starts_with_digit", "1user", true},

		// Should NOT match
		{"too_short", "a", true},
		// The regex checks shape; ValidateUsername enforces length separately.
		{"length_boundary_short", "a", true},
		{"length_boundary_long", "abcdefghij1234567890abcdefghij1234567890", true},
		{"starts_with_underscore", "_user", false},
		{"starts_with_hyphen", "-user", false},
		{"with_underscore", "user_name", false},
		{"ends_with_underscore", "user_", false},
		{"ends_with_hyphen", "user-", false},
		{"uppercase", "User", false},
		{"with_space", "user name", false},
		{"with_dot", "user.name", false},
		{"with_special", "user@name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := usernameRegex.MatchString(tt.username)
			if got != tt.want {
				t.Errorf("usernameRegex.MatchString(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}
