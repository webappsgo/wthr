package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/apimgr/weather/src/util"
)

// ServiceCommand handles service management operations
func ServiceCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no service command specified. Use: start, stop, restart, reload, --install, --uninstall, --disable, or --help")
	}

	cmd := args[0]

	switch cmd {
	case "--help", "help":
		showServiceHelp()
		return nil

	case "--install", "install":
		return installService()

	case "--uninstall", "uninstall":
		return uninstallService()

	case "--disable", "disable":
		return disableService()

	case "start":
		return startService()

	case "stop":
		return stopService()

	case "restart":
		return restartService()

	case "reload":
		return reloadService()

	default:
		return fmt.Errorf("unknown service command: %s", cmd)
	}
}

func showServiceHelp() {
	fmt.Println("Service Management Help")
	fmt.Println()
	fmt.Println("INSTALLATION:")
	fmt.Println("  weather --service --install     Install as system service (requires root/admin)")
	fmt.Println("  weather --service --uninstall   Remove system service (requires root/admin)")
	fmt.Println("  weather --service --disable     Disable system service (requires root/admin)")
	fmt.Println()
	fmt.Println("CONTROL:")
	fmt.Println("  weather --service start         Start the service")
	fmt.Println("  weather --service stop          Stop the service")
	fmt.Println("  weather --service restart       Restart the service")
	fmt.Println("  weather --service reload        Reload configuration (SIGHUP)")
	fmt.Println()
	fmt.Println("SUPPORTED SERVICE MANAGERS:")
	fmt.Println("  Linux:   systemd, runit")
	fmt.Println("  macOS:   launchd")
	fmt.Println("  BSD:     rc.d")
	fmt.Println("  Windows: Windows Service Manager")
	fmt.Println()
}

func installService() error {
	// Check if running as root/admin, attempt escalation if not
	// TEMPLATE.md PART 8: Attempt privilege escalation before failing
	if !utils.IsPrivileged() {
		// Attempt privilege escalation
		attempted, err := utils.EscalatePrivileges(append([]string{"--service", "--install"}, os.Args[1:]...))
		if attempted {
			// Escalation was attempted, command was re-run with privileges
			// Return the error (if any) from the elevated process
			return err
		}
		// Escalation not possible
		return fmt.Errorf("service installation requires root/administrator privileges (and escalation failed)")
	}

	switch runtime.GOOS {
	case "linux":
		// Detect service manager on Linux (runit or systemd)
		if isRunitAvailable() {
			return installRunitService()
		}
		return installSystemdService()
	case "darwin":
		return installLaunchdService()
	case "freebsd", "openbsd", "netbsd":
		return installRCDService()
	case "windows":
		return installWindowsService()
	default:
		return fmt.Errorf("service installation not supported on %s", runtime.GOOS)
	}
}

func uninstallService() error {
	// Check if running as root/admin, attempt escalation if not
	if !utils.IsPrivileged() {
		// Attempt privilege escalation
		attempted, err := utils.EscalatePrivileges(append([]string{"--service", "--uninstall"}, os.Args[1:]...))
		if attempted {
			return err
		}
		return fmt.Errorf("service uninstallation requires root/administrator privileges (and escalation failed)")
	}

	switch runtime.GOOS {
	case "linux":
		// Detect service manager on Linux (runit or systemd)
		if isRunitAvailable() {
			return uninstallRunitService()
		}
		return uninstallSystemdService()
	case "darwin":
		return uninstallLaunchdService()
	case "freebsd", "openbsd", "netbsd":
		return uninstallRCDService()
	case "windows":
		return uninstallWindowsService()
	default:
		return fmt.Errorf("service uninstallation not supported on %s", runtime.GOOS)
	}
}

func disableService() error {
	if !utils.IsPrivileged() {
		// Attempt privilege escalation
		attempted, err := utils.EscalatePrivileges(append([]string{"--service", "--disable"}, os.Args[1:]...))
		if attempted {
			return err
		}
		return fmt.Errorf("disabling service requires root/administrator privileges (and escalation failed)")
	}

	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "disable", "weather")
	case "darwin":
		return runCommand("launchctl", "unload", "/Library/LaunchDaemons/com.apimgr.weather.plist")
	case "freebsd", "openbsd", "netbsd":
		fmt.Println("Service disabled. Remove from /etc/rc.conf to prevent auto-start.")
		return nil
	case "windows":
		return runCommand("sc", "config", "weather", "start=", "disabled")
	default:
		return fmt.Errorf("service disable not supported on %s", runtime.GOOS)
	}
}

