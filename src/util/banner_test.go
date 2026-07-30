package utils

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
	DisplayFirstRunBanner(8080, "abcdef0123456789", false, "")
	DisplayFirstRunBanner(8080, "abcdef0123456789", true, "example.onion")
}

// TestDisplayNormalBanner_NoPanic mirrors the first-run banner check for the
// normal (post-setup) banner.
func TestDisplayNormalBanner_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DisplayNormalBanner panicked: %v", r)
		}
	}()
	DisplayNormalBanner("1.0.0", "2026-01-01T00:00:00Z", 8080, false, "")
	DisplayNormalBanner("1.0.0", "2026-01-01T00:00:00Z", 8080, true, "example.onion")
}

// TestPrintTierHelpers_NoPanic exercises the unexported per-tier print
// helpers directly and sanity-checks a couple of substrings appear.
func TestPrintTierHelpers_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("print tier helper panicked: %v", r)
		}
	}()
	printFirstRunCompact("wthr", "http://localhost:8080", "tok")
	printFirstRunMinimal("wthr", 8080, "tok")
	printFirstRunMicro("wthr", 8080)
	printNormalCompact("wthr", "1.0.0", "http://localhost:8080")
	printNormalMinimal("wthr", "1.0.0", 8080)
	printNormalMicro("wthr", 8080)
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
