# Configuration & Application Modes Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER put YAML comments inline (same line as a value) — comments always go on the line ABOVE. Exception: GitHub Actions SHA-pin comments (`uses: foo@<sha> # vX.Y.Z`).
- NEVER build a file path from user/config input without running it through `SafePath`/`validatePath`/`validatePathSegment` — reject `..`, absolute-path escapes, null bytes, and symlink traversal.
- NEVER fail server startup because a config value is invalid — warn and substitute the default instead.
- NEVER use `strconv.ParseBool()` for boolean config/env/flag/form/query values — always `config.ParseBool()`/`config.IsTruthy()`.
- NEVER trust `X-Forwarded-*` headers (Host, Proto, Port, Prefix, client-IP variants) from a peer that isn't in `trusted_proxies` (private ranges + `additional` allow-list) — drop them and fall back to `r.Host`/`r.RemoteAddr`.
- NEVER evaluate the `trusted_proxies` gate against a rewritten `r.RemoteAddr` — real-IP middleware must preserve the original TCP peer in context and trust-check against that, never the resolved client IP.
- NEVER apply the `trusted_proxies` gate to Tor requests — Tor Host-header match is priority 0, resolved from `tor.*` config with no IP check.
- NEVER let a Tor response leak a clearnet FQDN, clearnet email, or `Preferred-Languages` line — Tor responses use `tor.onion_address`/`tor.contact_email` only, omitting fields entirely rather than falling back to clearnet values.
- NEVER default `server.contact.abuse.email` to `abuse@{fqdn}` automatically — RFC 2142 recommends it, but the operator must opt in (unprovisioned mailbox would bounce reports).
- NEVER expose `server.contact.admin.email` or any `webhooks.*` URL publicly — admin email and webhook URLs (contain tokens/chat IDs) are server-internal only.
- NEVER let `--debug`/`DEBUG=true` bypass admin authentication or any security check, in any mode, including production — debug affects verbosity/diagnostics only.
- NEVER enable debug/pprof/expvar endpoints outside `--debug`/`DEBUG=true` — return 404 otherwise, regardless of `MODE`.
- NEVER use Redis/Valkey as the only cache option — `memory` must work standalone for Single Instance mode; Valkey/Redis is REQUIRED only for Cluster/Mixed Mode.
- NEVER specify both `cache.url` and individual `host`/`port`/`password` fields as if independent — `url` takes precedence when both are set.

## CRITICAL - ALWAYS DO

- ALWAYS keep config comments single-line, under 140 characters, placed above the setting they describe.
- ALWAYS run untrusted path input through the full path-security pipeline (`normalizePath` → `validatePathSegment` → `validatePath` → `SafePath`/`SafeFilePath`) before any filesystem operation; wrap the result in `PathSecurityMiddleware` for HTTP-facing paths.
- ALWAYS resolve mode via: `--mode` flag > `MODE` env > default `production`; resolve debug via: `--debug` flag > `DEBUG` env (truthy) > `--mode debug`/`MODE=debug` alias > default `false`. An explicit `--debug`/`DEBUG` always wins over the `debug` mode alias (`MODE=debug DEBUG=false` → development mode, debug off).
- ALWAYS treat Single Instance mode's `server.yml` as the configuration source of truth, and Cluster Mode's remote database as the source of truth with `server.yml` demoted to a synced cache/backup (`_cache:` block) used only in read-only fallback when the DB is unreachable.
- ALWAYS sync every database config change to local `server.yml` immediately, on startup, and every 5 minutes to catch drift.
- ALWAYS treat only two error classes as critical (DB connection failure, cannot write files) — enter Maintenance Mode (read-only, admin panel still reachable with fix guidance, self-healing retries every 30s) for these; everything else is recoverable in place.
- ALWAYS return `503` with the canonical error body (`error: "MAINTENANCE"`) plus `Retry-After`/`X-Maintenance-Mode`/`X-Maintenance-Reason` headers for writes attempted during maintenance mode — operational metadata goes in headers, never ad-hoc body fields.
- ALWAYS default the listen port to a random unused 64000-64999 port on first run only, then persist the chosen port to `server.yml` for all future restarts.
- ALWAYS strip `:80`/`:443` from displayed URLs; show every other port explicitly.
- ALWAYS resolve `{baseurl}` in order: `X-Forwarded-Prefix` → `X-Forwarded-Path` → `X-Script-Name` → `server.baseurl`/`--baseurl` config → default `/`; normalize trailing slash, treat empty as `/`.
- ALWAYS gate every `X-Forwarded-*`/real-IP header on the immediate peer being inside `trusted_proxies` (private ranges + link-local + same-`/24`-as-listen-address are always trusted; public proxies go in `additional` as IP/CIDR/DNS, refreshed every 5 minutes).
- ALWAYS sign outbound contact/notification webhooks with `X-Webhook-Signature` (HMAC-SHA256, per-webhook secret), `X-Webhook-Timestamp`, `X-Webhook-ID` (UUIDv7), and `X-Webhook-Event`; retry non-2xx with backoff (1m/5m/15m/1h/6h/24h then drop), reusing the same `X-Webhook-ID` for dedup.
- ALWAYS resolve contact-role fallbacks: `security` → `admin` if unset; `abuse` → `general` → `admin`; `general` → `admin`; `admin` itself has no fallback (empty triggers a startup warning) — recompute per-dispatch, never cache across requests.
- ALWAYS validate every config value on load and replace invalid values with sane defaults, logging a warning — never crash startup on bad config.
- ALWAYS make every setting in this PART's scope (request limits, compression, trusted proxies, session, rate limiting, i18n, cache, contact, tracking, privacy/consent) editable via the admin WebUI with live reload, except where PART 0 already carves out the restart-required exception (listen address/port/DB driver).

