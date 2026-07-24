# AI Task Ledger

This file is the repository-local mirror of the active task state so work
can move to a new development server without losing context.

Last updated: 2026-07-20. Build: clean. All 10 test suites: EXIT:0. GH CI: 2 of 3 failing (test coverage, lint) — Docker Build fixed and verified (`docker/Dockerfile.aio` builds clean, exit 0). Full PART 7-33 compliance sweep completed this session.

## Current In-Progress Task

No active task. Pick from "Remaining Spec Gaps" below.

## Session 2026-06-07/17 — Completed

| Commit | What |
|--------|------|
| f6028bc | Fix user_sessions column mismatch — INSERT used id (INTEGER PK) instead of session_id TEXT; added data TEXT column; schema v5→v6 |
| cac6e51 | Add src/common/{terminal,display,theme,banner} packages per PART 7 |
| afad097 | Hash session tokens with SHA-256 per IDEA.md security spec; schema v6→v7 (session_id→token_hash) |
| 6e41f72 | Fix LDAP/OIDC handlers using wrong cookie name |
| 93ad86f | Migrate auth routes /auth/* → /server/auth/* per PART 14 (29 files) |
| 09de777 | Move user saved-locations routes to /users/locations per PART 14 |
| 45cff80 | Standardise registration mode to spec values (open/invite/admin_only/disabled); default invite-only per IDEA.md |

## Remaining Spec Gaps

### High Priority (PART 14/34 compliance)

- [ ] `user-settings-routes` — verify user settings web routes are at `/users/settings/*`; check all PART 14 user scope routes are correctly mounted
  Read: AI.md PART 14
- [ ] `admin-config-routes` — verify ALL admin config routes use `/server/{admin_path}/config/*` prefix; grep for stray admin routes outside this scope
  Read: AI.md PART 14
- [ ] `graphql-audit` — partially audited in previous sessions; remaining: verify passkey GraphQL mutations match live WebAuthn runtime; check any remaining panic-stubs or runtime mismatches
  Read: AI.md PART 14

### Medium Priority (PART 7 / PART 33)

- [ ] `common-version-package` — `src/common/version/version.go` not yet created (PART 7 directory spec)
  Read: AI.md PART 7
- [ ] `terminal-resize-package` — `src/common/terminal/resize.go` (SIGWINCH handler) not yet created
  Read: AI.md PART 7
- [ ] `terminal-symbols-package` — `src/common/terminal/symbols.go` (Unicode/ASCII symbols) not yet created
  Read: AI.md PART 7
- [ ] `display-mode-file` — `src/common/display/mode.go` listed separately in PART 7 tree (currently merged into detect.go)
  Read: AI.md PART 7
- [ ] `client-common-display` — client cli.go uses own detectMode() instead of common/display.DetectDisplayEnv()
  Read: AI.md PART 7

### Lower Priority

- [ ] `ldap-oidc-session-timeout` — LDAP/OIDC CreateSession hardcodes 24h (`auth_oidc.go:327`, `auth_ldap.go:104`) instead of reading server config `auth.session_timeout`; password login already does this correctly (`auth.go:597 getSessionTimeout()`) — confirmed still present, inconsistent across auth flows
  Read: AI.md PART 12
- [ ] `registration-mode-normalise-on-load` — setup wizard and admin_users save raw mode strings; normalisation happens in GetRegistrationMode() which is correct, but server_config table may store legacy "public"/"private" values — acceptable since read path normalises
  Read: AI.md PART 34

### Foundational (PART 0-6, found during 2026-07-18 bootstrap reconciliation)

- [ ] `license-attribution-incomplete` — LICENSE.md third-party section lists only 3 of ~40 direct go.mod dependencies; needs each dependency's actual license verified (not guessed) before adding
  Read: AI.md PART 2
- [ ] `docker-dockerfile-dev-missing` — `docker/Dockerfile.dev` does not exist; project structure requires it alongside `Dockerfile` and `Dockerfile.aio`
  Read: AI.md PART 3
- [ ] `router-migration-gin-to-chi` — AI.md PART 3 mandates go-chi/chi/v5, codebase uses gin-gonic/gin across ~80 source files, no approved-deviation doc exists; needs a full migration plan before execution given the blast radius
  Read: AI.md PART 3

### CI/CD (found during 2026-07-20 GH Actions failure investigation)

- [ ] `unit-test-coverage-below-threshold` — CI coverage gate requires >=60% (PART 29 mandates >=60% unit floor); actual `go test -cover ./...` reports ~1% because most packages (`src/server/handler`, `src/graphql`, `src/database`, `src/scheduler`, `src/email`, `src/middleware`, `src/renderer`, `src/backup`, `src/cluster`, `src/cli`, `src/swagger`, `src/common/*`) have zero `*_test.go` files; only client/config/mode/server/model/server/service/util plus tests/e2e, tests/integration, tests/unit/* currently pass. Needs a systematic test-writing pass package by package — too large to fix inline
  Read: AI.md PART 29
- [ ] `staticcheck-lint-violations` — CI `lint` job (`staticcheck ./...`) fails with ~60+ violations: ST1005 (capitalized error strings) across `src/server/handler/auth_api.go`, `admin_passkey_helpers.go`, `admin_passkey_login.go`, `passkey_helpers.go`, `src/graphql/resolvers_helpers.go`, `schema.resolvers.go`; ST1008 (error not last return value) in `src/database/failover.go:57`; U1000 (unused func `getUserIDFromContextWithError`) in `src/graphql/schema.resolvers_impl.go:22`. Mechanical but high file count — needs a dedicated pass, not a drive-by fix
  Read: AI.md PART 28

### PART 7-33 Compliance Sweep (found 2026-07-20, four parallel agent passes)

- [ ] `cli-lang-flag-missing` — `--lang` global flag (required on all binaries) not implemented in `src/cli/cli.go`
  Read: AI.md PART 8
- [ ] `pidfile-dead-code` — `src/util/pidfile.go` `PIDFile` type (Create/Check/Remove/GetPID) is never instantiated from `main.go`; PID-file feature does not run; also missing PID-reuse identity verification and reuse of existing `isContainer()` (`src/util/host.go:248`) to skip PID handling in containers
  Read: AI.md PART 8
- [ ] `dual-db-pool-hardcoded` — `src/database/dual_db.go:71-73,125-127` hardcodes `SetConnMaxLifetime(3m)/SetMaxOpenConns(10)/SetMaxIdleConns(5)` instead of using the configurable tiered pooling already implemented in `src/database/timeouts.go`
  Read: AI.md PART 10
- [ ] `security-headers-middleware-duplicate` — `src/middleware/security.go` is dead code, never wired; only `src/server/middleware/security_headers.go` (wired via `main.go:453`) is active — remove the dead copy or reconcile
  Read: AI.md PART 11
- [ ] `security-headers-incomplete` — wired security headers diverge from spec: HSTS `max-age=31536000` (1yr) instead of `63072000` (2yr); missing `X-Permitted-Cross-Domain-Policies`, `Origin-Agent-Cluster`, `Reporting-Endpoints`/`Report-To`/`NEL`, `X-Request-ID`; CSP and Permissions-Policy hardcoded per-app instead of generated from `web.csp`/`web.permissions_policy` config; no `Clear-Site-Data` on logout/account-delete
  Read: AI.md PART 11
- [ ] `crypto-keys-system-missing` — entire "Cryptographic Keys" system unimplemented: no `app_secrets` table, no `installation_secret`, `cookie_signing_key`, or `csrf_token_secret` generation/rotation anywhere in the codebase
  Read: AI.md PART 11
- [ ] `sec-fetch-validation-missing` — no `Sec-Fetch-*` / `Sec-GPC` request header validation anywhere in codebase
  Read: AI.md PART 11
- [ ] `trusted-proxies-hardcoded` — `main.go:427` hardcodes `SetTrustedProxies([]string{...RFC1918...})`; no `server.trusted_proxies.additional` config support, no DNS-name refresh, no `X-Forwarded-Prefix`/`X-Script-Name` baseurl-resolution chain
  Read: AI.md PART 12
- [ ] `client-content-negotiation-missing` — client-type content negotiation system (`isOurCliClient`/`isTextBrowser`/`isHttpTool`/`isNonInteractiveClient`/`HTML2TextConverter`) entirely unimplemented; no `src/common/httputil/` package exists
  Read: AI.md PART 14
- [ ] `legacy-doc-routes-forbidden` — forbidden legacy Swagger/GraphQL routes still served with redirects in `src/main.go`: `/openapi`, `/openapi/*any`, `/openapi.json` (~line 3554-3560) and root `/graphql` GET/POST (~line 3584-3585); GraphQL versioned-alias GET handler redirects to `/graphql` instead of mounting directly (~line 3590). Required replacements missing: `/server/docs/swagger`, `/server/docs/graphql`, `/api/swagger`, `/api/graphql`
  Read: AI.md PART 14
- [ ] `healthz-root-alias-ungated` — `/healthz` root alias registered unconditionally at `main.go:1126`; no `server.healthz.root.enabled` config field exists (default must be disabled)
  Read: AI.md PART 14
- [ ] `dns01-admin-ui-missing` — DNS-01 admin UI (dynamic provider dropdown, encrypted credential storage, `validated_at`) not implemented; only a `DNS01Provider` stub exists; `GetDisplayURL`/`GetWildcardDomain`/`GetAllDomains` helpers also missing
  Read: AI.md PART 15
- [ ] `cors-hardcoded-wildcard` — CORS hardcoded to `AllowOrigins: []string{"*"}` in `main.go:463` instead of resolved allow-list (config → DOMAIN → proxy-learned, `*` only as last-resort fallback)
  Read: AI.md PART 16
- [ ] `middleware-order-wrong` — `RequestID` must run before `AccessLogger` so logs carry the request ID; actual order in `main.go` has `AccessLogger` before `RequestID`; reordering touches multiple interdependent middleware so needs a deliberate pass, not a blind fix
  Read: AI.md PART 16
- [ ] `email-send-no-queue` — `src/email/email.go` `Send()`/`SendTemplate()` send SMTP synchronously with no DB-backed queue and no retry-on-failure, violating the explicit "never send from goroutine without queue" rule
  Read: AI.md PART 18
- [ ] `backup-retention-not-tiered` — `src/backup/backup.go`/`src/scheduler/backup_task.go` implement only a flat hardcoded "keep last 4" cleanup; spec requires configurable `max_backups` (default 1), `keep_weekly`/`keep_monthly`/`keep_yearly`/`max_total_size` tiered retention, plus compliance-mode enforcement blocking unencrypted backups when `server.compliance.enabled: true`
  Read: AI.md PART 22
- [ ] `scheduled-update-check-missing` — on-demand `--update {check|yes|branch}` CLI is implemented, but the scheduled `update_check` task (daily 06:00, `server.update.defer_days`, `server.update.auto_install` config, admin banner/notification-center surfacing) is entirely missing from the scheduler's task list
  Read: AI.md PART 23
- [ ] `uid-gid-selection-algorithm-missing` — `src/cli/service.go` system-user creation has no `reservedIDs` map; Linux `useradd --system` lets the OS auto-assign UID/GID instead of computing a matching pair in the safe range while avoiding reserved well-known IDs; macOS uses range 100-499 instead of spec's 200-399; BSD hardcodes UID 800
  Read: AI.md PART 24
- [ ] `openrc-sysvinit-installers-missing` — `src/cli/service.go` implements systemd, launchd, FreeBSD rc.d, and runit installers, but has no OpenRC or SysVinit installer/detection (spec requires both, selected by init-system detection)
  Read: AI.md PART 25
- [ ] `weather-service-file-audit` — `scripts/weather.service` may be a stale/misnamed leftover (not wired into Makefile/build/service.go); needs a deliberate decision (rename to spec naming vs. delete), not an unreviewed removal
  Read: AI.md PART 25
- [ ] `rtl-direction-support-missing` — no `dir` attribute in `src/server/template/partial/head.tmpl` and no Go-side language-direction helper in `src/server/`; needed for `ar` locale RTL support
  Read: AI.md PART 31
- [ ] `tor-admin-route-paths-wrong` — actual Tor admin routes are `/server/network/tor` (web) and `/server/tor` (API) vs. required `/server/{admin_path}/config/network/tor` and `/api/{api_version}/server/{admin_path}/config/network/tor` (missing `config` segment)
  Read: AI.md PART 32
- [ ] `tor-restart-validate-endpoints-missing` — `POST .../tor/restart` and `POST .../tor/validate` API endpoints have no corresponding handlers in `src/server/handler/admin_tor.go`
  Read: AI.md PART 32
- [ ] `platforms-not-all-8` — `Makefile` line 38 `PLATFORMS` only lists `linux/amd64,linux/arm64`; must build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
  Read: AI.md PART 26
- [ ] `coverage-threshold-below-spec` — `.github/workflows/ci.yml` line 46 enforces `THRESHOLD=60` (60%); testing-rules.md and AI.md PART 29 require 80% minimum; actual coverage is far below either threshold and needs substantial new test-writing effort across ~20 zero-coverage packages before the threshold itself can be safely raised
  Read: AI.md PART 29
- [ ] `external-cron-forbidden` — `src/scheduler/scheduler.go` imports `github.com/robfig/cron/v3` and calls `cron.New()` (~line 75); spec requires an internal ticker-based scheduler only, no third-party cron library
  Read: AI.md PART 19
- [ ] `graphiql-client-side-rendering` — `src/server/handler/graphql.go` lines ~146-185 serve GraphiQL via a client-side-rendering HTML page; spec forbids client-side rendering for any served page — must be server-side Go templates or disabled outside development mode
  Read: AI.md PART 16
- [ ] `release-build-date-format` — `.github/workflows/release.yml` line 57 formats `BUILD_DATE` as `"%a %b %d, %Y at %H:%M:%S %Z"`; must be ISO 8601 UTC (`%Y-%m-%dT%H:%M:%SZ`) to match the Makefile/CI convention
  Read: AI.md PART 28
- [ ] `schema-migration-v2-deadlock` — `src/database/schema.go` `migrateToV2`: nested query/exec on the same `*sql.DB` while a cursor is open self-deadlocks on real SQLite; found while writing `src/database/schema_test.go`
  Read: AI.md PART 9
- [ ] `db-connection-leak-on-pragma-error` — `src/database/connection.go`: PRAGMA/schema-creation error paths return without `db.Close()`, leaking the connection; found while writing `src/database/connection_test.go`
  Read: AI.md PART 9
- [ ] `custom-domains-table-forbidden` — `src/database/server_schema.go:368` defines a `custom_domains` table; PART 35/36 explicitly forbids any reference to `custom_domains` in code since the feature is not implemented
  Read: AI.md PART 35, 36
- [ ] `backup-token-unchecked-rand-error` — `src/backup/restore.go:206` `generateSetupToken()` ignores the `rand.Read` error, risking an all-zero predictable token on read failure
  Read: AI.md PART 11
- [ ] `email-template-placeholder-mismatch` — `src/email/template/welcome.txt:3` uses uppercase `{APP_NAME}` which never matches the lowercase template variable map; `Template.Render` also silently leaves unmatched placeholders as literal text instead of erroring
  Read: AI.md PART 18
- [ ] `scheduler-taskstats-null-scan-error` — `src/scheduler/task_history.go` `GetTaskStats`: `SUM(CASE...)` scanned into `int` errors on `NULL` (zero rows) instead of returning zeroed stats
  Read: AI.md PART 19
- [ ] `scheduler-backup-task-enabled-flag-ignored` — `src/scheduler/backup_task.go` `RegisterBackupTask`: the `enabled` parameter only affects a log line — the task is always registered as enabled regardless of its value
  Read: AI.md PART 19
- [ ] `scheduler-cleanup-datetime-format-mismatch` — `src/scheduler/scheduler.go` (`CleanupOldSessions`, `CleanupExpiredTokens`, `CleanupRateLimitCounters`) compares Go time-formatted values against SQLite `datetime('now')` in incompatible formats, confirmed to delete non-expired rows
  Read: AI.md PART 19
- [ ] `cli-maintenance-wrong-sqlite-driver` — `src/cli/maintenance.go:349` `openDatabase()` calls `sql.Open("sqlite3", dbPath)` but the only registered driver (`_ "modernc.org/sqlite"`) registers as `"sqlite"`; every caller (`wthr maintenance update`, `wthr maintenance admin-recovery`/`setup`) fails at runtime with `sql: unknown driver "sqlite3"`
  Read: AI.md PART 9
- [ ] `renderer-ascii-lowercase-country-code` — `src/renderer/ascii.go:88` `capitalizeLocation()` only uppercases a 2-letter part if it's already all-uppercase; `capitalizeLocation("london, gb")` returns `"London, Gb"` instead of `"London, GB"`
  Read: AI.md PART 16
- [ ] `renderer-json-nil-weather-panic` — `src/renderer/json.go` `Render()` dereferences `weather.Location`/`Current`/`Forecast`/`Moon` with no nil check; `Render(nil)` panics instead of returning a clean error
  Read: AI.md PART 16
- [ ] `graphql-context-key-type-mismatch` — `src/graphql/graphql.go` stores auth values with a typed `contextKey` (`ctxKeyAdminID`, `ctxKeyUserRole`, etc.) but `resolvers_helpers.go:451,474,839,855`, `schema.resolvers_impl.go:12`, and ~50 sites in `schema.resolvers.go` read them back with raw untyped string literals; `contextKey("admin_id") != "admin_id"` as map keys, so every raw-string lookup returns `ok=false` and authorization/role checks silently fail to see correctly-attached admin/user context across nearly all authenticated GraphQL resolvers
  Read: AI.md PART 14
- [x] `admin-users-registration-mode-legacy-only` — `src/server/handler/admin_users.go` `UpdateUserSettings` only accepts legacy `public`/`private`/`disabled` registration modes, rejecting the spec-mandated `open`/`invite`/`admin_only`/`disabled` (config-rules.md)
  Read: AI.md PART 34
  Fixed: now accepts `open`/`invite`/`admin_only`/`disabled` and normalises legacy `public`→`open`, `private`→`invite` instead of rejecting them.
- [x] `setup-wizard-mark-complete-wrong-columns` — `src/server/handler/setup_wizard.go` `markSetupComplete()` inserted into `server_setup_state` using `id`/`setup_completed`/`setup_completed_at` columns that don't exist on that table (it is a generic `key TEXT PRIMARY KEY, value TEXT, updated_at DATETIME` store per `database.ServerSchema`); every call silently failed with only a warning log, so the setup-state write was permanently dead. `IsSetupComplete()` gates on admin count, not this table, so functional gating was unaffected, but the audit/state write never persisted.
  Read: AI.md PART 22
  Fixed: rewrote the INSERT to the key/value shape (`key='setup_completed', value='1'`) with `ON CONFLICT(key) DO UPDATE`.
- [ ] `dead-db-param-pattern` — widespread pattern: handler/model constructors across `src/server/handler` and `src/server/model` accept a `*sql.DB` parameter but ignore it in favor of `database.GetServerDB()`/`GetUsersDB()` globals, forcing tests through `database.SetGlobalDualDB(...)` instead of real dependency injection
  Read: AI.md PART 9
- [ ] `path-sync-once-test-unfriendly` — `src/path/paths.go` uses a package-level `sync.Once`; `CONFIG_DIR`/`DATA_DIR` overrides only take effect if pinned before any other code initializes it, making runtime reconfiguration and test isolation fragile
  Read: AI.md PART 4
- [ ] `config-dir-override-ignored-on-first-run` — `src/config/config.go` `findConfigFile()` only honors the `CONFIG_DIR` env var if `{CONFIG_DIR}/server.yml` already exists (`os.Stat` check); when it does not, it falls through to `getConfigPath()`'s hardcoded system path (`/etc/casapps/wthr/server.yml` as root) and `createDefaultConfig` writes there, silently ignoring an explicitly-set `CONFIG_DIR` on first run instead of creating `{CONFIG_DIR}/server.yml`
  Read: AI.md PART 4, 5

## Verified Progress Notes (from previous sessions)

- gqlgen toolchain note: project pins v0.17.60; generator has prelude template bug; hand-patched prelude.resolvers.go; skip_validation: true in gqlgen.yml; operational regen workflow documented in old TODO.AI.md entries
- Admin passkey foundation (server-side schema/model/REST CRUD) and GraphQL parity are landed
- Multi-user REST, web, and invite flows are implemented and tested
- Session token security: SHA-256 hashing, HttpOnly Secure cookie, SameSite Lax
- All weather data endpoints (weather, forecast, alerts, earthquakes, hurricanes, moon) are functional
- 2026-07-20 compliance sweep: PART 9, 13, 17, 19, 26, 30, 33 verified fully compliant, no action needed
- 2026-07-20 direct fixes applied (uncommitted, need `make test` + build verification before commit): metrics name prefix `weather_*` → `wthr_*` across `src/server/metrics/metrics.go`, `src/server/handler/admin_metrics.go`, `src/server/template/admin/admin_metrics.tmpl` (PART 21); `GetFQDN()` in `src/util/host.go` now splits comma-separated `DOMAIN` and returns first as primary (PART 15); `scripts/i18n-validate.sh` default locale dir corrected from `src/locale` to `src/common/i18n/locales`, now passes for all 7 locales (PART 31); `docker/Dockerfile.aio` `docker/file_system/` → `docker/rootfs/` path fix, verified with a full `--no-cache` build (exit 0) (PART 27)
