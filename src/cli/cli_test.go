// Tests for cli.go per AI.md PART 8 (CLI) / PART 29 (Testing).
//
// Parse() calls os.Exit(0) directly for --help/--version, so those are
// tested by calling ShowHelp()/ShowVersion() directly instead (never via
// Parse). c.flags is a flag.FlagSet built with flag.ExitOnError, so only
// well-formed flag combinations are exercised here -- an unrecognized flag
// would call os.Exit(2) and kill the test binary.
package cli

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// TestNewCLI_RegisterCommand covers basic construction and that a
// registered command is retrievable and invocable by name.
func TestNewCLI_RegisterCommand(t *testing.T) {
	c := NewCLI()
	if c == nil {
		t.Fatal("NewCLI() = nil")
	}
	if c.commands == nil {
		t.Fatal("NewCLI().commands = nil map")
	}

	called := false
	c.RegisterCommand(&Command{
		Name: "service",
		Handler: func(args []string) error {
			called = true
			return nil
		},
	})
	if _, ok := c.commands["service"]; !ok {
		t.Fatal("RegisterCommand() did not register under cmd.Name")
	}
	if err := c.commands["service"].Handler(nil); err != nil {
		t.Fatalf("Handler() error = %v", err)
	}
	if !called {
		t.Error("registered handler was not invoked")
	}
}

