package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written to it. Used because printUsage/printVersion/etc. write
// directly to os.Stdout rather than accepting an io.Writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return string(out)
}

// withTempHome points HOME at a fresh temp dir for the duration of fn,
// matching the pattern used in config_test.go / paths_test.go.
func withTempHome(t *testing.T) string {
	t.Helper()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", orig) })
	home := t.TempDir()
	os.Setenv("HOME", home)
	return home
}

func TestDetectMode(t *testing.T) {
	// In the go test sandbox, os.Stdout is never a real TTY, so
	// term.IsTerminal(os.Stdout.Fd()) is always false and detectMode always
	// returns "plain" regardless of args. This test documents that
	// constraint rather than exercising the TUI branch, which cannot be
	// reached without a real terminal attached to stdout.
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"config-only flag", []string{"--server=http://example.com"}},
		{"command arg", []string{"current"}},
		{"unknown flag", []string{"--bogus"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectMode(tt.args); got != "plain" {
				t.Errorf("detectMode(%v) = %q, want %q (no real TTY in test env)", tt.args, got, "plain")
			}
		})
	}
}

func TestHandleCommand(t *testing.T) {
	withTempHome(t)

	t.Run("empty args prints usage and returns nil", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := handleCommand(DefaultConfig(), nil, "wthr-cli"); err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
		})
		if !strings.Contains(out, "Usage:") {
			t.Errorf("expected usage text in output, got %q", out)
		}
	})

	t.Run("unknown command returns usage error", func(t *testing.T) {
		err := handleCommand(DefaultConfig(), []string{"bogus"}, "wthr-cli")
		if err == nil {
			t.Fatal("expected error for unknown command")
		}
		exitErr, ok := err.(*ExitError)
		if !ok {
			t.Fatalf("expected *ExitError, got %T", err)
		}
		if exitErr.Code != ExitUsageError {
			t.Errorf("expected ExitUsageError, got %d", exitErr.Code)
		}
	})

	t.Run("version command routes correctly", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := handleCommand(DefaultConfig(), []string{"version"}, "wthr-cli"); err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
		})
		if !strings.Contains(out, "wthr-cli v") {
			t.Errorf("expected version text, got %q", out)
		}
	})

	t.Run("logout routes correctly with no token file", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := handleCommand(DefaultConfig(), []string{"logout"}, "wthr-cli"); err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
		})
		if !strings.Contains(out, "Already logged out") {
			t.Errorf("expected already-logged-out message, got %q", out)
		}
	})

	t.Run("config with no subcommand returns usage error", func(t *testing.T) {
		err := handleCommand(DefaultConfig(), []string{"config"}, "wthr-cli")
		if err == nil {
			t.Fatal("expected error")
		}
		if exitErr, ok := err.(*ExitError); !ok || exitErr.Code != ExitUsageError {
			t.Errorf("expected usage error, got %v", err)
		}
	})
}

func TestPrintUsage(t *testing.T) {
	out := captureStdout(t, func() {
		printUsage("wthr-cli")
	})
	for _, want := range []string{"wthr-cli", "current", "forecast", "--server URL", "Environment Variables:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected usage output to contain %q", want)
		}
	}
}

func TestPrintVersion(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printVersion("wthr-cli"); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
	for _, want := range []string{"wthr-cli v", "Built:", "Go:", "OS/Arch:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected version output to contain %q, got %q", want, out)
		}
	}
}

func TestPrintShellCompletions(t *testing.T) {
	tests := []struct {
		shell   string
		wantErr bool
		want    string
	}{
		{"bash", false, "_wthr-cli_completions"},
		{"zsh", false, "#compdef wthr-cli"},
		{"fish", false, "complete -c wthr-cli"},
		{"powershell", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() {
				err = printShellCompletions("wthr-cli", tt.shell)
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for unsupported shell")
				}
				if exitErr, ok := err.(*ExitError); !ok || exitErr.Code != ExitUsageError {
					t.Errorf("expected usage error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("expected output to contain %q, got %q", tt.want, out)
			}
		})
	}
}

func TestPrintShellInit(t *testing.T) {
	tests := []struct {
		shell   string
		wantErr bool
		want    string
	}{
		{"bash", false, "eval \"$(wthr-cli --shell completions bash)\""},
		{"zsh", false, "eval \"$(wthr-cli --shell completions zsh)\""},
		{"fish", false, "wthr-cli --shell completions fish | source"},
		{"csh", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var err error
			out := captureStdout(t, func() {
				err = printShellInit("wthr-cli", tt.shell)
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for unsupported shell")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("expected output to contain %q, got %q", tt.want, out)
			}
		})
	}
}