## Key Rules Summary

### YAML Comment Style & Path Security (PART 5)
- Comments: above-only, ≤140 chars, single line. GitHub Actions SHA-pin trailing-tag comments are the sole inline exception (applies to workflow YAML, not `server.yml`).
- Path security pipeline (`src/util/path.go` pattern): `normalizePath` (clean `.`/`..`, collapse slashes) → `validatePathSegment` (reject empty, `.`, `..`, null bytes, path separators inside a single segment) → `validatePath` (re-join and confirm the result stays inside the allowed root) → `SafePath`/`SafeFilePath` (final absolute-path resolution + root-containment check) → `PathSecurityMiddleware` wraps any HTTP route accepting a path-like parameter.
- 10-layer middleware order (outermost/last-executed to innermost/first-executed): `LoggingMiddleware` → `RecoveryMiddleware` → `SecurityHeadersMiddleware` → `RateLimitMiddleware` → `CORSMiddleware` → `AuthMiddleware` → `CSRFMiddleware` → `SessionMiddleware` → `PathSecurityMiddleware` → `URLNormalizeMiddleware` (innermost, runs first).
- Configuration Storage: Single Instance → `server.yml` is source of truth (no DB config table). Cluster Mode → remote database `config`/`srv_config` table is source of truth, `server.yml` `_cache:` block is a synced read-only-fallback backup (never caches admin password). Sync triggers: immediately on change, on startup, every 5 minutes.
- Maintenance Mode: only DB-connection and file-write errors are critical; self-healing retries every 30s (unlimited by default), auto-cleans logs/temp/old backups on disk-pressure; admin panel stays reachable with diagnostics + suggested fix commands; recovery is automatic once health checks pass, with an optional notification email.
- Boolean parsing: `config.ParseBool(s, default)` / `config.IsTruthy(s)` accept a large truthy/falsy word list (`1/0`, `yes/no`, `true/false`, `enable/disable`, `on/off`, `y/n`, `t/f`, plus many international/colloquial synonyms), case-insensitive, trimmed; empty string → default; invalid → error (never silently defaulted). Applies to env vars, config file values, CLI flags, API params, form inputs — everywhere a boolean is parsed.
- Env vars: Runtime-checked every start (`NO_COLOR`, `TERM`, `DOMAIN`, `MODE`, `DATABASE_DRIVER`, `DATABASE_URL`, `SMTP_*`) vs Init-only (used once on first run then ignored: `CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`, `PORT`, `LISTEN`, `APPLICATION_NAME`, `APPLICATION_TAGLINE`).
- `server.yaml` auto-migrates to `server.yml` on startup if found; config file is always named `server.yml`, root at `/etc/{internal_org}/{internal_name}/`, user at `~/.config/{internal_org}/{internal_name}/`.
- Port rules: default random 64000-64999 on first run only, then persisted; `0` = OS-assigned; `80`/`443` enable Let's Encrypt HTTP-01/TLS-ALPN-01 respectively; dual-port format `"8090,8443"` (HTTP,HTTPS); privileged ports (<1024) bind while root then drop privileges, following the escalation-order/authorization rules detailed in `service-rules.md` (PART 24/25) — this file does not duplicate that flow.

