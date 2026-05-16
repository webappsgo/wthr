package utils

import (
	"testing"
)

// TestIsUsernameBlocked tests the username blocklist per AI.md PART 22
func TestIsUsernameBlocked(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		wantBlocked bool
	}{
		// Exact matches from blocklist
		{"admin", "admin@example.com", true},
		{"root", "root@example.com", true},
		{"system", "system@example.com", true},
		{"moderator", "moderator@example.com", true},
		{"official", "official@example.com", true},
		{"verified", "verified@example.com", true},

		// Critical terms as substrings (should be blocked per AI.md PART 22)
		{"contains_admin", "myadmin@example.com", true},
		{"contains_admin_middle", "myadminuser@example.com", true},
		{"contains_root", "rootuser@example.com", true},
		{"contains_root_end", "userroot@example.com", true},
		{"contains_system", "mysystem@example.com", true},
		{"contains_mod", "mymod@example.com", true},
		{"contains_official", "unofficialname@example.com", true},
		{"contains_verified", "verifieduser@example.com", true},

		// Variations with simple suffixes (should be blocked)
		{"test_with_numbers", "test123@example.com", true},
		{"user_with_underscore", "user_1@example.com", true},
		{"guest_with_hyphen", "guest-1@example.com", true},
		{"api_with_numbers", "api123@example.com", true},

		// Complex suffixes (should NOT be blocked)
		{"testing", "testing@example.com", false},
		{"username", "username@example.com", false},

		// Valid usernames (not on blocklist)
		{"valid_john", "john@example.com", false},
		{"valid_jane", "jane@example.com", false},
		{"valid_player", "player@example.com", false},
		{"valid_gamer", "gamer@example.com", false},

		// Edge cases
		{"empty_username", "@example.com", true},
		{"no_at_sign", "username", true},

		// Case insensitivity
		{"admin_uppercase", "ADMIN@example.com", true},
		{"admin_mixed", "AdMiN@example.com", true},

		// Project-specific
		{"weather", "weather@example.com", true},
		{"apimgr", "apimgr@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUsernameBlocked(tt.email)
			if got != tt.wantBlocked {
				t.Errorf("IsUsernameBlocked(%q) = %v, want %v", tt.email, got, tt.wantBlocked)
			}
		})
	}
}

// TestBlocklistSize verifies the blocklist has entries
func TestBlocklistSize(t *testing.T) {
	size := GetBlocklistSize()
	if size == 0 {
		t.Error("UsernameBlocklist should not be empty")
	}
	if size < 100 {
		t.Errorf("UsernameBlocklist size = %d, expected at least 100 entries per AI.md PART 22", size)
	}
	t.Logf("Blocklist contains %d entries", size)
}
