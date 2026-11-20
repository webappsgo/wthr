//go:build !windows

package cli

import "errors"

// newWindowsManager is never reached on this platform: only the Windows build selects it.
func newWindowsManager() serviceManager {
	return unsupportedWindowsManager{}
}

// unsupportedWindowsManager satisfies serviceManager on platforms that have no Windows service.
type unsupportedWindowsManager struct{}

var errWindowsOnly = errors.New("the Windows service manager is only available on Windows")

func (unsupportedWindowsManager) Name() string          { return "Windows Service" }
func (unsupportedWindowsManager) IsSystem() bool        { return true }
func (unsupportedWindowsManager) Install() error        { return errWindowsOnly }
func (unsupportedWindowsManager) Uninstall() error      { return errWindowsOnly }
func (unsupportedWindowsManager) Disable() error        { return errWindowsOnly }
func (unsupportedWindowsManager) Control(string) error  { return errWindowsOnly }
func (unsupportedWindowsManager) Status() serviceStatus { return serviceStatus{} }