func TestHandleConfigCommand(t *testing.T) {
	t.Run("no subcommand errors", func(t *testing.T) {
		err := handleConfigCommand(nil, "wthr-cli")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown subcommand errors", func(t *testing.T) {
		withTempHome(t)
		err := handleConfigCommand([]string{"bogus"}, "wthr-cli")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("init creates config file", func(t *testing.T) {
		withTempHome(t)
		if err := handleConfigCommand([]string{"init"}, "wthr-cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		configPath, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath() failed: %v", err)
		}
		if _, err := os.Stat(configPath); err != nil {
			t.Errorf("expected config file to exist after init: %v", err)
		}
	})

	t.Run("show prints defaults when no config exists", func(t *testing.T) {
		withTempHome(t)
		out := captureStdout(t, func() {
			if err := handleConfigCommand([]string{"show"}, "wthr-cli"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "No config file found") {
			t.Errorf("expected default-config message, got %q", out)
		}
	})

	t.Run("show prints file contents when config exists", func(t *testing.T) {
		withTempHome(t)
		if err := handleConfigCommand([]string{"init"}, "wthr-cli"); err != nil {
			t.Fatalf("init failed: %v", err)
		}
		out := captureStdout(t, func() {
			if err := handleConfigCommand([]string{"show"}, "wthr-cli"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if strings.Contains(out, "No config file found") {
			t.Errorf("expected actual config contents, got default message: %q", out)
		}
	})

	t.Run("get requires a key", func(t *testing.T) {
		withTempHome(t)
		err := handleConfigCommand([]string{"get"}, "wthr-cli")
		if err == nil {
			t.Fatal("expected error for missing key")
		}
	})

	t.Run("get returns value after init", func(t *testing.T) {
		withTempHome(t)
		if err := handleConfigCommand([]string{"init"}, "wthr-cli"); err != nil {
			t.Fatalf("init failed: %v", err)
		}
		out := captureStdout(t, func() {
			if err := handleConfigCommand([]string{"get", "output.format"}, "wthr-cli"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if strings.TrimSpace(out) == "" {
			t.Error("expected a non-empty value")
		}
	})

	t.Run("set requires key and value", func(t *testing.T) {
		withTempHome(t)
		if err := handleConfigCommand([]string{"init"}, "wthr-cli"); err != nil {
			t.Fatalf("init failed: %v", err)
		}
		if err := handleConfigCommand([]string{"set", "output.format"}, "wthr-cli"); err == nil {
			t.Fatal("expected error for missing value")
		}
	})

	t.Run("set updates value", func(t *testing.T) {
		withTempHome(t)
		if err := handleConfigCommand([]string{"init"}, "wthr-cli"); err != nil {
			t.Fatalf("init failed: %v", err)
		}
		out := captureStdout(t, func() {
			if err := handleConfigCommand([]string{"set", "output.format", "json"}, "wthr-cli"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "output.format = json") {
			t.Errorf("expected confirmation message, got %q", out)
		}
	})

	t.Run("path prints config file location", func(t *testing.T) {
		withTempHome(t)
		out := captureStdout(t, func() {
			if err := handleConfigCommand([]string{"path"}, "wthr-cli"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "cli.yml") {
			t.Errorf("expected config path in output, got %q", out)
		}
	})
}

func TestHandleLogoutCommand(t *testing.T) {
	t.Run("no token file present", func(t *testing.T) {
		withTempHome(t)
		out := captureStdout(t, func() {
			if err := handleLogoutCommand(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "Already logged out") {
			t.Errorf("expected already-logged-out message, got %q", out)
		}
	})

	t.Run("removes existing token file", func(t *testing.T) {
		withTempHome(t)
		tokenPath, err := TokenPath()
		if err != nil {
			t.Fatalf("TokenPath() failed: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(tokenPath), 0700); err != nil {
			t.Fatalf("failed to create token dir: %v", err)
		}
		if err := os.WriteFile(tokenPath, []byte("secret-token"), 0600); err != nil {
			t.Fatalf("failed to write token file: %v", err)
		}

		out := captureStdout(t, func() {
			if err := handleLogoutCommand(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
		if !strings.Contains(out, "Logged out") {
			t.Errorf("expected logged-out message, got %q", out)
		}
		if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
			t.Errorf("expected token file to be removed, stat err=%v", err)
		}
	})
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"empty", "", "(empty)"},
		{"short", "abc123", "***"},
		{"exactly 8 chars", "abcd1234", "abcd...1234"},
		{"long token", "sk_live_abcdefgh1234", "sk_l...1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskToken(tt.token); got != tt.want {
				t.Errorf("maskToken(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestHandleEarthquakesAndHurricanesCommandFlags(t *testing.T) {
	// These commands are also exercised in commands_test.go with an
	// httptest.Server. Here we only verify flag-parsing error paths that
	// don't require a live server since flag.ContinueOnError returns
	// before any network call is attempted.
	t.Run("earthquakes rejects unknown flag", func(t *testing.T) {
		err := handleEarthquakesCommand(DefaultConfig(), []string{"--bogus"})
		if err == nil {
			t.Fatal("expected usage error for unknown flag")
		}
		if exitErr, ok := err.(*ExitError); !ok || exitErr.Code != ExitUsageError {
			t.Errorf("expected usage error, got %v", err)
		}
	})

	t.Run("hurricanes rejects unknown flag", func(t *testing.T) {
		err := handleHurricanesCommand(DefaultConfig(), []string{"--bogus"})
		if err == nil {
			t.Fatal("expected usage error for unknown flag")
		}
		if exitErr, ok := err.(*ExitError); !ok || exitErr.Code != ExitUsageError {
			t.Errorf("expected usage error, got %v", err)
		}
	})
}
