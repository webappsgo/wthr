#!/usr/bin/env bash
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

echo "=== Building server binary in Docker ==="
docker run --rm \
  -v "$(pwd):/build" \
  -v "$BUILD_DIR:/output" \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine go build -o "/output/$PROJECTNAME" ./src

echo "=== Launching Incus container ==="
incus launch images:debian/trixie "$CONTAINER_NAME"
sleep 5

echo "=== Installing container dependencies ==="
incus exec "$CONTAINER_NAME" -- bash -lc "apt-get update -qq && apt-get install -y -qq bash curl file jq ca-certificates procps >/dev/null"

echo "=== Copying binary to container ==="
incus file push "$BUILD_DIR/$PROJECTNAME" "$CONTAINER_NAME/usr/local/bin/"
incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/$PROJECTNAME"

echo "=== Running route/header matrix in Incus ==="
incus exec "$CONTAINER_NAME" -- bash -s -- "$PROJECTNAME" <<'EOF'
set -euo pipefail

PROJECTNAME="$1"
BASE_URL="http://127.0.0.1:80"
FAILURES=0
mkdir -p /tmp/webappsgo
TEST_DIR=$(mktemp -d /tmp/webappsgo/wthr-XXXXXX)
SERVER_PID=""
SETUP_COOKIE="$TEST_DIR/setup.cookies"
USER_COOKIE="$TEST_DIR/user.cookies"
ADMIN_TOKEN=""

cleanup() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
        kill "$SERVER_PID" >/dev/null 2>&1 || true
        wait "$SERVER_PID" >/dev/null 2>&1 || true
    fi
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

log() {
    printf '\n=== %s ===\n' "$1"
}

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    FAILURES=$((FAILURES + 1))
}

cookie_value() {
    local cookie_file="$1"
    local cookie_name="$2"
    awk -v name="$cookie_name" '($0 !~ /^#/ || $0 ~ /^#HttpOnly_/) && $6 == name {print $7}' "$cookie_file" | tail -n1
}

http_code() {
    awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}' "$1"
}

content_type() {
    awk 'BEGIN{IGNORECASE=1} /^Content-Type:/ {print tolower($2); exit}' "$1" | tr -d '\r'
}

request() {
    local method="$1"
    local path="$2"
    local accept="$3"
    local expected_status="$4"
    local expected_ct="$5"
    local label="$6"
    shift 6

    local hdr="$TEST_DIR/headers.txt"
    local body="$TEST_DIR/body.txt"

    if ! curl -q -LSsf -X "$method" -D "$hdr" -o "$body" -H "Accept: $accept" "$@" "${BASE_URL}${path}" >/dev/null; then
        fail "$label :: curl transport failed"
        return
    fi

    local status
    status=$(http_code "$hdr")
    local ct
    ct=$(content_type "$hdr")

    if [ "$status" != "$expected_status" ]; then
        fail "$label :: expected status $expected_status, got $status"
        printf '  Path: %s\n' "$path" >&2
        printf '  Accept: %s\n' "$accept" >&2
        printf '  Content-Type: %s\n' "$ct" >&2
        head -c 200 "$body" >&2 || true
        printf '\n' >&2
        return
    fi

    case "$ct" in
        "$expected_ct"*) ;;
        *)
            fail "$label :: expected Content-Type $expected_ct, got $ct"
            printf '  Path: %s\n' "$path" >&2
            printf '  Accept: %s\n' "$accept" >&2
            head -c 200 "$body" >&2 || true
            printf '\n' >&2
            return
            ;;
    esac

    if [ ! -s "$body" ]; then
        fail "$label :: empty response body"
    fi
}

