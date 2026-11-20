package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/webappsgo/wthr/src/privilege"
	"github.com/webappsgo/wthr/src/util"
)

// serviceStatus is the state of the installed service shown by --service status.
type serviceStatus struct {
	Installed bool
	Running   bool
	Enabled   bool
	PID       int
}

// serviceManager is one service manager of AI.md PART 25 (systemd, OpenRC, SysVinit, runit,
// launchd, rc.d, Windows Service). The same flows run against every manager.
type serviceManager interface {
	// Name is the human readable manager name shown after an install.
	Name() string
	// IsSystem is true for a system service and false for a user service.
	IsSystem() bool
	// Install writes the service file, enables the service and starts it.
	Install() error
	// Uninstall stops the service, disables it and removes the service file.
	Uninstall() error
	// Disable stops the service and removes it from auto-start, keeping the service file.
	Disable() error
	// Control runs start, stop, restart or reload.
	Control(action string) error
	// Status reads the installed, running and enabled state.
	Status() serviceStatus
}

// systemServiceManager returns the system service manager of this host.
func systemServiceManager() (serviceManager, error) {
	switch runtime.GOOS {
	case "linux":
		kind, err := detectLinuxInit()
		if err != nil {
			return nil, errors.New(T("cli.service.no_manager"))
		}
		switch kind {
		case initSystemd:
			return &systemdManager{}, nil
		case initOpenRC:
			return &openRCManager{}, nil
		case initSysV:
			return &sysVManager{}, nil
		default:
			return &runitManager{}, nil
		}
	case "darwin":
		return &launchdManager{}, nil
	case "freebsd", "openbsd", "netbsd":
		return &rcdManager{}, nil
	case "windows":
		return newWindowsManager(), nil
	default:
		return nil, fmt.Errorf(T("cli.service.unsupported_platform"), runtime.GOOS)
	}
}

// userServiceManager returns the user-level service manager of this host: systemd --user on
// Linux and a LaunchAgent on macOS.
func userServiceManager() (serviceManager, error) {
	switch runtime.GOOS {
	case "linux":
		if !commandExists("systemctl") {
			return nil, errors.New(T("cli.service.no_user_manager"))
		}
		return &systemdManager{user: true}, nil
	case "darwin":
		return &launchdManager{user: true}, nil
	default:
		return nil, fmt.Errorf(T("cli.service.user_unsupported"), runtime.GOOS)
	}
}

// findInstalledManager returns the manager that has the service installed, the system one
// first. It returns nil when the service is not installed anywhere.
func findInstalledManager() (serviceManager, error) {
	var firstErr error

	for _, build := range []func() (serviceManager, error){systemServiceManager, userServiceManager} {
		manager, err := build()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if manager.Status().Installed {
			return manager, nil
		}
	}

	return nil, firstErr
}

