package utils

import "testing"

// TestEmoji covers the enabled/disabled fallback selection directly (does
// not depend on TTY detection).
func TestEmoji(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	got := Emoji("X", "[X]")
	if got != "X" {
		t.Errorf("Emoji enabled = %q, want %q", got, "X")
	}
}

// TestEmoji_Disabled_NoColor verifies NO_COLOR forces the plain fallback.
func TestEmoji_Disabled_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := Emoji("X", "[X]")
	if got != "[X]" {
		t.Errorf("Emoji with NO_COLOR set = %q, want %q", got, "[X]")
	}
}

// TestEmoji_Disabled_TermDumb verifies TERM=dumb forces the plain fallback.
func TestEmoji_Disabled_TermDumb(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	got := Emoji("X", "[X]")
	if got != "[X]" {
		t.Errorf("Emoji with TERM=dumb = %q, want %q", got, "[X]")
	}
}

// TestEmojiEnabled covers the three branches of EmojiEnabled directly.
func TestEmojiEnabled(t *testing.T) {
	tests := []struct {
		name    string
		noColor string
		term    string
		want    bool
	}{
		{"default_enabled", "", "xterm", true},
		{"no_color_disables", "1", "xterm", false},
		{"term_dumb_disables", "", "dumb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got := EmojiEnabled(); got != tt.want {
				t.Errorf("EmojiEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestColorEnabled_CLIColorModeOverrides verifies the CLI_COLOR_MODE flag
// takes priority over everything else, in both directions.
func TestColorEnabled_CLIColorModeOverrides(t *testing.T) {
	t.Run("yes_forces_true", func(t *testing.T) {
		t.Setenv("CLI_COLOR_MODE", "yes")
		t.Setenv("NO_COLOR", "1")
		t.Setenv("TERM", "dumb")
		if !ColorEnabled() {
			t.Error("ColorEnabled() = false, want true (CLI_COLOR_MODE=yes overrides NO_COLOR/TERM)")
		}
	})

	t.Run("no_forces_false", func(t *testing.T) {
		t.Setenv("CLI_COLOR_MODE", "no")
		t.Setenv("NO_COLOR", "")
		if ColorEnabled() {
			t.Error("ColorEnabled() = true, want false (CLI_COLOR_MODE=no)")
		}
	})
}

// TestColorEnabled_NoColorEnv verifies NO_COLOR disables color when
// CLI_COLOR_MODE is not "yes".
func TestColorEnabled_NoColorEnv(t *testing.T) {
	t.Setenv("CLI_COLOR_MODE", "")
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled() {
		t.Error("ColorEnabled() = true, want false with NO_COLOR set")
	}
}

// TestGetIndicators verifies each Get* helper returns either the emoji or
// its documented plain-text fallback depending on EmojiEnabled.
func TestGetIndicators(t *testing.T) {
	tests := []struct {
		name    string
		fn      func() string
		emoji   string
		plain   string
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
