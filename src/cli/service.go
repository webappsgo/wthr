package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/webappsgo/wthr/src/common/display"
	"github.com/webappsgo/wthr/src/util"
)

// plistName is the launchd label and file stem, {plist_name} in AI.md PART 3.
const plistName = "io.github." + serviceOrg + "." + serviceName

// ServiceCommand handles the --service operations of AI.md PART 24 and PART 25.
func ServiceCommand(args []string) error {
	if len(args) == 0 {
		return errors.New(T("cli.service.no_command"))
	}

	switch args[0] {
	case "--help", "help":
		showServiceHelp()
		return nil
	case "--install", "install":
		return installService()
	case "--uninstall", "uninstall":
		return uninstallService()
	case "--disable", "disable":
		return disableService()
	case "start", "stop", "restart", "reload":
		return controlService(args[0])
	case "status", "--status":
		showServiceStatus()
		return nil
	default:
		return fmt.Errorf(T("cli.service.unknown_command"), args[0])
	}
}

// showServiceHelp prints the service help output defined in AI.md PART 24 "Service Help Output".
func showServiceHelp() {
	fmt.Println(T("cli.service.help_title"))
	fmt.Println()
	fmt.Println(T("cli.service.help_start"))
	fmt.Println(T("cli.service.help_stop"))
	fmt.Println(T("cli.service.help_restart"))
	fmt.Println(T("cli.service.help_reload"))
	fmt.Println(T("cli.service.help_install"))
	fmt.Println(T("cli.service.help_disable"))
	fmt.Println(T("cli.service.help_uninstall"))
	fmt.Println()
	printServiceStatus(currentServiceStatus())
}

// showServiceStatus prints the current service status; it is read-only and needs no privileges.
func showServiceStatus() {
	printServiceStatus(currentServiceStatus())
}

// currentServiceStatus reads the status of the installed service, or an empty status when
// no service is installed or no service manager is available.
func currentServiceStatus() serviceStatus {
	manager, err := findInstalledManager()
	if err != nil || manager == nil {
		return serviceStatus{}
	}

	return manager.Status()
}

// printServiceStatus prints the "Current status" block of the service help output.
func printServiceStatus(status serviceStatus) {
	fmt.Println(T("cli.service.status_heading"))

	if !status.Installed {
		fmt.Printf(T("cli.service.status_service")+"\n", T("cli.service.value_not_installed"))
		return
	}

	autoStart := T("cli.service.value_disabled")
	if status.Enabled {
		autoStart = T("cli.service.value_enabled")
	}

	fmt.Printf(T("cli.service.status_service")+"\n", T("cli.service.value_installed"))
	fmt.Printf(T("cli.service.status_state")+"\n", stateLabel(status))
	fmt.Printf(T("cli.service.status_autostart")+"\n", autoStart)
	if status.Running && status.PID > 0 {
		fmt.Printf(T("cli.service.status_pid")+"\n", status.PID)
	}
}

// stateLabel names the run state: running, disabled (not enabled and not running) or stopped.
func stateLabel(status serviceStatus) string {
	switch {
	case status.Running:
		return T("cli.service.value_running")
	case !status.Enabled:
		return T("cli.service.value_disabled")
	default:
		return T("cli.service.value_stopped")
	}
}

// promptYesNo asks a yes/no question on the terminal. An empty answer, or no input at all,
// selects the default.
func promptYesNo(prompt string, defaultYes bool) bool {
	fmt.Print(prompt)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return defaultYes
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "":
		return defaultYes
	case "y", "yes":
		return true
	default:
		return false
	}
}

// escalateServiceCommand re-runs the current command line with elevated privileges. A user
// who cannot escalate gets the informative error and nothing is prompted (AI.md PART 5).
func escalateServiceCommand() error {
	attempted, err := util.EscalatePrivileges(os.Args[1:])
	if errors.Is(err, util.ErrCannotEscalate) {
		return errors.New(T("cli.service.cannot_escalate"))
	}
	if !attempted && err == nil {
		return errors.New(T("cli.service.cannot_escalate"))
	}

	return err
}

// installService installs, enables and starts the service. It installs the system service
// when privileged (or after the user agrees to escalate) and falls back to a user service
// otherwise (AI.md PART 24 "Service Installation Logic").
func installService() error {
	if util.IsPrivileged() {
		return installServiceWith(systemServiceManager, false)
	}

	if util.CanEscalate() {
		if promptYesNo(T("cli.service.escalate_prompt"), true) {
			return escalateServiceCommand()
		}
		fmt.Println(T("cli.service.user_fallback"))
	} else {
		fmt.Println(T("cli.service.no_admin_user_service"))
	}

	return installServiceWith(userServiceManager, true)
}

// installServiceWith runs the manager's install and reports the outcome.
func installServiceWith(newManager func() (serviceManager, error), userMode bool) error {
	manager, err := newManager()
	if err != nil {
		return err
	}

	if err := manager.Install(); err != nil {
		return fmt.Errorf(T("cli.service.install_failed"), err)
	}

	key := "cli.service.installed_started"
	if userMode {
		key = "cli.service.user_installed_started"
	}
	fmt.Printf(T(key)+"\n", display.Emoji("✓", "[OK]"), manager.Name())

	return nil
}

// uninstallService stops and removes the service, then deletes all data and the service
// account after the user confirms. The binary stays (AI.md PART 24 "Service Uninstall Logic").
func uninstallService() error {
	manager, err := findInstalledManager()
	if err != nil {
		return err
	}
	if manager == nil {
		return fmt.Errorf(T("cli.service.not_installed"), os.Args[0])
	}

	system := manager.IsSystem()
	if system && !util.IsPrivileged() {
		return escalateServiceCommand()
	}

	if !promptYesNo(T("cli.service.uninstall_confirm"), false) {
		fmt.Println(T("cli.service.uninstall_cancelled"))
		return nil
	}

	if err := manager.Uninstall(); err != nil {
		return fmt.Errorf(T("cli.service.uninstall_failed"), err)
	}

	if err := removeServiceData(system); err != nil {
		return err
	}

	fmt.Printf(T("cli.service.uninstalled")+"\n", display.Emoji("✓", "[OK]"))
	if executable, err := os.Executable(); err == nil {
		fmt.Printf(T("cli.service.uninstall_binary_hint")+"\n", executable)
	}

	return nil
}

// disableService stops the service and removes it from auto-start; the service file, data
// and the service account stay so --install can enable it again.
func disableService() error {
	manager, err := findInstalledManager()
	if err != nil {
		return err
	}
	if manager == nil {
		return fmt.Errorf(T("cli.service.not_installed"), os.Args[0])
	}

	if manager.IsSystem() && !util.IsPrivileged() {
		return escalateServiceCommand()
	}

	if err := manager.Disable(); err != nil {
		return fmt.Errorf(T("cli.service.disable_failed"), err)
	}

	fmt.Printf(T("cli.service.disabled")+"\n", display.Emoji("✓", "[OK]"))

	return nil
}

// controlService runs start, stop, restart or reload against the installed service.
func controlService(action string) error {
	manager, err := findInstalledManager()
	if err != nil {
		return err
	}
	if manager == nil {
		return fmt.Errorf(T("cli.service.not_installed"), os.Args[0])
	}

	if manager.IsSystem() && !util.IsPrivileged() {
		return escalateServiceCommand()
	}

	return manager.Control(action)
}
