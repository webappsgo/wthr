package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installServiceCommand(t *testing.T, binDir, name, body string) {
	t.Helper()
	path := filepath.Join(binDir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func serviceManagerTestEnvironment(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(os.TempDir(), "webappsgo")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(parent, "wthr-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return root
}

func TestSystemdUserManagerLifecycle(t *testing.T) {
	serviceManagerTestEnvironment(t)
	installServiceCommand(t, filepath.Join(os.Getenv("PATH")[:strings.Index(os.Getenv("PATH"), string(os.PathListSeparator))]), "systemctl", `
case "$*" in
  *"show -p MainPID --value"*) echo 4321 ;;
esac
exit 0`)

	manager := &systemdManager{user: true}
	if err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	path, err := manager.unitPath()
	if err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(unit), "ExecStart=") {
		t.Fatalf("user unit was not installed: err=%v content=%q", err, unit)
	}
	status := manager.Status()
	if !status.Installed || !status.Running || !status.Enabled || status.PID != 4321 {
		t.Fatalf("Status() = %+v", status)
	}
	if err := manager.Control("restart"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unit still exists after uninstall: %v", err)
	}
}

func TestLaunchdUserManagerLifecycle(t *testing.T) {
	serviceManagerTestEnvironment(t)
	binDir := filepath.Join(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0])
	installServiceCommand(t, binDir, "launchctl", `
case "$1" in
  print-disabled) printf '"`+plistName+`" => disabled\n' ;;
  list) printf '"PID" = 4321;\n' ;;
esac
exit 0`)

	manager := &launchdManager{user: true}
	if err := manager.Install(); err != nil {
		t.Fatal(err)
	}
	path, err := manager.plistPath()
	if err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if !status.Installed || status.Enabled || !status.Running || status.PID != 4321 {
		t.Fatalf("Status() = %+v", status)
	}
	for _, action := range []string{"start", "stop", "restart"} {
		if err := manager.Control(action); err != nil {
			t.Errorf("Control(%q): %v", action, err)
		}
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("plist still exists after uninstall: %v", err)
	}
}

func TestOpenRCManagerLifecycle(t *testing.T) {
	root := serviceManagerTestEnvironment(t)
	binDir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	installServiceCommand(t, binDir, "rc-service", `case "$*" in *status*) echo stopped ;; esac; exit 0`)
	installServiceCommand(t, binDir, "rc-update", `exit 0`)
	oldPath := initScriptPath
	initScriptPath = filepath.Join(root, "init.d", serviceName)
	t.Cleanup(func() { initScriptPath = oldPath })
	if err := os.MkdirAll(filepath.Dir(initScriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initScriptPath, []byte("service"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := &openRCManager{}
	if status := manager.Status(); !status.Installed || status.Running {
		t.Fatalf("Status() = %+v", status)
	}
	for _, action := range []string{"start", "stop", "restart"} {
		if err := manager.Control(action); err != nil {
			t.Errorf("Control(%q): %v", action, err)
		}
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(initScriptPath); !os.IsNotExist(err) {
		t.Fatalf("init script still exists: %v", err)
	}
}

func TestSysVManagerControlAndEnablement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("temporary init script execution is Linux-specific")
	}
	root := serviceManagerTestEnvironment(t)
	binDir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	installServiceCommand(t, binDir, "update-rc.d", `exit 0`)
	manager := &sysVManager{}
	if err := enableSysV(); err != nil {
		t.Fatal(err)
	}
	oldPath := initScriptPath
	initScriptPath = filepath.Join(root, "init.d", serviceName)
	t.Cleanup(func() { initScriptPath = oldPath })
	if err := os.MkdirAll(filepath.Dir(initScriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initScriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"start", "stop", "restart"} {
		if err := manager.Control(action); err != nil {
			t.Errorf("Control(%q): %v", action, err)
		}
	}
	if err := manager.Disable(); err != nil {
		t.Errorf("Disable: %v", err)
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(initScriptPath); !os.IsNotExist(err) {
		t.Fatalf("init script still exists: %v", err)
	}
}

func TestRunitManagerLifecycle(t *testing.T) {
	root := serviceManagerTestEnvironment(t)
	binDir := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	installServiceCommand(t, binDir, "sv", `case "$1" in status) echo 'run: wthr (pid 4321)' ;; esac; exit 0`)
	oldServiceDir, oldLinkDir := runitServiceDir, runitLinkDir
	runitServiceDir = filepath.Join(root, "sv", serviceName)
	runitLinkDir = filepath.Join(root, "service", serviceName)
	t.Cleanup(func() { runitServiceDir, runitLinkDir = oldServiceDir, oldLinkDir })
	if err := os.MkdirAll(runitServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(runitLinkDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(runitServiceDir, runitLinkDir); err != nil {
		t.Fatal(err)
	}
	manager := &runitManager{}
	status := manager.Status()
	if !status.Installed || !status.Running || !status.Enabled || status.PID != 4321 {
		t.Fatalf("Status() = %+v", status)
	}
	for _, action := range []string{"start", "stop", "restart", "reload"} {
		if err := manager.Control(action); err != nil {
			t.Errorf("Control(%q): %v", action, err)
		}
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runitServiceDir); !os.IsNotExist(err) {
		t.Fatalf("service directory still exists: %v", err)
	}
}
