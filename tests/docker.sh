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
# @@File             :  docker.sh
# @@Description      :  AI.md PART 29 container testing in Docker with a route/header matrix
# @@Changelog        :  Run the shared tests/lib/matrix.sh suite so Docker and Incus stay identical
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# AI.md PART 29: Container testing in Docker; Incus (tests/incus.sh) is preferred when available
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: docker not found. Install docker or use tests/incus.sh"
    exit 1
fi

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
CONTAINER_NAME="test-${PROJECTNAME}-$$"
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)

mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
trap 'docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true; rm -rf "$BUILD_DIR"' EXIT

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$PROJECTNAME}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "=== Building server and CLI binaries in Docker ==="
docker run --rm \
  --name "${PROJECTNAME}-dockerbuild-$$" \
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

cp "$SCRIPT_DIR/lib/matrix.sh" "$BUILD_DIR/matrix.sh"
chmod +x "$BUILD_DIR/matrix.sh"

echo "=== Running route/header matrix in alpine:latest ==="
MATRIX_LOG_DIR="${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-matrix-logs"
mkdir -p "$MATRIX_LOG_DIR"
docker run --rm \
  --name "$CONTAINER_NAME" \
  -v "$BUILD_DIR:/artifacts:ro" \
  -v "$MATRIX_LOG_DIR:/matrix-logs" \
  -e "PROJECTNAME=$PROJECTNAME" \
  -e "PROJECTORG=$PROJECTORG" \
  alpine:latest sh -c '
    set -e
    apk add --no-cache curl bash file jq >/dev/null
    cp "/artifacts/$PROJECTNAME" "/artifacts/${PROJECTNAME}-cli" /usr/local/bin/
    if [ -f "/artifacts/${PROJECTNAME}-agent" ]; then
      cp "/artifacts/${PROJECTNAME}-agent" /usr/local/bin/
    fi
    cp /artifacts/matrix.sh /usr/local/bin/matrix.sh
    chmod +x /usr/local/bin/matrix.sh "/usr/local/bin/$PROJECTNAME" "/usr/local/bin/${PROJECTNAME}-cli"
    exec /usr/local/bin/matrix.sh "$PROJECTNAME" "$PROJECTORG"
  '

echo "Docker tests completed successfully"

# ex: ts=2 sw=2 et filetype=sh
