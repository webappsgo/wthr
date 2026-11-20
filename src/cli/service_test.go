// Tests for service.go per AI.md PART 24 (Service Management) / PART 29 (Testing).
//
// Deliberately NOT covered here: installService/uninstallService/disableService
// and startService/stopService/restartService/reloadService. Those either
// write real files under /etc or /var/lib (outside any temp dir) or shell out
// to systemctl/launchctl/etc, which are external OS services per
// testing-rules.md ("NEVER write tests that depend on external services").
// Exercising them would mutate the host running the test suite.
package cli

import (
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// TestServiceCommand_Dispatch covers the command-routing paths that are
// side-effect free: no args, unknown command, and both spellings of help.
func TestServiceCommand_Dispatch(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no_args", []string{}, "No service command specified"},
		{"unknown_command", []string{"bogus"}, "Unknown service command: bogus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ServiceCommand(tt.args)
			if err == nil {
				t.Fatalf("ServiceCommand(%v) = nil, want error containing %q", tt.args, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ServiceCommand(%v) error = %q, want substring %q", tt.args, err.Error(), tt.wantErr)
			}
		})
	}

	t.Run("help_variants_return_nil", func(t *testing.T) {
		for _, args := range [][]string{{"--help"}, {"help"}} {
			out := captureStdout(t, func() {
				if err := ServiceCommand(args); err != nil {
					t.Errorf("ServiceCommand(%v) error = %v, want nil", args, err)
				}
			})
			if !strings.Contains(out, "Service management commands:") {
				t.Errorf("ServiceCommand(%v) output = %q, want it to contain help banner", args, out)
			}
		}
	})
}

// TestShowServiceHelp verifies the help text matches the PART 24 service help output:
// every command and the current status block.
func TestShowServiceHelp(t *testing.T) {
	out := captureStdout(t, showServiceHelp)

	for _, want := range []string{
		"Service management commands:", "start  ", "stop  ", "restart  ", "reload  ",
		"--install  ", "--disable  ", "--uninstall  ", "Current status:", "Service:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("showServiceHelp() output missing %q; got %q", want, out)
		}
	}
}

