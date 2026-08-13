#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202608131924-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  linux.sh
# @@Description      :  Linux installer for the wthr weather service with a systemd service unit
# @@Changelog        :  Bring script into CasjaysDev header and lint compliance
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  yes
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# Weather Service - Linux Installer with systemd service

set -e

VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
SERVICE_USER="${SERVICE_USER:-wthr}"
DATA_DIR="${DATA_DIR:-/var/lib/webappsgo/wthr}"
CONFIG_DIR="${CONFIG_DIR:-/etc/webappsgo/wthr}"
REPO="webappsgo/wthr"
BINARY_NAME="wthr"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🌤️  Weather Service - Linux Installer"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ This script must be run as root${NC}"
    echo "Please run: sudo $0"
    exit 1
fi

# Detect architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)     ARCH_TYPE=amd64;;
    amd64)      ARCH_TYPE=amd64;;
    arm64)      ARCH_TYPE=arm64;;
    aarch64)    ARCH_TYPE=arm64;;
    *)          echo -e "${RED}❌ Unsupported architecture: ${ARCH}${NC}"; exit 1;;
esac

echo -e "${GREEN}✓${NC} Detected: linux/${ARCH_TYPE}"

# Get latest version
if [ "${VERSION}" = "latest" ]; then
    echo "🔍 Fetching latest version..."
    VERSION=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep -- '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
fi

BINARY_FILE="${BINARY_NAME}-linux-${ARCH_TYPE}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILE}"

echo "📥 Downloading ${VERSION}..."
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

curl -L -o "${TMP_DIR}/${BINARY_FILE}" "${DOWNLOAD_URL}"
chmod +x "${TMP_DIR}/${BINARY_FILE}"

# Install binary
echo "📦 Installing binary..."
mv "${TMP_DIR}/${BINARY_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"

# Create user
echo "👤 Creating service user..."
if ! id "${SERVICE_USER}" &>/dev/null; then
    useradd -r -s /bin/false -d "${DATA_DIR}" "${SERVICE_USER}"
fi

# Create directories
echo "📁 Creating directories..."
mkdir -p "${DATA_DIR}/db"
mkdir -p "${DATA_DIR}/backups"
mkdir -p "${CONFIG_DIR}/certs"
mkdir -p "${CONFIG_DIR}/databases"
mkdir -p "/var/log/webappsgo/wthr"
mkdir -p "/var/cache/webappsgo/wthr/weather"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "/var/log/webappsgo/wthr"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "/var/cache/webappsgo/wthr"

# Create systemd service
echo "⚙️  Creating systemd service..."
cat > /etc/systemd/system/wthr.service <<EOF
[Unit]
Description=Weather Service
After=network.target
Documentation=https://github.com/${REPO}

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
Environment="PORT=3000"
Environment="GIN_MODE=release"
Environment="TZ=UTC"
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=wthr

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${CONFIG_DIR} /var/log/webappsgo/wthr /var/cache/webappsgo/wthr
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictRealtime=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
echo "🔄 Reloading systemd..."
systemctl daemon-reload

echo ""
echo -e "${GREEN}✅ Installation complete!${NC}"
echo ""
echo "Next steps:"
echo "  sudo systemctl start wthr    # Start service"
echo "  sudo systemctl enable wthr   # Enable on boot"
echo "  sudo systemctl status wthr   # Check status"
echo ""
echo "  journalctl -u wthr -f        # View logs"
echo ""
echo "Service will run on: http://localhost:3000"

# ex: ts=2 sw=2 et filetype=sh
