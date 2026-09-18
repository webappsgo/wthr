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
# @@File             :  test-server.sh
# @@Description      :  Runs the wthr server locally with an isolated temp data directory for manual testing
# @@Changelog        :  Bring script into CasjaysDev header and lint compliance
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# Test script that runs the server with isolated temp directory

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
# No Color
NC='\033[0m'

PROJECTNAME=$(basename "$(cd -- "$(dirname -- "$0")/.." && pwd)")
PROJECTORG=$(basename "$(cd -- "$(dirname -- "$0")/../.." && pwd)")

# AI.md PART 29: runtime/test data always lives under {TMPDIR}/{project_org}/{internal_name}-XXXXXX
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")

echo -e "${BLUE}🧪 Weather Service Test Server${NC}"
echo -e "${BLUE}================================${NC}"
echo -e "Test directory: ${YELLOW}$TEST_DIR${NC}"
echo ""

# Cleanup function
__cleanup() {
    echo ""
    echo -e "${YELLOW}🧹 Cleaning up...${NC}"
    if [ -n "${SERVER_PID:-}" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    if [ "$KEEP_TEMP" != "1" ]; then
        rm -rf "$TEST_DIR"
        echo -e "${GREEN}✅ Temp directory removed${NC}"
    else
        echo -e "${YELLOW}📁 Temp directory kept: $TEST_DIR${NC}"
    fi
}

trap __cleanup EXIT INT TERM

PROJECT_ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)
BINARY="$PROJECT_ROOT/binaries/$PROJECTNAME"

# AI.md PART 29: the host has no Go toolchain, every build runs in casjaysdev/go:latest
if [ ! -x "$BINARY" ]; then
    echo -e "${BLUE}🔨 Building binary in Docker...${NC}"
    GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
    GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$PROJECTNAME}"
    mkdir -p "$GO_CACHE" "$GO_BUILD" "$PROJECT_ROOT/binaries"
    docker run --rm \
        --name "${PROJECTNAME}-testserver-$$" \
        -v "$PROJECT_ROOT:/build" \
        -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
        -v "$GO_BUILD:/usr/local/share/go/cache" \
        -w /build \
        -e CGO_ENABLED=0 \
        -e GOFLAGS=-buildvcs=false \
        casjaysdev/go:latest \
        go build -trimpath -ldflags '-s -w' -o "binaries/$PROJECTNAME" ./src || {
        echo -e "${YELLOW}❌ Build failed${NC}"
        exit 1
    }
fi

# Start server with temp directory
echo -e "${BLUE}🚀 Starting server...${NC}"
PORT="${PORT:-3053}"
"$BINARY" \
    --port "$PORT" \
    --data "$TEST_DIR" \
    > "$TEST_DIR/server.log" 2>&1 &

SERVER_PID=$!
echo -e "Server PID: ${YELLOW}$SERVER_PID${NC}"

# Wait for server to start
echo -e "${BLUE}⏳ Waiting for server...${NC}"
for i in {1..30}; do
    if curl -s "http://localhost:$PORT/server/healthz" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Server is ready!${NC}"
        break
    fi
    sleep 0.5
done

# Display information
echo ""
echo -e "${GREEN}🌤️  Server running at: ${YELLOW}http://localhost:$PORT${NC}"
echo -e "${GREEN}📊 Health check: ${YELLOW}http://localhost:$PORT/server/healthz${NC}"
echo -e "${GREEN}📝 API docs: ${YELLOW}http://localhost:$PORT/server/docs/swagger${NC}"
echo -e "${GREEN}📁 Data directory: ${YELLOW}$TEST_DIR${NC}"
echo -e "${GREEN}📋 Server log: ${YELLOW}$TEST_DIR/server.log${NC}"
echo ""
echo -e "${BLUE}Press Ctrl+C to stop the server${NC}"
echo ""

# Follow logs
tail -f "$TEST_DIR/server.log"

# ex: ts=2 sw=2 et filetype=sh
