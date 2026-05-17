#!/usr/bin/env bash
# Weather Service Installer
# Detects OS and architecture, downloads the appropriate binary

set -e

VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
REPO="casapps/wthr"
BINARY_NAME="wthr"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🌤️  Weather Service Installer"
echo ""

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_TYPE=linux;;
    Darwin*)    OS_TYPE=darwin;;
    CYGWIN*)    OS_TYPE=windows;;
    MINGW*)     OS_TYPE=windows;;
    *)          echo -e "${RED}❌ Unsupported operating system: ${OS}${NC}"; exit 1;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)     ARCH_TYPE=amd64;;
    amd64)      ARCH_TYPE=amd64;;
    arm64)      ARCH_TYPE=arm64;;
    aarch64)    ARCH_TYPE=arm64;;
    *)          echo -e "${RED}❌ Unsupported architecture: ${ARCH}${NC}"; exit 1;;
esac

echo -e "${GREEN}✓${NC} Detected: ${OS_TYPE}/${ARCH_TYPE}"

# Construct binary name
BINARY_FILE="${BINARY_NAME}-${OS_TYPE}-${ARCH_TYPE}"
if [ "${OS_TYPE}" = "windows" ]; then
    BINARY_FILE="${BINARY_FILE}.exe"
fi

# Get latest release version if not specified
if [ "${VERSION}" = "latest" ]; then
    echo "🔍 Fetching latest version..."
    VERSION=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "${VERSION}" ]; then
        echo -e "${RED}❌ Failed to fetch latest version${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}✓${NC} Version: ${VERSION}"

# Download URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILE}"

echo "📥 Downloading from: ${DOWNLOAD_URL}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

# Download binary
if ! curl -L -o "${TMP_DIR}/${BINARY_FILE}" "${DOWNLOAD_URL}"; then
    echo -e "${RED}❌ Download failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Downloaded successfully"

# Make executable
chmod +x "${TMP_DIR}/${BINARY_FILE}"

# Install
echo "📦 Installing to ${INSTALL_DIR}..."

if [ ! -w "${INSTALL_DIR}" ]; then
    echo -e "${YELLOW}⚠️  ${INSTALL_DIR} is not writable, using sudo${NC}"
    sudo mv "${TMP_DIR}/${BINARY_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    mv "${TMP_DIR}/${BINARY_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo -e "${GREEN}✅ Installation complete!${NC}"
echo ""
echo "Run: ${BINARY_NAME}"
echo "Or:  ${BINARY_NAME} --help"
