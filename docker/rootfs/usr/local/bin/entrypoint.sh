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
# @@File             :  entrypoint.sh
# @@Description      :  Minimal container entrypoint that sets env defaults, handles signals, and execs the wthr binary
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
set -e

# =============================================================================
# Container Entrypoint Script - MINIMAL
# Only: set env, start services, start binary, handle signals
# Binary handles: directories, permissions, user/group, Tor, etc.
# =============================================================================

APP_NAME="wthr"
APP_BIN="/usr/local/bin/${APP_NAME}"

# Export environment defaults (binary reads these)
export TZ="${TZ:-America/New_York}"
export CONFIG_DIR="${CONFIG_DIR:-/config/${APP_NAME}}"
export DATA_DIR="${DATA_DIR:-/data/${APP_NAME}}"

# Track background PIDs for cleanup
declare -a PIDS=()

__log() { echo "[entrypoint] $(date '+%Y-%m-%d %H:%M:%S') $*"; }

# Signal handling for graceful shutdown
__cleanup() {
    __log "Shutdown signal received..."
    for ((i=${#PIDS[@]}-1; i>=0; i--)); do
        kill -TERM "${PIDS[i]}" 2>/dev/null || true
    done
    wait
    exit 0
}
trap __cleanup SIGTERM SIGINT SIGQUIT

# =============================================================================
# Start services (add supervisord, etc. here if needed)
# =============================================================================
# Example: Start supervisord for multi-service containers
# if [ -f /etc/supervisord.conf ]; then
#     /usr/bin/supervisord -c /etc/supervisord.conf &
#     PIDS+=($!)
# fi

# =============================================================================
# Start main application
# =============================================================================
__log "Starting ${APP_NAME}..."

# Build flags from environment
FLAGS="--address ${ADDRESS:-0.0.0.0} --port ${PORT:-80}"
[ "${DEBUG:-false}" = "true" ] && FLAGS="$FLAGS --debug"

# Start binary (binary handles ALL setup: dirs, perms, user/group, Tor, etc.)
exec "$APP_BIN" $FLAGS "$@"

# ex: ts=2 sw=2 et filetype=sh