func startService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "start", "weather")
	case "darwin":
		return runCommand("launchctl", "start", "com.apimgr.weather")
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "weather", "start")
	case "windows":
		return runCommand("sc", "start", "weather")
	default:
		return fmt.Errorf("service start not supported on %s", runtime.GOOS)
	}
}

func stopService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "stop", "weather")
	case "darwin":
		return runCommand("launchctl", "stop", "com.apimgr.weather")
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "weather", "stop")
	case "windows":
		return runCommand("sc", "stop", "weather")
	default:
		return fmt.Errorf("service stop not supported on %s", runtime.GOOS)
	}
}

func restartService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "restart", "weather")
	case "darwin":
		if err := stopService(); err != nil {
			return err
		}
		return startService()
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "weather", "restart")
	case "windows":
		if err := stopService(); err != nil {
			return err
		}
		return startService()
	default:
		return fmt.Errorf("service restart not supported on %s", runtime.GOOS)
	}
}

func reloadService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "reload", "weather")
	case "darwin":
		return runCommand("launchctl", "kickstart", "-k", "system/com.apimgr.weather")
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "weather", "reload")
	case "windows":
		fmt.Println("Config reload via Windows Service Manager not supported. Use restart instead.")
		return restartService()
	default:
		return fmt.Errorf("service reload not supported on %s", runtime.GOOS)
	}
}

// Platform-specific installation functions
func installSystemdService() error {
	// Create system user if it doesn't exist
	if err := createSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}

	// Create required directories
	if err := createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	serviceContent := `[Unit]
Description=Weather Service - Production-grade weather API server
Documentation=https://github.com/apimgr/weather
After=network.target

[Service]
Type=simple
User=weather
Group=weather
ExecStart=/usr/local/bin/weather
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=weather

# Security hardening
PrivateTmp=true
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/apimgr/weather /var/log/apimgr/weather

[Install]
WantedBy=multi-user.target
`

	// Write service file
	servicePath := "/etc/systemd/system/weather.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	if err := runCommand("systemctl", "enable", "weather"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Println("✓ Systemd service installed successfully")
	fmt.Println("  Use: systemctl start weather")
	return nil
}

func uninstallSystemdService() error {
	// Stop service
	runCommand("systemctl", "stop", "weather")

	// Disable service
	runCommand("systemctl", "disable", "weather")

	// Remove service file
	os.Remove("/etc/systemd/system/weather.service")

	// Reload systemd
	runCommand("systemctl", "daemon-reload")

	fmt.Println("✓ Systemd service uninstalled")
	return nil
}

