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
# @@File             :  matrix.sh
# @@Description      :  AI.md PART 29 in-container route/header/auth/CLI matrix shared by incus.sh and docker.sh
# @@Changelog        :  Add the PART 31 accessibility and language/direction matrices
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
# AI.md PART 29: runs INSIDE the test container; expects the server and CLI in /usr/local/bin
set -euo pipefail

PROJECTNAME="$1"
PROJECTORG="$2"
BASE_URL="http://127.0.0.1:80"
FAILURES=0
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
SERVER_PID=""
SETUP_COOKIE="$TEST_DIR/setup.cookies"
USER_COOKIE="$TEST_DIR/user.cookies"
ADMIN_TOKEN=""
ADMIN_USERNAME="primaryadmin"
ADMIN_PASSWORD="VeryStrongPassword123!"

__cleanup() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
        kill "$SERVER_PID" >/dev/null 2>&1 || true
        wait "$SERVER_PID" >/dev/null 2>&1 || true
    fi
    rm -rf "$TEST_DIR"
}
trap __cleanup EXIT

__log() {
    printf '\n=== %s ===\n' "$1"
}

__fail() {
    printf 'FAIL: %s\n' "$1" >&2
    FAILURES=$((FAILURES + 1))
}

__cookie_value() {
    local cookie_file="$1"
    local cookie_name="$2"
    awk -v name="$cookie_name" '($0 !~ /^#/ || $0 ~ /^#HttpOnly_/) && $6 == name {print $7}' "$cookie_file" | tail -n1
}

__http_code() {
    awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}' "$1"
}

__content_type() {
    awk 'BEGIN{IGNORECASE=1} /^Content-Type:/ {print tolower($2); exit}' "$1" | tr -d '\r'
}

__request() {
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
        __fail "$label :: curl transport failed"
        return
    fi

    local status
    status=$(__http_code "$hdr")
    local ct
    ct=$(__content_type "$hdr")

    if [ "$status" != "$expected_status" ]; then
        __fail "$label :: expected status $expected_status, got $status"
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
            __fail "$label :: expected Content-Type $expected_ct, got $ct"
            printf '  Path: %s\n' "$path" >&2
            printf '  Accept: %s\n' "$accept" >&2
            head -c 200 "$body" >&2 || true
            printf '\n' >&2
            return
            ;;
    esac

    if [ ! -s "$body" ]; then
        __fail "$label :: empty response body"
    fi
}

