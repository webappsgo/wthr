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
# AI.md PART 29: Docker is the basic binary-validation tier; Incus is standalone
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: docker not found. Install docker or use tests/incus.sh"
    exit 1
fi

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
INTERNAL_NAME="${INTERNAL_NAME:-$PROJECTNAME}"
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)

new_container_name() {
  local suffix
  suffix=$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')
  printf '%s-%s\n' "$PROJECTNAME" "$suffix"
}

mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${INTERNAL_NAME}-XXXXXX")
BUILD_DIR=$(cd -- "$BUILD_DIR" && pwd)
BUILD_CONTAINER=$(new_container_name)
TEST_CONTAINER=$(new_container_name)
cleanup() {
  local status=$?
  docker rm -f "$BUILD_CONTAINER" "$TEST_CONTAINER" >/dev/null 2>&1 || true
  if [ "$status" -eq 0 ]; then
    rm -rf "$BUILD_DIR"
  else
    printf 'Test artifacts retained at %s\n' "$BUILD_DIR" >&2
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$INTERNAL_NAME}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "=== Building server and CLI binaries through the Makefile ==="
mkdir -p "$BUILD_DIR/binaries" "$BUILD_DIR/releases"
timeout -k 30 "${DOCKER_TIMEOUT:-3600}" make \
  BINDIR="$BUILD_DIR/binaries" \
  RELDIR="$BUILD_DIR/releases" \
  GO_CACHE="$GO_CACHE" \
  GO_BUILD="$GO_BUILD" \
  GO_DOCKER_RUN="timeout -k 30 ${DOCKER_TIMEOUT:-3600} docker run --rm --no-healthcheck --memory=4g --cpus=2 --cpu-shares=512 --name $BUILD_CONTAINER -v $PWD:/app -v $BUILD_DIR:$BUILD_DIR -v $GO_CACHE:/usr/local/share/go/pkg/mod -v $GO_BUILD:/usr/local/share/go/cache -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false" \
  build

cp "$SCRIPT_DIR/lib/matrix.sh" "$BUILD_DIR/binaries/matrix.sh"
chmod +x "$BUILD_DIR/binaries/matrix.sh"
mkdir -p "$BUILD_DIR/logs"

echo "=== Running route/header matrix in alpine:latest ==="
timeout -k 30 "${DOCKER_TIMEOUT:-3600}" docker run --rm --no-healthcheck --memory=4g --cpus=2 --cpu-shares=512 \
  --name "$TEST_CONTAINER" \
  -v "$BUILD_DIR/binaries:/artifacts:ro" \
  -v "$BUILD_DIR/logs:/matrix-logs" \
  -e "PROJECTNAME=$PROJECTNAME" \
  -e "PROJECTORG=$PROJECTORG" \
  alpine:latest sh -c '
    set -e
    apk add --no-cache curl bash file jq >/dev/null
    cp "/artifacts/$PROJECTNAME" /usr/local/bin/
    cp "/artifacts/${PROJECTNAME}-cli-linux-amd64" "/usr/local/bin/${PROJECTNAME}-cli"
    if [ -f "/artifacts/${PROJECTNAME}-agent" ]; then
      cp "/artifacts/${PROJECTNAME}-agent" /usr/local/bin/
    fi
    cp /artifacts/matrix.sh /usr/local/bin/matrix.sh
    chmod +x /usr/local/bin/matrix.sh "/usr/local/bin/$PROJECTNAME" "/usr/local/bin/${PROJECTNAME}-cli"
    exec /usr/local/bin/matrix.sh "$PROJECTNAME" "$PROJECTORG"
  '

echo "Docker tests completed successfully"

# ex: ts=2 sw=2 et filetype=sh
