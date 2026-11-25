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
# @@Description      :  AI.md PART 29 full integration testing in Incus system containers, once per init system
# @@Changelog        :  Show matrix server logs on failure; let the last I2P test drop-in override the default
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# AI.md PART 29: Full integration testing in Incus system containers, once per init system
set -euo pipefail

if ! command -v incus >/dev/null 2>&1; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh"
    exit 1
fi

# Usage: tests/incus.sh [systemd|openrc|all]   (default: all)
# systemd runs in a Debian container, openrc in an Alpine container; each gets the full test run
MODE="${1:-all}"
case "$MODE" in
    systemd|openrc) MODES="$MODE" ;;
    all) MODES="systemd openrc" ;;
    *) echo "Usage: $0 [systemd|openrc|all]"; exit 2 ;;
esac

PROJECT_NAME=$(basename "$PWD")
PROJECT_ORG=$(basename "$(dirname "$PWD")")
INTERNAL_NAME="${INTERNAL_NAME:-$PROJECT_NAME}"
INTERNAL_ORG="${INTERNAL_ORG:-$PROJECT_ORG}"
CONTAINER_SUFFIX=$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')
CONTAINER_PREFIX="${PROJECT_NAME}-${CONTAINER_SUFFIX}"
CONTAINERS=()
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)

# Incus images, one per init system: systemd uses the latest Debian stable, OpenRC uses the
# latest stable Alpine release (update both when new stable releases ship)
INCUS_IMAGE_SYSTEMD="images:debian/trixie"
INCUS_IMAGE_OPENRC="images:alpine/latest_stable"

INCUS_CPUS="${INCUS_CPUS:-2}"
INCUS_MEM="${INCUS_MEM:-4GiB}"
INCUS_CPU_PRIORITY="${INCUS_CPU_PRIORITY:-2}"
INCUS_OP_TIMEOUT="${INCUS_OP_TIMEOUT:-60}"
INCUS_INSTALL_TIMEOUT="${INCUS_INSTALL_TIMEOUT:-900}"
INCUS_RUN_TIMEOUT="${INCUS_RUN_TIMEOUT:-3600}"
DOCKER_TIMEOUT="${DOCKER_TIMEOUT:-3600}"

mkdir -p "${TMPDIR:-/tmp}/${PROJECT_ORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECT_ORG}/${INTERNAL_NAME}-XXXXXX")
BUILD_DIR=$(cd -- "$BUILD_DIR" && pwd)
BUILD_CONTAINER="${PROJECT_NAME}-${CONTAINER_SUFFIX}"

cleanup() {
    local c
    for c in "${CONTAINERS[@]}"; do timeout -k 5 "$INCUS_OP_TIMEOUT" incus delete --force "$c" >/dev/null 2>&1 || true; done
    rm -rf "$BUILD_DIR"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/$INTERNAL_NAME}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "=== Building binaries through the Makefile in Docker ==="
mkdir -p "$BUILD_DIR/binaries" "$BUILD_DIR/releases"
timeout -k 30 "$DOCKER_TIMEOUT" make \
  BINDIR="$BUILD_DIR/binaries" \
  RELDIR="$BUILD_DIR/releases" \
  GO_CACHE="$GO_CACHE" \
  GO_BUILD="$GO_BUILD" \
  GO_DOCKER_RUN="timeout -k 30 $DOCKER_TIMEOUT docker run --rm --no-healthcheck --memory=4g --cpus=2 --cpu-shares=512 --name $BUILD_CONTAINER -v $PWD:/app -v $BUILD_DIR:$BUILD_DIR -v $GO_CACHE:/usr/local/share/go/pkg/mod -v $GO_BUILD:/usr/local/share/go/cache -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false" \
  build
cp "$BUILD_DIR/binaries/${PROJECT_NAME}-cli-linux-amd64" "$BUILD_DIR/binaries/${PROJECT_NAME}-cli"

BIN_DIR="$BUILD_DIR/binaries"

iexec() {
    local container="$1"
    shift
    timeout -k 5 "$INCUS_OP_TIMEOUT" incus exec "$container" -- "$@"
}

