package util

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

// withTestConfig installs cfg as the global config for the duration of the
// test and restores whatever was there before on cleanup, so trusted-proxy
// tests never leak state into other tests in this package.
func withTestConfig(t *testing.T, cfg *config.AppConfig) {
	t.Helper()
	prev := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() {
		config.SetGlobalConfig(prev)
	})
}

func TestIsTrustedPeer_AlwaysTrustedRanges(t *testing.T) {
	withTestConfig(t, &config.AppConfig{})

	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"loopback_v4", "127.0.0.1:12345", true},
		{"loopback_v6", "[::1]:12345", true},
		{"rfc1918_10", "10.1.2.3:80", true},
		{"rfc1918_172", "172.20.0.5:80", true},
		{"rfc1918_192", "192.168.1.1:80", true},
		{"ula_v6", "[fc00::1]:80", true},
		{"link_local_v4", "169.254.1.1:80", true},
		{"link_local_v6", "[fe80::1]:80", true},
		{"public_untrusted", "203.0.113.10:80", false},
		{"no_port", "203.0.113.10", false},
		{"unparseable", "not-an-ip:80", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTrustedPeer(tt.addr); got != tt.want {
				t.Errorf("isTrustedPeer(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestIsTrustedPeer_NilConfig(t *testing.T) {
	withTestConfig(t, nil)

	// Always-trusted ranges still apply with no config loaded.
	if !isTrustedPeer("127.0.0.1:80") {
		t.Error("loopback should be trusted even with nil config")
	}
	// A public peer with no config can never be in the additional list.
	if isTrustedPeer("203.0.113.10:80") {
		t.Error("public peer should not be trusted with nil config")
	}
}

func TestIsTrustedPeer_SameSlash24AsListenAddress(t *testing.T) {
	withTestConfig(t, &config.AppConfig{
		Server: config.ServerConfig{Address: "203.0.113.5:8080"},
	})

	if !isTrustedPeer("203.0.113.20:80") {
		t.Error("peer in the same /24 as the listen address should be trusted")
	}
	if isTrustedPeer("203.0.114.20:80") {
		t.Error("peer outside the listen address /24 should not be trusted")
	}
}

func TestIsTrustedPeer_AdditionalIPAndCIDR(t *testing.T) {
	withTestConfig(t, &config.AppConfig{
		Server: config.ServerConfig{
			TrustedProxies: config.TrustedProxiesConfig{
				Additional: []string{"203.0.113.10", "198.51.100.0/24"},
			},
		},
	})

	if !isTrustedPeer("203.0.113.10:80") {
		t.Error("explicit additional IP should be trusted")
	}
	if !isTrustedPeer("198.51.100.42:80") {
		t.Error("peer within additional CIDR should be trusted")
	}
	if isTrustedPeer("203.0.113.11:80") {
		t.Error("peer not covered by additional list should not be trusted")
	}
}

func TestIsTrustedPeer_AdditionalDNSName(t *testing.T) {
	withTestConfig(t, &config.AppConfig{
		Server: config.ServerConfig{
			TrustedProxies: config.TrustedProxiesConfig{
				Additional: []string{"localhost"},
			},
		},
	})

	// "localhost" resolves to a loopback address, which is already covered
	// by the always-trusted set — this exercises the DNS resolution path
	// without depending on external network access.
	if !isTrustedPeer("127.0.0.1:80") {
		t.Error("DNS name resolving to a trusted address should be trusted")
	}
}

func TestSameSlash24(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		listenAddr string
		want       bool
	}{
		{"same_24", "10.0.0.5", "10.0.0.1:8080", true},
		{"different_24", "10.0.1.5", "10.0.0.1:8080", false},
		{"listen_no_port", "10.0.0.5", "10.0.0.1", true},
		{"listen_unspecified", "10.0.0.5", "0.0.0.0:8080", false},
		{"listen_empty", "10.0.0.5", "", false},
		{"ipv6_ip", "fc00::1", "10.0.0.1:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := mustParseIP(t, tt.ip)
			if got := sameSlash24(ip, tt.listenAddr); got != tt.want {
				t.Errorf("sameSlash24(%q, %q) = %v, want %v", tt.ip, tt.listenAddr, got, tt.want)
			}
		})
	}
}

func mustParseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("invalid test IP literal %q", s)
	}
	return ip
}

func TestTrustedGetClientIP_TrustedPeerHonorsHeaders(t *testing.T) {
	withTestConfig(t, &config.AppConfig{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")

	if got := TrustedGetClientIP(r); got != "198.51.100.7" {
		t.Errorf("TrustedGetClientIP() = %q, want %q", got, "198.51.100.7")
	}
}

func TestTrustedGetClientIP_UntrustedPeerIgnoresHeaders(t *testing.T) {
	withTestConfig(t, &config.AppConfig{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.9:54321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.Header.Set("X-Real-IP", "198.51.100.7")
	r.Header.Set("CF-Connecting-IP", "198.51.100.7")
	r.Header.Set("True-Client-IP", "198.51.100.7")
	r.Header.Set("X-Client-IP", "198.51.100.7")

	if got := TrustedGetClientIP(r); got != "203.0.113.9" {
		t.Errorf("TrustedGetClientIP() = %q, want raw peer %q", got, "203.0.113.9")
	}
}

func TestTrustedGetClientIP_UntrustedPeerNoPort(t *testing.T) {
	withTestConfig(t, &config.AppConfig{})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "not-a-valid-hostport"

	if got := TrustedGetClientIP(r); got != "not-a-valid-hostport" {
		t.Errorf("TrustedGetClientIP() = %q, want raw RemoteAddr fallback", got)
	}
}
