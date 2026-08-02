package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(method, target string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c, w
}

// TestGetHostFromRequest covers reverse-proxy header priority, port
// stripping, and the GetFQDN() fallback.
func TestGetHostFromRequest(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"forwarded_host", map[string]string{"X-Forwarded-Host": "proxy.example.com"}, "proxy.example.com"},
		{"forwarded_host_with_port", map[string]string{"X-Forwarded-Host": "proxy.example.com:8443"}, "proxy.example.com"},
		{"real_host", map[string]string{"X-Real-Host": "real.example.com"}, "real.example.com"},
		{"original_host", map[string]string{"X-Original-Host": "orig.example.com"}, "orig.example.com"},
		{"priority_forwarded_over_real", map[string]string{
			"X-Forwarded-Host": "forwarded.example.com",
			"X-Real-Host":      "real.example.com",
		}, "forwarded.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestContext(http.MethodGet, "/", tt.headers)
			if got := GetHostFromRequest(c); got != tt.want {
				t.Errorf("GetHostFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("no_headers_falls_back_to_fqdn", func(t *testing.T) {
		c, _ := newTestContext(http.MethodGet, "/", nil)
		if got := GetHostFromRequest(c); got != GetFQDN() {
			t.Errorf("GetHostFromRequest() = %q, want GetFQDN() = %q", got, GetFQDN())
		}
	})
}

// TestGetHostInfo verifies protocol detection and derived fields.
func TestGetHostInfo(t *testing.T) {
	t.Run("http_default", func(t *testing.T) {
		c, _ := newTestContext(http.MethodGet, "/", map[string]string{"X-Forwarded-Host": "example.com"})
		info := GetHostInfo(c)
		if info.Protocol != "http" {
			t.Errorf("Protocol = %q, want http", info.Protocol)
		}
		if info.Hostname != "example.com" {
			t.Errorf("Hostname = %q, want example.com", info.Hostname)
		}
		if info.FullHost != "http://example.com" {
			t.Errorf("FullHost = %q, want http://example.com", info.FullHost)
		}
		if info.ExampleURL != "http://example.com/London" {
			t.Errorf("ExampleURL = %q, want http://example.com/London", info.ExampleURL)
		}
	})

	t.Run("forwarded_proto_https", func(t *testing.T) {
		c, _ := newTestContext(http.MethodGet, "/", map[string]string{
			"X-Forwarded-Proto": "https",
			"X-Forwarded-Host":  "example.com",
		})
		info := GetHostInfo(c)
		if info.Protocol != "https" {
			t.Errorf("Protocol = %q, want https", info.Protocol)
		}
	})
}

