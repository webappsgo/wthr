package display

import (
	"os"

	"golang.org/x/term"
)

// ColorModeEnvVar carries the resolved value of the shared `--color` flag
// between the flag parser and this gate. It holds "auto", "yes" or "no";
// any other value (including empty) is treated as "auto".
const ColorModeEnvVar = "CLI_COLOR_MODE"

// ColorModeAuto leaves the decision to the NO_COLOR / TTY / TERM chain.
const ColorModeAuto = "auto"

// ColorModeYes forces color on, overriding NO_COLOR and auto-detection.
const ColorModeYes = "yes"

// ColorModeNo forces color off.
const ColorModeNo = "no"

// ColorEnabled reports whether ANSI color output should be used.
// Per AI.md PART 8 this is the single color gate shared by every binary
// (server, CLI, agent), with precedence: CLI flag -> config -> NO_COLOR ->
// auto-detect (TTY + TERM). The CLI flag reaches this gate through
// ColorModeEnvVar; there is no separate config surface for output.color in
// this project, so the config layer is a no-op here.
// NO_COLOR disables colors and emojis only — bold/underline styling and
// Unicode box drawing are never gated by it.
func ColorEnabled() bool {
	// 1. CLI flag overrides everything (--color=auto|yes|no)
	switch os.Getenv(ColorModeEnvVar) {
	case ColorModeYes:
		return true
	case ColorModeNo:
		return false
	case ColorModeAuto:
		// Explicit auto: fall through to the NO_COLOR / TTY / TERM chain
	}

	// 2. NO_COLOR env var (non-empty = disable, value ignored)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 3. Auto-detect: stdout must be a TTY
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	// 4. Auto-detect: a dumb terminal supports no escape sequences
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}