__frontend_matrix() {
    local scope="$1"
    local cookie_file="${2:-}"
    shift $(( $# > 0 ? 2 : 1 )) || true
    local path
    for path in "$@"; do
        if [ -n "$cookie_file" ]; then
            __request GET "$path" "text/html" "200" "text/html" "$scope html $path" -b "$cookie_file" -c "$cookie_file"
            __request GET "$path" "text/plain" "200" "text/plain" "$scope text $path" -b "$cookie_file" -c "$cookie_file"
        else
            __request GET "$path" "text/html" "200" "text/html" "$scope html $path"
            __request GET "$path" "text/plain" "200" "text/plain" "$scope text $path"
        fi
    done
}

__api_matrix() {
    local scope="$1"
    local cookie_file="${2:-}"
    shift $(( $# > 0 ? 2 : 1 )) || true
    local path
    for path in "$@"; do
        if [ -n "$cookie_file" ]; then
            __request GET "$path" "application/json" "200" "application/json" "$scope json $path" -b "$cookie_file" -c "$cookie_file"
            __request GET "$path" "text/plain" "200" "text/plain" "$scope text $path" -b "$cookie_file" -c "$cookie_file"
        else
            __request GET "$path" "application/json" "200" "application/json" "$scope json $path"
            __request GET "$path" "text/plain" "200" "text/plain" "$scope text $path"
        fi
    done
}

__bootstrap_server() {
    mkdir -p \
        "$TEST_DIR/volumes/config" \
        "$TEST_DIR/volumes/data" \
        "$TEST_DIR/volumes/logs" \
        "$TEST_DIR/volumes/cache" \
        "$TEST_DIR/volumes/backup"

    COLUMNS=120 "/usr/local/bin/$PROJECTNAME" \
        --mode development \
        --config "$TEST_DIR/volumes/config" \
        --data "$TEST_DIR/volumes/data" \
        --log "$TEST_DIR/volumes/logs" \
        --cache "$TEST_DIR/volumes/cache" \
        --backup "$TEST_DIR/volumes/backup" \
        --address 127.0.0.1 \
        --port 80 \
        >"$TEST_DIR/server.log" 2>&1 &
    SERVER_PID=$!

    for _ in $(seq 1 60); do
        if [ "$(curl -q -LSs -o /dev/null -w '%{http_code}' "$BASE_URL/server/healthz" || true)" = "200" ]; then
            return 0
        fi
        sleep 2
    done

    echo "Server failed to start" >&2
    tail -n 200 "$TEST_DIR/server.log" >&2 || true
    exit 1
}

__extract_setup_token() {
    local token=""
    for _ in $(seq 1 30); do
        token=$(grep -o -- 'Setup Token: [A-Za-z0-9]\{16,\}' "$TEST_DIR/server.log" | head -n1 | awk '{print $3}' || true)
        if [ -n "$token" ]; then
            printf '%s\n' "$token"
            return 0
        fi
        sleep 1
    done
    return 1
}

__create_admin() {
    local setup_token="$1"
    local hdr="$TEST_DIR/admin-setup.headers"
    local body="$TEST_DIR/admin-setup.body"

    curl -q -LSsf -L -c "$SETUP_COOKIE" -b "$SETUP_COOKIE" \
        --data-urlencode "setup_token=$setup_token" \
        "$BASE_URL/server/admin/verify-token" >/dev/null

    curl -q -LSsf -D "$hdr" -o "$body" -c "$SETUP_COOKIE" -b "$SETUP_COOKIE" \
        -H "Accept: application/json" \
        --data-urlencode "username=$ADMIN_USERNAME" \
        --data-urlencode "email=admin@example.com" \
        --data-urlencode "password=$ADMIN_PASSWORD" \
        --data-urlencode "confirm_password=$ADMIN_PASSWORD" \
        "$BASE_URL/server/admin/config/setup"

    if [ "$(__http_code "$hdr")" != "200" ]; then
        __fail "admin bootstrap :: expected status 200, got $(__http_code "$hdr")"
        head -c 200 "$body" >&2 || true
        printf '\n' >&2
        return
    fi

    ADMIN_TOKEN=$(jq -r '.api_token // empty' "$body")

    if [ -z "$(__cookie_value "$SETUP_COOKIE" "admin_session")" ]; then
        __fail "admin bootstrap :: admin_session cookie missing"
    fi
    if [ -z "$ADMIN_TOKEN" ]; then
        __fail "admin bootstrap :: admin API token missing"
    fi
}

__admin_api_matrix() {
    local path
    for path in "$@"; do
        __request GET "$path" "application/json" "200" "application/json" "admin api json $path" -H "Authorization: Bearer $ADMIN_TOKEN"
        __request GET "$path" "text/plain" "200" "text/plain" "admin api text $path" -H "Authorization: Bearer $ADMIN_TOKEN"
    done
}

__create_user() {
    curl -q -LSsf -L -c "$USER_COOKIE" -b "$USER_COOKIE" \
        --data-urlencode "username=matrixuser" \
        --data-urlencode "email=matrixuser@example.com" \
        --data-urlencode "password=MatrixPassword123!" \
        --data-urlencode "confirm_password=MatrixPassword123!" \
        "$BASE_URL/server/auth/register" >/dev/null

    if [ -z "$(__cookie_value "$USER_COOKIE" "weather_session")" ]; then
        __fail "user bootstrap :: weather_session cookie missing"
    fi
}

__check_user_login() {
    local hdr="$TEST_DIR/login.headers"
    local body="$TEST_DIR/login.body"

    curl -q -LSs -D "$hdr" -o "$body" -c "$USER_COOKIE" -b "$USER_COOKIE" \
        --data-urlencode "username=matrixuser" \
        --data-urlencode "password=MatrixPassword123!" \
        "$BASE_URL/server/auth/login" >/dev/null
    case "$(__http_code "$hdr")" in
        200 | 302 | 303) ;;
        *) __fail "user login :: expected 200/302/303, got $(__http_code "$hdr")" ;;
    esac

    curl -q -LSs -D "$hdr" -o "$body" \
        -H "Accept: application/json" \
        --data-urlencode "username=matrixuser" \
        --data-urlencode "password=WrongPassword000!" \
        "$BASE_URL/server/auth/login" >/dev/null
    case "$(__http_code "$hdr")" in
        401 | 403) ;;
        *) __fail "invalid credentials :: expected 401/403, got $(__http_code "$hdr")" ;;
    esac
}