func installLaunchdService() error {
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.apimgr.weather</string>
	<key>Program</key>
	<string>/usr/local/bin/weather</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/Library/Logs/apimgr/weather/weather.log</string>
	<key>StandardErrorPath</key>
	<string>/Library/Logs/apimgr/weather/error.log</string>
</dict>
</plist>
`

	plistPath := "/Library/LaunchDaemons/com.apimgr.weather.plist"
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	// Load service
	if err := runCommand("launchctl", "load", plistPath); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	fmt.Println("✓ Launchd service installed successfully")
	fmt.Println("  Use: launchctl start com.apimgr.weather")
	return nil
}

func uninstallLaunchdService() error {
	plistPath := "/Library/LaunchDaemons/com.apimgr.weather.plist"

	// Unload service
	runCommand("launchctl", "unload", plistPath)

	// Remove plist
	os.Remove(plistPath)

	fmt.Println("✓ Launchd service uninstalled")
	return nil
}

func installRCDService() error {
	rcScript := `#!/bin/sh
#
# PROVIDE: weather
# REQUIRE: networking
# KEYWORD: shutdown

. /etc/rc.subr

name="weather"
rcvar=weather_enable
command="/usr/local/bin/weather"
pidfile="/var/run/weather.pid"

load_rc_config $name
run_rc_command "$1"
`

	rcPath := "/usr/local/etc/rc.d/weather"
	if err := os.WriteFile(rcPath, []byte(rcScript), 0755); err != nil {
		return fmt.Errorf("failed to write rc.d script: %w", err)
	}

	fmt.Println("✓ RC.d service installed successfully")
	fmt.Println("  Add to /etc/rc.conf: weather_enable=\"YES\"")
	fmt.Println("  Use: service weather start")
	return nil
}

func uninstallRCDService() error {
	os.Remove("/usr/local/etc/rc.d/weather")
	fmt.Println("✓ RC.d service uninstalled")
	fmt.Println("  Remove from /etc/rc.conf: weather_enable")
	return nil
}

func installWindowsService() error {
	// Check for NSSM
	if _, err := exec.LookPath("nssm"); err != nil {
		return fmt.Errorf("NSSM not found. Install from https://nssm.cc/download")
	}

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Install service with NSSM
	if err := runCommand("nssm", "install", "weather", binPath); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	// Set service description
	runCommand("nssm", "set", "weather", "Description", "Weather Service - Production-grade weather API server")

	// Set startup type to automatic
	runCommand("nssm", "set", "weather", "Start", "SERVICE_AUTO_START")

	fmt.Println("✓ Windows service installed successfully")
	fmt.Println("  Use: sc start weather")
	return nil
}

func uninstallWindowsService() error {
	if _, err := exec.LookPath("nssm"); err == nil {
		// Uninstall with NSSM
		runCommand("nssm", "stop", "weather")
		runCommand("nssm", "remove", "weather", "confirm")
	} else {
		// Fallback to sc
		runCommand("sc", "stop", "weather")
		runCommand("sc", "delete", "weather")
	}

	fmt.Println("✓ Windows service uninstalled")
	return nil
}

// Runit service manager support
func isRunitAvailable() bool {
	// Check if runit is available by looking for /etc/runit or /var/service
	if _, err := os.Stat("/etc/runit"); err == nil {
		return true
	}
	if _, err := os.Stat("/var/service"); err == nil {
		return true
	}
	// Check if sv command exists
	return commandExists("sv")
}

func installRunitService() error {
	// Create system user if it doesn't exist
	if err := createSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}

	// Create required directories
	if err := createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	serviceDir := "/etc/sv/weather"

	// Create service directory
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	// Create run script
	runScript := `#!/bin/sh
exec 2>&1
exec chpst -u weather:weather /usr/local/bin/weather
`

	runPath := serviceDir + "/run"
	if err := os.WriteFile(runPath, []byte(runScript), 0755); err != nil {
		return fmt.Errorf("failed to create run script: %w", err)
	}

	// Create log directory
	logDir := serviceDir + "/log"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log run script
	logRunScript := `#!/bin/sh
exec svlogd -tt /var/log/apimgr/weather
`

	logRunPath := logDir + "/run"
	if err := os.WriteFile(logRunPath, []byte(logRunScript), 0755); err != nil {
		return fmt.Errorf("failed to create log run script: %w", err)
	}

	// Create log directory
	if err := os.MkdirAll("/var/log/apimgr/weather", 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Enable service by symlinking to /var/service
	linkPath := "/var/service/weather"
	// Remove if exists
	os.Remove(linkPath)
	if err := os.Symlink(serviceDir, linkPath); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Println("✓ Runit service installed successfully")
	fmt.Println("  Service directory: /etc/sv/weather")
	fmt.Println("  Service link: /var/service/weather")
	return nil
}

func uninstallRunitService() error {
	// Remove service link
	if err := os.Remove("/var/service/weather"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service link: %w", err)
	}

	// Stop the service first if sv command exists
	if commandExists("sv") {
		runCommand("sv", "stop", "weather")
	}

	// Remove service directory
	if err := os.RemoveAll("/etc/sv/weather"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service directory: %w", err)
	}

	fmt.Println("✓ Runit service uninstalled")
	return nil
}

// Helper functions
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// createSystemUser creates a system user and group for the service
func createSystemUser() error {
	// Check if user already exists
	if userExists("weather") {
		fmt.Println("✓ System user 'weather' already exists")
		return nil
	}

	switch runtime.GOOS {
	case "linux":
		return createLinuxUser()
	case "darwin":
		return createMacOSUser()
	case "freebsd", "openbsd", "netbsd":
		return createBSDUser()
	default:
		return fmt.Errorf("user creation not implemented for %s", runtime.GOOS)
	}
}

// createLinuxUser creates a system user on Linux
func createLinuxUser() error {
	// Create group first
	if err := runCommand("groupadd", "--system", "weather"); err != nil {
		// Group might already exist, check if that's the error
		if !groupExists("weather") {
			return fmt.Errorf("failed to create group: %w", err)
		}
	}

	// Create user with system flag, no login shell, no home directory
	err := runCommand("useradd",
		"--system",
		"--gid", "weather",
		"--no-create-home",
		"--shell", "/sbin/nologin",
		"--comment", "Weather service account",
		"weather")

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Println("✓ Created system user 'weather'")
	return nil
}

// createMacOSUser creates a system user on macOS
func createMacOSUser() error {
	// Find next available UID in system range (100-499)
	uid := findAvailableUID(100, 499)
	if uid == 0 {
		return fmt.Errorf("no available UID in system range")
	}

	// Create group
	if err := runCommand("dscl", ".", "-create", "/Groups/weather"); err != nil {
		if !groupExists("weather") {
			return fmt.Errorf("failed to create group: %w", err)
		}
	}
	runCommand("dscl", ".", "-create", "/Groups/weather", "PrimaryGroupID", fmt.Sprintf("%d", uid))
	runCommand("dscl", ".", "-create", "/Groups/weather", "RealName", "Weather Service")

	// Create user
	if err := runCommand("dscl", ".", "-create", "/Users/weather"); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	runCommand("dscl", ".", "-create", "/Users/weather", "UniqueID", fmt.Sprintf("%d", uid))
	runCommand("dscl", ".", "-create", "/Users/weather", "PrimaryGroupID", fmt.Sprintf("%d", uid))
	runCommand("dscl", ".", "-create", "/Users/weather", "UserShell", "/usr/bin/false")
	runCommand("dscl", ".", "-create", "/Users/weather", "RealName", "Weather Service")
	runCommand("dscl", ".", "-create", "/Users/weather", "NFSHomeDirectory", "/var/empty")

	fmt.Println("✓ Created system user 'weather'")
	return nil
}

// createBSDUser creates a system user on BSD
func createBSDUser() error {
	// Create group
	if err := runCommand("pw", "groupadd", "weather", "-g", "800"); err != nil {
		if !groupExists("weather") {
			return fmt.Errorf("failed to create group: %w", err)
		}
	}

	// Create user
	err := runCommand("pw", "useradd", "weather",
		"-u", "800",
		"-g", "weather",
		"-s", "/usr/sbin/nologin",
		"-d", "/nonexistent",
		"-c", "Weather service account")

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Println("✓ Created system user 'weather'")
	return nil
}

// createServiceDirectories creates required directories with correct ownership
func createServiceDirectories() error {
	dirs := []string{
		"/var/lib/apimgr/weather",
		"/var/lib/apimgr/weather/db",
		"/var/log/apimgr/weather",
		"/etc/apimgr/weather",
	}

	for _, dir := range dirs {
		// Create directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Set ownership to weather:weather (skip if user doesn't exist)
		if userExists("weather") {
			if runtime.GOOS != "windows" {
				runCommand("chown", "-R", "weather:weather", dir)
			}
		}
	}

	fmt.Println("✓ Created service directories")
	return nil
}

// userExists checks if a user exists on the system
func userExists(username string) bool {
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		// Check /etc/passwd
		cmd := exec.Command("id", "-u", username)
		return cmd.Run() == nil
	case "darwin":
		// Use dscl on macOS
		cmd := exec.Command("dscl", ".", "-read", "/Users/"+username)
		return cmd.Run() == nil
	default:
		return false
	}
}

// groupExists checks if a group exists on the system
func groupExists(groupname string) bool {
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		cmd := exec.Command("getent", "group", groupname)
		return cmd.Run() == nil
	case "darwin":
		cmd := exec.Command("dscl", ".", "-read", "/Groups/"+groupname)
		return cmd.Run() == nil
	default:
		return false
	}
}

// findAvailableUID finds an available UID in the given range
func findAvailableUID(min, max int) int {
	for uid := min; uid <= max; uid++ {
		cmd := exec.Command("dscl", ".", "-list", "/Users", "UniqueID")
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		// Simple check - in production you'd want more robust checking
		if !contains(string(output), fmt.Sprintf("%d", uid)) {
			return uid
		}
	}
	return 0
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