// TestCLI_Parse_ServiceDispatch covers Parse routing --service to the
// registered "service" command handler with the flag value as its sole arg,
// and the error returned when no such command is registered.
func TestCLI_Parse_ServiceDispatch(t *testing.T) {
	t.Run("dispatches_to_registered_handler", func(t *testing.T) {
		c := NewCLI()
		var gotArgs []string
		c.RegisterCommand(&Command{
			Name: "service",
			Handler: func(args []string) error {
				gotArgs = args
				return nil
			},
		})

		if err := c.Parse([]string{"--service", "start"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(gotArgs) != 1 || gotArgs[0] != "start" {
			t.Errorf("service handler args = %v, want [start]", gotArgs)
		}
	})

	t.Run("unregistered_command_errors", func(t *testing.T) {
		c := NewCLI()
		err := c.Parse([]string{"--service", "start"})
		if err == nil {
			t.Fatal("Parse() = nil, want error for unregistered service command")
		}
		if !strings.Contains(err.Error(), "service command not registered") {
			t.Errorf("error = %q, want substring %q", err.Error(), "service command not registered")
		}
	})
}

// TestCLI_Parse_MaintenanceAndUpdateDispatch covers Parse routing
// --maintenance/--update to their registered handlers with the remaining
// positional args (not just the flag value, unlike --service).
func TestCLI_Parse_MaintenanceAndUpdateDispatch(t *testing.T) {
	t.Run("maintenance_dispatches_with_trailing_args", func(t *testing.T) {
		c := NewCLI()
		var gotArgs []string
		c.RegisterCommand(&Command{
			Name: "maintenance",
			Handler: func(args []string) error {
				gotArgs = args
				return nil
			},
		})

		if err := c.Parse([]string{"--maintenance", "backup", "extra-positional"}); err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(gotArgs) != 1 || gotArgs[0] != "extra-positional" {
			t.Errorf("maintenance handler args = %v, want [extra-positional]", gotArgs)
		}
	})

	t.Run("update_unregistered_command_errors", func(t *testing.T) {
		c := NewCLI()
		err := c.Parse([]string{"--update", "check"})
		if err == nil {
			t.Fatal("Parse() = nil, want error for unregistered update command")
		}
		if !strings.Contains(err.Error(), "update command not registered") {
			t.Errorf("error = %q, want substring %q", err.Error(), "update command not registered")
		}
	})
}

// TestCLI_Parse_ShellDispatch covers Parse routing --shell through to
// handleShellCommand's safe (non-os.Exit) unknown-command branch.
func TestCLI_Parse_ShellDispatch(t *testing.T) {
	c := NewCLI()
	err := c.Parse([]string{"--shell", "bogus"})
	if err == nil {
		t.Fatal("Parse() = nil, want error for unknown shell command")
	}
	if !strings.Contains(err.Error(), "unknown shell command: bogus") {
		t.Errorf("error = %q, want substring %q", err.Error(), "unknown shell command: bogus")
	}
}

// TestCLI_Parse_StatusFlag covers the --status short-circuit, which sets an
// env var for main.go to observe rather than dispatching to a command.
func TestCLI_Parse_StatusFlag(t *testing.T) {
	c := NewCLI()
	t.Cleanup(func() { os.Unsetenv("CLI_STATUS_FLAG") })

	if err := c.Parse([]string{"--status"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if os.Getenv("CLI_STATUS_FLAG") != "1" {
		t.Error("Parse(--status) did not set CLI_STATUS_FLAG=1")
	}
}

// TestCLI_Parse_EnvVarPropagation covers that each configuration flag sets
// its documented environment variable (AI.md PART 5).
func TestCLI_Parse_EnvVarPropagation(t *testing.T) {
	envVars := []string{
		"MODE", "DEBUG", "PORT", "LISTEN", "CONFIG_DIR", "DATA_DIR",
		"CACHE_DIR", "LOG_DIR", "BACKUP_DIR", "PID_FILE", "DAEMON",
		"BASE_URL", "CLI_COLOR_MODE",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
	t.Cleanup(func() {
		for _, v := range envVars {
			os.Unsetenv(v)
		}
	})

	c := NewCLI()
	err := c.Parse([]string{
		"--mode", "development",
		"--debug",
		"--port", "9090",
		"--address", "127.0.0.1",
		"--config", "/tmp/cfg",
		"--data", "/tmp/data",
		"--cache", "/tmp/cache",
		"--log", "/tmp/log",
		"--backup", "/tmp/backup",
		"--pid", "/tmp/wthr.pid",
		"--daemon",
		"--baseurl", "/wthr",
		"--color", "always",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := map[string]string{
		"MODE": "development", "DEBUG": "true", "PORT": "9090",
		"LISTEN": "127.0.0.1", "CONFIG_DIR": "/tmp/cfg", "DATA_DIR": "/tmp/data",
		"CACHE_DIR": "/tmp/cache", "LOG_DIR": "/tmp/log", "BACKUP_DIR": "/tmp/backup",
		"PID_FILE": "/tmp/wthr.pid", "DAEMON": "true", "BASE_URL": "/wthr",
		"CLI_COLOR_MODE": "always",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("env %s = %q, want %q", k, got, v)
		}
	}
}

// TestCLI_IsFlagSet covers that IsFlagSet only reports flags explicitly
// passed on the command line, not flags left at their zero value.
func TestCLI_IsFlagSet(t *testing.T) {
	c := NewCLI()
	if err := c.Parse([]string{"--mode", "development"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	t.Cleanup(func() { os.Unsetenv("MODE") })

	if !c.IsFlagSet("mode") {
		t.Error("IsFlagSet(\"mode\") = false, want true (was passed on command line)")
	}
	if c.IsFlagSet("debug") {
		t.Error("IsFlagSet(\"debug\") = true, want false (was not passed)")
	}
	if c.IsFlagSet("nonexistent-flag") {
		t.Error("IsFlagSet(\"nonexistent-flag\") = true, want false")
	}
}

// TestCLI_GetFlag covers lookup of a known flag (non-nil) versus an unknown
// flag name (nil).
func TestCLI_GetFlag(t *testing.T) {
	c := NewCLI()
	if err := c.Parse(nil); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got := c.GetFlag("mode"); got == nil {
		t.Error("GetFlag(\"mode\") = nil, want non-nil *flag.Flag")
	}

	// GetFlag returns interface{} wrapping *flag.Flag, so a lookup miss
	// (typed nil *flag.Flag boxed into interface{}) does NOT compare equal
	// to a bare nil literal -- classic Go typed-nil-in-interface gotcha.
	// Assert on the underlying *flag.Flag instead.
	got, ok := c.GetFlag("nonexistent-flag").(*flag.Flag)
	if !ok {
		t.Fatalf("GetFlag(\"nonexistent-flag\") type = %T, want *flag.Flag", c.GetFlag("nonexistent-flag"))
	}
	if got != nil {
		t.Errorf("GetFlag(\"nonexistent-flag\") = %v, want nil *flag.Flag", got)
	}
}

// TestCLI_ShowHelp covers that help output documents the informational,
// shell-integration, server-configuration, and service-management sections.
func TestCLI_ShowHelp(t *testing.T) {
	c := NewCLI()
	out := captureStdout(t, c.ShowHelp)

	for _, want := range []string{
		"Usage:", "Information:", "--help", "--version", "--status",
		"Shell Integration:", "--shell completions",
		"Server Configuration:", "--mode {production|development}",
		"Service Management:", "--service CMD",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ShowHelp() output missing %q; got %q", want, out)
		}
	}
}

// TestCLI_ShowVersion covers that version output includes the version,
// build date, Go version, and OS/Arch fields.
func TestCLI_ShowVersion(t *testing.T) {
	c := NewCLI()
	out := captureStdout(t, c.ShowVersion)

	for _, want := range []string{"v" + Version, "Built:", "Go:", "OS/Arch:"} {
		if !strings.Contains(out, want) {
			t.Errorf("ShowVersion() output missing %q; got %q", want, out)
		}
	}
}