iexec_install() {
    local container="$1"
    shift
    timeout -k 30 "$INCUS_INSTALL_TIMEOUT" incus exec "$container" -- "$@"
}

iexec_run() {
    local container="$1"
    shift
    timeout -k 30 "$INCUS_RUN_TIMEOUT" incus exec "$container" -- "$@"
}

ipush() {
    timeout -k 5 "$INCUS_OP_TIMEOUT" incus file push "$@"
}

REMOTE_LIB=$(cat <<'REMOTE_LIB_EOF'
INTERNAL_NAME="${INTERNAL_NAME:-$PROJECT_NAME}"
INTERNAL_ORG="${INTERNAL_ORG:-$PROJECT_ORG}"
FAILURES=0
fail() { echo "✗ FAILED: $*"; FAILURES=$((FAILURES + 1)); }
pass() { echo "✓ $*"; }
check() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then pass "$d"; else fail "$d"; fi; }
check_not() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then fail "$d"; else pass "$d"; fi; }
wait_for() { local n="$1" i; shift; for i in $(seq 1 "$n"); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done; return 1; }

# The service listens on the port it saved in server.yml (random 64xxx on first run), never a fixed one
svc_port() { awk '$1 == "port:" {gsub(/"/, "", $2); print $2; exit}' "/etc/${INTERNAL_ORG}/${INTERNAL_NAME}/server.yml"; }
api_url() { echo "http://localhost:$(svc_port)/api/v1/server/healthz"; }
healthz() { curl -q -LSsf --max-time 10 -H 'Accept: application/json' "$(api_url)"; }
healthz_up() { healthz | jq -e '.status' >/dev/null; }
overlay() { healthz | jq -r ".features.$1.$2"; }

# One set of service helpers per init system; the test steps are identical for both
if [ "$INIT_MODE" = "systemd" ]; then
    UNIT_FILE="/etc/systemd/system/${INTERNAL_NAME}.service"
    svc_active()  { systemctl is-active --quiet "$INTERNAL_NAME"; }
    svc_enabled() { systemctl is-enabled --quiet "$INTERNAL_NAME"; }
    svc_pid()     { systemctl show -p MainPID --value "$INTERNAL_NAME"; }
else
    UNIT_FILE="/etc/init.d/${INTERNAL_NAME}"
    svc_active()  { rc-service "$INTERNAL_NAME" status 2>&1 | grep -q started; }
    svc_enabled() { rc-update show default | grep -qw "$INTERNAL_NAME"; }
    svc_pid()     { cat "/var/run/${INTERNAL_ORG}/${INTERNAL_NAME}/${INTERNAL_NAME}.pid"; }
fi
svc_stopped() { ! svc_active; }
REMOTE_LIB_EOF
)