__check_unauth_protection() {
    local hdr="$TEST_DIR/protected.headers"
    local body="$TEST_DIR/protected.body"

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: text/html" "$BASE_URL/users" >/dev/null
    if [ "$(__http_code "$hdr")" != "302" ]; then
        __fail "unauth html /users :: expected 302 redirect"
    fi

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: application/json" "$BASE_URL/api/v1/users" >/dev/null
    if [ "$(__http_code "$hdr")" != "401" ]; then
        __fail "unauth api /api/v1/users :: expected 401"
    fi

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: application/json" "$BASE_URL/api/v1/server/admin/config/users" >/dev/null
    if [ "$(__http_code "$hdr")" != "401" ]; then
        __fail "unauth api /api/v1/server/admin/config/users :: expected 401"
    fi

    curl -q -LSs -D "$hdr" -o "$body" -H "Accept: text/html" "$BASE_URL/server/admin/dashboard" >/dev/null
    case "$(__http_code "$hdr")" in
        302 | 401 | 403) ;;
        *) __fail "unauth html /server/admin/dashboard :: expected 302/401/403, got $(__http_code "$hdr")" ;;
    esac
}

PUBLIC_FRONTEND_ROUTES=(
    "/"
    "/health"
    "/server/healthz"
    "/server/auth/login"
    "/server/auth/register"
    "/server/auth/password/forgot"
    "/server/auth/password/reset"
    "/server/auth/2fa"
    "/server/auth/passkey"
    "/server/auth/username/forgot"
    "/server/auth/recovery/use"
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
    "/api/v1/server/healthz"
    "/api/v1/blocklist"
    "/api/v1/server/about"
    "/api/v1/server/privacy"
    "/api/v1/server/help"
    "/api/v1/server/terms"
    "/api/v1/weather?location=London"
    "/api/v1/weather/London"
    "/api/v1/weather?lat=40.7128&lon=-74.0060"
    "/api/v1/weather?city_id=5128581"
    "/api/v1/weather?lat=40.7128&lon=-74.0060&nearest=true"
    "/api/v1/weather/forecast?lat=40.7128&lon=-74.0060&days=7"
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

TXT_ROUTES=(
    "/robots.txt"
    "/.well-known/security.txt"
    "/server/healthz.txt"
    "/api/healthz.txt"
    "/api/v1/server/healthz.txt"
    "/api/v1/weather.txt?location=London"
    "/api/v1/weather/London.txt"
    "/api/v1/weather/forecast.txt?location=London"
    "/api/v1/moon.txt?location=London"
    "/api/v1/sun.txt?location=London"
    "/api/v1/earthquakes.txt"
    "/api/v1/hurricanes.txt"
    "/api/v1/ip.txt"
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
)

USER_API_ROUTES=(
    "/api/v1/users"
    "/api/v1/users/settings"
    "/api/v1/users/tokens"
    "/api/v1/users/avatar"
    "/api/v1/users/security/2fa"
    "/api/v1/users/security/2fa/setup"
    "/api/v1/users/sessions"
    "/api/v1/users/locations"
    "/api/v1/users/preferences"
    "/api/v1/users/notifications"
    "/api/v1/users/notifications/unread"
    "/api/v1/users/notifications/count"
    "/api/v1/users/notifications/stats"
    "/api/v1/users/notifications/preferences"
)

ADMIN_FRONTEND_ROUTES=(
    "/server/admin"
    "/server/admin/dashboard"
    "/server/admin/config/settings"
    "/server/admin/config/web"
    "/server/admin/config/users"
    "/server/admin/config/security"
    "/server/admin/config/scheduler"
    "/server/admin/config/backup"
    "/server/admin/config/logs"
    "/server/admin/config/branding"
    "/server/admin/config/pages"
    "/server/admin/config/admins"
    "/server/admin/$ADMIN_USERNAME/notifications"
)

ADMIN_API_ROUTES=(
    "/api/v1/server/admin/config/setup"
    "/api/v1/server/admin/config/users"
    "/api/v1/server/admin/config/settings"
    "/api/v1/server/admin/config/settings/all"
    "/api/v1/server/admin/config/security/tokens"
    "/api/v1/server/admin/config/stats"
    "/api/v1/server/admin/config/email"
    "/api/v1/server/admin/config/branding"
    "/api/v1/server/admin/config/pages"
    "/api/v1/server/admin/config/web"
    "/api/v1/server/admin/config/status"
    "/api/v1/server/admin/config/health"
    "/api/v1/server/admin/config/scheduler"
    "/api/v1/server/admin/config/channels"
    "/api/v1/server/admin/config/admins"
    "/api/v1/server/admin/config/backup"
    "/api/v1/server/admin/config/templates"
    "/api/v1/server/admin/config/smtp/providers"
    "/api/v1/server/admin/$ADMIN_USERNAME/preferences"
    "/api/v1/server/admin/$ADMIN_USERNAME/profile/sessions"
    "/api/v1/server/admin/$ADMIN_USERNAME/notifications"
    "/api/v1/server/admin/$ADMIN_USERNAME/notifications/unread"
    "/api/v1/server/admin/$ADMIN_USERNAME/notifications/count"
    "/api/v1/server/admin/$ADMIN_USERNAME/notifications/stats"
    "/api/v1/server/admin/$ADMIN_USERNAME/notifications/preferences"
)

__check_binary_rename() {
    local src="$1"
    local label="$2"
    local renamed="$TEST_DIR/renamed-${label}-binary"

    cp "$src" "$renamed"
    chmod +x "$renamed"

    if ! "$renamed" --version >"$TEST_DIR/rename.out" 2>&1; then
        __fail "$label rename :: --version failed after rename"
        return
    fi
    if ! "$renamed" --help >"$TEST_DIR/rename.out" 2>&1; then
        __fail "$label rename :: --help failed after rename"
        return
    fi
    if ! grep -q -- "renamed-${label}-binary" "$TEST_DIR/rename.out"; then
        __fail "$label rename :: --help does not report the renamed binary name"
    fi
}

__check_cli_functionality() {
    local out="$TEST_DIR/cli.out"

    if ! "/usr/local/bin/${PROJECTNAME}-cli" --server "$BASE_URL" --token "$ADMIN_TOKEN" \
        --output json current --location London >"$out" 2>&1; then
        __fail "cli current :: command failed"
        head -c 200 "$out" >&2 || true
        printf '\n' >&2
        return
    fi
    if ! jq -e . "$out" >/dev/null 2>&1; then
        __fail "cli current :: output is not valid JSON"
    fi

    if ! "/usr/local/bin/${PROJECTNAME}-cli" --server "$BASE_URL" --token "$ADMIN_TOKEN" \
        --output json forecast --location London >"$out" 2>&1; then
        __fail "cli forecast :: command failed"
    fi

    if ! "/usr/local/bin/${PROJECTNAME}-cli" --shell completions bash >/dev/null 2>&1; then
        __fail "cli --shell completions :: command failed"
    fi
}

__txt_matrix() {
    local scope="$1"
    shift
    local path
    for path in "$@"; do
        __request GET "$path" "*/*" "200" "text/plain" "$scope txt-extension $path"
    done
}

# AI.md PART 31: pages that must pass the automated accessibility checks.
# One representative page per layout family (public, auth, user, admin).
A11Y_FRONTEND_ROUTES=(
    "/"
    "/server/about"
    "/server/auth/login"
)

A11Y_USER_ROUTES=(
    "/users/dashboard"
)

A11Y_ADMIN_ROUTES=(
    "/server/admin/dashboard"
)

__a11y_fetch() {
    local path="$1"
    local cookie_file="${2:-}"
    local out="$3"

    if [ -n "$cookie_file" ]; then
        curl -q -LSsf -o "$out" -H "Accept: text/html" -b "$cookie_file" -c "$cookie_file" "${BASE_URL}${path}" >/dev/null
    else
        curl -q -LSsf -o "$out" -H "Accept: text/html" "${BASE_URL}${path}" >/dev/null
    fi
}

# AI.md PART 31: "Verify skip link exists" and skip links must be the first
# focusable elements on the page.
__a11y_check_skip_link() {
    local flat="$1"
    local label="$2"
    local body before

    body=$(sed 's/.*<body[^>]*>//' "$flat")
    if ! printf '%s' "$body" | grep -q 'class="[^"]*skip-link'; then
        __fail "$label :: no skip link found"
        return
    fi

    before=$(printf '%s' "$body" | sed 's/<a [^>]*class="[^"]*skip-link.*//')
    if printf '%s' "$before" | grep -qiE '<(a |button|select|textarea|input)'; then
        __fail "$label :: skip link is not the first focusable element"
    fi
}

# AI.md PART 31: "Verify all images have alt text".
__a11y_check_images() {
    local flat="$1"
    local label="$2"
    local missing

    missing=$(grep -o '<img[^>]*>' "$flat" | grep -cv 'alt=' || true)
    if [ "${missing:-0}" -gt 0 ]; then
        __fail "$label :: $missing <img> element(s) without an alt attribute"
    fi
}

# AI.md PART 31: "Verify form labels are associated" — every visible control
# needs a <label for>, an aria-label, or an aria-labelledby.
__a11y_check_form_labels() {
    local flat="$1"
    local label="$2"
    local tag id

    while IFS= read -r tag; do
        [ -n "$tag" ] || continue
        case "$tag" in
            *'type="hidden"'* | *'type="submit"'* | *'type="button"'* | *'type="reset"'* | *'type="image"'*) continue ;;
            *aria-label=* | *aria-labelledby=*) continue ;;
        esac

        id=$(printf '%s' "$tag" | sed -n 's/.*[^-]id="\([^"]*\)".*/\1/p')
        if [ -z "$id" ]; then
            __fail "$label :: form control without id or aria-label: $tag"
            continue
        fi
        if ! grep -q "for=\"$id\"" "$flat"; then
            __fail "$label :: form control id=\"$id\" has no associated <label for>"
        fi
    done <<EOF
