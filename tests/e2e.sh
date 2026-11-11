#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202611101800-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Tuesday, November 10, 2026 18:00 EST
# @@File             :  e2e.sh
# @@Description      :  AI.md PART 29 browser E2E and layout review matrix in headless Chromium
# @@Changelog        :  Build the server in Docker, run it beside Playwright, assert layout at every viewport
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# AI.md PART 29: headless Chromium review matrix (viewports x themes x direction x pages)
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: docker not found"
    exit 1
fi

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
CONTAINER_NAME="e2e-${PROJECTNAME}-$$"
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)

mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
trap 'docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true; rm -rf "$BUILD_DIR"' EXIT

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$PROJECTNAME}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "=== Building the server in Docker ==="
docker run --rm \
  --name "${PROJECTNAME}-e2ebuild-$$" \
  -v "$PWD:/build" \
  -v "$BUILD_DIR:/output" \
  -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
  -v "$GO_BUILD:/usr/local/share/go/cache" \
  -w /build \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest go build -trimpath -ldflags '-s -w' -o "/output/$PROJECTNAME" ./src

cp "$SCRIPT_DIR/e2e/layout.mjs" "$BUILD_DIR/layout.mjs"

echo "=== Running the layout review matrix in headless Chromium ==="
docker run --rm \
  --name "$CONTAINER_NAME" \
  --ipc=host \
  -v "$BUILD_DIR:/artifacts:ro" \
  -e "PROJECTNAME=$PROJECTNAME" \
  mcr.microsoft.com/playwright:latest bash -c '
    set -euo pipefail
    mkdir -p /tmp/e2e/config /tmp/e2e/data /tmp/e2e/log
    cp "/artifacts/$PROJECTNAME" /tmp/e2e/server
    chmod +x /tmp/e2e/server
    /tmp/e2e/server --config /tmp/e2e/config --data /tmp/e2e/data --log /tmp/e2e/log --port 64999 >/tmp/e2e/server.out 2>&1 &
    SERVER_PID=$!
    trap "kill $SERVER_PID 2>/dev/null || true" EXIT
    for _ in $(seq 1 60); do
      curl -q -LSsf -o /dev/null http://127.0.0.1:64999/server/healthz && break
      sleep 1
    done
    cd /tmp/e2e
    npm init -y >/dev/null 2>&1
    npm install --silent playwright >/dev/null 2>&1
    cp /artifacts/layout.mjs /tmp/e2e/layout.mjs
    BASE_URL=http://127.0.0.1:64999 node /tmp/e2e/layout.mjs
  '

echo "E2E layout matrix completed successfully"

# ex: ts=2 sw=2 et filetype=sh
