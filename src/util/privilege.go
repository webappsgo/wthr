package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// IsPrivileged checks if the current process has administrator/root privileges
func IsPrivileged() bool {
	if runtime.GOOS == "windows" {
		// On Windows, try to create a file in system directory
		testPath := os.Getenv("SystemRoot") + "\\test_admin.tmp"
		file, err := os.Create(testPath)
		if err != nil {
			return false
		}
		file.Close()
		os.Remove(testPath)
		return true
	}

	// On Unix-like systems, check if UID is 0
	return os.Geteuid() == 0
}

// EscalatePrivileges attempts to re-run the current command with elevated privileges
// Per TEMPLATE.md PART 8: Try sudo, su, pkexec, doas on Linux; osascript on macOS
// Returns true if escalation was attempted (caller should exit), false if no escalation possible
func EscalatePrivileges(args []string) (bool, error) {
	if IsPrivileged() {
		// Already privileged
		return false, nil
	}

	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to get executable path: %w", err)
	}

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		return escalateUnix(exePath, args)
	case "darwin":
		return escalateMacOS(exePath, args)
	case "windows":
		return escalateWindows(exePath, args)
	default:
		return false, fmt.Errorf("privilege escalation not supported on %s", runtime.GOOS)
	}
}

// escalateUnix tries privilege escalation methods in order: sudo, pkexec, doas, su
func escalateUnix(exePath string, args []string) (bool, error) {
	// Try sudo first (most common)
	if _, err := exec.LookPath("sudo"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (sudo)...")
		cmd := exec.Command("sudo", append([]string{exePath}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	// Try pkexec (PolicyKit)
	if _, err := exec.LookPath("pkexec"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (pkexec)...")
		cmd := exec.Command("pkexec", append([]string{exePath}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	// Try doas (OpenBSD/some Linux)
	if _, err := exec.LookPath("doas"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (doas)...")
		cmd := exec.Command("doas", append([]string{exePath}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	// Try su (fallback, requires root password)
	if _, err := exec.LookPath("su"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (su)...")
		// su requires "-c" to run a command
		cmdStr := exePath + " " + strings.Join(args, " ")
		cmd := exec.Command("su", "-c", cmdStr)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	// No privilege escalation method available
	return false, fmt.Errorf("no privilege escalation method available (sudo, pkexec, doas, su not found)")
}

// escalateMacOS tries macOS-specific privilege escalation
func escalateMacOS(exePath string, args []string) (bool, error) {
	// Try sudo first
	if _, err := exec.LookPath("sudo"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (sudo)...")
		cmd := exec.Command("sudo", append([]string{exePath}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	// Try osascript for graphical prompt
	if _, err := exec.LookPath("osascript"); err == nil {
		fmt.Println("🔒 Requesting administrator privileges (GUI)...")
		cmdStr := exePath + " " + strings.Join(args, " ")
		script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, cmdStr)
		cmd := exec.Command("osascript", "-e", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return true, err
	}

	return false, fmt.Errorf("no privilege escalation method available on macOS")
}

// escalateWindows attempts UAC elevation on Windows
func escalateWindows(exePath string, args []string) (bool, error) {
	// On Windows, we need to use ShellExecute with "runas" verb
	// This requires using PowerShell or a platform-specific approach

	// Check if already running as admin
	if IsPrivileged() {
		return false, nil
	}

	fmt.Println("🔒 Requesting administrator privileges (UAC)...")

	// Use PowerShell to trigger UAC
	psCmd := fmt.Sprintf(`Start-Process -FilePath "%s" -ArgumentList "%s" -Verb RunAs -Wait`,
		exePath, strings.Join(args, `","`))

	cmd := exec.Command("powershell.exe", "-Command", psCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return true, err
}

// RequirePrivileges checks if running with privileges, and if not, attempts escalation
// Returns true if escalation was attempted (caller should exit)
func RequirePrivileges(args []string) bool {
	if IsPrivileged() {
		return false
	}

	attempted, err := EscalatePrivileges(args)
	if attempted {
		// Escalation was attempted, exit with appropriate code
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Escalation not possible
	fmt.Fprintln(os.Stderr, "Error: This operation requires administrator/root privileges")
	fmt.Fprintln(os.Stderr, "Please run with sudo or as administrator")
	os.Exit(1)
	return true
}