$(grep -o '<\(input\|select\|textarea\)[^>]*>' "$flat" || true)
EOF
}

# AI.md PART 31: "Verify heading hierarchy" — exactly one h1, no skipped levels.
__a11y_check_headings() {
    local flat="$1"
    local label="$2"
    local levels count_h1 previous=0 level

    levels=$(grep -o '<h[1-6][ />]' "$flat" | sed 's/<h\([1-6]\).*/\1/' || true)
    if [ -z "$levels" ]; then
        __fail "$label :: page has no headings"
        return
    fi

    count_h1=$(printf '%s\n' "$levels" | grep -c '^1$' || true)
    if [ "${count_h1:-0}" -ne 1 ]; then
        __fail "$label :: expected exactly one <h1>, found ${count_h1:-0}"
    fi

    for level in $levels; do
        if [ "$previous" -ne 0 ] && [ "$level" -gt $((previous + 1)) ]; then
            __fail "$label :: heading hierarchy skips from h$previous to h$level"
            break
        fi
        previous="$level"
    done
}

# AI.md PART 31: "Verify landmarks exist" — banner, navigation, main, contentinfo.
__a11y_check_landmarks() {
    local flat="$1"
    local label="$2"

    grep -qE '<main[ >]|role="main"' "$flat" || __fail "$label :: missing main landmark"
    grep -qE '<nav[ >]|role="navigation"' "$flat" || __fail "$label :: missing navigation landmark"
    grep -qE '<header[ >]|role="banner"' "$flat" || __fail "$label :: missing banner landmark"
    grep -qE '<footer[ >]|role="contentinfo"' "$flat" || __fail "$label :: missing contentinfo landmark"
}

