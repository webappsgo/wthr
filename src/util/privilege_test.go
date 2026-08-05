package util

import (
	"os"
	"runtime"
	"testing"
)

// TestIsPrivileged verifies the Unix branch matches os.Geteuid() == 0.
// Windows uses a filesystem-write probe instead and is not exercised here
// since CI runs on Linux.
func TestIsPrivileged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows privilege detection uses a filesystem probe, not exercised in this environment")
	}

	want := os.Geteuid() == 0
	if got := IsPrivileged(); got != want {
		t.Errorf("IsPrivileged() = %v, want %v (os.Geteuid()==0)", got, want)
	}
}

// TestRequirePrivileges_AlreadyPrivileged exercises only the safe
// early-return branch of RequirePrivileges: when IsPrivileged() is true,
// it returns false immediately without touching EscalatePrivileges or
// os.Exit. Every other branch of RequirePrivileges calls os.Exit and must
// never be invoked from a test process.
func TestRequirePrivileges_AlreadyPrivileged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows privilege detection uses a filesystem probe, not exercised in this environment")
	}
	if !IsPrivileged() {
		t.Skip("not running as root: RequirePrivileges' early-return branch is not reachable")
	}

	if got := RequirePrivileges(nil); got != false {
		t.Errorf("RequirePrivileges(nil) while already privileged = %v, want false", got)
	}
}
