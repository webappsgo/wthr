package utils

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
		{"valid_with_underscore", "player_name", false, ""},
		{"valid_with_hyphen", "player-name", false, ""},
		{"valid_mixed", "player_123-test", false, ""},
		// 32 chars
		{"valid_max_length", "abcdefghij1234567890abcdefghij12", false, ""},

		// Invalid: Length
		{"too_short", "ab", true, "at least 3 characters"},
		// 33 chars
		{"too_long", "abcdefghij1234567890abcdefghij123", true, "no more than 32 characters"},

		// Invalid: Must start with letter
		{"starts_with_number", "1user", true, "must start with a lowercase letter"},
		{"starts_with_underscore", "_user", true, "must start with a lowercase letter"},
		{"starts_with_hyphen", "-user", true, "must start with a lowercase letter"},

		// Invalid: Cannot end with underscore or hyphen
		{"ends_with_underscore", "user_", true, "cannot end with underscore or hyphen"},
		{"ends_with_hyphen", "user-", true, "cannot end with underscore or hyphen"},

		// Invalid: Uppercase letters
		// Converted to lowercase "user" which is blocked
		{"uppercase", "User", true, "reserved"},
		// Converted to "username" which has complex suffix
		{"mixed_case", "UsErNaMe", false, ""},

		// Invalid: Consecutive special characters
		{"consecutive_underscores", "user__name", true, "consecutive underscores"},
		{"consecutive_hyphens", "user--name", true, "consecutive hyphens"},
		{"consecutive_mixed_1", "user_-name", true, "consecutive underscore and hyphen"},
		{"consecutive_mixed_2", "user-_name", true, "consecutive underscore and hyphen"},

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
		{"empty", "", true, "at least 3 characters"},
		{"whitespace_only", "   ", true, "at least 3 characters"},
		// Trimmed to "user" which is blocked
		{"with_leading_space", "  user", true, "reserved"},
		// Trimmed to "user" which is blocked
		{"with_trailing_space", "user  ", true, "reserved"},
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

// TestUsernameRegex tests the username regex pattern directly
func TestUsernameRegex(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		// Should match
		{"min_length", "abc", true},
		// 32 chars
		{"max_length", "abcdefghij1234567890abcdefghij12", true},
		{"with_numbers", "user123", true},
		{"with_underscore", "user_name", true},
		{"with_hyphen", "user-name", true},
		{"complex", "user_123-test-456_name", true},

		// Should NOT match
		{"too_short", "ab", false},
		// 33 chars
		{"too_long", "abcdefghij1234567890abcdefghij123", false},
		{"starts_with_number", "1user", false},
		{"starts_with_underscore", "_user", false},
		{"starts_with_hyphen", "-user", false},
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

