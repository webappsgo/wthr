// Package mode resolves and exposes the application execution mode and the
// independent debug flag per AI.md PART 6.
package mode

import (
	"os"
	"runtime"
	"strings"

	"github.com/webappsgo/wthr/src/config"
)

var (
	currentMode  = Production
	debugEnabled = false
)

// AppMode represents the application execution mode
type AppMode int

const (
	// Production mode (default) - strict security, minimal logging
	Production AppMode = iota
	// Development mode - relaxed security, verbose logging
	Development
	// Debug mode - development behavior with the debug flag defaulted on
	Debug
)

// String returns the string representation of the mode
func (m AppMode) String() string {
	switch m {
	case Development:
		return "development"
	case Debug:
		return "debug"
	default:
		return "production"
	}
}

// SetAppMode sets the application mode
func SetAppMode(m string) {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "dev", "devel", "development":
		currentMode = Development
	case "debug":
		// Debug mode defaults the debug flag on; an explicit --debug flag or
		// DEBUG env var evaluated afterwards still wins
		currentMode = Debug
		SetDebugEnabled(true)
	default:
		currentMode = Production
	}
	updateAppModeProfilingSettings()
}

// SetDebugEnabled enables or disables debug mode
func SetDebugEnabled(enabled bool) {
	debugEnabled = enabled
	updateAppModeProfilingSettings()
}

// updateAppModeProfilingSettings enables/disables profiling based on debug flag
func updateAppModeProfilingSettings() {
	if debugEnabled {
		// Enable profiling when debug is on
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
	} else {
		// Disable profiling when debug is off
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(0)
	}
}

// GetCurrentAppMode returns the current application mode
func GetCurrentAppMode() AppMode {
	return currentMode
}

// IsAppModeDev returns true if in development mode
func IsAppModeDev() bool {
	return currentMode == Development
}

// IsAppModeProd returns true if in production mode
func IsAppModeProd() bool {
	return currentMode == Production
}

// IsDebugEnabled returns true if debug mode is enabled (--debug or DEBUG=true)
func IsDebugEnabled() bool {
	return debugEnabled
}

// GetAppModeString returns mode string with debug suffix if enabled
func GetAppModeString() string {
	s := currentMode.String()
	if debugEnabled {
		s += " [debugging]"
	}
	return s
}

// ModeString returns mode string with debug suffix if enabled.
// Callers should use GetAppModeString, which is the name AI.md PART 6 defines.
func ModeString() string {
	return GetAppModeString()
}

// FromEnv sets mode and debug from environment variables per AI.md PART 6.
// An explicitly set DEBUG env var always wins over the MODE=debug default, so
// MODE=debug DEBUG=false runs debug mode with the /debug/* endpoints off.
func FromEnv() {
	if m := os.Getenv("MODE"); m != "" {
		SetAppMode(m)
	}
	if d, ok := os.LookupEnv("DEBUG"); ok && d != "" {
		SetDebugEnabled(config.IsTruthy(d))
	}
}
