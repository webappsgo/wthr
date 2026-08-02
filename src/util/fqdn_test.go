package utils

import "testing"

// TestIsValidHost covers protocol/path/port stripping, IP rejection,
// localhost dev-mode gating, dev-only TLDs, project TLDs, .onion/.i2p, and
// production ICANN-TLD enforcement.
func TestIsValidHost(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		devMode     bool
		projectName string
		want        bool
	}{
		{"valid_com_prod", "example.com", false, "", true},
		{"valid_com_dev", "example.com", true, "", true},
		{"with_https_prefix", "https://example.com", false, "", true},
		{"with_path", "example.com/foo/bar", false, "", true},
		{"with_port", "example.com:8080", false, "", true},
		{"empty", "", false, "", false},
		{"ip_address_rejected", "192.168.1.1", false, "", false},
		{"localhost_dev", "localhost", true, "", true},
		{"localhost_prod", "localhost", false, "", false},
		{"no_dot", "examplecom", false, "", false},
		{"leading_dot", ".example.com", false, "", false},
		{"trailing_dot", "example.com.", false, "", false},
		{"leading_hyphen", "-example.com", false, "", false},
		{"trailing_hyphen", "example.com-", false, "", false},
		{"onion_valid_prod", "abc123.onion", false, "", true},
		{"i2p_valid_prod", "abc123.i2p", false, "", true},
		{"dev_tld_test_dev", "example.test", true, "", true},
		{"dev_tld_test_prod", "example.test", false, "", false},
		{"dev_tld_localdomain_dev", "host.localdomain", true, "", true},
		{"project_tld_dev", "app.weather", true, "weather", true},
		{"project_tld_prod", "app.weather", false, "weather", false},
		{"subdomain_valid", "www.example.com", false, "", true},
		{"co_uk_etld", "example.co.uk", false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidHost(tt.host, tt.devMode, tt.projectName); got != tt.want {
				t.Errorf("IsValidHost(%q, dev=%v, proj=%q) = %v, want %v",
					tt.host, tt.devMode, tt.projectName, got, tt.want)
			}
		})
	}
}

// TestValidateFQDN verifies the backward-compatibility wrapper always uses
// production mode with no project name.
func TestValidateFQDN(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   bool
	}{
		{"valid", "example.com", true},
		{"localhost_rejected_in_prod", "localhost", false},
		{"dev_tld_rejected_in_prod", "example.test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateFQDN(tt.domain); got != tt.want {
				t.Errorf("ValidateFQDN(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

// TestValidateURL covers scheme/host requirements and hostname extraction.
func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		devMode   bool
		wantValid bool
		wantHost  string
		wantErr   bool
	}{
		{"valid_https", "https://example.com/path", false, true, "example.com", false},
		{"valid_http_dev", "http://localhost:8080", true, true, "localhost", false},
		{"no_scheme", "example.com", false, false, "", false},
		{"no_host", "file:///path", false, false, "", false},
		{"invalid_hostname_prod", "http://localhost", false, false, "localhost", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, host, err := ValidateURL(tt.rawURL, tt.devMode, "")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateURL(%q): err = nil, want error", tt.rawURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateURL(%q): unexpected error: %v", tt.rawURL, err)
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateURL(%q) valid = %v, want %v", tt.rawURL, valid, tt.wantValid)
			}
			if host != tt.wantHost {
				t.Errorf("ValidateURL(%q) host = %q, want %q", tt.rawURL, host, tt.wantHost)
			}
		})
	}
}

// TestGetPublicSuffix covers a simple TLD and a multi-part eTLD.
func TestGetPublicSuffix(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{"simple_com", "example.com", "com"},
		{"co_uk", "example.co.uk", "co.uk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPublicSuffix(tt.domain); got != tt.want {
				t.Errorf("GetPublicSuffix(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

// TestGetEffectiveTLDPlusOne covers subdomain stripping and the error
// fallback to "".
func TestGetEffectiveTLDPlusOne(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{"www_subdomain", "www.example.com", "example.com"},
		{"bare_domain", "example.com", "example.com"},
		{"deep_subdomain", "a.b.example.com", "example.com"},
		{"just_tld_errors_to_empty", "com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetEffectiveTLDPlusOne(tt.domain); got != tt.want {
				t.Errorf("GetEffectiveTLDPlusOne(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}