frontend_matrix() {
    local scope="$1"
    local cookie_file="${2:-}"
    shift $(( $# > 0 ? 2 : 1 )) || true
    local path
    for path in "$@"; do
        if [ -n "$cookie_file" ]; then
            request GET "$path" "text/html" "200" "text/html" "$scope html $path" -b "$cookie_file" -c "$cookie_file"
            request GET "$path" "text/plain" "200" "text/plain" "$scope text $path" -b "$cookie_file" -c "$cookie_file"
        else
            request GET "$path" "text/html" "200" "text/html" "$scope html $path"
            request GET "$path" "text/plain" "200" "text/plain" "$scope text $path"
        fi
    done
}

api_matrix() {
    local scope="$1"
    local cookie_file="${2:-}"
    shift $(( $# > 0 ? 2 : 1 )) || true
    local path
    for path in "$@"; do
        if [ -n "$cookie_file" ]; then
            request GET "$path" "application/json" "200" "application/json" "$scope json $path" -b "$cookie_file" -c "$cookie_file"
            request GET "$path" "text/plain" "200" "text/plain" "$scope text $path" -b "$cookie_file" -c "$cookie_file"
        else
            request GET "$path" "application/json" "200" "application/json" "$scope json $path"
            request GET "$path" "text/plain" "200" "text/plain" "$scope text $path"
        fi
    done
}

bootstrap_server() {
    mkdir -p \
        "$TEST_DIR/rootfs/config" \
        "$TEST_DIR/rootfs/data" \
        "$TEST_DIR/rootfs/logs" \
        "$TEST_DIR/rootfs/cache" \
        "$TEST_DIR/rootfs/backup"

    COLUMNS=120 "/usr/local/bin/$PROJECTNAME" \
        --mode development \
        --config "$TEST_DIR/rootfs/config" \
        --data "$TEST_DIR/rootfs/data" \
        --log "$TEST_DIR/rootfs/logs" \
        --cache "$TEST_DIR/rootfs/cache" \
        --backup "$TEST_DIR/rootfs/backup" \
        --address 127.0.0.1 \
        --port 80 \
        >"$TEST_DIR/server.log" 2>&1 &
    SERVER_PID=$!

    for _ in $(seq 1 60); do
        if [ "$(curl -q -LSs -o /dev/null -w '%{http_code}' "$BASE_URL/healthz" || true)" = "200" ]; then
            return 0
        fi
        sleep 2
    done

    echo "Server failed to start" >&2
    tail -n 200 "$TEST_DIR/server.log" >&2 || true
    exit 1
}

extract_setup_token() {
    local token=""
    for _ in $(seq 1 30); do
        token=$(grep -Eo '[a-f0-9]{32}' "$TEST_DIR/server.log" | head -n1 || true)
        if [ -n "$token" ]; then
            printf '%s\n' "$token"
            return 0
        fi
        sleep 1
    done
    return 1
}

create_admin() {
    local setup_token="$1"
    local hdr="$TEST_DIR/admin-setup.headers"
    local body="$TEST_DIR/admin-setup.body"

    curl -q -LSsf -L -c "$SETUP_COOKIE" -b "$SETUP_COOKIE" \
        --data-urlencode "setup_token=$setup_token" \
        "$BASE_URL/admin/verify-token" >/dev/null

    curl -q -LSsf -D "$hdr" -o "$body" -c "$SETUP_COOKIE" -b "$SETUP_COOKIE" \
        -H "Accept: application/json" \
        --data-urlencode "username=primaryadmin" \
        --data-urlencode "email=admin@example.com" \
        --data-urlencode "password=VeryStrongPassword123!" \
        --data-urlencode "confirm_password=VeryStrongPassword123!" \
        "$BASE_URL/admin/server/setup"

    if [ "$(http_code "$hdr")" != "200" ]; then
        fail "admin bootstrap :: expected status 200, got $(http_code "$hdr")"
        head -c 200 "$body" >&2 || true
        printf '\n' >&2
        return
    fi

    ADMIN_TOKEN=$(jq -r '.api_token // empty' "$body")

    if [ -z "$(cookie_value "$SETUP_COOKIE" "admin_session")" ]; then
        fail "admin bootstrap :: admin_session cookie missing"
    fi
    if [ -z "$ADMIN_TOKEN" ]; then
        fail "admin bootstrap :: admin API token missing"
    fi
}

admin_api_matrix() {
    local path
    for path in "$@"; do
        request GET "$path" "application/json" "200" "application/json" "admin api json $path" -H "Authorization: Bearer $ADMIN_TOKEN"
        request GET "$path" "text/plain" "200" "text/plain" "admin api text $path" -H "Authorization: Bearer $ADMIN_TOKEN"
    done
}

create_user() {
    curl -q -LSsf -L -c "$USER_COOKIE" -b "$USER_COOKIE" \
        --data-urlencode "username=matrixuser" \
        --data-urlencode "email=matrixuser@example.com" \
        --data-urlencode "password=MatrixPassword123!" \
        --data-urlencode "confirm_password=MatrixPassword123!" \
        "$BASE_URL/auth/register" >/dev/null

    if [ -z "$(cookie_value "$USER_COOKIE" "weather_session")" ]; then
        fail "user bootstrap :: weather_session cookie missing"
    fi
}

check_unauth_protection() {
    local hdr="$TEST_DIR/protected.headers"
    local body="$TEST_DIR/protected.body"

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: text/html" "$BASE_URL/users" >/dev/null
    if [ "$(http_code "$hdr")" != "302" ]; then
        fail "unauth html /users :: expected 302 redirect"
    fi

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: application/json" "$BASE_URL/api/v1/users" >/dev/null
    if [ "$(http_code "$hdr")" != "401" ]; then
        fail "unauth api /api/v1/users :: expected 401"
    fi

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: application/json" "$BASE_URL/api/v1/admin/server/users" >/dev/null
    if [ "$(http_code "$hdr")" != "401" ]; then
        fail "unauth api /api/v1/admin/server/users :: expected 401"
    fi
}

PUBLIC_FRONTEND_ROUTES=(
    "/"
    "/health"
    "/healthz"
    "/auth/login"
    "/auth/register"
    "/auth/password/forgot"
    "/auth/password/reset"
    "/auth/2fa"
    "/auth/passkey"
    "/auth/username/forgot"
    "/auth/recovery/use"
    "/docs"
    "/server/about"
    "/server/privacy"
    "/server/contact"
    "/server/help"
    "/server/terms"
    "/examples"
    "/web"
    "/moon"
    "/earthquakes"
    "/hurricanes"
    "/severe-weather"
    "/weather/London"
    "/London"
)

PUBLIC_API_ROUTES=(
    "/api/v1"
    "/api/v1/healthz"
    "/api/v1/blocklist"
    "/api/v1/server/about"
    "/api/v1/server/privacy"
    "/api/v1/server/help"
    "/api/v1/server/terms"
    "/api/v1/weather?location=London"
    "/api/v1/weather/London"
    "/api/v1/weather/forecast?location=London"
    "/api/v1/forecasts?location=London"
    "/api/v1/ip"
    "/api/v1/docs"
    "/api/v1/earthquakes"
    "/api/v1/hurricanes"
    "/api/v1/severe-weather?location=London"
    "/api/v1/moon?location=London"
    "/api/v1/moon/calendar?location=London&year=2026&month=4"
    "/api/v1/sun?location=London"
    "/api/v1/history?location=London&date=2024-04-01&years=1"
    "/api/v1/locations/search?q=London"
    "/api/v1/locations/lookup/zip/10001"
    "/api/v1/locations/lookup/coords?lat=40.7128&lon=-74.0060"
)

USER_FRONTEND_ROUTES=(
    "/users"
    "/users/dashboard"
    "/users/settings"
    "/users/settings/privacy"
    "/users/settings/notifications"
    "/users/settings/appearance"
    "/users/tokens"
    "/users/notifications"
    "/users/profile"
    "/users/security"
    "/users/preferences"
    "/locations/new"
)

USER_API_ROUTES=(
    "/api/v1/users"
    "/api/v1/users/settings"
    "/api/v1/users/tokens"
    "/api/v1/users/avatar"
    "/api/v1/users/security/2fa"
    "/api/v1/users/security/2fa/setup"
    "/api/v1/locations"
    "/api/v1/users/notifications"
    "/api/v1/users/notifications/unread"
    "/api/v1/users/notifications/count"
    "/api/v1/users/notifications/stats"
    "/api/v1/users/notifications/preferences"
)

ADMIN_FRONTEND_ROUTES=(
    "/admin"
    "/admin/dashboard"
    "/admin/server/settings"
    "/admin/server/web"
    "/admin/server/users"
    "/admin/notifications"
)

ADMIN_API_ROUTES=(
    "/api/v1/admin/server/setup"
    "/api/v1/admin/server/users"
    "/api/v1/admin/server/settings"
    "/api/v1/admin/server/settings/all"
    "/api/v1/admin/server/security/tokens"
    "/api/v1/admin/server/stats"
    "/api/v1/admin/server/email"
    "/api/v1/admin/server/branding"
    "/api/v1/admin/server/pages"
    "/api/v1/admin/server/web"
    "/api/v1/admin/server/status"
    "/api/v1/admin/server/health"
    "/api/v1/admin/server/scheduler"
    "/api/v1/admin/server/channels"
    "/api/v1/admin/profile"
    "/api/v1/admin/profile/preferences"
    "/api/v1/admin/server/admins"
    "/api/v1/admin/notifications"
    "/api/v1/admin/notifications/unread"
    "/api/v1/admin/notifications/count"
    "/api/v1/admin/notifications/stats"
    "/api/v1/admin/notifications/preferences"
    "/api/v1/admin/server/smtp/providers"
)

log "Version and binary checks"
"/usr/local/bin/$PROJECTNAME" --version
"/usr/local/bin/$PROJECTNAME" --help >/dev/null
file "/usr/local/bin/$PROJECTNAME"

log "Starting server in temp rootfs"
bootstrap_server

log "Extracting setup token"
SETUP_TOKEN=$(extract_setup_token || true)
if [ -z "$SETUP_TOKEN" ]; then
    echo "Failed to extract setup token from server log" >&2
    tail -n 200 "$TEST_DIR/server.log" >&2 || true
    exit 1
fi
printf 'Setup token extracted: %s\n' "$SETUP_TOKEN"

log "Checking unauthenticated protection"
check_unauth_protection

log "Public frontend matrix"
frontend_matrix "public frontend" "" "${PUBLIC_FRONTEND_ROUTES[@]}"

log "Public API matrix"
api_matrix "public api" "" "${PUBLIC_API_ROUTES[@]}"

log "Creating primary admin"
create_admin "$SETUP_TOKEN"

log "Creating regular user"
create_user

log "User frontend matrix"
frontend_matrix "user frontend" "$USER_COOKIE" "${USER_FRONTEND_ROUTES[@]}"

log "User API matrix"
api_matrix "user api" "$USER_COOKIE" "${USER_API_ROUTES[@]}"

log "Admin frontend matrix"
frontend_matrix "admin frontend" "$SETUP_COOKIE" "${ADMIN_FRONTEND_ROUTES[@]}"

log "Admin API matrix"
admin_api_matrix "${ADMIN_API_ROUTES[@]}"

log "Summary"
if [ "$FAILURES" -ne 0 ]; then
    printf 'Detected %d failures.\n' "$FAILURES" >&2
    exit 1
fi

printf 'All route/header checks passed.\n'
EOF

echo "Incus tests completed successfully"