__a11y_page() {
    local path="$1"
    local cookie_file="${2:-}"
    local label="a11y $path"
    local raw="$TEST_DIR/a11y.html"
    local flat="$TEST_DIR/a11y-flat.html"

    if ! __a11y_fetch "$path" "$cookie_file" "$raw"; then
        __fail "$label :: curl transport failed"
        return
    fi

    tr '\n' ' ' < "$raw" > "$flat"

    __a11y_check_skip_link "$flat" "$label"
    __a11y_check_images "$flat" "$label"
    __a11y_check_form_labels "$flat" "$label"
    __a11y_check_headings "$flat" "$label"
    __a11y_check_landmarks "$flat" "$label"
}

__a11y_matrix() {
    local cookie_file="$1"
    shift
    local path
    for path in "$@"; do
        __a11y_page "$path" "$cookie_file"
    done
}

# AI.md PART 31: Arabic is served RTL, driven by meta.direction in the locale
# file, and an unsupported language falls back to English without erroring.
__i18n_matrix() {
    local raw="$TEST_DIR/i18n.html"
    local flat="$TEST_DIR/i18n-flat.html"
    local lang

    for lang in en es zh fr ar de ja; do
        if ! __a11y_fetch "/?lang=$lang" "" "$raw"; then
            __fail "i18n ?lang=$lang :: curl transport failed"
            continue
        fi
        tr '\n' ' ' < "$raw" > "$flat"

        if ! grep -q "lang=\"$lang\"" "$flat"; then
            __fail "i18n ?lang=$lang :: <html lang> was not set to $lang"
        fi

        if [ "$lang" = "ar" ]; then
            grep -q 'dir="rtl"' "$flat" || __fail "i18n ?lang=ar :: <html dir> is not rtl"
        elif grep -q 'dir="rtl"' "$flat"; then
            __fail "i18n ?lang=$lang :: <html dir> must not be rtl"
        fi
    done

    if ! __a11y_fetch "/?lang=zz" "" "$raw"; then
        __fail "i18n ?lang=zz :: unsupported language must fall back, not error"
        return
    fi
    tr '\n' ' ' < "$raw" > "$flat"
    grep -q 'lang="en"' "$flat" || __fail "i18n ?lang=zz :: unsupported language did not fall back to en"
}