// runQuiet runs a command and reports whether it exited successfully.
func runQuiet(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

// commandOutput runs a command and returns its trimmed standard output.
func commandOutput(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// readPIDFile reads the process ID in a PID file; it returns 0 when the file is missing or
// does not hold a number.
func readPIDFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	return pid
}

// reloadByPID reloads a service by sending SIGHUP to its main process. The server ignores
// SIGHUP because its config reloads automatically through the file watcher (AI.md PART 8),
// so the signal is the harmless "reload via init system" for managers with no reload verb.
func reloadByPID(manager serviceManager) error {
	status := manager.Status()
	if !status.Running || status.PID <= 0 {
		return errors.New(T("cli.service.not_running"))
	}

	process, err := os.FindProcess(status.PID)
	if err != nil {
		return err
	}

	return process.Signal(syscall.SIGHUP)
}

// systemdManager manages the service through systemd, as a system unit or a user unit.
type systemdManager struct {
	user bool
}

func (m *systemdManager) Name() string {
	if m.user {
		return "systemd --user"
	}

	return "systemd"
}

func (m *systemdManager) IsSystem() bool {
	return !m.user
}

// unitPath is the unit file of this manager.
func (m *systemdManager) unitPath() (string, error) {
	if !m.user {
		return "/etc/systemd/system/" + serviceName + ".service", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config", "systemd", "user", serviceName+".service"), nil
}

// systemctlArgs prefixes --user for a user unit.
func (m *systemdManager) systemctlArgs(args ...string) []string {
	if m.user {
		return append([]string{"--user"}, args...)
	}

	return args
}

func (m *systemdManager) ctl(args ...string) error {
	return runCommand("systemctl", m.systemctlArgs(args...)...)
}

func (m *systemdManager) Install() error {
	// the unit sandbox (ProtectSystem=strict) makes /etc read-only, so the service itself cannot
	// create its account at startup; a system unit gets the account and directories here
	if !m.user {
		if err := prepareServiceHost(); err != nil {
			return err
		}
	}

	path, err := m.unitPath()
	if err != nil {
		return err
	}

	content := serviceTemplate(systemdUnitTemplate)
	if m.user {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		content = serviceTemplateFor(systemdUserUnitTemplate, executable)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	for _, step := range [][]string{{"daemon-reload"}, {"enable", serviceName}, {"start", serviceName}} {
		if err := m.ctl(step...); err != nil {
			return err
		}
	}

	return nil
}

func (m *systemdManager) Uninstall() error {
	_ = m.ctl("stop", serviceName)
	_ = m.ctl("disable", serviceName)

	path, err := m.unitPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return m.ctl("daemon-reload")
}

func (m *systemdManager) Disable() error {
	if err := m.ctl("stop", serviceName); err != nil {
		return err
	}

	return m.ctl("disable", serviceName)
}

func (m *systemdManager) Control(action string) error {
	if action == "reload" {
		return m.ctl("kill", "--signal=SIGHUP", "--kill-whom=main", serviceName)
	}

	return m.ctl(action, serviceName)
}

func (m *systemdManager) Status() serviceStatus {
	path, err := m.unitPath()
	if err != nil {
		return serviceStatus{}
	}
	if _, err := os.Stat(path); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true}
	status.Running = runQuiet("systemctl", m.systemctlArgs("is-active", "--quiet", serviceName)...)
	status.Enabled = runQuiet("systemctl", m.systemctlArgs("is-enabled", "--quiet", serviceName)...)
	if status.Running {
		status.PID, _ = strconv.Atoi(commandOutput("systemctl", m.systemctlArgs("show", "-p", "MainPID", "--value", serviceName)...))
	}

	return status
}

// initScriptPath is the init script of OpenRC and SysVinit.
var initScriptPath = "/etc/init.d/" + serviceName

// prepareServiceHost creates the service account, its directories and their ownership.
// OpenRC and SysVinit scripts chown the pid and log directories to the {internal_name} user
// before the binary has ever run, so the account must exist when the script first starts.
func prepareServiceHost() error {
	dirs, err := serviceDataDirs(true)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return errors.New(T("cli.service.no_directories"))
	}

	account, err := privilege.EnsureAccount(serviceName, dirs[0])
	if err != nil {
		return fmt.Errorf(T("cli.service.account_failed"), err)
	}

	pidDir := filepath.Dir(util.SystemPIDFilePath())
	for _, dir := range append(dirs, pidDir) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := privilege.ChownTree(dir, account); err != nil {
			return err
		}
	}

	return nil
}

// openRCManager manages the service through OpenRC (Alpine, Gentoo, Devuan).
type openRCManager struct{}

func (m *openRCManager) Name() string   { return "OpenRC" }
func (m *openRCManager) IsSystem() bool { return true }

func (m *openRCManager) Install() error {
	if err := prepareServiceHost(); err != nil {
		return err
	}
	if err := os.WriteFile(initScriptPath, []byte(serviceTemplate(openRCScriptTemplate)), 0o755); err != nil {
		return err
	}
	if err := runCommand("rc-update", "add", serviceName, "default"); err != nil {
		return err
	}

	return runCommand("rc-service", serviceName, "start")
}

