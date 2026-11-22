package config

import "testing"

func TestGeoIPCountryPolicy(t *testing.T) {
	tests := []struct {
		name string
		cfg  GeoIPConfig
		code string
		want bool
	}{
		{name: "empty policy allows country", cfg: GeoIPConfig{}, code: "CN", want: false},
		{name: "deny list blocks match case insensitively", cfg: GeoIPConfig{DenyCountries: []string{"cn"}}, code: "CN", want: true},
		{name: "allow list takes precedence", cfg: GeoIPConfig{DenyCountries: []string{"CN"}, AllowCountries: []string{"US"}}, code: "CN", want: true},
		{name: "allow list permits match", cfg: GeoIPConfig{AllowCountries: []string{"us"}}, code: "US", want: false},
		{name: "unknown country fails open", cfg: GeoIPConfig{AllowCountries: []string{"US"}}, code: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.BlocksCountry(tt.code); got != tt.want {
				t.Errorf("BlocksCountry(%q) = %t, want %t", tt.code, got, tt.want)
			}
		})
	}
}

func TestIsIPAllowlisted(t *testing.T) {
	previous := GetGlobalConfig()
	cfg := &AppConfig{}
	cfg.Server.Security.Allowlist = []IPAllowlistEntry{
		{CIDR: "203.0.113.0/24"},
		{CIDR: "2001:db8::1"},
	}
	SetGlobalConfig(cfg)
	t.Cleanup(func() { SetGlobalConfig(previous) })

	if !IsIPAllowlisted("203.0.113.8") || !IsIPAllowlisted("2001:db8::1") {
		t.Fatal("configured CIDR and single-address allowlist entries should match")
	}
	if IsIPAllowlisted("198.51.100.8") || IsIPAllowlisted("invalid") {
		t.Fatal("unlisted or invalid addresses must not match")
	}
}