REMOTE_MAIN=$(cat <<'REMOTE_MAIN_EOF'
echo "=== [$INIT_MODE] Init system identity ==="
if [ "$INIT_MODE" = "systemd" ]; then
    check "PID 1 is systemd" test "$(cat /proc/1/comm)" = "systemd"
else
    check "OpenRC is running" test -d /run/openrc
    check "no systemd in this container" test ! -d /run/systemd/system
fi

echo "=== Version Check ==="
${PROJECT_NAME} --version

echo "=== Help Check ==="
${PROJECT_NAME} --help

echo "=== Binary Info ==="
ls -lh /usr/local/bin/${PROJECT_NAME}
file /usr/local/bin/${PROJECT_NAME}

echo "=== [$INIT_MODE] Service install (--service --install also enables and starts) ==="
${PROJECT_NAME} --service --install
check "service file installed at ${UNIT_FILE}" test -f "$UNIT_FILE"
if [ "$INIT_MODE" = "openrc" ]; then
    check "init script starts with #!/sbin/openrc-run" sh -c "head -1 '$UNIT_FILE' | grep -q '^#!/sbin/openrc-run'"
    check "init script is executable" test -x "$UNIT_FILE"
    check_not "init script is not a SysV script" grep -q 'BEGIN INIT INFO' "$UNIT_FILE"
fi
check "service enabled at boot" svc_enabled
check "service active after install" wait_for 30 svc_active
check "health endpoint answers after install" wait_for 180 healthz_up

echo "=== [$INIT_MODE] Service restart ==="
OLD_PID=$(svc_pid)
${PROJECT_NAME} --service restart
check "service active after restart" wait_for 30 svc_active
check "health endpoint answers after restart" wait_for 180 healthz_up
check "restart replaced the process" test "$(svc_pid)" != "$OLD_PID"

echo "=== [$INIT_MODE] Service stop and start ==="
${PROJECT_NAME} --service stop
check "service stopped" wait_for 30 svc_stopped
check_not "health endpoint is down while stopped" curl -q -LSsf --max-time 3 "$(api_url)"
${PROJECT_NAME} --service start
check "service active after start" wait_for 30 svc_active
check "health endpoint answers after start" wait_for 180 healthz_up

echo "=== API Endpoint Tests ==="
curl -q -LSsf "$(api_url)" >/dev/null || fail "/api/v1/server/healthz"
curl -q -LSsf "$(api_url).txt" >/dev/null || fail "/api/v1/server/healthz.txt"
curl -q -LSsf -H 'Accept: application/json' "http://localhost:$(svc_port)/server/healthz" >/dev/null || fail "Accept JSON"
curl -q -LSsf -H 'Accept: text/plain' "http://localhost:$(svc_port)/server/healthz" >/dev/null || fail "Accept text/plain"

echo "=== Binary Rename Tests ==="
cp /usr/local/bin/${PROJECT_NAME} /tmp/renamed-server
chmod +x /tmp/renamed-server
if /tmp/renamed-server --help 2>&1 | grep -q 'renamed-server'; then pass "Server binary rename works (--help shows actual name)"; else fail "Server --help does not show renamed binary name"; fi

echo "=== Client Tests ==="
if [ -f /usr/local/bin/${PROJECT_NAME}-cli ]; then
    ${PROJECT_NAME}-cli --version || fail "CLI --version"
    ${PROJECT_NAME}-cli --help || fail "CLI --help"
    cp /usr/local/bin/${PROJECT_NAME}-cli /tmp/renamed-cli
    chmod +x /tmp/renamed-cli
    if /tmp/renamed-cli --help 2>&1 | grep -q 'renamed-cli'; then pass "CLI binary rename works"; else fail "CLI --help does not show renamed binary name"; fi
else
    echo 'client not installed - skipping'
fi

echo "=== [$INIT_MODE] Tor hidden service (tor is installed in this container) ==="
DATA_ROOT="/var/lib/${INTERNAL_ORG}/${INTERNAL_NAME}"
CONF_ROOT="/etc/${INTERNAL_ORG}/${INTERNAL_NAME}"
check "tor binary is installed" command -v tor
check "features.tor.enabled is true" test "$(overlay tor enabled)" = "true"
case "$(overlay tor status)" in
    disabled|starting|healthy|unavailable) pass "features.tor.status is one of the four contract words" ;;
    *) fail "features.tor.status is not one of the four contract words" ;;
esac
# Bootstrap needs outbound network. REQUIRE_OVERLAY=1 makes a missing network a failure instead of a SKIP.
for _ in $(seq 1 "${TOR_WAIT:-300}"); do [ "$(overlay tor status)" = "healthy" ] && break; sleep 1; done
if [ "$(overlay tor status)" = "healthy" ]; then
    if [[ "$(overlay tor hostname)" =~ ^[a-z2-7]{56}\.onion$ ]]; then pass "tor hostname is a v3 onion address"; else fail "tor hostname is not a v3 onion address"; fi
    check "features.tor.running is true" test "$(overlay tor running)" = "true"
    check "checks.tor is ok" test "$(healthz | jq -r '.checks.tor')" = "ok"
    check "onion hostname file persisted" test -s "${DATA_ROOT}/tor/site/hostname"
    for d in "${CONF_ROOT}/tor" "${DATA_ROOT}/tor" "${DATA_ROOT}/tor/site"; do
        check "$d is mode 700" test "$(stat -c %a "$d")" = "700"
    done