__log "Version and binary checks"
"/usr/local/bin/$PROJECTNAME" --version
"/usr/local/bin/$PROJECTNAME" --help >/dev/null
"/usr/local/bin/${PROJECTNAME}-cli" --version
"/usr/local/bin/${PROJECTNAME}-cli" --help >/dev/null
file "/usr/local/bin/$PROJECTNAME"
file "/usr/local/bin/${PROJECTNAME}-cli"

__log "Binary rename checks"
__check_binary_rename "/usr/local/bin/$PROJECTNAME" "server"
__check_binary_rename "/usr/local/bin/${PROJECTNAME}-cli" "cli"

__log "Starting server in temp volumes dir"
__bootstrap_server

__log "Extracting setup token"
SETUP_TOKEN=$(__extract_setup_token || true)
if [ -z "$SETUP_TOKEN" ]; then
    echo "Failed to extract setup token from server log" >&2
    tail -n 200 "$TEST_DIR/server.log" >&2 || true
    exit 1
fi
printf 'Setup token extracted: %s\n' "$SETUP_TOKEN"

__log "Checking unauthenticated protection"
__check_unauth_protection

__log "Public frontend matrix"
__frontend_matrix "public frontend" "" "${PUBLIC_FRONTEND_ROUTES[@]}"

__log "Public API matrix"
__api_matrix "public api" "" "${PUBLIC_API_ROUTES[@]}"

__log "Creating primary admin"
__create_admin "$SETUP_TOKEN"

__log "CLI functionality against running server"
__check_cli_functionality

__log "Text extension (.txt) matrix"
__txt_matrix "public" "${TXT_ROUTES[@]}"

__log "Creating regular user"
__create_user

__log "User login and invalid credential rejection"
__check_user_login

__log "User frontend matrix"
__frontend_matrix "user frontend" "$USER_COOKIE" "${USER_FRONTEND_ROUTES[@]}"

__log "User API matrix"
__api_matrix "user api" "$USER_COOKIE" "${USER_API_ROUTES[@]}"

__log "Admin frontend matrix"
__frontend_matrix "admin frontend" "$SETUP_COOKIE" "${ADMIN_FRONTEND_ROUTES[@]}"

__log "Admin API matrix"
__admin_api_matrix "${ADMIN_API_ROUTES[@]}"

__log "Accessibility matrix"
__a11y_matrix "" "${A11Y_FRONTEND_ROUTES[@]}"
__a11y_matrix "$USER_COOKIE" "${A11Y_USER_ROUTES[@]}"
__a11y_matrix "$SETUP_COOKIE" "${A11Y_ADMIN_ROUTES[@]}"

__log "Language and direction matrix"
__i18n_matrix

__log "Summary"
if [ "$FAILURES" -ne 0 ]; then
    printf 'Detected %d failures.\n' "$FAILURES" >&2
    exit 1
fi

printf 'All route/header checks passed.\n'
