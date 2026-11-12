package util

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// IsContainer reports whether the process runs inside an application
// container (AI.md PART 8 Container Detection). It is a variable so tests can
// pin the answer.
var IsContainer = detectContainer

func detectContainer() bool {
	for _, file := range []string{"/.dockerenv", "/run/.containerenv", "/dev/lxc"} {
		if _, err := os.Stat(file); err == nil {
			return true
		}
	}
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	switch parentProcessName() {
	case "tini", "dumb-init", "s6-svscan", "runsv", "runsvdir", "catatonit":
		return true
	case executableBaseName():
		return true
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		for _, marker := range []string{"docker", "kubepods", "lxc"} {
			if strings.Contains(content, marker) {
				return true
			}
		}
	}

	return false
}

// DetectServiceManager names the service manager that launched the process:
// systemd, launchd, runit, s6, container, sysv, rcd or manual. A systemd
// launch (INVOCATION_ID set) wins over generic container detection, so Incus,
// LXC and nspawn system containers still report systemd (AI.md PART 8).
func DetectServiceManager() string {
	if os.Getenv("INVOCATION_ID") != "" {
		return "systemd"
	}
	if IsContainer() {
		return "container"
	}

	ppid := os.Getppid()
	if ppid == 1 {
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			return "systemd"
		}
	}
	if runtime.GOOS == "darwin" && ppid == 1 {
		return "launchd"
	}
	if os.Getenv("SVDIR") != "" {
		return "runit"
	}
	if os.Getenv("S6_LOGGING") != "" {
		return "s6"
	}
	if ppid == 1 {
		if _, err := os.Stat("/etc/init.d"); err == nil {
			if _, err := os.Stat("/run/systemd/system"); os.IsNotExist(err) {
				return "sysv"
			}
		}
	}
	if _, err := os.Stat("/etc/rc.subr"); err == nil {
		return "rcd"
	}

	return "manual"
}

// ShouldDaemonize decides whether to detach: a service start follows the
// detected manager and ignores config; a manual start honors the flag and
// config (AI.md PART 8).
func ShouldDaemonize(isServiceStart, daemonFlag, configDaemonize bool) bool {
	if isServiceStart {
		switch DetectServiceManager() {
		case "sysv", "rcd":
			return true
		}

		return false
	}

	return daemonFlag || configDaemonize
}

func executableBaseName() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	return baseName(exe)
}

func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}

	return path
}

func parentProcessName() string {
	ppid := os.Getppid()
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid)); err == nil {
		return strings.TrimSpace(string(data))
	}
	if output, err := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=").Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	return ""
}
