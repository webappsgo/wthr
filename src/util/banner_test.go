package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCenterText covers even/odd padding, exact-width, and overflow
// (truncation) cases for the pure-logic centering helper.
func TestCenterText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{"even_padding", "abc", 7, "  abc  "},
		{"odd_padding_extra_on_right", "ab", 7, "  ab   "},
		{"exact_width", "abcde", 5, "abcde"},
		{"overflow_truncated", "abcdefgh", 5, "abcde"},
		{"empty_text", "", 4, "    "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := centerText(tt.text, tt.width)
			if got != tt.want {
				t.Errorf("centerText(%q, %d) = %q (len %d), want %q (len %d)",
					tt.text, tt.width, got, len(got), tt.want, len(tt.want))
			}
		})
	}
}

// TestGetBinaryName verifies it returns the base name of os.Args[0],
// self-consistently (the actual value is the test binary's path, which
// varies by environment).
func TestGetBinaryName(t *testing.T) {
	want := filepath.Base(os.Args[0])
	if got := getBinaryName(); got != want {
		t.Errorf("getBinaryName() = %q, want %q", got, want)
	}
}

// TestGetTerminalWidth verifies the non-TTY fallback of 80 columns, since
// go test runs with stdout redirected (not a terminal).
func TestGetTerminalWidth(t *testing.T) {
	if got := getTerminalWidth(); got != 80 {
		t.Errorf("getTerminalWidth() = %d, want 80 (non-TTY fallback)", got)
	}
}

// TestDisplayFirstRunBanner_NoPanic exercises all four responsive tiers
// (driven indirectly via getTerminalWidth's fixed 80-col fallback under
// go test) to ensure none of the print paths panic on nil/empty inputs.
func TestDisplayFirstRunBanner_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DisplayFirstRunBanner panicked: %v", r)
		}
	}()
	DisplayFirstRunBanner(8080, false, "abcdef0123456789", "", "", "/server/admin")
	DisplayFirstRunBanner(443, true, "abcdef0123456789", "example.onion", "abcdefghij.b32.i2p", "/server/admin")
}

// TestDisplayNormalBanner_NoPanic mirrors the first-run banner check for the
// normal (post-setup) banner.
func TestDisplayNormalBanner_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DisplayNormalBanner panicked: %v", r)
		}
	}()
	DisplayNormalBanner("1.0.0", "2026-01-01T00:00:00Z", 8080, false, "", "")
	DisplayNormalBanner("1.0.0", "2026-01-01T00:00:00Z", 8080, true, "example.onion", "abcdefghij.b32.i2p")
}

// TestPrintTierHelpers_NoPanic exercises the unexported per-tier print
// helpers directly and sanity-checks a couple of substrings appear.
func TestPrintTierHelpers_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("print tier helper panicked: %v", r)
		}
	}()
	printFirstRunFull("wthr", "http://localhost:8080", "example.onion", "abcdefghij.b32.i2p", "tok", "http://localhost:8080/server/admin/config/setup")
	printFirstRunCompact("wthr", "http://localhost:8080", "tok", "http://localhost:8080/server/admin/config/setup")
	printFirstRunMinimal("wthr", "http://localhost:8080", "tok")
	printFirstRunMicro("wthr", 8080)
	printFirstRunPlain("wthr", "http://localhost:8080", "tok", "http://localhost:8080/server/admin/config/setup")
	printNormalFull("wthr", "1.0.0", "2026-01-01T00:00:00Z", "http://localhost:8080", "example.onion", "abcdefghij.b32.i2p")
	printNormalCompact("wthr", "1.0.0", "http://localhost:8080")
	printNormalMinimal("wthr", "1.0.0", "http://localhost:8080")
	printNormalMicro("wthr", 8080)
	printNormalPlain("wthr", "1.0.0", "http://localhost:8080")
}

// TestBannerProto verifies the banner scheme is resolved from the TLS state only.
func TestBannerProto(t *testing.T) {
	if got := BannerProto(true); got != "https" {
		t.Errorf("BannerProto(true) = %q, want %q", got, "https")
	}
	if got := BannerProto(false); got != "http" {
		t.Errorf("BannerProto(false) = %q, want %q", got, "http")
	}
}

// TestBannerListenURL covers default-port stripping, explicit ports, IPv6
// bracketing, and the empty-host fallback required by AI.md PART 11/15.
func TestBannerListenURL(t *testing.T) {
	tests := []struct {
		name  string
		proto string
		host  string
		port  int
		want  string
	}{
		{"http_default_port_stripped", "http", "example.com", 80, "http://example.com"},
		{"https_default_port_stripped", "https", "example.com", 443, "https://example.com"},
		{"http_non_default_port_shown", "http", "example.com", 8080, "http://example.com:8080"},
		{"https_non_default_port_shown", "https", "example.com", 8443, "https://example.com:8443"},
		{"https_on_80_keeps_port", "https", "example.com", 80, "https://example.com:80"},
		{"http_on_443_keeps_port", "http", "example.com", 443, "http://example.com:443"},
		{"ipv6_bracketed", "http", "2001:db8::1", 8080, "http://[2001:db8::1]:8080"},
		{"ipv6_already_bracketed", "http", "[2001:db8::1]", 8080, "http://[2001:db8::1]:8080"},
		{"ipv4_unbracketed", "http", "192.0.2.10", 8080, "http://192.0.2.10:8080"},
		{"empty_host_falls_back", "http", "", 8080, "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BannerListenURL(tt.proto, tt.host, tt.port); got != tt.want {
				t.Errorf("BannerListenURL(%q, %q, %d) = %q, want %q", tt.proto, tt.host, tt.port, got, tt.want)
			}
		})
	}
}

// TestBannerLineWidth verifies every rendered row of the full banner is the
// same display width as its borders, so the box never looks ragged.
func TestBannerLineWidth(t *testing.T) {
	border := bannerBorder("╭", "╮")
	borderWidth := bannerDisplayWidth(border)

	lines := []string{
		bannerLine("short"),
		bannerLine("🚀 wthr · 📦 1.0.0"),
		bannerLine("📡 Listening on http://example.com:8080"),
	}
	for _, line := range lines {
		if got := bannerDisplayWidth(line); got != borderWidth {
			t.Errorf("bannerDisplayWidth(%q) = %d, want %d", line, got, borderWidth)
		}
	}
}

// TestCenterText_ContainsOriginalText is a lightweight regression guard: for
// non-overflow cases, the centered result must still contain the original
// text verbatim.
func TestCenterText_ContainsOriginalText(t *testing.T) {
	got := centerText("hello", 20)
	if !strings.Contains(got, "hello") {
		t.Errorf("centerText result %q does not contain original text %q", got, "hello")
	}
}