func (m *openRCManager) Uninstall() error {
	_ = runCommand("rc-service", serviceName, "stop")
	_ = runCommand("rc-update", "del", serviceName, "default")
	if err := os.Remove(initScriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *openRCManager) Disable() error {
	_ = runCommand("rc-service", serviceName, "stop")

	return runCommand("rc-update", "del", serviceName, "default")
}

func (m *openRCManager) Control(action string) error {
	if action == "reload" {
		return reloadByPID(m)
	}

	return runCommand("rc-service", serviceName, action)
}

func (m *openRCManager) Status() serviceStatus {
	if _, err := os.Stat(initScriptPath); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true}
	status.Running = strings.Contains(commandOutput("rc-service", serviceName, "status"), "started")
	status.Enabled = strings.Contains(commandOutput("rc-update", "show", "default"), serviceName)
	if status.Running {
		status.PID = readPIDFile(util.SystemPIDFilePath())
	}

	return status
}

// sysVManager manages the service through a SysVinit script.
type sysVManager struct{}

func (m *sysVManager) Name() string   { return "SysVinit" }
func (m *sysVManager) IsSystem() bool { return true }

func (m *sysVManager) Install() error {
	if err := prepareServiceHost(); err != nil {
		return err
	}
	if err := os.WriteFile(initScriptPath, []byte(serviceTemplate(sysVScriptTemplate)), 0o755); err != nil {
		return err
	}
	if err := enableSysV(); err != nil {
		return err
	}

	return runCommand(initScriptPath, "start")
}

func (m *sysVManager) Uninstall() error {
	_ = runCommand(initScriptPath, "stop")
	disableSysV()
	if err := os.Remove(initScriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *sysVManager) Disable() error {
	_ = runCommand(initScriptPath, "stop")
	disableSysV()

	return nil
}

func (m *sysVManager) Control(action string) error {
	if action == "reload" {
		return reloadByPID(m)
	}

	return runCommand(initScriptPath, action)
}

func (m *sysVManager) Status() serviceStatus {
	if _, err := os.Stat(initScriptPath); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true}
	status.PID = readPIDFile(util.SystemPIDFilePath())
	status.Running = runQuiet(initScriptPath, "status")
	status.Enabled = sysVEnabled()
	if !status.Running {
		status.PID = 0
	}

	return status
}

// sysVEnabled reports whether the script is linked into a runlevel start directory.
func sysVEnabled() bool {
	matches, _ := filepath.Glob("/etc/rc[2-5].d/S*" + serviceName)

	return len(matches) > 0
}

// enableSysV enables the init script at boot with update-rc.d (Debian) or chkconfig (RHEL).
func enableSysV() error {
	if commandExists("update-rc.d") {
		return runCommand("update-rc.d", serviceName, "defaults")
	}
	if err := runCommand("chkconfig", "--add", serviceName); err != nil {
		return err
	}

	return runCommand("chkconfig", serviceName, "on")
}

// disableSysV removes the init script from boot.
func disableSysV() {
	if commandExists("update-rc.d") {
		_ = exec.Command("update-rc.d", "-f", serviceName, "remove").Run()
		return
	}
	if commandExists("chkconfig") {
		_ = exec.Command("chkconfig", serviceName, "off").Run()
		_ = exec.Command("chkconfig", "--del", serviceName).Run()
	}
}

// runitManager manages the service through runit.
type runitManager struct{}

// runitServiceDir and runitLinkDir are the service definition and its enabled link.
var (
	runitServiceDir = "/etc/sv/" + serviceName
	runitLinkDir    = "/var/service/" + serviceName
)

func (m *runitManager) Name() string   { return "runit" }
func (m *runitManager) IsSystem() bool { return true }

