package utils

import (
	"strings"
	"testing"
)

// TestValidateEmail covers RFC-ish validation rules: length limits,
// local/domain part rules, dot placement, and TLD checks.
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid_simple", "user@example.com", nil},
		{"valid_with_plus", "user+tag@example.com", nil},
		{"valid_with_dot", "first.last@example.com", nil},
		{"valid_with_dash", "user-name@example.com", nil},
		{"valid_subdomain", "user@mail.example.com", nil},
		{"valid_uppercase_normalized", "User@Example.COM", nil},
		{"valid_single_char_domain", "user@x.co", nil},

		{"empty", "", ErrEmailInvalidFormat},
		{"no_at", "userexample.com", ErrEmailInvalidFormat},
		{"multiple_at", "user@ex@ample.com", ErrEmailInvalidFormat},
		{"too_long", strings.Repeat("a", 250) + "@a.com", ErrEmailTooLong},
		{"empty_local", "@example.com", ErrEmailLocalPartLength},
		{"local_too_long", strings.Repeat("a", 65) + "@example.com", ErrEmailLocalPartLength},
		{"local_starts_dot", ".user@example.com", ErrEmailLocalPartDot},
		{"local_ends_dot", "user.@example.com", ErrEmailLocalPartDot},
		{"local_consecutive_dots", "us..er@example.com", ErrEmailConsecutiveDots},
		{"empty_domain", "user@", ErrEmailDomainLength},
		{"domain_no_tld", "user@localhost", ErrEmailDomainNoTLD},
		{"local_invalid_chars", "user name@example.com", ErrEmailLocalPartChars},
		{"local_invalid_special", "user!name@example.com", ErrEmailLocalPartChars},
		{"domain_invalid_format", "user@-example.com", ErrEmailDomainFormat},
		{"domain_trailing_hyphen", "user@example-.com", ErrEmailDomainFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateEmail(%q) = %v, want nil", tt.email, err)
				}
				return
			}
			if err != tt.wantErr {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

// TestValidateEmail_DomainTooLong exercises a domain long enough to exceed
// the 255-char domain limit in email.go. Because ValidateEmail rejects any
// email over 254 total characters before it ever inspects the domain
// separately (email.go checks len(email) > 254 first), a domain long enough
// to trip the domain-length branch on its own always also pushes the total
// email length past 254 first, so ErrEmailTooLong - not
// ErrEmailDomainLength - is the error actually reachable through the public
// API for this input shape.
func TestValidateEmail_DomainTooLong(t *testing.T) {
	longDomain := strings.Repeat("a", 252) + ".com"
	if len(longDomain) <= 255 {
		t.Fatalf("test fixture domain not long enough: %d", len(longDomain))
	}
	err := ValidateEmail("user@" + longDomain)
	if err != ErrEmailTooLong {
		t.Errorf("ValidateEmail with long domain = %v, want %v", err, ErrEmailTooLong)
	}
}

// TestValidateEmailWithBlocklist verifies disposable-domain rejection layered
// on top of standard format validation.
func TestValidateEmailWithBlocklist(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid_not_blocked", "user@example.com", nil},
		{"blocked_mailinator", "user@mailinator.com", ErrEmailDisposableDomain},
		{"blocked_yopmail", "user@yopmail.com", ErrEmailDisposableDomain},
		{"blocked_case_insensitive", "user@MAILINATOR.COM", ErrEmailDisposableDomain},
		{"invalid_format_first", "not-an-email", ErrEmailInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmailWithBlocklist(tt.email)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateEmailWithBlocklist(%q) = %v, want nil", tt.email, err)
				}
				return
			}
			if err != tt.wantErr {
				t.Errorf("ValidateEmailWithBlocklist(%q) = %v, want %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

// TestNormalizeEmail verifies trimming and lowercasing.
func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"already_normal", "user@example.com", "user@example.com"},
		{"uppercase", "User@Example.COM", "user@example.com"},
		{"whitespace", "  user@example.com  ", "user@example.com"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeEmail(tt.email)
			if got != tt.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

// TestMaskEmail covers masking of short/long local parts and domains, plus
// malformed input which must degrade to "***" rather than panic.
func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"long_local_and_domain", "johndoe@example.com", "j*****e@e******.com"},
		{"single_char_local", "j@example.com", "*@e******.com"},
		{"two_char_local", "jo@example.com", "**@e******.com"},
		{"single_char_domain_first_part", "user@x.com", "u**r@x.com"},
		{"no_at_sign", "notanemail", "***"},
		{"multiple_at_signs", "a@b@c", "***"},
		{"empty", "", "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskEmail(tt.email)
			if got != tt.want {
				t.Errorf("MaskEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}
