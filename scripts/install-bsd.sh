#!/bin/sh
# shellcheck shell=sh
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202608131924-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  install-bsd.sh
# @@Description      :  BSD installer for the wthr weather service (FreeBSD/OpenBSD/NetBSD, rc.d)
# @@Changelog        :  Bring script into CasjaysDev header and lint compliance
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  yes
# @@Template         :  shell/sh
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# install-bsd.sh - BSD installer for Weather Service
# Supports: FreeBSD, OpenBSD, NetBSD with rc.d

PROJECTNAME="wthr"
GITHUB_REPO="webappsgo/wthr"
VERSION="latest"

echo "=== Weather Service Installer for BSD ==="

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    amd64|x86_64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Architecture: $ARCH"

# Detect BSD variant
BSD_VARIANT=$(uname -s)
echo "BSD variant: $BSD_VARIANT"

# Check if running as root
if [ "$(id -u)" -eq 0 ]; then
    IS_ROOT=true
    BIN_DIR="/usr/local/bin"
    CONFIG_DIR="/usr/local/etc/webappsgo/${PROJECTNAME}"
    DATA_DIR="/var/db/webappsgo/${PROJECTNAME}"
    LOG_DIR="/var/log/webappsgo/${PROJECTNAME}"
    RC_DIR="/usr/local/etc/rc.d"
else
    IS_ROOT=false
    BIN_DIR="$HOME/.local/bin"
    CONFIG_DIR="$HOME/.config/webappsgo/${PROJECTNAME}"
    DATA_DIR="$HOME/.local/share/webappsgo/${PROJECTNAME}"
    LOG_DIR="$HOME/.local/log/webappsgo/${PROJECTNAME}"
    RC_DIR=""
fi

echo "Install mode: $([ "$IS_ROOT" = "true" ] && echo "System (root)" || echo "User")"

# Create directories
mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
mkdir -p "$DATA_DIR/db"

# Download binary
echo "Downloading ${PROJECTNAME}-bsd-${ARCH}..."
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/${VERSION}/download/${PROJECTNAME}-bsd-${ARCH}"

if command -v fetch > /dev/null 2>&1; then
    fetch -o "${BIN_DIR}/${PROJECTNAME}" "$DOWNLOAD_URL"
elif command -v curl > /dev/null 2>&1; then
    curl -L -o "${BIN_DIR}/${PROJECTNAME}" "$DOWNLOAD_URL"
elif command -v wget > /dev/null 2>&1; then
    wget -O "${BIN_DIR}/${PROJECTNAME}" "$DOWNLOAD_URL"
else
    echo "Error: fetch, curl, or wget required"
    exit 1
fi

chmod +x "${BIN_DIR}/${PROJECTNAME}"
echo "✓ Binary installed to ${BIN_DIR}/${PROJECTNAME}"

# Install rc.d service (system only)
if [ "$IS_ROOT" = "true" ]; then
    cat > "${RC_DIR}/${PROJECTNAME}" << 'RCEOF'
#!/bin/sh
# PROVIDE: wthr
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="wthr"
rcvar="wthr_enable"
command="/usr/local/bin/wthr"
pidfile="/var/run/${name}.pid"
command_args="&"

export CONFIG_DIR="/usr/local/etc/webappsgo/wthr"
export DATA_DIR="/var/db/webappsgo/wthr"
export LOG_DIR="/var/log/webappsgo/wthr"

load_rc_config $name
: ${wthr_enable:="NO"}

run_rc_command "$1"
RCEOF

    chmod +x "${RC_DIR}/${PROJECTNAME}"

    # Add to rc.conf
    if ! grep -q -- "wthr_enable" /etc/rc.conf 2>/dev/null; then
        echo "wthr_enable=\"YES\"" >> /etc/rc.conf
    else
        # Use sysrc if available
        if command -v sysrc > /dev/null 2>&1; then
            sysrc wthr_enable="YES"
        fi
    fi

    # Start service
    service ${PROJECTNAME} start

    echo "✓ rc.d service installed and started"
    echo
    echo "Commands:"
    echo "  service ${PROJECTNAME} status"
    echo "  service ${PROJECTNAME} stop"
    echo "  service ${PROJECTNAME} restart"
    echo "  tail -f ${LOG_DIR}/${PROJECTNAME}.log"
else
    echo "✓ User installation complete (no service created)"
    echo "Run manually: ${BIN_DIR}/${PROJECTNAME}"
fi

# Print summary
echo
echo "════════════════════════════════════════"
echo "✅ Installation Complete!"
echo "════════════════════════════════════════"
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

# ex: ts=2 sw=2 et filetype=sh
