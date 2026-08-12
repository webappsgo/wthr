package display

import "os"

// EmojiEnabled reports whether emoji output should be used.
// Per AI.md PART 8 / PART 33: emojis are disabled when NO_COLOR is set
// (non-empty, value ignored) or when TERM=dumb. This is the shared, env-only
// gate used by every binary (server, CLI, agent) so emoji suppression behaves
// identically across the project. lipgloss already strips ANSI color for
// NO_COLOR on its own, but it does not strip emoji runes — this gate does.
func EmojiEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

// Emoji returns emoji when EmojiEnabled() is true, otherwise the plain-text
// fallback. Callers pass an ASCII fallback so output stays readable on dumb
// terminals and under NO_COLOR.
func Emoji(emoji, fallback string) string {
	if EmojiEnabled() {
		return emoji
	}
	return fallback
}
