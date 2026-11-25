#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202611241200-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Tuesday, November 24, 2026 12:00 EST
# @@File             :  e2e.sh
# @@Description      :  AI.md PART 29 chromedp E2E suite for SSR, no-JS, and full-browser flows
# @@Changelog        :  Replace the Playwright layout pass with a hermetic chromedp test suite
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker not found. Install Docker to run the browser E2E suite." >&2
  exit 69
fi

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
INTERNAL_NAME="${INTERNAL_NAME:-$PROJECTNAME}"
E2E_TIMEOUT="${E2E_TIMEOUT:-3600}"
GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$INTERNAL_NAME}"
new_suffix() { od -An -N4 -tx1 /dev/urandom | tr -d ' \n'; }

mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}" "$GO_CACHE" "$GO_BUILD"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${INTERNAL_NAME}-XXXXXX")
BUILD_DIR=$(cd -- "$BUILD_DIR" && pwd)
chmod 0700 "$BUILD_DIR"
mkdir -p "$BUILD_DIR/artifacts" "$BUILD_DIR/test-tmp"
chmod 0700 "$BUILD_DIR/artifacts" "$BUILD_DIR/test-tmp"

NETWORK_NAME="${PROJECTNAME}-$(new_suffix)"
BROWSER_CONTAINER="${PROJECTNAME}-$(new_suffix)"
TEST_CONTAINER="${PROJECTNAME}-$(new_suffix)"
BROWSER_ALIAS="${PROJECTNAME}-headless"
TEST_ALIAS="${PROJECTNAME}-e2e"

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    timeout -k 5 30 docker logs "$BROWSER_CONTAINER" >&2 2>/dev/null || true
  fi
  timeout -k 5 60 docker rm -f "$BROWSER_CONTAINER" "$TEST_CONTAINER" >/dev/null 2>&1 || true
  timeout -k 5 60 docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
  if [ "$status" -eq 0 ]; then
    rm -rf "$BUILD_DIR"
  else
    printf 'E2E failure artifacts retained at %s\n' "$BUILD_DIR" >&2
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "=== Creating isolated Docker network ==="
timeout -k 5 60 docker network create --internal --driver bridge "$NETWORK_NAME" >/dev/null

echo "=== Starting headless Chromium sidecar ==="
timeout -k 30 "$E2E_TIMEOUT" docker run --detach --rm --no-healthcheck \
  --name "$BROWSER_CONTAINER" \
  --network "$NETWORK_NAME" \
  --network-alias "$BROWSER_ALIAS" \
  --memory=2g --cpus=2 --cpu-shares=512 --shm-size=1g \
  chromedp/headless-shell:latest \
  --no-sandbox --remote-debugging-address=0.0.0.0 --remote-debugging-port=9222

echo "=== Building the server and running all three E2E tiers in Docker ==="
timeout -k 30 "$E2E_TIMEOUT" docker run --rm --no-healthcheck \
  --name "$TEST_CONTAINER" \
  --network "$NETWORK_NAME" \
  --network-alias "$TEST_ALIAS" \
  --memory=4g --cpus=2 --cpu-shares=512 \
  -v "$PWD:/app:ro" \
  -v "$BUILD_DIR/artifacts:/tmp/e2e" \
  -v "$BUILD_DIR/test-tmp:/tmp/${PROJECTORG}" \
  -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
  -v "$GO_BUILD:/usr/local/share/go/cache" \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  -e "PROJECT_ORG=$PROJECTORG" \
  -e "INTERNAL_NAME=$INTERNAL_NAME" \
  -e "E2E_SERVER_BINARY=/tmp/e2e/$PROJECTNAME" \
  -e "E2E_SERVER_PORT=64127" \
  -e "E2E_BASE_URL=http://$TEST_ALIAS:64127" \
  -e "E2E_CHROME_WS_URL=ws://$BROWSER_ALIAS:9222" \
  casjaysdev/go:latest sh -c "set -e; go build -trimpath -ldflags '-s -w' -o /tmp/e2e/$PROJECTNAME ./src; go test -tags e2e -count=1 -timeout=50m ./tests/e2e/..."

echo "E2E suite completed successfully"
