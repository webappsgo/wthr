// Tests for shell.go per AI.md PART 8 (Shell Integration) / PART 29 (Testing).
//
// handleShellCommand's "--help", "help", "completions", and "init" branches
// all call os.Exit(0) and are therefore NOT exercised directly (that would
// kill the test binary). Only the safe "unknown command" default branch is
// tested via handleShellCommand; the underlying print*Completions/printInit
// functions are tested directly instead, via captureStdout.
package cli

import (
	"strings"
	"testing"
)

// TestHandleShellCommand_UnknownCommand covers the only branch of
// handleShellCommand that returns instead of calling os.Exit.
func TestHandleShellCommand_UnknownCommand(t *testing.T) {
	c := NewCLI()
	err := c.handleShellCommand("bogus", nil)
	if err == nil {
		t.Fatal("handleShellCommand(\"bogus\", nil) = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown shell command: bogus") {
		t.Errorf("error = %q, want substring %q", err.Error(), "unknown shell command: bogus")
	}
}

// TestShowShellHelp asserts the help text documents both subcommands and
// gives usage examples for the three primary shells.
func TestShowShellHelp(t *testing.T) {
	c := NewCLI()
	out := captureStdout(t, func() { c.showShellHelp("wthr") })

	for _, want := range []string{
		"completions [SHELL]", "init [SHELL]", "wthr --shell init",
		"bash", "zsh", "fish",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("showShellHelp() output missing %q; got %q", want, out)
		}
	}
}

// TestDetectShell covers the SHELL-env-var path (both a recognized shell
// and an unrecognized one) using t.Setenv so the value is restored
// automatically. The /proc/<ppid>/comm fallback and final "bash" default
// are environment-dependent and not asserted beyond "detectShell never
// returns empty".
func TestDetectShell(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{"recognized_zsh", "/usr/bin/zsh", "zsh"},
		{"recognized_bash", "/bin/bash", "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			if got := detectShell(); got != tt.want {
				t.Errorf("detectShell() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("never_returns_empty", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := detectShell(); got == "" {
			t.Error("detectShell() = \"\", want a non-empty fallback")
		}
	})
}

// TestPrintCompletions covers dispatch to each shell's completion printer,
// including the unknown-shell fallback to bash.
func TestPrintCompletions(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{"bash", "bash", "Bash completion for wthr"},
		{"zsh", "zsh", "Zsh completion for wthr"},
		{"fish", "fish", "Fish completion for wthr"},
		{"powershell", "powershell", "PowerShell completion for wthr"},
		{"pwsh_alias", "pwsh", "PowerShell completion for wthr"},
		{"unknown_falls_back_to_bash", "cobol-shell", "Bash completion for wthr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() { printCompletions("wthr", tt.shell) })
			if !strings.Contains(out, tt.want) {
				t.Errorf("printCompletions(%q) output missing %q; got %q", tt.shell, tt.want, out)
			}
		})
	}
}

// TestPrintInit covers dispatch to each shell's init snippet, including the
// unknown-shell fallback to bash.
func TestPrintInit(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{"bash", "bash", "completions bash"},
		{"zsh", "zsh", "completions zsh"},
		{"fish", "fish", "completions fish | source"},
		{"powershell", "powershell", "completions powershell"},
		{"unknown_falls_back_to_bash", "cobol-shell", "completions bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() { printInit("wthr", tt.shell) })
			if !strings.Contains(out, tt.want) {
				t.Errorf("printInit(%q) output missing %q; got %q", tt.shell, tt.want, out)
			}
		})
	}
}
