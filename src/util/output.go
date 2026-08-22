package util

import (
	"github.com/webappsgo/wthr/src/common/display"
)

// ColorEnabled reports whether ANSI color output should be used.
// AI.md PART 8 mandates a single color gate shared by every binary, so this
// delegates to display.ColorEnabled rather than re-reading the environment.
// Precedence: CLI flag -> config -> NO_COLOR -> auto-detect (TTY + TERM).
func ColorEnabled() bool {
	return display.ColorEnabled()
}

// EmojiEnabled reports whether console emoji output should be used.
// AI.md PART 8 mandates a single emoji gate shared by every binary, so this
// delegates to display.EmojiEnabled rather than re-reading the environment.
// Only CONSOLE output is gated — weather icons, moon-phase glyphs, ASCII-art
// table content, template content and API payload values are data, never gated.
func EmojiEnabled() bool {
	return display.EmojiEnabled()
}

// Emoji returns the emoji or its plain-text fallback, delegating to the
// canonical gate in src/common/display.
func Emoji(emoji, fallback string) string {
	return display.Emoji(emoji, fallback)
}

// Emoji constants with fallbacks
// AI.md PART 11: Emoji fallbacks (when NO_COLOR set or TERM=dumb)
const (
	EmojiOK      = "✅"
	EmojiError   = "❌"
	EmojiWarning = "⚠️"
	EmojiInfo    = "ℹ️"
	EmojiRocket  = "🚀"
	EmojiSun     = "🌤️"
	EmojiGlobe   = "🌐"
	EmojiDocker  = "🐳"
	EmojiOnion   = "🧅"
	EmojiLock    = "🔐"
)

// Plain text fallbacks
const (
	PlainOK      = "[OK]"
	PlainError   = "[ERROR]"
	PlainWarning = "[WARN]"
	PlainInfo    = "[INFO]"
	PlainRocket  = "[START]"
	PlainSun     = "[SUN]"
	PlainGlobe   = "[WEB]"
	PlainDocker  = "[DOCKER]"
	PlainOnion   = "[TOR]"
	PlainLock    = "[TOKEN]"
)

// GetOK returns appropriate OK indicator
func GetOK() string {
	return Emoji(EmojiOK, PlainOK)
}

// GetError returns appropriate error indicator
func GetError() string {
	return Emoji(EmojiError, PlainError)
}

// GetWarning returns appropriate warning indicator
func GetWarning() string {
	return Emoji(EmojiWarning, PlainWarning)
}

// GetInfo returns appropriate info indicator
func GetInfo() string {
	return Emoji(EmojiInfo, PlainInfo)
}

// GetRocket returns appropriate startup indicator
func GetRocket() string {
	return Emoji(EmojiRocket, PlainRocket)
}

// GetSun returns appropriate weather/app indicator
func GetSun() string {
	return Emoji(EmojiSun, PlainSun)
}

// GetGlobe returns appropriate web/URL indicator
func GetGlobe() string {
	return Emoji(EmojiGlobe, PlainGlobe)
}

// GetDocker returns appropriate Docker indicator
func GetDocker() string {
	return Emoji(EmojiDocker, PlainDocker)
}

// GetOnion returns appropriate Tor indicator
func GetOnion() string {
	return Emoji(EmojiOnion, PlainOnion)
}

// GetLock returns appropriate token/security indicator
func GetLock() string {
	return Emoji(EmojiLock, PlainLock)
}