// TestIsLoopback covers localhost, loopback IPs, and non-loopback hosts.
func TestIsLoopback(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"localhost_string", "localhost", true},
		{"localhost_uppercase", "LOCALHOST", true},
		{"ipv4_loopback", "127.0.0.1", true},
		{"ipv6_loopback", "::1", true},
		{"public_hostname", "example.com", false},
		{"private_ip_not_loopback", "192.168.1.1", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLoopback(tt.host); got != tt.want {
				t.Errorf("isLoopback(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// TestGetClientIP covers header priority order and the Gin ClientIP fallback.
func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"cloudflare", map[string]string{"CF-Connecting-IP": "1.1.1.1"}, "1.1.1.1"},
		{"x_real_ip", map[string]string{"X-Real-IP": "2.2.2.2"}, "2.2.2.2"},
		{"x_forwarded_for_single", map[string]string{"X-Forwarded-For": "3.3.3.3"}, "3.3.3.3"},
		{"x_forwarded_for_multi_takes_first", map[string]string{"X-Forwarded-For": "4.4.4.4, 5.5.5.5"}, "4.4.4.4"},
		{"true_client_ip", map[string]string{"True-Client-IP": "6.6.6.6"}, "6.6.6.6"},
		{"cloudflare_beats_others", map[string]string{
			"CF-Connecting-IP": "1.1.1.1",
			"X-Real-IP":        "2.2.2.2",
		}, "1.1.1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestContext(http.MethodGet, "/", tt.headers)
			if got := GetClientIP(c); got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIsLocalhost covers standard localhost forms and RFC 1918 private
// ranges, plus rejection of public IPs.
func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"127001", "127.0.0.1", true},
		{"ipv6_loopback", "::1", true},
		{"localhost_string", "localhost", true},
		{"rfc1918_10", "10.0.0.5", true},
		{"rfc1918_172_16", "172.16.0.5", true},
		{"rfc1918_172_31", "172.31.255.255", true},
		{"not_rfc1918_172_15", "172.15.0.1", false},
		{"not_rfc1918_172_32", "172.32.0.1", false},
		{"rfc1918_192_168", "192.168.1.1", true},
		{"public_ip", "8.8.8.8", false},
		{"invalid_ip", "not-an-ip", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLocalhost(tt.ip); got != tt.want {
				t.Errorf("IsLocalhost(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestIsBrowser covers Accept-header content negotiation priority and the
// User-Agent fallback.
func TestIsBrowser(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{"accept_html", map[string]string{"Accept": "text/html,application/xhtml+xml"}, true},
		{"accept_text_plain", map[string]string{"Accept": "text/plain"}, false},
		{"accept_json", map[string]string{"Accept": "application/json"}, false},
		{"no_accept_chrome_ua", map[string]string{
			"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) Chrome/100.0",
		}, true},
		{"no_accept_firefox_ua", map[string]string{
			"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) Firefox/100.0",
		}, true},
		{"no_accept_curl_ua", map[string]string{"User-Agent": "curl/8.0.0"}, false},
		{"no_headers", map[string]string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestContext(http.MethodGet, "/", tt.headers)
			if got := IsBrowser(c); got != tt.want {
				t.Errorf("IsBrowser() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestContains is a lightweight substring wrapper check.
func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"found", "hello world", "world", true},
		{"not_found", "hello world", "xyz", false},
		{"empty_substr", "hello", "", true},
		{"substr_longer_than_s", "hi", "hello", false},
		{"exact_match", "hello", "hello", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// TestParseHexIP covers the little-endian hex-to-dotted-decimal conversion
// used for /proc/net/route parsing, plus malformed-input rejection.
func TestParseHexIP(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want string
	}{
		{"192_168_1_1", "0101A8C0", "192.168.1.1"},
		{"127_0_0_1", "0100007F", "127.0.0.1"},
		{"0_0_0_0", "00000000", "0.0.0.0"},
		{"too_short", "ABCD", ""},
		{"too_long", "0101A8C000", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseHexIP(tt.hex); got != tt.want {
				t.Errorf("parseHexIP(%q) = %q, want %q", tt.hex, got, tt.want)
			}
		})
	}
}

// TestIsDockerized_NoPanic exercises the environment-detection function
// without asserting a specific outcome, since it depends on the actual
// runtime environment (may or may not be inside a container).
func TestIsDockerized_NoPanic(t *testing.T) {
	_ = IsDockerized()
}

// TestGetDockerBridgeIPs_NoPanic verifies it returns a non-nil slice and
// never panics regardless of host networking configuration.
func TestGetDockerBridgeIPs_NoPanic(t *testing.T) {
	ips := GetDockerBridgeIPs()
	if ips == nil {
		t.Error("GetDockerBridgeIPs() returned nil, want non-nil (possibly empty) slice")
	}
}

// TestGetDockerGatewayIP_NoPanic exercises /proc/net/route parsing without
// asserting a specific gateway (environment-dependent).
func TestGetDockerGatewayIP_NoPanic(t *testing.T) {
	_ = GetDockerGatewayIP()
}

// TestIsTorAvailable_NoPanic exercises the exec.LookPath-based detection
// without asserting a specific outcome (environment-dependent).
func TestIsTorAvailable_NoPanic(t *testing.T) {
	_ = IsTorAvailable()
}

// TestGetFQDN_DomainEnvOverride verifies the highest-priority DOMAIN env
// var branch, including the comma-separated-list case.
func TestGetFQDN_DomainEnvOverride(t *testing.T) {
	t.Run("single_domain", func(t *testing.T) {
		t.Setenv("DOMAIN", "example.com")
		if got := GetFQDN(); got != "example.com" {
			t.Errorf("GetFQDN() = %q, want example.com", got)
		}
	})

	t.Run("comma_separated_takes_first", func(t *testing.T) {
		t.Setenv("DOMAIN", "example.com, second.example.com")
		if got := GetFQDN(); got != "example.com" {
			t.Errorf("GetFQDN() = %q, want example.com", got)
		}
	})
}
