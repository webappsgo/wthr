package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/webappsgo/wthr/src/common/display"
)

// ErrCannotEscalate is returned by EscalatePrivileges when the current user has no way
// to gain root/administrator access. Callers inform the user or fall back to a user-level
// alternative; they never prompt for a password that cannot work (AI.md PART 5).
var ErrCannotEscalate = errors.New("the current user cannot escalate privileges")

// IsPrivileged reports whether the current process runs as root (Unix) or with an
// elevated Administrator token (Windows).
func IsPrivileged() bool {
	return isElevatedProcess()
}

// CanEscalate reports whether the current user is able to gain root/administrator access.
// An already privileged process can always "escalate". The `su` method is not probed: it
// needs the root password, which cannot be known without prompting.
func CanEscalate() bool {
	if IsPrivileged() {
		return true
	}

	return canEscalateUser()
}

// escalationMethods lists the escalation commands to try on this OS, in the order AI.md
// PART 24 defines: Linux sudo, pkexec, doas; BSD doas, sudo; macOS sudo, osascript.
func escalationMethods(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"sudo", "osascript"}
	case "freebsd", "openbsd", "netbsd":
		return []string{"doas", "sudo"}
	case "windows":
		return []string{"runas"}
	default:
		return []string{"sudo", "pkexec", "doas"}
	}
}

// EscalatePrivileges re-runs the current executable with the given arguments and elevated
// privileges. It returns attempted=true once an elevated child ran (the caller then returns
// the child's error). It returns ErrCannotEscalate, without prompting, when the user has no
// way to escalate.
func EscalatePrivileges(args []string) (bool, error) {
	if IsPrivileged() {
		return false, nil
	}

	if !CanEscalate() {
		return false, ErrCannotEscalate
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to get executable path: %w", err)
	}

	if runtime.GOOS == "windows" {
		return escalateWindows(exePath, args)
	}

	for _, method := range escalationMethods(runtime.GOOS) {
		if _, err := exec.LookPath(method); err != nil {
			continue
		}

		return runEscalated(method, exePath, args)
	}

	return false, ErrCannotEscalate
}

// runEscalated runs the executable through one escalation command with the terminal attached.
func runEscalated(method, exePath string, args []string) (bool, error) {
	var cmd *exec.Cmd

	switch method {
	case "osascript":
		quoted := make([]string, 0, len(args)+1)
		for _, part := range append([]string{exePath}, args...) {
			quoted = append(quoted, shellQuote(part))
		}
		script := fmt.Sprintf(`do shell script %q with administrator privileges`, strings.Join(quoted, " "))
		cmd = exec.Command("osascript", "-e", script)
	default:
		cmd = exec.Command(method, append([]string{exePath}, args...)...)
	}

	fmt.Printf("%s Requesting administrator privileges (%s)...\n", display.Emoji("🔒", "*"), method)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return true, cmd.Run()
}

// shellQuote wraps a value in single quotes for a POSIX shell command line.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
