package util

import (
	"testing"

	"github.com/webappsgo/wthr/src/common/display"
)

// emojiGateCases enumerates the NO_COLOR / TERM=dumb gate behavior mandated by
// AI.md PART 8. EmojiEnabled and Emoji both delegate to src/common/display, so
// these cases double as a parity check against the canonical gate.
var emojiGateCases = []struct {
	name    string
	noColor string
	term    string
	want    bool
}{
	{"default_enabled", "", "xterm", true},
	{"no_color_1_disables", "1", "xterm", false},
	{"no_color_0_disables", "0", "xterm", false},
	{"no_color_any_value_disables", "anything", "xterm", false},
	{"term_dumb_disables", "", "dumb", false},
	{"no_color_and_term_dumb_disables", "1", "dumb", false},
	{"empty_term_still_enabled", "", "", true},
}

// TestEmojiEnabled covers every branch of the emoji gate.
func TestEmojiEnabled(t *testing.T) {
	for _, tt := range emojiGateCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got := EmojiEnabled(); got != tt.want {
				t.Errorf("EmojiEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEmojiEnabled_DelegatesToDisplay verifies util does not maintain a second
// gate: its answer must match display.EmojiEnabled for every case.
func TestEmojiEnabled_DelegatesToDisplay(t *testing.T) {
	for _, tt := range emojiGateCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got, want := EmojiEnabled(), display.EmojiEnabled(); got != want {
				t.Errorf("EmojiEnabled() = %v, display.EmojiEnabled() = %v; gates must agree", got, want)
			}
		})
	}
}

// TestEmoji verifies the emoji/fallback selection follows the same gate.
func TestEmoji(t *testing.T) {
	for _, tt := range emojiGateCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)

			want := "[X]"
			if tt.want {
				want = "X"
			}
			if got := Emoji("X", "[X]"); got != want {
				t.Errorf("Emoji(\"X\", \"[X]\") = %q, want %q", got, want)
			}
			if got, canonical := Emoji("X", "[X]"), display.Emoji("X", "[X]"); got != canonical {
				t.Errorf("Emoji() = %q, display.Emoji() = %q; must delegate", got, canonical)
			}
		})
	}
}

// colorGateCases enumerates the AI.md PART 8 color precedence chain:
// CLI flag (CLI_COLOR_MODE) -> config -> NO_COLOR -> auto-detect (TTY + TERM).
// Under `go test` stdout is never a TTY, so every case that reaches the
// auto-detect layer resolves to false regardless of TERM.
var colorGateCases = []struct {
	name      string
	colorMode string
	noColor   string
	term      string
	want      bool
}{
	{"flag_yes_beats_no_color_and_dumb_term", "yes", "1", "dumb", true},
	{"flag_yes_beats_non_tty", "yes", "", "xterm", true},
	{"flag_no_forces_off", "no", "", "xterm", false},
	{"flag_no_beats_everything", "no", "", "dumb", false},
	{"flag_auto_falls_through_to_autodetect", "auto", "", "xterm", false},
	{"unset_flag_falls_through_to_autodetect", "", "", "xterm", false},
	{"unknown_flag_value_treated_as_auto", "maybe", "", "xterm", false},
	{"no_color_disables", "", "1", "xterm", false},
	{"no_color_any_value_disables", "", "anything", "xterm", false},
	{"no_color_empty_falls_through", "", "", "", false},
	{"term_dumb_disables", "", "", "dumb", false},
}

// TestColorEnabled covers the full precedence chain of the color gate.
func TestColorEnabled(t *testing.T) {
	for _, tt := range colorGateCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLI_COLOR_MODE", tt.colorMode)
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got := ColorEnabled(); got != tt.want {
				t.Errorf("ColorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestColorEnabled_DelegatesToDisplay verifies util does not maintain a second
// color gate: its answer must match display.ColorEnabled for every case.
func TestColorEnabled_DelegatesToDisplay(t *testing.T) {
	for _, tt := range colorGateCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLI_COLOR_MODE", tt.colorMode)
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got, want := ColorEnabled(), display.ColorEnabled(); got != want {
				t.Errorf("ColorEnabled() = %v, display.ColorEnabled() = %v; gates must agree", got, want)
			}
		})
	}
}

// TestColorModeEnvVarName pins the env-var name the CLI flag parser writes,
// so the flag plumbing and the gate cannot drift apart.
func TestColorModeEnvVarName(t *testing.T) {
	if display.ColorModeEnvVar != "CLI_COLOR_MODE" {
		t.Errorf("display.ColorModeEnvVar = %q, want %q", display.ColorModeEnvVar, "CLI_COLOR_MODE")
	}
}

// TestGetIndicators verifies each Get* helper returns either the emoji or
// its documented plain-text fallback depending on EmojiEnabled.
func TestGetIndicators(t *testing.T) {
	tests := []struct {
		name  string
		fn    func() string
		emoji string
		plain string
	}{
		{"GetOK", GetOK, EmojiOK, PlainOK},
		{"GetError", GetError, EmojiError, PlainError},
		{"GetWarning", GetWarning, EmojiWarning, PlainWarning},
		{"GetInfo", GetInfo, EmojiInfo, PlainInfo},
		{"GetRocket", GetRocket, EmojiRocket, PlainRocket},
		{"GetSun", GetSun, EmojiSun, PlainSun},
		{"GetGlobe", GetGlobe, EmojiGlobe, PlainGlobe},
		{"GetDocker", GetDocker, EmojiDocker, PlainDocker},
		{"GetOnion", GetOnion, EmojiOnion, PlainOnion},
		{"GetLock", GetLock, EmojiLock, PlainLock},
	}

	t.Run("emoji_enabled", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "xterm")
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.fn(); got != tt.emoji {
					t.Errorf("%s() = %q, want %q", tt.name, got, tt.emoji)
				}
			})
		}
	})

	t.Run("emoji_disabled", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.fn(); got != tt.plain {
					t.Errorf("%s() = %q, want %q", tt.name, got, tt.plain)
				}
			})
		}
	})
}