// isRunitAvailable reports whether runit is the service supervisor of this host.
func isRunitAvailable() bool {
	if _, err := os.Stat("/etc/runit"); err == nil {
		return true
	}
	if _, err := os.Stat("/var/service"); err == nil {
		return true
	}

	return commandExists("sv")
}

func (m *runitManager) Install() error {
	logDir := filepath.Join("/var/log", serviceOrg, serviceName)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(runitServiceDir, "log"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runitServiceDir, "run"), []byte(serviceTemplate(runitRunTemplate)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runitServiceDir, "log", "run"), []byte(serviceTemplate(runitLogRunTemplate)), 0o755); err != nil {
		return err
	}

	if err := os.Remove(runitLinkDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	// runit supervises and starts the service as soon as the link exists
	return os.Symlink(runitServiceDir, runitLinkDir)
}

func (m *runitManager) Uninstall() error {
	if err := os.Remove(runitLinkDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if commandExists("sv") {
		_ = runCommand("sv", "stop", serviceName)
	}
	if err := os.RemoveAll(runitServiceDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *runitManager) Disable() error {
	if commandExists("sv") {
		_ = runCommand("sv", "stop", serviceName)
	}
	if err := os.Remove(runitLinkDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *runitManager) Control(action string) error {
	if action == "reload" {
		return runCommand("sv", "hup", serviceName)
	}

	return runCommand("sv", action, serviceName)
}

func (m *runitManager) Status() serviceStatus {
	if _, err := os.Stat(runitServiceDir); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true}
	if _, err := os.Lstat(runitLinkDir); err == nil {
		status.Enabled = true
	}

	output := commandOutput("sv", "status", serviceName)
	status.Running = strings.HasPrefix(output, "run:")
	if status.Running {
		if open := strings.Index(output, "(pid "); open >= 0 {
			status.PID, _ = strconv.Atoi(strings.TrimRight(strings.Fields(output[open+len("(pid "):])[0], ")"))
		}
	}

	return status
}

// launchdManager manages the service through launchd, as a LaunchDaemon or a LaunchAgent.
type launchdManager struct {
	user bool
}

func (m *launchdManager) Name() string {
	if m.user {
		return "launchd agent"
	}

	return "launchd"
}

func (m *launchdManager) IsSystem() bool {
	return !m.user
}

// plistPath is the property list of this manager.
func (m *launchdManager) plistPath() (string, error) {
	if !m.user {
		return "/Library/LaunchDaemons/" + plistName + ".plist", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, "Library", "LaunchAgents", plistName+".plist"), nil
}

// logDir is the directory of the stdout and stderr logs.
func (m *launchdManager) logDir() (string, error) {
	if !m.user {
		return filepath.Join("/var/log", serviceOrg, serviceName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, "Library", "Logs", serviceOrg, serviceName), nil
}

// domain is the launchctl domain used for print-disabled queries.
func (m *launchdManager) domain() string {
	if !m.user {
		return "system"
	}

	return "gui/" + strconv.Itoa(os.Getuid())
}

func (m *launchdManager) Install() error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}
	logDir, err := m.logDir()
	if err != nil {
		return err
	}

	binary := serviceBinary
	if m.user {
		if binary, err = os.Executable(); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := strings.NewReplacer("{log_dir}", logDir).Replace(serviceTemplateFor(launchdPlistTemplate, binary))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	// load -w also clears a disabled override, and RunAtLoad starts the service
	return runCommand("launchctl", "load", "-w", path)
}

func (m *launchdManager) Uninstall() error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}

	_ = runCommand("launchctl", "unload", "-w", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *launchdManager) Disable() error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}

	return runCommand("launchctl", "unload", "-w", path)
}

