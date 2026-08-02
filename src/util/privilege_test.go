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
