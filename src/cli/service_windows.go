//go:build windows

package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// windowsAccount is the Virtual Service Account of the service. Windows creates and manages
// it when the service is installed; it has no password and minimal privileges (AI.md PART 24).
var windowsAccount = `NT SERVICE\` + serviceName

// windowsManager manages the service through the Windows Service Control Manager.
type windowsManager struct{}

// newWindowsManager returns the Windows Service manager.
func newWindowsManager() serviceManager {
	return &windowsManager{}
}

func (m *windowsManager) Name() string   { return "Windows Service" }
func (m *windowsManager) IsSystem() bool { return true }

func (m *windowsManager) Install() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()

	baseDir := windowsServiceBaseDir()
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	service, err := manager.CreateService(serviceName, executable, mgr.Config{
		DisplayName:      serviceName,
		Description:      serviceName + " service",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: windowsAccount,
	})
	if err != nil {
		return err
	}
	defer service.Close()

	// the account exists once the service does; it needs full control of its directory
	grant := exec.Command("icacls", baseDir, "/grant", windowsAccount+":(OI)(CI)F")
	if output, err := grant.CombinedOutput(); err != nil {
		return fmt.Errorf("granting %s access to %s: %w: %s", windowsAccount, baseDir, err, output)
	}

	return service.Start()
}

// open connects to the service control manager and opens the service.
func (m *windowsManager) open() (*mgr.Mgr, *mgr.Service, error) {
	manager, err := mgr.Connect()
	if err != nil {
		return nil, nil, err
	}

	service, err := manager.OpenService(serviceName)
	if err != nil {
		manager.Disconnect()
		return nil, nil, err
	}

	return manager, service, nil
}

// stopAndWait stops the service and waits for it to reach the stopped state.
func stopAndWait(service *mgr.Service) error {
	status, err := service.Control(svc.Stop)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(30 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for the service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		if status, err = service.Query(); err != nil {
			return err
		}
	}

	return nil
}

// stopIfRunning stops the service when it is not already stopped.
func stopIfRunning(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil || status.State == svc.Stopped {
		return nil
	}

	return stopAndWait(service)
}

func (m *windowsManager) Uninstall() error {
	manager, service, err := m.open()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	defer service.Close()

	_ = stopIfRunning(service)

	return service.Delete()
}

func (m *windowsManager) Disable() error {
	manager, service, err := m.open()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	defer service.Close()

	if err := stopIfRunning(service); err != nil {
		return err
	}

	config, err := service.Config()
	if err != nil {
		return err
	}
	config.StartType = mgr.StartDisabled

	return service.UpdateConfig(config)
}

func (m *windowsManager) Control(action string) error {
	manager, service, err := m.open()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	defer service.Close()

	switch action {
	case "start":
		return startWindowsService(service)
	case "stop":
		return stopAndWait(service)
	default:
		if action == "reload" {
			fmt.Println(T("cli.service.reload_windows_unsupported"))
		}
		if err := stopIfRunning(service); err != nil {
			return err
		}
		return startWindowsService(service)
	}
}

// startWindowsService starts the service, turning a disabled start type back to automatic.
func startWindowsService(service *mgr.Service) error {
	config, err := service.Config()
	if err != nil {
		return err
	}
	if config.StartType == mgr.StartDisabled {
		config.StartType = mgr.StartAutomatic
		if err := service.UpdateConfig(config); err != nil {
			return err
		}
	}

	return service.Start()
}

func (m *windowsManager) Status() serviceStatus {
	manager, service, err := m.open()
	if err != nil {
		return serviceStatus{}
	}
	defer manager.Disconnect()
	defer service.Close()

	status := serviceStatus{Installed: true}
	if query, err := service.Query(); err == nil {
		status.Running = query.State == svc.Running
		status.PID = int(query.ProcessId)
	}
	if config, err := service.Config(); err == nil {
		status.Enabled = config.StartType != mgr.StartDisabled
	}

	return status
}
