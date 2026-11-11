package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	serviceName   = "wthr"
	serviceOrg    = "webappsgo"
	serviceBinary = "/usr/local/bin/wthr"
	serviceApp    = "Wthr Weather"
	serviceDocs   = "https://webappsgo.github.io/wthr"
)

// linuxInit names the Linux service managers AI.md PART 25 requires.
type linuxInit string

const (
	initSystemd linuxInit = "systemd"
	initOpenRC  linuxInit = "openrc"
	initRunit   linuxInit = "runit"
	initSysV    linuxInit = "sysv"
)

// detectLinuxInit picks the service manager of this host. systemd wins when it
// is the running init, then OpenRC, runit and finally SysVinit, which is only
// chosen when /etc/init.d exists with a working update-rc.d or chkconfig.
func detectLinuxInit() (linuxInit, error) {
	if _, err := os.Stat("/run/systemd/system"); err == nil && commandExists("systemctl") {
		return initSystemd, nil
	}
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return initOpenRC, nil
	}
	if isRunitAvailable() {
		return initRunit, nil
	}
	if commandExists("systemctl") {
		return initSystemd, nil
	}
	if info, err := os.Stat("/etc/init.d"); err == nil && info.IsDir() &&
		(commandExists("update-rc.d") || commandExists("chkconfig")) {
		return initSysV, nil
	}

	return "", fmt.Errorf("no supported service manager found (systemd, OpenRC, runit or SysVinit)")
}

// serviceTemplate fills the {placeholders} of a service file template.
func serviceTemplate(text string) string {
	return strings.NewReplacer(
		"{internal_name}", serviceName,
		"{internal_org}", serviceOrg,
		"{project_name}", serviceName,
		"{app_name}", serviceApp,
		"{docs}", serviceDocs,
		"{binary}", serviceBinary,
	).Replace(text)
}

const systemdUnitTemplate = `[Unit]
Description={app_name} service
Documentation={docs}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={binary}
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/{internal_org}/{internal_name}
ReadWritePaths=/var/lib/{internal_org}/{internal_name}
ReadWritePaths=/var/cache/{internal_org}/{internal_name}
ReadWritePaths=/var/log/{internal_org}/{internal_name}

[Install]
WantedBy=multi-user.target
`

const openRCScriptTemplate = `#!/sbin/openrc-run
name="{internal_name}"
description="{app_name}"
command="{binary}"
command_args=""
pidfile="/var/run/{internal_org}/{internal_name}.pid"
command_background=true
output_log="/var/log/{internal_org}/{internal_name}/server.log"
error_log="/var/log/{internal_org}/{internal_name}/error.log"

depend() {
    need net
    after firewall
    use dns logger
}

start_pre() {
    checkpath -d -m 0755 -o {internal_name}:{internal_name} /var/run/{internal_org}
    checkpath -d -m 0755 -o {internal_name}:{internal_name} /var/log/{internal_org}/{internal_name}
}
`

const sysVScriptTemplate = `#!/bin/sh
### BEGIN INIT INFO
# Provides:          {internal_name}
# Required-Start:    $network $remote_fs $syslog
# Required-Stop:     $network $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: {app_name}
# Description:       {app_name} daemon
### END INIT INFO

NAME={internal_name}
DAEMON={binary}
DAEMON_USER={internal_name}
PIDFILE=/var/run/{internal_org}/{internal_name}.pid
LOGFILE=/var/log/{internal_org}/{internal_name}/server.log

case "$1" in
    start)
        echo "Starting $NAME..."
        mkdir -p $(dirname $PIDFILE) $(dirname $LOGFILE)
        chown -R $DAEMON_USER:$DAEMON_USER $(dirname $PIDFILE) $(dirname $LOGFILE)
        start-stop-daemon --start --quiet --background --make-pidfile \
            --pidfile $PIDFILE --exec $DAEMON \
            --no-close >> $LOGFILE 2>&1
        ;;
    stop)
        echo "Stopping $NAME..."
        start-stop-daemon --stop --quiet --pidfile $PIDFILE --retry 30
        rm -f $PIDFILE
        ;;
    restart)
        $0 stop
        sleep 1
        $0 start
        ;;
    status)
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "$NAME is running (pid $(cat $PIDFILE))"
            exit 0
        else
            echo "$NAME is stopped"
            exit 3
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
exit 0
`

const runitRunTemplate = `#!/bin/sh
exec {binary} 2>&1
`

const runitLogRunTemplate = `#!/bin/sh
exec svlogd -tt /var/log/{internal_org}/{internal_name}
`

// installLinuxService installs the unit of the detected Linux service manager.
func installLinuxService() error {
	manager, err := detectLinuxInit()
	if err != nil {
		return err
	}

	switch manager {
	case initSystemd:
		return installSystemdService()
	case initOpenRC:
		if err := prepareServiceHost(); err != nil {
			return err
		}
		return installInitScript("openrc", openRCScriptTemplate, func() error {
			return runCommand("rc-update", "add", serviceName, "default")
		})
	case initSysV:
		if err := prepareServiceHost(); err != nil {
			return err
		}
		return installInitScript("sysv", sysVScriptTemplate, enableSysV)
	default:
		return installRunitService()
	}
}

// uninstallLinuxService removes the unit of the detected Linux service manager.
func uninstallLinuxService() error {
	manager, err := detectLinuxInit()
	if err != nil {
		return err
	}

	switch manager {
	case initSystemd:
		return uninstallSystemdService()
	case initOpenRC:
		_ = runCommand("rc-service", serviceName, "stop")
		_ = runCommand("rc-update", "del", serviceName, "default")
		return removeInitScript()
	case initSysV:
		_ = runCommand("/etc/init.d/"+serviceName, "stop")
		disableSysV()
		return removeInitScript()
	default:
		return uninstallRunitService()
	}
}

// linuxServiceControl runs start, stop, restart, reload or disable against the
// detected Linux service manager.
func linuxServiceControl(action string) error {
	manager, err := detectLinuxInit()
	if err != nil {
		return err
	}

	switch manager {
	case initSystemd:
		return runCommand("systemctl", action, serviceName)
	case initOpenRC:
		if action == "disable" {
			return runCommand("rc-update", "del", serviceName, "default")
		}
		return runCommand("rc-service", serviceName, action)
	case initSysV:
		if action == "disable" {
			disableSysV()
			return nil
		}
		if action == "reload" {
			action = "restart"
		}
		return runCommand("/etc/init.d/"+serviceName, action)
	default:
		switch action {
		case "reload":
			return runCommand("sv", "hup", serviceName)
		case "disable":
			if err := os.Remove("/var/service/" + serviceName); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		return runCommand("sv", action, serviceName)
	}
}

// prepareServiceHost creates the service account and its directories.
func prepareServiceHost() error {
	if err := createSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return nil
}

func installInitScript(kind, template string, enable func() error) error {
	path := "/etc/init.d/" + serviceName
	if err := os.WriteFile(path, []byte(serviceTemplate(template)), 0755); err != nil {
		return fmt.Errorf("failed to write %s init script: %w", kind, err)
	}
	if err := enable(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}
	fmt.Printf("Installed %s service: %s\n", kind, path)

	return nil
}

func removeInitScript() error {
	if err := os.Remove("/etc/init.d/" + serviceName); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove init script: %w", err)
	}

	return nil
}

func enableSysV() error {
	if commandExists("update-rc.d") {
		return runCommand("update-rc.d", serviceName, "defaults")
	}
	if err := runCommand("chkconfig", "--add", serviceName); err != nil {
		return err
	}

	return runCommand("chkconfig", serviceName, "on")
}

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
