#!/bin/bash
# install-linux.sh - Distro-agnostic installer for Weather Service
# Supports: systemd, OpenRC, init.d, runit
# Auto-detects: architecture, init system, package manager

set -e

PROJECTNAME="wthr"
GITHUB_REPO="casapps/wthr"
VERSION="latest"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Weather Service Installer ===${NC}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo "Architecture: $ARCH"

# Detect if running as root
if [ "$EUID" -eq 0 ]; then
    IS_ROOT=true
    BIN_DIR="/usr/local/bin"
    CONFIG_DIR="/etc/${PROJECTNAME}"
    DATA_DIR="/var/lib/${PROJECTNAME}"
    LOG_DIR="/var/log/${PROJECTNAME}"
else
    IS_ROOT=false
    BIN_DIR="$HOME/.local/bin"
    CONFIG_DIR="$HOME/.config/${PROJECTNAME}"
    DATA_DIR="$HOME/.local/share/${PROJECTNAME}"
    LOG_DIR="$HOME/.local/state/${PROJECTNAME}"
fi

echo "Install mode: $([ "$IS_ROOT" = true ] && echo "System (root)" || echo "User")"

# Create directories
mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
mkdir -p "$DATA_DIR/db"

# Download binary
echo "Downloading ${PROJECTNAME}-linux-${ARCH}..."
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/${VERSION}/download/${PROJECTNAME}-linux-${ARCH}"

if command -v curl &> /dev/null; then
    curl -L -o "${BIN_DIR}/${PROJECTNAME}" "$DOWNLOAD_URL"
elif command -v wget &> /dev/null; then
    wget -O "${BIN_DIR}/${PROJECTNAME}" "$DOWNLOAD_URL"
else
    echo -e "${RED}Error: curl or wget required${NC}"
    exit 1
fi

chmod +x "${BIN_DIR}/${PROJECTNAME}"
echo -e "${GREEN}✓ Binary installed to ${BIN_DIR}/${PROJECTNAME}${NC}"

# Detect init system
detect_init() {
    if [ -d /run/systemd/system ] || command -v systemctl &> /dev/null; then
        echo "systemd"
    elif [ -f /sbin/openrc-run ] || [ -d /etc/init.d ] && grep -q "openrc" /sbin/init 2>/dev/null; then
        echo "openrc"
    elif [ -d /etc/init.d ] && [ ! -d /run/systemd/system ]; then
        echo "sysvinit"
    elif command -v sv &> /dev/null; then
        echo "runit"
    else
        echo "unknown"
    fi
}

INIT_SYSTEM=$(detect_init)
echo "Init system: $INIT_SYSTEM"

# Install service based on init system
case $INIT_SYSTEM in
    systemd)
        if [ "$IS_ROOT" = true ]; then
            cat > /etc/systemd/system/${PROJECTNAME}.service << EOF
[Unit]
Description=Weather API Service
After=network.target

[Service]
Type=simple
User=nobody
Group=nogroup
ExecStart=${BIN_DIR}/${PROJECTNAME}
Restart=always
RestartSec=5
Environment="CONFIG_DIR=${CONFIG_DIR}"
Environment="DATA_DIR=${DATA_DIR}"
Environment="LOG_DIR=${LOG_DIR}"
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

            systemctl daemon-reload
            systemctl enable ${PROJECTNAME}
            systemctl start ${PROJECTNAME}

            echo -e "${GREEN}✓ Service installed and started${NC}"
            echo "Commands:"
            echo "  sudo systemctl status ${PROJECTNAME}"
            echo "  sudo systemctl stop ${PROJECTNAME}"
            echo "  sudo systemctl restart ${PROJECTNAME}"
            echo "  sudo journalctl -u ${PROJECTNAME} -f"
        else
            # User systemd service
            mkdir -p ~/.config/systemd/user
            cat > ~/.config/systemd/user/${PROJECTNAME}.service << EOF
[Unit]
Description=Weather API Service
After=network.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/${PROJECTNAME}
Restart=always
RestartSec=5
Environment="CONFIG_DIR=${CONFIG_DIR}"
Environment="DATA_DIR=${DATA_DIR}"
Environment="LOG_DIR=${LOG_DIR}"

[Install]
WantedBy=default.target
EOF

            systemctl --user daemon-reload
            systemctl --user enable ${PROJECTNAME}
            systemctl --user start ${PROJECTNAME}

            echo -e "${GREEN}✓ User service installed and started${NC}"
            echo "Commands:"
            echo "  systemctl --user status ${PROJECTNAME}"
            echo "  systemctl --user stop ${PROJECTNAME}"
            echo "  systemctl --user restart ${PROJECTNAME}"
            echo "  journalctl --user -u ${PROJECTNAME} -f"
        fi
        ;;

    openrc)
        if [ "$IS_ROOT" = true ]; then
            cat > /etc/init.d/${PROJECTNAME} << EOF