elif [ "${REQUIRE_OVERLAY:-0}" = "1" ]; then
    fail "Tor did not become healthy within ${TOR_WAIT:-300}s"
else
    echo "SKIP: Tor did not bootstrap within ${TOR_WAIT:-300}s (no outbound network?). The contract is still checked."
    case "$(overlay tor status)" in
        starting) check "starting maps to checks.tor ok" test "$(healthz | jq -r '.checks.tor')" = "ok" ;;
        unavailable) check "unavailable maps to checks.tor degraded" test "$(healthz | jq -r '.checks.tor')" = "degraded" ;;
        *) fail "Tor is neither starting nor unavailable while not healthy" ;;
    esac
fi
if healthz | grep -Eq '/(etc|var|home|config|data)/'; then fail "health response leaks a filesystem path"; else pass "health response carries no filesystem path"; fi

echo "=== [$INIT_MODE] Tor missing binary (must be disabled, never an error) ==="
TOR_BIN=$(readlink -f "$(command -v tor)")
mv "$TOR_BIN" "${TOR_BIN}.hidden"
${PROJECT_NAME} --service restart
wait_for 180 healthz_up
check "enabled is false without the binary" test "$(overlay tor enabled)" = "false"
check "running is false" test "$(overlay tor running)" = "false"
check "status is disabled" test "$(overlay tor status)" = "disabled"
check "hostname is empty" test -z "$(overlay tor hostname)"
check "checks.tor is omitted" test "$(healthz | jq '(.checks | has("tor"))')" = "false"
mv "${TOR_BIN}.hidden" "$TOR_BIN"
${PROJECT_NAME} --service restart
wait_for 180 healthz_up

echo "=== [$INIT_MODE] I2P (opt-in, i2pd is installed in this container) ==="
check "i2pd binary is installed" command -v i2pd
check "i2p is disabled by default" test "$(overlay i2p status)" = "disabled"
check "features.i2p.enabled is false by default" test "$(overlay i2p enabled)" = "false"
check "checks.i2p is omitted by default" test "$(healthz | jq '(.checks | has("i2p"))')" = "false"
# Opt in through a yaml.d drop-in (this also exercises the drop-in directory)
mkdir -p "${CONF_ROOT}/yaml.d"
printf 'server:\n  i2p:\n    enabled: true\n' > "${CONF_ROOT}/yaml.d/zz-i2p.yml"
chmod 0644 "${CONF_ROOT}/yaml.d/zz-i2p.yml"
${PROJECT_NAME} --service restart
wait_for 180 healthz_up
check "features.i2p.enabled is true after opt-in" test "$(overlay i2p enabled)" = "true"
check "features.i2p.provider is i2pd" test "$(overlay i2p provider)" = "i2pd"
case "$(overlay i2p status)" in
    starting|healthy|unavailable) pass "features.i2p.status is a contract word for an enabled network" ;;
    *) fail "features.i2p.status is not valid for an enabled network" ;;
esac
for _ in $(seq 1 "${I2P_WAIT:-600}"); do [ "$(overlay i2p status)" = "healthy" ] && break; sleep 1; done
if [ "$(overlay i2p status)" = "healthy" ]; then
    if [[ "$(overlay i2p hostname)" =~ ^[a-z2-7]{52}\.b32\.i2p$ ]]; then pass "i2p hostname is a .b32.i2p address"; else fail "i2p hostname is not a .b32.i2p address"; fi
    check "features.i2p.running is true" test "$(overlay i2p running)" = "true"
    check "checks.i2p is ok" test "$(healthz | jq -r '.checks.i2p')" = "ok"
    check "destination key persisted" sh -c "find '${DATA_ROOT}/i2p/site' -type f -size +0 | grep -q ."
elif [ "${REQUIRE_OVERLAY:-0}" = "1" ]; then
    fail "I2P did not become healthy within ${I2P_WAIT:-600}s"
else
    echo "SKIP: I2P did not build tunnels within ${I2P_WAIT:-600}s (no outbound network?). The contract is still checked."