// TestCommandExists covers a binary guaranteed to be on PATH in any POSIX
// container ("sh") and a name that cannot plausibly exist.
func TestCommandExists(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"existing_command", "sh", true},
		{"nonexistent_command", "wthr-definitely-not-a-real-binary-xyz", false},
		{"empty_string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandExists(tt.cmd); got != tt.want {
				t.Errorf("commandExists(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestIsRunitAvailable is a smoke test: it only asserts the function
// executes without panicking and returns a bool, since the actual runit
// filesystem markers (/etc/runit, /var/service) are outside test control.
func TestIsRunitAvailable(t *testing.T) {
	got := isRunitAvailable()
	if got != true && got != false {
		t.Fatalf("isRunitAvailable() returned a non-bool-like value: %v", got)
	}
}

// TestRunCommand covers a command that succeeds, a command that exits
// non-zero, and a binary that does not exist on PATH at all.
func TestRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		args    []string
		wantErr bool
	}{
		{"succeeds", "true", nil, false},
		{"nonzero_exit", "false", nil, true},
		{"binary_not_found", "wthr-definitely-not-a-real-binary-xyz", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCommand(tt.cmd, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCommand(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
		})
	}
}

// TestIsServiceOwnedDir verifies the delete guard accepts only absolute paths under
// {internal_org}/{internal_name}.
func TestIsServiceOwnedDir(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{"/etc/webappsgo/wthr", true},
		{"/var/lib/webappsgo/wthr/db", true},
		{"/etc/webappsgo", false},
		{"/etc/webappsgo/other", false},
		{"/etc", false},
		{"/", false},
		{"webappsgo/wthr", false},
		{"/etc/webappsgo/wthr-extra", false},
	}

	for _, tt := range tests {
		if got := isServiceOwnedDir(tt.dir); got != tt.want {
			t.Errorf("isServiceOwnedDir(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

// TestRemoveServiceDirs verifies owned directories are deleted, the empty organization
// directory goes with them and a directory outside {internal_org}/{internal_name} is kept.
func TestRemoveServiceDirs(t *testing.T) {
	root := t.TempDir()
	owned := filepath.Join(root, serviceOrg, serviceName, "data")
	foreign := filepath.Join(root, "foreign")
	for _, dir := range []string{owned, foreign} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeServiceDirs([]string{filepath.Join(root, serviceOrg, serviceName), foreign}); err != nil {
		t.Fatalf("removeServiceDirs() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, serviceOrg)); !os.IsNotExist(err) {
		t.Errorf("organization directory still exists, stat error = %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("a directory outside the service identity was deleted: %v", err)
	}
}

// TestStateLabel verifies the run state names.
func TestStateLabel(t *testing.T) {
	tests := []struct {
		status serviceStatus
		want   string
	}{
		{serviceStatus{Installed: true, Running: true, Enabled: true}, "running"},
		{serviceStatus{Installed: true, Enabled: true}, "stopped"},
		{serviceStatus{Installed: true}, "disabled"},
	}

	for _, tt := range tests {
		if got := stateLabel(tt.status); got != tt.want {
			t.Errorf("stateLabel(%+v) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestPrintServiceStatus verifies the status block for installed and missing services.
func TestPrintServiceStatus(t *testing.T) {
	missing := captureStdout(t, func() { printServiceStatus(serviceStatus{}) })
	if !strings.Contains(missing, "not installed") || strings.Contains(missing, "State:") {
		t.Errorf("not installed status = %q", missing)
	}

	running := captureStdout(t, func() {
		printServiceStatus(serviceStatus{Installed: true, Running: true, Enabled: true, PID: 4242})
	})
	for _, want := range []string{"installed", "running", "enabled", "4242"} {
		if !strings.Contains(running, want) {
			t.Errorf("running status missing %q: %q", want, running)
		}
	}
}

// TestPromptYesNo verifies answers, the default for an empty answer and for no input.
func TestPromptYesNo(t *testing.T) {
	tests := []struct {
		input      string
		defaultYes bool
		want       bool
	}{
		{"y\n", false, true},
		{"YES\n", false, true},
		{"n\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
		{"", true, true},
		{"", false, false},
		{"maybe\n", true, false},
	}

	for _, tt := range tests {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = writer.WriteString(tt.input)
		writer.Close()

		original := os.Stdin
		os.Stdin = reader
		var got bool
		captureStdout(t, func() { got = promptYesNo("? ", tt.defaultYes) })
		os.Stdin = original
		reader.Close()

		if got != tt.want {
			t.Errorf("promptYesNo(input %q, default %v) = %v, want %v", tt.input, tt.defaultYes, got, tt.want)
		}
	}
}

// TestReadPIDFile verifies a valid, a missing and a malformed PID file.
func TestReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.pid")
	malformed := filepath.Join(dir, "bad.pid")
	_ = os.WriteFile(valid, []byte("1234\n"), 0o644)
	_ = os.WriteFile(malformed, []byte("not a number"), 0o644)

	if got := readPIDFile(valid); got != 1234 {
		t.Errorf("readPIDFile(valid) = %d, want 1234", got)
	}
	if got := readPIDFile(malformed); got != 0 {
		t.Errorf("readPIDFile(malformed) = %d, want 0", got)
	}
	if got := readPIDFile(filepath.Join(dir, "missing.pid")); got != 0 {
		t.Errorf("readPIDFile(missing) = %d, want 0", got)
	}
}

// TestUserUnitPaths verifies the user-level unit and agent files live in the user's home.
func TestUserUnitPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	unit, err := (&systemdManager{user: true}).unitPath()
	if err != nil || unit != filepath.Join(home, ".config", "systemd", "user", serviceName+".service") {
		t.Errorf("user unit path = %q, %v", unit, err)
	}

	system, _ := (&systemdManager{}).unitPath()
	if system != "/etc/systemd/system/"+serviceName+".service" {
		t.Errorf("system unit path = %q", system)
	}

	agent, err := (&launchdManager{user: true}).plistPath()
	if err != nil || agent != filepath.Join(home, "Library", "LaunchAgents", plistName+".plist") {
		t.Errorf("user agent path = %q, %v", agent, err)
	}

	daemon, _ := (&launchdManager{}).plistPath()
	if daemon != "/Library/LaunchDaemons/"+plistName+".plist" {
		t.Errorf("system daemon path = %q", daemon)
	}
}

// TestManagerIdentity verifies which managers are system services.
func TestManagerIdentity(t *testing.T) {
	system := []serviceManager{&systemdManager{}, &openRCManager{}, &sysVManager{}, &runitManager{}, &launchdManager{}, &rcdManager{}, newWindowsManager()}
	for _, manager := range system {
		if !manager.IsSystem() || manager.Name() == "" {
			t.Errorf("%T must be a named system manager", manager)
		}
	}

	for _, manager := range []serviceManager{&systemdManager{user: true}, &launchdManager{user: true}} {
		if manager.IsSystem() {
			t.Errorf("%T with user=true must not be a system manager", manager)
		}
	}
}

// TestWindowsManagerOffWindows verifies the non-Windows stub reports an error and no status.
func TestWindowsManagerOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exercises the non-Windows stub")
	}

	manager := newWindowsManager()
	if manager.Install() == nil || manager.Uninstall() == nil || manager.Disable() == nil || manager.Control("start") == nil {
		t.Error("the non-Windows stub must return an error from every operation")
	}
	if manager.Status().Installed {
		t.Error("the non-Windows stub must never report an installed service")
	}
}

// TestReloadByPIDWithoutProcess verifies reload fails when the service is not running.
func TestReloadByPIDWithoutProcess(t *testing.T) {
	if err := reloadByPID(&stubManager{}); err == nil {
		t.Error("reloadByPID() = nil for a stopped service, want an error")
	}

	signal.Ignore(syscall.SIGHUP)
	defer signal.Reset(syscall.SIGHUP)

	if err := reloadByPID(&stubManager{status: serviceStatus{Installed: true, Running: true, PID: os.Getpid()}}); err != nil {
		t.Errorf("reloadByPID() = %v for a running process, want nil (SIGHUP is ignored by the server, but this test process must survive the signal)", err)
	}
}

// stubManager is a serviceManager whose status the test controls.
type stubManager struct {
	status serviceStatus
}

func (s *stubManager) Name() string          { return "stub" }
func (s *stubManager) IsSystem() bool        { return false }
func (s *stubManager) Install() error        { return nil }
func (s *stubManager) Uninstall() error      { return nil }
func (s *stubManager) Disable() error        { return nil }
func (s *stubManager) Control(string) error  { return nil }
func (s *stubManager) Status() serviceStatus { return s.status }
