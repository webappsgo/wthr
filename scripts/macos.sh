#!/usr/bin/env bash
# Weather Service - macOS Installer with LaunchAgent

set -e

VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-$HOME/Library/Application Support/Weather}"
CONFIG_DIR="${CONFIG_DIR:-$HOME/Library/Application Support/Weather/config}"
REPO="webappsgo/wthr"
BINARY_NAME="weather"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🌤️  Weather Service - macOS Installer"
echo ""

# Detect architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)     ARCH_TYPE=amd64;;
    arm64)      ARCH_TYPE=arm64;;
    *)          echo -e "${RED}❌ Unsupported architecture: ${ARCH}${NC}"; exit 1;;
esac

echo -e "${GREEN}✓${NC} Detected: darwin/${ARCH_TYPE}"

# Get latest version
if [ "${VERSION}" = "latest" ]; then
    echo "🔍 Fetching latest version..."
    VERSION=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
fi

BINARY_FILE="${BINARY_NAME}-darwin-${ARCH_TYPE}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILE}"

echo "📥 Downloading ${VERSION}..."
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

curl -L -o "${TMP_DIR}/${BINARY_FILE}" "${DOWNLOAD_URL}"
chmod +x "${TMP_DIR}/${BINARY_FILE}"

# Install binary
echo "📦 Installing binary..."
if [ ! -w "${INSTALL_DIR}" ]; then
    echo -e "${YELLOW}⚠️  ${INSTALL_DIR} requires sudo${NC}"
    sudo mv "${TMP_DIR}/${BINARY_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    mv "${TMP_DIR}/${BINARY_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# Create data and config directories
echo "📁 Creating directories..."
mkdir -p "${DATA_DIR}/db"
mkdir -p "${DATA_DIR}/backups"
mkdir -p "${CONFIG_DIR}/certs"
mkdir -p "${CONFIG_DIR}/databases"
mkdir -p "$HOME/Library/Logs/weather"
mkdir -p "$HOME/Library/Caches/weather/weather"

# Create LaunchAgent
echo "⚙️  Creating LaunchAgent..."
LAUNCH_AGENT="$HOME/Library/LaunchAgents/io.github.webappsgo.wthr.plist"
mkdir -p "$HOME/Library/LaunchAgents"

cat > "${LAUNCH_AGENT}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.github.webappsgo.wthr</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PORT</key>
        <string>3000</string>
        <key>GIN_MODE</key>
        <string>release</string>
        <key>TZ</key>
        <string>UTC</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${DATA_DIR}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>${DATA_DIR}/stderr.log</string>
</dict>
</plist>
EOF

echo ""
echo -e "${GREEN}✅ Installation complete!${NC}"
echo ""
echo "Next steps:"
echo "  launchctl load ${LAUNCH_AGENT}     # Start service"
echo "  launchctl unload ${LAUNCH_AGENT}   # Stop service"
echo ""
echo "  tail -f '${DATA_DIR}/stdout.log'   # View logs"
echo ""
echo "Service will run on: http://localhost:3000"
