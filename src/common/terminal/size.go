// Package terminal provides shared terminal size detection per AI.md PART 7.
package terminal

import (
	"os"

	"golang.org/x/term"
)

// SizeMode categorises terminal dimensions into named breakpoints.
// Per AI.md PART 7 — used by server banner, TUI, and CLI output.
type SizeMode int

const (
	SizeModeMicro     SizeMode = iota // <40 cols or <10 rows
	SizeModeMinimal                   // 40-59 cols or 10-15 rows
	SizeModeCompact                   // 60-79 cols or 16-23 rows
	SizeModeStandard                  // 80-119 cols and 24-39 rows
	SizeModeWide                      // 120-199 cols and 40-59 rows
	SizeModeUltrawide                 // 200-399 cols and 60-79 rows
	SizeModeMassive                   // 400+ cols and 80+ rows
)

// TerminalSize holds the measured dimensions and the computed SizeMode.
type TerminalSize struct {
	Cols int
	Rows int
	Mode SizeMode
}

// GetTerminalSize queries the terminal dimensions and returns a TerminalSize.
// Falls back to 80×24 when stdout is not a terminal or the query fails.
func GetTerminalSize() TerminalSize {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols == 0 {
		cols = 80
	}
	if err != nil || rows == 0 {
		rows = 24
	}
	return TerminalSize{
		Cols: cols,
		Rows: rows,
		Mode: calculateMode(cols, rows),
	}
}

// calculateMode maps column and row counts to a SizeMode breakpoint.
func calculateMode(cols, rows int) SizeMode {
	switch {
	case cols < 40 || rows < 10:
		return SizeModeMicro
	case cols < 60 || rows < 16:
		return SizeModeMinimal
	case cols < 80 || rows < 24:
		return SizeModeCompact
	case cols < 120 || rows < 40:
		return SizeModeStandard
	case cols < 200 || rows < 60:
		return SizeModeWide
	case cols < 400 || rows < 80:
		return SizeModeUltrawide
	default:
		return SizeModeMassive
	}
}

// ShowASCIIArt returns true when the terminal is wide enough to render ASCII art.
func (s SizeMode) ShowASCIIArt() bool { return s >= SizeModeStandard }

// ShowBorders returns true when the terminal is wide enough for border decorations.
func (s SizeMode) ShowBorders() bool { return s >= SizeModeCompact }

// ShowSidebar returns true when the terminal is wide enough for a sidebar pane.
func (s SizeMode) ShowSidebar() bool { return s >= SizeModeWide }

// ShowIcons returns true when the terminal has room for inline icons.
func (s SizeMode) ShowIcons() bool { return s >= SizeModeMinimal }

// MaxTableColumns returns the maximum number of table columns for this size mode.
func (s SizeMode) MaxTableColumns() int {
	switch s {
	case SizeModeMicro:
		return 2
	case SizeModeMinimal:
		return 3
	case SizeModeCompact:
		return 4
	case SizeModeStandard:
		return 6
	default:
		return 10
	}
}
