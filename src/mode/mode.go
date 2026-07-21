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
)

// String returns the string representation of the mode
func (m AppMode) String() string {
	switch m {
	case Development:
		return "development"
	default:
		return "production"
	}
}

// SetAppMode sets the application mode
func SetAppMode(m string) {
	switch strings.ToLower(m) {
	case "dev", "development":
		currentMode = Development
	default:
		currentMode = Production
	}
	updateProfilingSettings()
}

// SetDebugEnabled enables or disables debug mode
func SetDebugEnabled(enabled bool) {
	debugEnabled = enabled
	updateProfilingSettings()
}

// updateProfilingSettings enables/disables profiling based on debug flag
func updateProfilingSettings() {
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

// Current returns the current mode
func Current() AppMode {
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

// ModeString returns mode string with debug suffix if enabled
func ModeString() string {
	s := currentMode.String()
	if debugEnabled {
		s += " [debugging]"
	}
	return s
}

// FromEnv sets mode and debug from environment variables
// AI.md PART 5: Environment Variables
func FromEnv() {
	if m := os.Getenv("MODE"); m != "" {
		SetAppMode(m)
	}
	if config.IsTruthy(os.Getenv("DEBUG")) {
		SetDebugEnabled(true)
	}
}
