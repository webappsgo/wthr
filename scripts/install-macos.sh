#!/bin/bash
# install-macos.sh - macOS installer for Weather Service
# Installs as launchd service

set -e

PROJECTNAME="wthr"
GITHUB_REPO="webappsgo/wthr"
VERSION="latest"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Weather Service Installer for macOS ===${NC}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64" ;;
    arm64)   ARCH="arm64" ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo "Architecture: $ARCH ($([ "$ARCH" = "arm64" ] && echo "Apple Silicon" || echo "Intel"))"

# Detect if running with sudo/as root
if [ "$EUID" -eq 0 ]; then
    IS_ROOT=true
    BIN_DIR="/usr/local/bin"
    CONFIG_DIR="/Library/Application Support/Weather"
    DATA_DIR="/Library/Application Support/Weather/data"
    LOG_DIR="/Library/Logs/Weather"
    PLIST_DIR="/Library/LaunchDaemons"
    PLIST_NAME="io.github.webappsgo.wthr.plist"
else
    IS_ROOT=false
    BIN_DIR="$HOME/.local/bin"
    CONFIG_DIR="$HOME/Library/Application Support/Weather"
    DATA_DIR="$HOME/Library/Application Support/Weather/data"
    LOG_DIR="$HOME/Library/Logs/Weather"
    PLIST_DIR="$HOME/Library/LaunchAgents"
    PLIST_NAME="io.github.webappsgo.wthr.plist"
fi

echo "Install mode: $([ "$IS_ROOT" = true ] && echo "System (requires sudo)" || echo "User")"

# Create directories
mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
mkdir -p "$DATA_DIR/db"

# Download binary
echo "Downloading ${PROJECTNAME}-darwin-${ARCH}..."
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/${VERSION}/download/${PROJECTNAME}-darwin-${ARCH}"

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

# Create launchd plist
cat > "${PLIST_DIR}/${PLIST_NAME}" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.github.webappsgo.wthr</string>

    <key>ProgramArguments</key>
    <array>
        <string>${BIN_DIR}/${PROJECTNAME}</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>${LOG_DIR}/output.log</string>

    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/error.log</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PORT</key>
        <string>80</string>
        <key>CONFIG_DIR</key>
        <string>${CONFIG_DIR}</string>
        <key>DATA_DIR</key>
        <string>${DATA_DIR}</string>
        <key>LOG_DIR</key>
        <string>${LOG_DIR}</string>
    </dict>

    <key>WorkingDirectory</key>
    <string>${DATA_DIR}</string>
</dict>
</plist>
EOF

echo -e "${GREEN}✓ Launchd plist created${NC}"

# Load and start service
if [ "$IS_ROOT" = true ]; then
    launchctl load "${PLIST_DIR}/${PLIST_NAME}"
    echo -e "${GREEN}✓ Service loaded and started${NC}"
    echo
    echo "Commands:"
    echo "  sudo launchctl list | grep weather"
    echo "  sudo launchctl stop ${PLIST_NAME}"
    echo "  sudo launchctl start ${PLIST_NAME}"
    echo "  sudo launchctl unload ${PLIST_DIR}/${PLIST_NAME}"
    echo "  tail -f ${LOG_DIR}/output.log"
else
    launchctl load "${PLIST_DIR}/${PLIST_NAME}"
    echo -e "${GREEN}✓ Service loaded and started${NC}"
    echo
    echo "Commands:"
    echo "  launchctl list | grep weather"
    echo "  launchctl stop ${PLIST_NAME}"
    echo "  launchctl start ${PLIST_NAME}"
    echo "  launchctl unload ${PLIST_DIR}/${PLIST_NAME}"
    echo "  tail -f ${LOG_DIR}/output.log"
fi

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
