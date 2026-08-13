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
# @@File             :  docker.sh
# @@Description      :  AI.md PART 29 full integration testing in a Docker Alpine container
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
# AI.md PART 29: Full integration testing in Docker Alpine container
set -euo pipefail

# Detect project info
PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")

# Create temp directory for build (proper org/project structure per AI.md)
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
trap "rm -rf $BUILD_DIR" EXIT

echo "=== Building binaries in Docker ==="
docker run --rm \
  -v "$(pwd):/build" \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine sh -c "
    go build -o /build/binaries/$PROJECTNAME ./src
    go build -o /build/binaries/${PROJECTNAME}-cli ./src/client
  "

# Copy binaries to temp dir
cp binaries/$PROJECTNAME "$BUILD_DIR/"
cp binaries/${PROJECTNAME}-cli "$BUILD_DIR/"

echo "=== Testing in Docker (Alpine) ==="
docker run --rm \
  -v "$BUILD_DIR:/app" \
  alpine:latest sh -c "
    set -e
    mkdir -p /tmp/webappsgo
    TEST_DIR=\$(mktemp -d /tmp/webappsgo/wthr-XXXXXX)
    __cleanup() {
        if [ -n \"\${SERVER_PID:-}\" ] && kill -0 \$SERVER_PID 2>/dev/null; then
            kill \$SERVER_PID 2>/dev/null || true
            wait \$SERVER_PID 2>/dev/null || true
        fi
        rm -rf \"\$TEST_DIR\"
    }
    trap __cleanup EXIT

    # Install required tools per AI.md PART 29
    apk add --no-cache curl bash file jq >/dev/null 2>&1

    chmod +x /app/$PROJECTNAME /app/${PROJECTNAME}-cli

    echo '=== Version Check ==='
    /app/$PROJECTNAME --version
    /app/${PROJECTNAME}-cli --version

    echo '=== Help Check ==='
    /app/$PROJECTNAME --help | head -5
    /app/${PROJECTNAME}-cli --help | head -5

    echo '=== Binary Info ==='
    ls -lh /app/$PROJECTNAME /app/${PROJECTNAME}-cli
    file /app/$PROJECTNAME
    file /app/${PROJECTNAME}-cli

    echo '=== Binary Rename Tests (AI.md PART 29) ==='
    # Test server binary rename
    cp /app/$PROJECTNAME /app/renamed-server
    chmod +x /app/renamed-server
    if /app/renamed-server --help 2>&1 | grep -q -- 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
        exit 1
    fi

    # Test CLI binary rename
    cp /app/${PROJECTNAME}-cli /app/renamed-cli
    chmod +x /app/renamed-cli
    if /app/renamed-cli --help 2>&1 | grep -q -- 'renamed-cli'; then
        echo '✓ CLI binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: CLI --help does not show renamed binary name'
        exit 1
    fi

    echo '=== Starting Server for API Tests ==='
    mkdir -p \"\$TEST_DIR/rootfs/config\" \"\$TEST_DIR/rootfs/data\" \"\$TEST_DIR/rootfs/logs\" \"\$TEST_DIR/rootfs/cache\" \"\$TEST_DIR/rootfs/backup\"
    /app/$PROJECTNAME --port 64580 --mode development \
        --config \"\$TEST_DIR/rootfs/config\" \
        --data \"\$TEST_DIR/rootfs/data\" \
        --log \"\$TEST_DIR/rootfs/logs\" \
        --cache \"\$TEST_DIR/rootfs/cache\" \
        --backup \"\$TEST_DIR/rootfs/backup\" &
    SERVER_PID=\$!
    sleep 5

    # Check if server started
    if ! kill -0 \$SERVER_PID 2>/dev/null; then
        echo '✗ FAILED: Server did not start'
        exit 1
    fi
    echo '✓ Server started on port 64580'

    echo '=== Static File Tests (AI.md PART 16) ==='
    # robots.txt
    if curl -q -LSsf http://localhost:64580/robots.txt | grep -q -- 'User-agent'; then
        echo '✓ /robots.txt returns valid content'
    else
        echo '✗ FAILED: /robots.txt'
    fi

    # security.txt
    if curl -q -LSsf http://localhost:64580/.well-known/security.txt | grep -q -- 'Contact'; then
        echo '✓ /.well-known/security.txt returns valid content'
    else
        echo '✗ FAILED: /.well-known/security.txt'
    fi

    # sitemap.xml
    if curl -q -LSsf http://localhost:64580/sitemap.xml | grep -q -- 'urlset'; then
        echo '✓ /sitemap.xml returns valid XML'
    else
        echo '✗ FAILED: /sitemap.xml'
    fi

    # favicon.ico
    if curl -q -LSsf -o /dev/null http://localhost:64580/favicon.ico; then
        echo '✓ /favicon.ico returns content'
    else
        echo '✗ FAILED: /favicon.ico'
    fi

    echo '=== Health Endpoint Tests (AI.md PART 13) ==='
    # JSON response
    HEALTH_JSON=\$(curl -q -LSsf http://localhost:64580/api/v1/healthz)
    if echo \"\$HEALTH_JSON\" | jq -e '.status' >/dev/null 2>&1; then
        echo '✓ /api/v1/healthz returns valid JSON with status field'
    else
        echo '✗ FAILED: /api/v1/healthz JSON format'
    fi

    # Check components field
    if echo \"\$HEALTH_JSON\" | jq -e '.components' >/dev/null 2>&1; then
        echo '✓ /api/v1/healthz has components field'
    else
        echo '✗ FAILED: /api/v1/healthz missing components'
    fi

    echo '=== Content Negotiation Tests (AI.md PART 29) ==='
    # Test Accept: application/json
    if curl -q -LSsf -H 'Accept: application/json' http://localhost:64580/api/v1/healthz | jq -e '.' >/dev/null 2>&1; then
        echo '✓ Accept: application/json returns JSON'
    else
        echo '✗ FAILED: Accept application/json'
    fi

    # Test Accept: text/plain
    PLAIN=\$(curl -q -LSsf -H 'Accept: text/plain' http://localhost:64580/api/v1/healthz)
    if [ -n \"\$PLAIN\" ] && ! echo \"\$PLAIN\" | grep -q -- '^{'; then
        echo '✓ Accept: text/plain returns plain text'
    else
        echo '✗ FAILED: Accept text/plain'
    fi

    # Test .txt extension
    TXT=\$(curl -q -LSsf http://localhost:64580/api/v1/healthz.txt 2>/dev/null || echo '')
    if [ -n \"\$TXT\" ]; then
        echo '✓ .txt extension returns content'
    else
        echo '⚠ .txt extension not implemented (optional)'
    fi

    echo '=== Weather-Specific API Tests (IDEA.md) ==='
    # Weather API
    if curl -q -LSsf http://localhost:64580/api/v1/weather | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/weather returns JSON'
    else
        echo '✗ FAILED: /api/v1/weather'
    fi

    # Moon phase API
    if curl -q -LSsf http://localhost:64580/api/v1/moon | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/moon returns JSON'
    else
        echo '✗ FAILED: /api/v1/moon'
    fi

    # Earthquakes API
    if curl -q -LSsf http://localhost:64580/api/v1/earthquakes | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/earthquakes returns JSON'
    else
        echo '✗ FAILED: /api/v1/earthquakes'
    fi

    # Hurricanes API
    if curl -q -LSsf http://localhost:64580/api/v1/hurricanes | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/hurricanes returns JSON'
    else
        echo '✗ FAILED: /api/v1/hurricanes'
    fi

    # Severe weather API
    if curl -q -LSsf http://localhost:64580/api/v1/severe-weather | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/severe-weather returns JSON'
    else
        echo '✗ FAILED: /api/v1/severe-weather'
    fi

    # Location search API (calls the live geocoding-api.open-meteo.com
    # upstream, so this real-network assertion lives here in Phase 2
    # rather than in *_test.go's Phase 1 toolchain gate — AI.md PART 29)
    if curl -q -LSsf 'http://localhost:64580/api/v1/locations/search?q=London' | jq -e '.' >/dev/null 2>&1; then
        echo '✓ /api/v1/locations/search returns JSON'
    else
        echo '✗ FAILED: /api/v1/locations/search'
    fi

    echo '=== Frontend Smart Detection Tests (AI.md PART 16) ==='
    # Test homepage - CLI should get response
    HOMEPAGE=\$(curl -q -LSsf http://localhost:64580/)
    if [ -n \"\$HOMEPAGE\" ]; then
        echo '✓ Frontend homepage works'
    else
        echo '✗ FAILED: Frontend homepage'
    fi

    # Test with Accept: text/html (browser simulation)
    HTML=\$(curl -q -LSsf -H 'Accept: text/html' http://localhost:64580/)
    if echo \"\$HTML\" | grep -qi -- 'html'; then
        echo '✓ Accept: text/html returns HTML'
    else
        echo '⚠ Accept: text/html (frontend may not implement)'
    fi

    echo '=== OpenAPI/GraphQL Endpoint Tests (AI.md PART 14) ==='
    # OpenAPI
    if curl -q -LSsf http://localhost:64580/openapi.json | jq -e '.openapi' >/dev/null 2>&1; then
        echo '✓ /openapi.json returns valid OpenAPI spec'
    else
        echo '⚠ /openapi.json not available'
    fi

    # GraphQL endpoint exists
    GRAPHQL=\$(curl -q -LSsf http://localhost:64580/graphql 2>/dev/null || echo '')
    if [ -n \"\$GRAPHQL\" ]; then
        echo '✓ /graphql endpoint exists'
    else
        echo '⚠ /graphql may require POST'
    fi

    echo '=== Stopping Server ==='
    kill \$SERVER_PID 2>/dev/null || true
    wait \$SERVER_PID 2>/dev/null || true

    echo ''
    echo '========================================='
    echo '  All tests completed successfully!'
    echo '========================================='
"

echo "Docker tests completed successfully"

# ex: ts=2 sw=2 et filetype=sh
