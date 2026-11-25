#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202611241200-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  run_tests.sh
# @@Description      :  AI.md PART 29 test runner for the Docker basic tier
# @@Changelog        :  Run the Docker basic tier only; Incus and E2E remain explicit
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
    echo "ERROR: docker not found. Install Docker to run the basic binary-validation suite." >&2
    exit 69
fi

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec "$SCRIPT_DIR/docker.sh" "$@"

# ex: ts=2 sw=2 et filetype=sh
