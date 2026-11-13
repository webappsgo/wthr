package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestNormalizeClientIPVectors(t *testing.T) {
	cases := map[string]string{
		"192.0.2.1":         "192.0.2.1",
		"192.0.2.1:8080":    "192.0.2.1",
		"::ffff:192.0.2.1":  "192.0.2.1",
		"[2001:DB8::1]:443": "2001:db8::1",
		"2001:0db8:0000:0000:0000:0000:0000:0001": "2001:db8::1",
		"fe80::1%eth0": "fe80::1",
	}
	for in, want := range cases {
		got, ok := NormalizeClientIP(in)
		if !ok || got.String() != want {
			t.Errorf("NormalizeClientIP(%q) = %v,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := NormalizeClientIP("not-an-ip"); ok {
		t.Error("not-an-ip must be rejected")
	}
}

func TestTrustedGetClientIPChainVectors(t *testing.T) {
	cases := []struct {
		name       string
		xff        string
		additional []string
		want       string
	}{
		{"single", "203.0.113.9", nil, "203.0.113.9"},
		{"forged leftmost", "1.2.3.4, 203.0.113.9", nil, "203.0.113.9"},
		{"trusted public proxy", "203.0.113.9, 198.51.100.7", []string{"198.51.100.0/24"}, "203.0.113.9"},
		{"all trusted", "10.1.1.1, 10.0.0.2", nil, "10.1.1.1"},
		{"obfuscated hop", "203.0.113.9, unknown", nil, "10.0.0.1"},
		{"ipv6 chain", "2001:db8::5, 2001:db8::6", []string{"2001:db8::6/128"}, "2001:db8::5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTestConfig(t, &config.AppConfig{Server: config.ServerConfig{TrustedProxies: config.TrustedProxiesConfig{Additional: tc.additional}}})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "10.0.0.1:5000"
			r.Header.Set("X-Forwarded-For", tc.xff)
			if got := TrustedGetClientIP(r); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedGetClientIPForwardedAndPinned(t *testing.T) {
	withTestConfig(t, &config.AppConfig{Server: config.ServerConfig{TrustedProxies: config.TrustedProxiesConfig{ClientIPHeader: "CF-Connecting-IP"}}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if got := TrustedGetClientIP(r); got != "203.0.113.9" {
		t.Errorf("pinned header: got %q", got)
	}

	withTestConfig(t, &config.AppConfig{})
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::ffff:10.0.0.1]:5000"
	r.Header.Set("Forwarded", `for=192.0.2.60;proto=https, for="[2001:db8:cafe::17]:4711"`)
	if got := TrustedGetClientIP(r); got != "2001:db8:cafe::17" {
		t.Errorf("Forwarded header: got %q", got)
	}
}
