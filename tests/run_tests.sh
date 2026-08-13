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
# @@File             :  run_tests.sh
# @@Description      :  AI.md PART 29 test runner - detects Incus or Docker and dispatches to the right test script
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
set -euo pipefail

# Detect available container runtime and run appropriate test
if command -v incus &>/dev/null; then
    echo "Incus detected - running full systemd tests..."
    exec "$(dirname "$0")/incus.sh"
elif command -v docker &>/dev/null; then
    echo "Docker detected - running container tests..."
    exec "$(dirname "$0")/docker.sh"
else
    echo "ERROR: Neither incus nor docker found"
    echo "Please install one of the following:"
    echo "  - Incus (preferred): https://linuxcontainers.org/incus/"
    echo "  - Docker (fallback): https://docker.com/"
    exit 1
fi

# ex: ts=2 sw=2 et filetype=sh
