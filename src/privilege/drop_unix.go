//go:build !windows

package privilege

import (
	"fmt"
	"os"
	"syscall"
)

// IsRoot reports whether the process runs as root.
func IsRoot() bool {
	return os.Geteuid() == 0
}

// DropPrivileges moves the process from root to the service account: supplementary
// groups first, then the GID, then the UID (the order matters, because after setuid
// the process can no longer change its groups). It then proves the drop worked: the
// process is no longer root and cannot become root again (AI.md PART 8 step 8h and 8i).
func DropPrivileges(account *Account) error {
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("clearing supplementary groups: %w", err)
	}
	if err := syscall.Setgid(account.GID); err != nil {
		return fmt.Errorf("setting GID %d: %w", account.GID, err)
	}
	if err := syscall.Setuid(account.UID); err != nil {
		return fmt.Errorf("setting UID %d: %w", account.UID, err)
	}

	if os.Getuid() == 0 || os.Geteuid() == 0 || os.Getgid() == 0 || os.Getegid() == 0 {
		return fmt.Errorf("privilege drop did not take effect: still uid=%d gid=%d", os.Getuid(), os.Getgid())
	}
	if err := syscall.Setuid(0); err == nil {
		return fmt.Errorf("privilege drop is reversible: the process could become root again")
	}

	// The account's home replaces the root home so no later lookup points into /root
	if account.Home != "" {
		if err := os.Setenv("HOME", account.Home); err != nil {
			return fmt.Errorf("setting HOME: %w", err)
		}
	}
	return nil
}
