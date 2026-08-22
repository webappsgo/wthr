package cli

import (
	"os"
	"testing"
)

// TestCLI_Parse_LangFlag covers the AI.md PART 31 --lang flag on the server
// binary: it accepts both --lang=value and --lang value, and exports the
// resolved value as CLI_LANG for the i18n bootstrap in main.go.
func TestCLI_Parse_LangFlag(t *testing.T) {
	t.Run("space-separated value", func(t *testing.T) {
		os.Unsetenv("CLI_LANG")
		t.Cleanup(func() { os.Unsetenv("CLI_LANG") })

		c := NewCLI()
		if err := c.Parse([]string{"--lang", "es"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got := os.Getenv("CLI_LANG"); got != "es" {
			t.Fatalf("CLI_LANG = %q, want %q", got, "es")
		}
	})

	t.Run("equals-separated value", func(t *testing.T) {
		os.Unsetenv("CLI_LANG")
		t.Cleanup(func() { os.Unsetenv("CLI_LANG") })

		c := NewCLI()
		if err := c.Parse([]string{"--lang=ja"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got := os.Getenv("CLI_LANG"); got != "ja" {
			t.Fatalf("CLI_LANG = %q, want %q", got, "ja")
		}
	})

	t.Run("omitted flag leaves CLI_LANG unset", func(t *testing.T) {
		os.Unsetenv("CLI_LANG")
		t.Cleanup(func() { os.Unsetenv("CLI_LANG") })

		c := NewCLI()
		if err := c.Parse([]string{}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if _, set := os.LookupEnv("CLI_LANG"); set {
			t.Fatal("CLI_LANG was set without a --lang flag")
		}
	})
}