// Control stops with unload and starts with load: the plist has KeepAlive, so
// "launchctl stop" alone would start the service again immediately.
func (m *launchdManager) Control(action string) error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}

	switch action {
	case "start":
		return runCommand("launchctl", "load", path)
	case "stop":
		return runCommand("launchctl", "unload", path)
	case "restart":
		_ = runCommand("launchctl", "unload", path)
		return runCommand("launchctl", "load", path)
	default:
		return reloadByPID(m)
	}
}

func (m *launchdManager) Status() serviceStatus {
	path, err := m.plistPath()
	if err != nil {
		return serviceStatus{}
	}
	if _, err := os.Stat(path); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true, Enabled: true}
	disabled := commandOutput("launchctl", "print-disabled", m.domain())
	for _, marker := range []string{`"` + plistName + `" => disabled`, `"` + plistName + `" => true`} {
		if strings.Contains(disabled, marker) {
			status.Enabled = false
		}
	}

	listing := commandOutput("launchctl", "list", plistName)
	if listing != "" {
		for _, line := range strings.Split(listing, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, `"PID"`) {
				value := strings.Trim(strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), "; ")
				status.PID, _ = strconv.Atoi(value)
				status.Running = status.PID > 0
			}
		}
	}

	return status
}

// rcdManager manages the service through the BSD rc.d system.
type rcdManager struct{}

// rcdScriptPath and rcdConfPath are the rc.d script and its rc.conf.d enable file.
var (
	rcdScriptPath = "/usr/local/etc/rc.d/" + serviceName
	rcdConfPath   = "/etc/rc.conf.d/" + serviceName
)

func (m *rcdManager) Name() string   { return "rc.d" }
func (m *rcdManager) IsSystem() bool { return true }

// setEnabled writes the rc.conf.d file that turns the service on or off at boot.
func (m *rcdManager) setEnabled(enabled bool) error {
	value := "NO"
	if enabled {
		value = "YES"
	}

	if err := os.MkdirAll(filepath.Dir(rcdConfPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(rcdConfPath, []byte(serviceName+"_enable=\""+value+"\"\n"), 0o644)
}

func (m *rcdManager) Install() error {
	if err := os.MkdirAll(filepath.Dir(rcdScriptPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(rcdScriptPath, []byte(serviceTemplate(rcdScriptTemplate)), 0o755); err != nil {
		return err
	}
	if err := m.setEnabled(true); err != nil {
		return err
	}

	// the rc.d script runs the binary in the foreground, so the start is detached
	start := exec.Command("service", serviceName, "start")
	if err := start.Start(); err != nil {
		return err
	}
	go func() { _ = start.Wait() }()

	return nil
}

func (m *rcdManager) Uninstall() error {
	_ = runCommand("service", serviceName, "onestop")
	if err := os.Remove(rcdConfPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(rcdScriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *rcdManager) Disable() error {
	_ = runCommand("service", serviceName, "onestop")

	return m.setEnabled(false)
}

func (m *rcdManager) Control(action string) error {
	if action == "reload" {
		return reloadByPID(m)
	}
	if action == "start" || action == "restart" {
		command := exec.Command("service", serviceName, action)
		if err := command.Start(); err != nil {
			return err
		}
		go func() { _ = command.Wait() }()

		return nil
	}

	return runCommand("service", serviceName, action)
}

func (m *rcdManager) Status() serviceStatus {
	if _, err := os.Stat(rcdScriptPath); err != nil {
		return serviceStatus{}
	}

	status := serviceStatus{Installed: true}
	if data, err := os.ReadFile(rcdConfPath); err == nil {
		status.Enabled = strings.Contains(string(data), `"YES"`)
	}

	output := commandOutput("service", serviceName, "onestatus")
	status.Running = strings.Contains(output, "is running")
	if status.Running {
		if open := strings.Index(output, "pid "); open >= 0 {
			status.PID, _ = strconv.Atoi(strings.TrimRight(strings.Fields(output[open+len("pid "):])[0], ")."))
		}
	}

	return status
}

// runCommand runs a command with its output attached to the terminal.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// commandExists reports whether an executable is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)

	return err == nil
}
