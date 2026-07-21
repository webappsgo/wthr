// Package banner provides responsive startup banner printing per AI.md PART 7.
// Used by the server binary on startup; the CLI has its own TUI setup wizard.
package banner

import (
	"fmt"
	"strings"

	"github.com/webappsgo/wthr/src/common/terminal"
)

// BannerConfig carries the data displayed in the startup banner.
type BannerConfig struct {
	AppName    string
	Version    string
	AppMode    string // "production" or "development"
	Debug      bool
	URLs       []string
	ShowSetup  bool   // true on first run only
	SetupToken string // first-run setup token
}

// PrintStartupBanner prints a banner sized to the current terminal.
func PrintStartupBanner(cfg BannerConfig) {
	size := terminal.GetTerminalSize()

	switch {
	case size.Mode >= terminal.SizeModeStandard:
		printFull(cfg, size)
	case size.Mode >= terminal.SizeModeCompact:
		printCompact(cfg)
	case size.Mode >= terminal.SizeModeMinimal:
		printMinimal(cfg)
	default:
		printMicro(cfg)
	}
}

// printFull renders a full-width banner for standard and larger terminals.
func printFull(cfg BannerConfig, size terminal.TerminalSize) {
	width := size.Cols
	if width > 120 {
		width = 120
	}

	line := strings.Repeat("─", width-2)
	fmt.Printf("┌%s┐\n", line)
	fmt.Printf("│  %-*s│\n", width-4, fmt.Sprintf("%s %s (%s)", cfg.AppName, cfg.Version, cfg.AppMode))
	if len(cfg.URLs) > 0 {
		fmt.Printf("├%s┤\n", line)
		for _, u := range cfg.URLs {
			fmt.Printf("│  %-*s│\n", width-4, u)
		}
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Printf("├%s┤\n", line)
		fmt.Printf("│  %-*s│\n", width-4, "Setup token: "+cfg.SetupToken)
	}
	fmt.Printf("└%s┘\n", line)
}

// printCompact renders a compact banner for 60-79 column terminals.
func printCompact(cfg BannerConfig) {
	fmt.Printf("=== %s %s (%s) ===\n", cfg.AppName, cfg.Version, cfg.AppMode)
	for _, u := range cfg.URLs {
		fmt.Println(u)
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Println("Setup: " + cfg.SetupToken)
	}
}

// printMinimal renders a minimal single-line banner for 40-59 column terminals.
func printMinimal(cfg BannerConfig) {
	fmt.Printf("%s %s\n", cfg.AppName, cfg.Version)
	for _, u := range cfg.URLs {
		fmt.Println(u)
	}
}

// printMicro renders a single-line output for terminals narrower than 40 columns.
func printMicro(cfg BannerConfig) {
	fmt.Printf("%s\n", cfg.AppName)
}