#!/sbin/openrc-run

name="Weather Service"
command="${BIN_DIR}/${PROJECTNAME}"
command_background=true
pidfile="/run/${PROJECTNAME}.pid"

depend() {
    need net
    after firewall
}

start_pre() {
    export CONFIG_DIR="${CONFIG_DIR}"
    export DATA_DIR="${DATA_DIR}"
    export LOG_DIR="${LOG_DIR}"
}
EOF

            chmod +x /etc/init.d/${PROJECTNAME}
            rc-update add ${PROJECTNAME} default
            rc-service ${PROJECTNAME} start

            echo -e "${GREEN}✓ OpenRC service installed and started${NC}"
            echo "Commands:"
            echo "  rc-service ${PROJECTNAME} status"
            echo "  rc-service ${PROJECTNAME} stop"
            echo "  rc-service ${PROJECTNAME} restart"
        fi
        ;;

    sysvinit)
        if [ "$IS_ROOT" = true ]; then
            cat > /etc/init.d/${PROJECTNAME} << EOF
#!/bin/sh
### BEGIN INIT INFO
# Provides:          ${PROJECTNAME}
# Required-Start:    \$network \$remote_fs
# Required-Stop:     \$network \$remote_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Weather API Service
### END INIT INFO

DAEMON=${BIN_DIR}/${PROJECTNAME}
PIDFILE=/var/run/${PROJECTNAME}.pid

export CONFIG_DIR="${CONFIG_DIR}"
export DATA_DIR="${DATA_DIR}"
export LOG_DIR="${LOG_DIR}"

case "\$1" in
    start)
        echo "Starting ${PROJECTNAME}..."
        start-stop-daemon --start --background --make-pidfile --pidfile \$PIDFILE --exec \$DAEMON
        ;;
    stop)
        echo "Stopping ${PROJECTNAME}..."
        start-stop-daemon --stop --pidfile \$PIDFILE
        rm -f \$PIDFILE
        ;;
    restart)
        \$0 stop
        sleep 2
        \$0 start
        ;;
    status)
        if [ -f \$PIDFILE ]; then
            echo "${PROJECTNAME} is running (PID: \$(cat \$PIDFILE))"
        else
            echo "${PROJECTNAME} is not running"
        fi
        ;;
    *)
        echo "Usage: \$0 {start|stop|restart|status}"
        exit 1
        ;;
esac
EOF

            chmod +x /etc/init.d/${PROJECTNAME}
            update-rc.d ${PROJECTNAME} defaults 2>/dev/null || true
            service ${PROJECTNAME} start

            echo -e "${GREEN}✓ SysVinit service installed and started${NC}"
            echo "Commands:"
            echo "  service ${PROJECTNAME} status"
            echo "  service ${PROJECTNAME} stop"
            echo "  service ${PROJECTNAME} restart"
        fi
        ;;

    runit)
        if [ "$IS_ROOT" = true ]; then
            mkdir -p /etc/sv/${PROJECTNAME}
            cat > /etc/sv/${PROJECTNAME}/run << EOF
#!/bin/sh
exec 2>&1
export CONFIG_DIR="${CONFIG_DIR}"
export DATA_DIR="${DATA_DIR}"
export LOG_DIR="${LOG_DIR}"
exec ${BIN_DIR}/${PROJECTNAME}
EOF

            chmod +x /etc/sv/${PROJECTNAME}/run
            ln -sf /etc/sv/${PROJECTNAME} /var/service/

            echo -e "${GREEN}✓ runit service installed and started${NC}"
            echo "Commands:"
            echo "  sv status ${PROJECTNAME}"
            echo "  sv stop ${PROJECTNAME}"
            echo "  sv restart ${PROJECTNAME}"
        fi
        ;;

    unknown)
        echo -e "${YELLOW}⚠️  Unknown init system${NC}"
        echo "Binary installed to: ${BIN_DIR}/${PROJECTNAME}"
        echo "Run manually: ${BIN_DIR}/${PROJECTNAME}"
        ;;
esac

# Print summary
echo -e "\n${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}✅ Installation Complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo
echo "Binary:  ${BIN_DIR}/${PROJECTNAME}"
echo "Config:  ${CONFIG_DIR}"
echo "Data:    ${DATA_DIR}"
echo "Logs:    ${LOG_DIR}"
echo
echo "To access the service:"
echo "  http://localhost"
echo
echo "For more information:"
echo "  ${PROJECTNAME} --help"
echo "  ${PROJECTNAME} --version"
echo