### Application Modes & Debug (PART 6)
- Four operational states: Production, Production+Debug, Development, Development+Debug — mode controls logging verbosity/caching/CORS strictness; debug flag independently controls `/debug/*`, pprof, expvar, verbose DB/cache logging, memory/goroutine profiling.
- Mode shortcuts: `dev`/`devel`/`development` → development; `prod`/`production` → production; `debug` → development + debug on (alias only, explicit `--debug`/`DEBUG` still wins).
- Debug endpoints (`/debug/pprof/*`, `/debug/vars`, `/debug/config` (sanitized), `/debug/routes`, `/debug/cache`, `/debug/db`, `/debug/scheduler`) exist only when `--debug`/`DEBUG=true`; otherwise 404, in every mode including development.
- Debug console banner: `🔒 Running in mode: production [debugging]` / `🔧 Running in mode: development [debugging]`.
- `mode.FromEnv()` pattern: `MODE` env sets mode via `SetAppMode`; a separately-checked `DEBUG` env (if present, even `DEBUG=false`) always overrides the `MODE=debug` alias's implicit debug-on.

### Server Configuration (PART 12)
- Request limits: `max_body_size` 10MB, `read_timeout`/`write_timeout` 30s, `idle_timeout` 120s — all admin-configurable.
- Response compression: enabled by default, level 5, MIME allowlist (html/css/js/json/xml).
- Trusted proxies: private ranges + link-local + same-`/24` always trusted with no config; `additional` allow-list (IP/CIDR/DNS, 5-min refresh) adds public upstream proxies; ungated direct deployments should leave `additional: []` so forged headers from random peers are dropped.
- Tor: request detected when `Host` matches `tor.onion_address` (priority 0, no trusted-proxy check); FQDN/proto(always `http`)/port(always stripped) resolved from `tor.*`; separate Tor `security.txt` variant omits `Preferred-Languages` and any clearnet reference. Full Tor process lifecycle is PART 32 (`backend-rules.md`), not duplicated here.
- Session: admin default `max_age` 30d / `idle_timeout` 24h; user default `max_age` 7d / `idle_timeout` 24h; `same_site: strict` default, `secure: auto`, `http_only: true` always.
- Rate limiting: read 120/min, write 10/min, health 120/min, global burst 240/min, login 5/15min, password-reset 3/hr, registration 5/hr — sliding window per IP in `server.db`; `429` + `Retry-After` + canonical `RATE_LIMITED` body on exceed.
- i18n: `server.i18n.default_language`/`supported` config knobs (full i18n behavior is PART 31, see `testing-rules.md`).
- Contact config (`server.contact.*`): four roles (admin/security/abuse/general), each with `email` + `webhooks` (telegram/discord/slack/mattermost/pushover/gotify/generic adapters, open schema for custom transports); canonical keys only, no aliases; admin panel at `/server/{admin_path}/config/notify` with per-transport test buttons.
- Analytics tracking (`server.tracking.*`): pluggable `type` (google/matomo/piwik/owa/fathom/plausible/umami/simple/cloudflare/none), `id`, optional `url` depending on platform; admin panel at `/server/{admin_path}/config/tracking`.
- Privacy & consent (`server.privacy.*`): data-sharing disclosure table, retention statement, export/deletion availability flags, opt-out consent banner (`default_enabled: true`, dynamic message when `data.sold` true/false), configurable via `/server/{admin_path}/config/privacy`.
- Cache: `memory` (default, Single Instance) vs `valkey`/`redis` (REQUIRED for Cluster/Mixed Mode — sessions, rate limiting, heartbeat, pub/sub, distributed locks); connection via `url` (takes precedence) or discrete `host`/`port`/`username`/`password`/`db`; TLS, pool sizing, key `prefix` (unique per app), default `ttl`, and native cluster mode (`cluster: true` + `cluster_nodes`) all supported.
- Admin panel `/server/{admin_path}/config/settings` exposes Request Limits, Compression, Trusted Proxies, Session, Rate Limiting, i18n, and Cache sections.

For complete details, see AI.md PART 5, 6, 12
