//go:build windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/webappsgo/wthr/src/common/display"
)

// isElevatedProcess reports whether the process token is elevated (running as Administrator).
func isElevatedProcess() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// canEscalateUser reports whether the user belongs to the Administrators group, which is
// what lets a UAC prompt succeed.
func canEscalateUser() bool {
	var adminSID *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&adminSID)
	if err != nil {
		return false
	}
	defer windows.FreeSid(adminSID)

	groups, err := windows.GetCurrentProcessToken().GetTokenGroups()
	if err != nil {
		return false
	}

	for _, group := range groups.AllGroups() {
		if windows.EqualSid(group.Sid, adminSID) {
			return true
		}
	}

	return false
}

// escalateWindows re-runs the executable through a UAC prompt and waits for it to finish.
func escalateWindows(exePath string, args []string) (bool, error) {
	fmt.Printf("%s Requesting administrator privileges (UAC)...\n", display.Emoji("🔒", "*"))

	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
	}

	command := fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs -Wait", strings.ReplaceAll(exePath, "'", "''"))
	if len(quoted) > 0 {
		command += " -ArgumentList " + strings.Join(quoted, ",")
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return true, cmd.Run()
}
