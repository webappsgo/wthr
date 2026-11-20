package util

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

// TestIsPrivileged verifies the Unix branch matches os.Geteuid() == 0.
func TestIsPrivileged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses the process token, not exercised on this platform")
	}

	want := os.Geteuid() == 0
	if got := IsPrivileged(); got != want {
		t.Errorf("IsPrivileged() = %v, want %v (os.Geteuid()==0)", got, want)
	}
}

// TestCanEscalate_PrivilegedProcess verifies an already privileged process can always escalate.
func TestCanEscalate_PrivilegedProcess(t *testing.T) {
	if !IsPrivileged() {
		t.Skip("not running as root")
	}

	if !CanEscalate() {
		t.Error("CanEscalate() = false for a privileged process, want true")
	}
}

// TestEscalatePrivileges_AlreadyPrivileged verifies no escalation is attempted for root.
func TestEscalatePrivileges_AlreadyPrivileged(t *testing.T) {
	if !IsPrivileged() {
		t.Skip("not running as root")
	}

	attempted, err := EscalatePrivileges([]string{"--version"})
	if attempted || err != nil {
		t.Errorf("EscalatePrivileges() = (%v, %v), want (false, nil) for a privileged process", attempted, err)
	}
}

// TestErrCannotEscalate verifies the sentinel is matchable with errors.Is.
func TestErrCannotEscalate(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), ErrCannotEscalate)
	if !errors.Is(wrapped, ErrCannotEscalate) {
		t.Error("errors.Is(wrapped, ErrCannotEscalate) = false, want true")
	}
}

// TestEscalationMethods verifies the per-OS method order from AI.md PART 24.
func TestEscalationMethods(t *testing.T) {
	cases := map[string][]string{
		"linux":   {"sudo", "pkexec", "doas"},
		"darwin":  {"sudo", "osascript"},
		"freebsd": {"doas", "sudo"},
		"windows": {"runas"},
	}

	for goos, want := range cases {
		got := escalationMethods(goos)
		if len(got) != len(want) {
			t.Errorf("escalationMethods(%q) = %v, want %v", goos, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("escalationMethods(%q) = %v, want %v", goos, got, want)
				break
			}
		}
	}
}

// TestShellQuote verifies single quotes inside a value cannot break out of the quoting.
func TestShellQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf("shellQuote(\"a'b\") = %s", got)
	}
}
