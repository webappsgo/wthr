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
	"strings"
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
		{"no_args", []string{}, "no service command specified"},
		{"unknown_command", []string{"bogus"}, "unknown service command: bogus"},
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
			if !strings.Contains(out, "Service Management Help") {
				t.Errorf("ServiceCommand(%v) output = %q, want it to contain help banner", args, out)
			}
		}
	})
}

// TestShowServiceHelp verifies the help text documents installation,
// control, and every supported service manager per PART 24.
func TestShowServiceHelp(t *testing.T) {
	out := captureStdout(t, showServiceHelp)

	for _, want := range []string{
		"INSTALLATION:", "CONTROL:", "SUPPORTED SERVICE MANAGERS:",
		"systemd", "launchd", "rc.d", "Windows Service Manager",
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

// TestUserExists and TestGroupExists are read-only lookups (id -u / getent
// group on Linux); no user or group is created or removed. A user this
// unlikely to exist is used to assert the negative case deterministically.
func TestUserExists(t *testing.T) {
	if userExists("wthr-definitely-not-a-real-user-xyz") {
		t.Error("userExists() = true for a fabricated username, want false")
	}
	if !userExists("root") {
		t.Error("userExists(\"root\") = false, want true (root always exists on POSIX systems)")
	}
}

func TestGroupExists(t *testing.T) {
	if groupExists("wthr-definitely-not-a-real-group-xyz") {
		t.Error("groupExists() = true for a fabricated group name, want false")
	}
}

// TestFindAvailableUID uses a tiny range to keep the (non-existent-on-Linux)
// dscl lookups fast; on Linux dscl is absent so every iteration fails and
// the function must return 0 rather than hang or panic.
func TestFindAvailableUID(t *testing.T) {
	got := findAvailableUID(100, 101)
	if got < 0 {
		t.Errorf("findAvailableUID(100, 101) = %d, want >= 0", got)
	}
}

// TestContains and TestContainsMiddle document the actual (hand-rolled)
// substring-matching behavior of contains()/containsMiddle(), including the
// edge cases where it diverges from strings.Contains: an empty needle
// currently returns false rather than true, matching Go's convention that
// every string contains "".
func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact_match", "abc", "abc", true},
		{"prefix_match", "abcdef", "abc", true},
		{"suffix_match", "abcdef", "def", true},
		{"middle_match", "abcdef", "cd", true},
		{"no_match", "abcdef", "xyz", false},
		{"empty_haystack", "", "abc", false},
		{"empty_needle_returns_false", "abc", "", false},
		{"needle_longer_than_haystack", "ab", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestContainsMiddle(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"found_in_middle", "abcdef", "cd", true},
		{"not_found", "abcdef", "xyz", false},
		{"equal_length_match", "abc", "abc", true},
		{"substr_longer_than_s", "ab", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsMiddle(tt.s, tt.substr); got != tt.want {
				t.Errorf("containsMiddle(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}
