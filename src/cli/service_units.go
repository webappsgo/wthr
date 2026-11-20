package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/webappsgo/wthr/src/privilege"
)

const (
	serviceName   = privilege.InternalName
	serviceOrg    = privilege.InternalOrg
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
		if _, err := os.Stat("/run/openrc"); err == nil {
			return initOpenRC, nil
		}
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

// serviceTemplate fills the {placeholders} of a service file template with the system
// binary path of AI.md PART 25.
func serviceTemplate(text string) string {
	return serviceTemplateFor(text, serviceBinary)
}

// serviceTemplateFor fills the {placeholders} of a service file template for a binary at
// the given path, which a user-level service takes from the running executable.
func serviceTemplateFor(text, binary string) string {
	return strings.NewReplacer(
		"{internal_name}", serviceName,
		"{internal_org}", serviceOrg,
		"{project_name}", serviceName,
		"{app_name}", serviceApp,
		"{docs}", serviceDocs,
		"{plist_name}", plistName,
		"{binary}", binary,
	).Replace(text)
}

const systemdUnitTemplate = `[Unit]
Description={project_name} service
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
# The + prefix runs the command with full privileges outside the sandbox, so the directories exist before ProtectSystem=strict applies
ExecStartPre=+/usr/bin/install -d -m 0755 /etc/{internal_org}/{internal_name} /var/lib/{internal_org}/{internal_name} /var/cache/{internal_org}/{internal_name} /var/log/{internal_org}/{internal_name} /var/run/{internal_org}/{internal_name}
# A leading - makes a path optional, so a path that does not exist yet never fails the namespace setup
ReadWritePaths=-/etc/{internal_org}/{internal_name}
ReadWritePaths=-/var/lib/{internal_org}/{internal_name}
ReadWritePaths=-/var/cache/{internal_org}/{internal_name}
ReadWritePaths=-/var/log/{internal_org}/{internal_name}
ReadWritePaths=-/var/run/{internal_org}/{internal_name}
ReadWritePaths=-/mnt/Backups/{internal_org}/{internal_name}

[Install]
WantedBy=multi-user.target
`

const openRCScriptTemplate = `#!/sbin/openrc-run
name="{internal_name}"
description="{app_name}"
command="{binary}"
command_args=""
pidfile="/var/run/{internal_org}/{internal_name}/{internal_name}.pid"
command_background=true
output_log="/var/log/{internal_org}/{internal_name}/server.log"
error_log="/var/log/{internal_org}/{internal_name}/error.log"

depend() {
    need net
    after firewall
    use dns logger
}

start_pre() {
    checkpath -d -m 0755 /var/run/{internal_org}
    checkpath -d -m 0755 -o {internal_name}:{internal_name} /var/run/{internal_org}/{internal_name}
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
PIDFILE=/var/run/{internal_org}/{internal_name}/{internal_name}.pid
LOGFILE=/var/log/{internal_org}/{internal_name}/server.log

case "$1" in
    start)
        echo "Starting $NAME..."
        mkdir -p $(dirname $PIDFILE) $(dirname $LOGFILE)
        chown -R $DAEMON_USER:$DAEMON_USER $(dirname $PIDFILE) $(dirname $LOGFILE)
        # No --chuid: the binary drops privileges itself after binding its ports (PART 8)
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

const systemdUserUnitTemplate = `[Unit]
Description={project_name} service
Documentation={docs}
After=network-online.target

[Service]
Type=simple
ExecStart={binary}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

const rcdScriptTemplate = `#!/bin/sh

# PROVIDE: {internal_name}
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="{internal_name}"
rcvar="{internal_name}_enable"
command="{binary}"

load_rc_config $name
run_rc_command "$1"
`

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{plist_name}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{binary}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{log_dir}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{log_dir}/stderr.log</string>
</dict>
</plist>
`
