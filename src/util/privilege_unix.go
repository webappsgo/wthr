//go:build !windows

package util

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
)

// isElevatedProcess reports whether the effective user is root.
func isElevatedProcess() bool {
	return os.Geteuid() == 0
}

// escalationGroups are the groups whose members may use sudo with a password.
var escalationGroups = map[string]bool{"sudo": true, "wheel": true, "admin": true}

// canEscalateUser checks whether this Unix user can obtain root: passwordless sudo, sudo
// group membership, passwordless doas, or PolicyKit in a graphical session.
func canEscalateUser() bool {
	if _, err := exec.LookPath("sudo"); err == nil {
		if exec.Command("sudo", "-n", "true").Run() == nil {
			return true
		}
		if isInEscalationGroup() {
			return true
		}
	}

	if _, err := exec.LookPath("doas"); err == nil {
		if exec.Command("doas", "-n", "true").Run() == nil {
			return true
		}
	}

	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("pkexec"); err == nil {
			if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
				return true
			}
		}
	}

	return false
}

// isInEscalationGroup reports whether the current user belongs to sudo, wheel or admin.
func isInEscalationGroup() bool {
	current, err := user.Current()
	if err != nil {
		return false
	}

	groupIDs, err := current.GroupIds()
	if err != nil {
		return false
	}

	for _, groupID := range groupIDs {
		group, err := user.LookupGroupId(groupID)
		if err == nil && escalationGroups[group.Name] {
			return true
		}
	}

	return false
}

// escalateWindows is never reached on Unix; the Windows build provides the real UAC path.
func escalateWindows(exePath string, args []string) (bool, error) {
	return false, ErrCannotEscalate
}
