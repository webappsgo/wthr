# AI Task Ledger

This file is the repository-local mirror of the active task state so work
can move to a new development server without losing context.

Last updated: 2026-06-17. Build: clean. All 10 test suites: EXIT:0.

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
- [ ] `admin-config-routes` — verify ALL admin config routes use `/server/{admin_path}/config/*` prefix; grep for stray admin routes outside this scope
- [ ] `graphql-audit` — partially audited in previous sessions; remaining: verify passkey GraphQL mutations match live WebAuthn runtime; check any remaining panic-stubs or runtime mismatches

### Medium Priority (PART 7 / PART 33)

- [ ] `common-version-package` — `src/common/version/version.go` not yet created (PART 7 directory spec)
- [ ] `terminal-resize-package` — `src/common/terminal/resize.go` (SIGWINCH handler) not yet created
- [ ] `terminal-symbols-package` — `src/common/terminal/symbols.go` (Unicode/ASCII symbols) not yet created
- [ ] `display-mode-file` — `src/common/display/mode.go` listed separately in PART 7 tree (currently merged into detect.go)
- [ ] `client-common-display` — client cli.go uses own detectMode() instead of common/display.DetectDisplayEnv()

### Lower Priority

- [ ] `ldap-oidc-session-timeout` — LDAP/OIDC CreateSession hardcodes 24h instead of reading server config `auth.session_timeout`
- [ ] `registration-mode-normalise-on-load` — setup wizard and admin_users save raw mode strings; normalisation happens in GetRegistrationMode() which is correct, but server_config table may store legacy "public"/"private" values — acceptable since read path normalises

## Verified Progress Notes (from previous sessions)

- gqlgen toolchain note: project pins v0.17.60; generator has prelude template bug; hand-patched prelude.resolvers.go; skip_validation: true in gqlgen.yml; operational regen workflow documented in old TODO.AI.md entries
- Admin passkey foundation (server-side schema/model/REST CRUD) and GraphQL parity are landed
- Multi-user REST, web, and invite flows are implemented and tested
- Session token security: SHA-256 hashing, HttpOnly Secure cookie, SameSite Lax
- All weather data endpoints (weather, forecast, alerts, earthquakes, hurricanes, moon) are functional