fi
if healthz | grep -Eq '/(etc|var|home|config|data)/'; then fail "health response leaks a filesystem path"; else pass "health response carries no filesystem path"; fi

echo "=== [$INIT_MODE] I2P missing provider (opted in, no i2pd: disabled, never an error) ==="
I2PD_BIN=$(readlink -f "$(command -v i2pd)")
mv "$I2PD_BIN" "${I2PD_BIN}.hidden"
${PROJECT_NAME} --service restart
wait_for 180 healthz_up
check "enabled is false without a provider" test "$(overlay i2p enabled)" = "false"
check "status is disabled" test "$(overlay i2p status)" = "disabled"
check "provider is none" test "$(overlay i2p provider)" = "none"
check "checks.i2p is omitted" test "$(healthz | jq '(.checks | has("i2p"))')" = "false"
mv "${I2PD_BIN}.hidden" "$I2PD_BIN"
rm -f "${CONF_ROOT}/yaml.d/zz-i2p.yml"
${PROJECT_NAME} --service restart
wait_for 180 healthz_up

[ "$FAILURES" -eq 0 ] || { echo "$FAILURES check(s) failed"; exit 1; }
echo "=== [$INIT_MODE] Main tests passed ==="
REMOTE_MAIN_EOF
)

REMOTE_AFTER=$(cat <<'REMOTE_AFTER_EOF'
echo "=== [$INIT_MODE] Boot persistence (container was restarted) ==="
check "service still enabled after reboot" svc_enabled
check "service came back after reboot" wait_for 120 svc_active
check "health endpoint answers after reboot" wait_for 120 healthz_up

echo "=== [$INIT_MODE] Service disable (keeps the service file) ==="
${PROJECT_NAME} --service --disable
check_not "service not enabled after --disable" svc_enabled
check "service not active after --disable" wait_for 30 svc_stopped
check "service file kept after --disable" test -f "$UNIT_FILE"

echo "=== [$INIT_MODE] Service re-install ==="
${PROJECT_NAME} --service --install
check "service enabled again" svc_enabled
check "service active again" wait_for 30 svc_active
check "health endpoint answers again" wait_for 180 healthz_up

echo "=== [$INIT_MODE] Service uninstall (confirmation prompt answered with y) ==="
printf 'y\n' | ${PROJECT_NAME} --service --uninstall
check_not "service file removed" test -e "$UNIT_FILE"
check_not "service not enabled" svc_enabled
check "service not active" wait_for 30 svc_stopped
check_not "data directory removed" test -d "/var/lib/${INTERNAL_ORG}/${INTERNAL_NAME}"

[ "$FAILURES" -eq 0 ] || { echo "$FAILURES check(s) failed"; exit 1; }
echo "=== [$INIT_MODE] All tests passed ==="
REMOTE_AFTER_EOF
)

# Run a script inside the container with the init mode and project names
remote() {
    local container="$1" init="$2" script="$3"
    printf '%s\n' "$script" | timeout -k 30 "$INCUS_RUN_TIMEOUT" incus exec "$container" -- env INIT_MODE="$init" PROJECT_NAME="$PROJECT_NAME" PROJECT_ORG="$PROJECT_ORG" \
        INTERNAL_NAME="$INTERNAL_NAME" INTERNAL_ORG="$INTERNAL_ORG" REQUIRE_OVERLAY="${REQUIRE_OVERLAY:-0}" \
        TOR_WAIT="${TOR_WAIT:-300}" I2P_WAIT="${I2P_WAIT:-600}" bash -s
}

# Wait until the container accepts commands, its init system finished booting and the network is up
wait_boot() {
    local container="$1" init="$2" state
    for _ in $(seq 1 60); do timeout -k 2 10 incus exec "$container" -- true 2>/dev/null && break; sleep 1; done
    if [ "$init" = "systemd" ]; then
        # "degraded" is normal in a container; only "starting" means boot is unfinished
        for _ in $(seq 1 90); do
            state=$(timeout -k 2 10 incus exec "$container" -- systemctl is-system-running 2>/dev/null || true)
            [ -n "$state" ] && [ "$state" != "starting" ] && break
            sleep 1
        done
    else
        for _ in $(seq 1 60); do timeout -k 2 10 incus exec "$container" -- test -d /run/openrc 2>/dev/null && break; sleep 1; done
    fi
    for _ in $(seq 1 60); do timeout -k 2 10 incus exec "$container" -- sh -c 'ip route | grep -q "^default"' 2>/dev/null && break; sleep 1; done
}

