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

## Verified Progress Notes (from previous sessions)

- gqlgen toolchain note: project pins v0.17.60; generator has prelude template bug; hand-patched prelude.resolvers.go; skip_validation: true in gqlgen.yml; operational regen workflow documented in old TODO.AI.md entries
- Admin passkey foundation (server-side schema/model/REST CRUD) and GraphQL parity are landed
- Multi-user REST, web, and invite flows are implemented and tested
- Session token security: SHA-256 hashing, HttpOnly Secure cookie, SameSite Lax
- All weather data endpoints (weather, forecast, alerts, earthquakes, hurricanes, moon) are functional
- 2026-07-20 compliance sweep: PART 9, 13, 17, 19, 26, 30, 33 verified fully compliant, no action needed
- 2026-07-20 direct fixes applied (uncommitted, need `make test` + build verification before commit): metrics name prefix `weather_*` → `wthr_*` across `src/server/metrics/metrics.go`, `src/server/handler/admin_metrics.go`, `src/server/template/admin/admin_metrics.tmpl` (PART 21); `GetFQDN()` in `src/util/host.go` now splits comma-separated `DOMAIN` and returns first as primary (PART 15); `scripts/i18n-validate.sh` default locale dir corrected from `src/locale` to `src/common/i18n/locales`, now passes for all 7 locales (PART 31); `docker/Dockerfile.aio` `docker/file_system/` → `docker/rootfs/` path fix, verified with a full `--no-cache` build (exit 0) (PART 27)
