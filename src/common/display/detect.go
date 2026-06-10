// Package display provides shared display environment detection per AI.md PART 7.
// All binaries (server, CLI, agent) use this package to detect the display mode.
package display

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// DisplayMode is the UI display mode (NOT the application runtime mode).
// Per AI.md PART 7: Headless < CLI < TUI < GUI.
type DisplayMode int

const (
	DisplayModeHeadless DisplayMode = iota // No display, no TTY
	DisplayModeCLI                         // Command-line only (piped or command provided)
	DisplayModeTUI                         // Terminal UI (interactive terminal)
	DisplayModeGUI                         // Native graphical UI
)

// DisplayEnv holds the detected display environment for the current process.
type DisplayEnv struct {
	Mode         DisplayMode
	HasDisplay   bool   // X11, Wayland, Windows, or macOS display present
	DisplayType  string // "x11", "wayland", "windows", "macos", "none"
	IsTerminal   bool   // stdout is a TTY
	IsSSH        bool   // running over SSH
	IsMosh       bool   // running over mosh
	IsScreen     bool   // running inside screen or tmux
	TerminalType string // value of $TERM
	Cols         int    // terminal columns (0 when not a terminal)
	Rows         int    // terminal rows (0 when not a terminal)
}

// DetectDisplayEnv probes the current process environment and returns a
// populated DisplayEnv. Callers should call this once at startup and cache
// the result.
func DetectDisplayEnv() DisplayEnv {
	env := DisplayEnv{}

	env.IsTerminal = term.IsTerminal(int(os.Stdout.Fd()))
	if env.IsTerminal {
		env.Cols, env.Rows, _ = term.GetSize(int(os.Stdout.Fd()))
	}
	env.TerminalType = os.Getenv("TERM")

	env.IsSSH = os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
	env.IsMosh = os.Getenv("MOSH") != "" || strings.Contains(os.Getenv("TERM"), "mosh")
	env.IsScreen = os.Getenv("STY") != "" || os.Getenv("TMUX") != ""

	env.detectPlatformDisplay()
	env.Mode = env.autoDetectDisplayMode()

	return env
}

// autoDetectDisplayMode maps the raw environment flags to a DisplayMode.
func (e *DisplayEnv) autoDetectDisplayMode() DisplayMode {
	if !e.IsTerminal && !e.HasDisplay {
		return DisplayModeHeadless
	}
	// TERM=dumb: force CLI (no ANSI, no TUI).
	if e.TerminalType == "dumb" {
		return DisplayModeCLI
	}
	if e.HasDisplay && !e.IsSSH && !e.IsMosh {
		return DisplayModeGUI
	}
	if e.IsTerminal {
		return DisplayModeTUI
	}
	return DisplayModeCLI
}

// IsDumbTerminal returns true when TERM=dumb (no ANSI escapes supported).
func (e *DisplayEnv) IsDumbTerminal() bool { return e.TerminalType == "dumb" }

// IsAutoDetectDisplayModeGUI reports whether the resolved mode is GUI.
func (e DisplayEnv) IsAutoDetectDisplayModeGUI() bool { return e.Mode == DisplayModeGUI }

// IsAutoDetectDisplayModeTUI reports whether the resolved mode is TUI.
func (e DisplayEnv) IsAutoDetectDisplayModeTUI() bool { return e.Mode == DisplayModeTUI }

// IsAutoDetectDisplayModeCLI reports whether the resolved mode is CLI.
func (e DisplayEnv) IsAutoDetectDisplayModeCLI() bool { return e.Mode == DisplayModeCLI }

// IsAutoDetectDisplayModeHeadless reports whether the resolved mode is Headless.
func (e DisplayEnv) IsAutoDetectDisplayModeHeadless() bool { return e.Mode == DisplayModeHeadless }

// SupportsUnicode returns true when the terminal is expected to render Unicode correctly.
// We assume Unicode support unless the terminal explicitly signals otherwise.
func (e DisplayEnv) SupportsUnicode() bool {
	if e.TerminalType == "dumb" || e.TerminalType == "" {
		return false
	}
	// Terminals advertising xterm, vt100, and most modern types support Unicode.
	return true
}
