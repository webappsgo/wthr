#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202608211414-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  incus.sh
# @@Description      :  AI.md PART 29 full integration testing in Incus with a route/header matrix
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
# AI.md PART 29: Full integration testing in Incus with route/header matrix
set -euo pipefail

if ! command -v incus >/dev/null 2>&1; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh"
    exit 1
fi

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
CONTAINER_NAME="test-${PROJECTNAME}-$$"

mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
trap 'rm -rf "$BUILD_DIR"; incus delete "$CONTAINER_NAME" --force >/dev/null 2>&1 || true' EXIT

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$PROJECTNAME}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "=== Building server and CLI binaries in Docker ==="
docker run --rm \
  --name "${PROJECTNAME}-incusbuild-$$" \
  -v "$PWD:/build" \
  -v "$BUILD_DIR:/output" \
  -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
  -v "$GO_BUILD:/usr/local/share/go/cache" \
  -w /build \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest sh -c "
    set -e
    go build -trimpath -ldflags '-s -w' -o /output/$PROJECTNAME ./src
    go build -trimpath -ldflags '-s -w' -o /output/${PROJECTNAME}-cli ./src/client
    if [ -d ./src/agent ]; then
      go build -trimpath -ldflags '-s -w' -o /output/${PROJECTNAME}-agent ./src/agent
    fi
  "

echo "=== Launching Incus container ==="
incus launch images:debian/trixie "$CONTAINER_NAME"
sleep 5

echo "=== Installing container dependencies ==="
incus exec "$CONTAINER_NAME" -- bash -lc "apt-get update -qq && apt-get install -y -qq bash curl file jq ca-certificates procps >/dev/null"

echo "=== Copying binaries to container ==="
incus file push "$BUILD_DIR/$PROJECTNAME" "$CONTAINER_NAME/usr/local/bin/"
incus file push "$BUILD_DIR/${PROJECTNAME}-cli" "$CONTAINER_NAME/usr/local/bin/"
incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/$PROJECTNAME" "/usr/local/bin/${PROJECTNAME}-cli"

echo "=== Copying shared test matrix to container ==="
incus file push "$(dirname "$0")/lib/matrix.sh" "$CONTAINER_NAME/usr/local/bin/matrix.sh"
incus exec "$CONTAINER_NAME" -- chmod +x /usr/local/bin/matrix.sh

echo "=== Running route/header matrix in Incus ==="
incus exec "$CONTAINER_NAME" -- /usr/local/bin/matrix.sh "$PROJECTNAME" "$PROJECTORG"

echo "Incus tests completed successfully"

# ex: ts=2 sw=2 et filetype=sh