run_mode() {
    local init="$1" image container
    if [ "$init" = "systemd" ]; then image="$INCUS_IMAGE_SYSTEMD"; else image="$INCUS_IMAGE_OPENRC"; fi
    container="${CONTAINER_PREFIX}-${init}"
    CONTAINERS+=("$container")

    echo "=== [$init] Launching Incus container ($image) ==="
    timeout -k 30 "$INCUS_INSTALL_TIMEOUT" incus launch "$image" "$container" \
        -c "limits.cpu=$INCUS_CPUS" \
        -c "limits.memory=$INCUS_MEM" \
        -c "limits.cpu.priority=$INCUS_CPU_PRIORITY"
    wait_boot "$container" "$init"

    echo "=== [$init] Installing test tools, tor and i2pd ==="
    if [ "$init" = "systemd" ]; then
        iexec_install "$container" bash -c 'apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl jq file tor i2pd' >/dev/null
        # The distro tor/i2pd services are NOT what is tested: the server runs its own dedicated, isolated instances
        iexec "$container" systemctl disable --now tor.service tor@default.service i2pd.service 2>/dev/null || true
    else
        iexec_install "$container" sh -c 'apk add --no-cache bash curl jq file tor i2pd' >/dev/null
        iexec "$container" sh -c 'rc-service tor stop; rc-service i2pd stop; rc-update del tor; rc-update del i2pd' 2>/dev/null || true
    fi

    echo "=== [$init] Copying binaries to container ==="
    ipush "${BIN_DIR}/${PROJECT_NAME}" "$container/usr/local/bin/"
    iexec "$container" chmod +x "/usr/local/bin/${PROJECT_NAME}"
    if [ -f "${BIN_DIR}/${PROJECT_NAME}-cli" ]; then
        ipush "${BIN_DIR}/${PROJECT_NAME}-cli" "$container/usr/local/bin/"
        iexec "$container" chmod +x "/usr/local/bin/${PROJECT_NAME}-cli"
    fi
    ipush "${SCRIPT_DIR}/lib/matrix.sh" "$container/usr/local/bin/matrix.sh"
    iexec "$container" chmod +x /usr/local/bin/matrix.sh

    echo "=== [$init] Running tests in Incus ==="
    remote "$container" "$init" "${REMOTE_LIB}"$'\n'"${REMOTE_MAIN}"

    echo "=== [$init] Restarting the container (boot persistence) ==="
    timeout -k 30 "$INCUS_OP_TIMEOUT" incus restart "$container"
    wait_boot "$container" "$init"
    remote "$container" "$init" "${REMOTE_LIB}"$'\n'"${REMOTE_AFTER}"

    # The shared route/header/auth matrix starts its own server on port 80, so it runs after the
    # service is uninstalled and nothing else holds the port
    echo "=== [$init] Running the route/header matrix ==="
    iexec "$container" mkdir -p /matrix-logs
    if ! iexec_run "$container" /usr/local/bin/matrix.sh "$PROJECT_NAME" "$PROJECT_ORG"; then
        echo "=== [$init] Route/header matrix server logs ===" >&2
        iexec "$container" sh -c 'for log_file in /matrix-logs/stdout-server.log /matrix-logs/app-server.log; do if [ -f "$log_file" ]; then echo "--- $log_file"; grep -n -C 4 -E "template .* failed|panic:" "$log_file" || tail -n 200 "$log_file"; fi; done' >&2 || true
        return 1
    fi
}

# One container per init system, run one after the other (never in parallel)
run_mode_list() {
    local init
    for init in $MODES; do
        run_mode "$init"
    done
}
run_mode_list

echo "Incus tests completed successfully ($MODES)"

# ex: ts=2 sw=2 et filetype=sh
