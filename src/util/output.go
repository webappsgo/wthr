package utils

import (
	"os"

	"golang.org/x/term"
)

// ColorEnabled checks if color output should be used
// AI.md PART 8: NO_COLOR support
// Priority: 1. CLI flag -> 2. NO_COLOR env -> 3. Auto-detect
func ColorEnabled() bool {
	// 1. CLI flag overrides everything (--color=always|never|auto)
	colorMode := os.Getenv("CLI_COLOR_MODE")
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	case "auto":
		// Fall through to auto-detection
	}

	// 2. NO_COLOR env var (non-empty = disable)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 3. Auto-detect: TTY + TERM support
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}

// EmojiEnabled checks if emoji output should be used
// AI.md PART 8: Emojis disabled when NO_COLOR set or TERM=dumb
func EmojiEnabled() bool {
	// 1. NO_COLOR disables emojis (practical plain output)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 2. TERM=dumb disables emojis
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	// 3. Default: enabled
	return true
}

// Emoji returns the emoji or plain text fallback based on EmojiEnabled
// AI.md PART 11 line 12999: Emoji fallbacks
func Emoji(emoji, fallback string) string {
	if EmojiEnabled() {
		return emoji
	}
	return fallback
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
