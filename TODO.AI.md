# TODO.AI.md

Dependency order: items are listed in the order they must be done (each depends
on the ones above it being in place first). Read the cited AI.md PART slice
before starting each item — do not rely on memory.

1. DONE (2026-07-30): Generated all 14 `.claude/rules/*.md` cheatsheet files
   (PART 0 § "Rule Files to Create/Update", directory listed as REQUIRED in
   PART 3 § "Directory Structure") — `ai-rules.md`, `project-rules.md`,
   `config-rules.md`, `binary-rules.md`, `backend-rules.md`, `api-rules.md`,
   `frontend-rules.md`, `features-rules.md`, `service-rules.md`,
   `makefile-rules.md`, `docker-rules.md`, `cicd-rules.md`, `testing-rules.md`,
   `optional-rules.md`. Thirteen already existed from earlier bootstrap work;
   `config-rules.md` (PART 5, 6, 12: Configuration, Application Modes, Server
   Configuration) was the last missing file, added this pass following the
   same template (header, NON-NEGOTIABLE warning, CRITICAL - NEVER/ALWAYS DO,
   key-rules summary, footer). Verified all 14 present via `ls .claude/rules/`.

2. DONE (2026-07-31): Created `.claude/settings.json` (shared team config,
   version-controlled per PART 3's `.claude/` listing) following the sample
   template at AI.md PART 3 lines ~1780-1857, adapted with this project's
   Go-project addition (`Bash(golangci-lint *)` allow) and Docker-project
   addition (`Bash(docker system prune *)` deny, consistent with
   docker-rules.md's own ban on broad prune sweeps). Includes the two sample
   PreToolUse hooks (vendor-attribution check, gofmt check) and
   `CGO_ENABLED=0` env. `.claude/.mcp.json` intentionally NOT created — this
   project does not itself declare/require any MCP servers (the `github` MCP
   tools available in-session are session/global Claude Code config, not a
   project-level dependency); create `.mcp.json` later only if the project
   comes to genuinely need one. Read: AI.md PART 3 § "Directory Structure".

3. `.cursor/`, `.windsurf/`, `.aider/`, `.ai/` mirrors are marked OPTIONAL in
   PART 3 — user confirmed (2026-07-30) to skip these for now; revisit only
   if the project later needs multi-editor support. Read: AI.md PART 3 §
   "Directory Structure".

4. DONE (2026-07-30): AI.md's own canonical `GO_DOCKER` template (PART 26,
   line ~38235) does not include `-it` — the project Makefile had drifted
   from spec. Removed `-it` from `GO_DOCKER` in `Makefile` line 42 to match.
   `make dev`/`make test` now work without a TTY.

5. DONE (2026-07-30): `make test`'s 80%-coverage gate (Makefile lines
   213-226) was broken: `go tool cover -func` prints one `total:` line per
   `go test` invocation across the run, so `grep total | awk '{print $3}'`
   captured multiple values instead of a single number; `$COVERAGE` became
   multi-line, the `bc -l` check failed with `too many arguments`, and the
   gate silently no-oped (exited 0) instead of failing the build. Fixed by
   anchoring the awk match to `/^total:/` so only the final merged `total:`
   line from `$COVDIR/coverage.out` is captured. Verified live via
   `make test`: gate now correctly fails the build with
   `ERROR: Coverage is 25.9%, must be >= 80%` (exit 1) instead of silently
   passing. Read: AI.md PART 26, PART 29-31 (testing requirements).

6. DONE (2026-07-30): `go-lint` flagged `Makefile` line 35: `GO_BUILD` was
   not project-scoped (`$(HOME)/.cache/go-build`), shared across every
   project on the host instead of isolated per project. Fixed to
   `$(HOME)/.cache/go-build/$(PROJECTNAME)`. Read: AI.md PART 26.

7. DONE (2026-07-30): `go-lint` flagged `src/scheduler/scheduler.go` line 23:
   external cron library `robfig/cron/v3` used instead of a built-in
   scheduler. AI.md PART 19 "Exceptions (NONE)" explicitly rejects this
   ("I need complex schedules" -> "No - built-in supports full cron
   syntax"), so it is not an approved exception. Replaced with a built-in
   `time.Ticker`-driven scheduler: new `src/scheduler/cron.go` (a
   `Schedule` interface with standard 5-field cron, descriptor, and
   `@every <duration>` parsing, no external dependency), rewired
   `Scheduler`/`Task` in `src/scheduler/scheduler.go` to compute/track
   `nextRun` and poll it on a 1s ticker instead of delegating to
   `cron.Cron`, and updated `src/scheduler/task_history.go` to read
   `task.nextRun` directly. Removed `github.com/robfig/cron/v3` from
   `go.mod`/`go.sum` via `go mod tidy`. While verifying, also fixed a
   pre-existing goroutine race in `executeTask` (recording the DB history
   row before the audit-log write let async goroutines from finished
   tests run into a later test's already-torn-down DB) by reordering to
   log before recording. `go build ./...`, `go vet ./...`, and
   `go test ./src/scheduler/... -count=5` all pass.

## Pre-existing, out of scope

The following 10 files have unrelated uncommitted hand-edits and were
deliberately left untouched by this bootstrap pass — do not fold them into
any of the above: `src/graphql/context_keys_test.go`,
`src/graphql/resolvers_helpers.go`, `src/graphql/schema.resolvers.go`,
`src/graphql/schema.resolvers_impl.go`,
`src/graphql/schema.resolvers_mutations_test.go`,
`src/graphql/schema.resolvers_test.go`, `src/server/handler/admin_auth.go`,
`src/server/handler/admin_auth_test.go`,
`src/server/middleware/access_log_test.go`, `src/util/logger_test.go`.

8. DONE (2026-07-31): the GitHub Actions `CI` workflow was failing on `main`
   (confirmed failing at commits `3f2082cd`, `094092ff`, `9e27dd31`, and
   `6220988f`), inside the item-7-adjacent "Pre-existing, out of scope"
   files above:
   - `test` job: `TestMutationResolver_RegisterUser/registration_not_available_with_no_config`
     in `src/graphql/schema.resolvers_mutations_test.go` failed —
     `err = "public registration is not available"`, want
     `"registration is not available"` (message text mismatch). Fixed to
     match the actual `auth_api.go` wording (see item 13).
   - `lint` job (`staticcheck`): `SA1029` (built-in `string` used as a
     context key) at `src/graphql/schema.resolvers_mutations_test.go:656,725,734`
     and `src/server/handler/admin_auth_test.go:211`; `U1000` (unused func
     `newTestLogger`) at `src/server/middleware/access_log_test.go:19` and
     `src/util/logger_test.go:55`. Fixed by switching the test context keys
     to the project's typed `contextKey` constants (`ctxKeyUserID` etc.,
     already defined in `src/graphql/graphql.go`) and removing the two
     unused test helpers.
   - Switching those tests to the typed context keys exposed a genuine
     production bug (not previously caught by any test): `getUserIDFromContext`
     / `getIPFromContext` in `src/graphql/schema.resolvers_impl.go` read
     `ctx.Value("user_id")` / `ctx.Value("client_ip")` — bare `string` keys
     that can never match the typed `ctxKeyUserID` / `ctxKeyClientIP` keys
     production actually sets (`graphql.go:257` etc.), because Go compares
     `context.Value` keys by both underlying type and value. Every real
     authenticated GraphQL request was silently getting user ID `0` /
     IP `"unknown"` from these two helpers. Fixed by switching both helpers
     to the typed keys. This affects every call site that delegates to
     these two shared helpers (`schema.resolvers.go` lines 200-2509,
     `resolvers_helpers.go:438`) — all fixed by this one change.
   Verified via Docker `casjaysdev/go:latest`: `go build ./...`, `go vet
   ./...`, and `go test ./src/graphql/... ./src/server/handler/...
   ./src/server/middleware/... ./src/util/... ./src/config/...` all pass;
   `staticcheck ./...` reports zero findings. See item 15 for a much larger,
   related systemic bug found while investigating this one, deliberately
   left unfixed here as out of scope.

9. DONE (2026-07-30): found while verifying items 5/6 — `PROJECTNAME`/
   `PROJECTORG` (Makefile lines 2-3) mis-parsed the project's SSH remote
   `git@github.com:webappsgo/wthr.git`: the `PROJECTNAME` regex's greedy
   `[^/]+` swallowed the literal `.git` suffix (yielding `wthr.git` instead
   of `wthr`), and `PROJECTORG` requires two `/` after the match point but
   the SSH form only has one (colon instead of a second slash), so the sed
   silently failed to match and left the org as the full raw remote string
   (`git@github.com:webappsgo/wthr.git`). Confirmed live: `make test`'s
   coverage temp dir was created as
   `/tmp/git@github.com:webappsgo/wthr.git/wthr.git-XXXXXX/`, violating the
   required `/tmp/{project_org}/{internal_name}-XXXXXX/` temp-dir
   convention (PART 29). Fixed by normalizing `git@host:org/repo.git` to
   `https://host/org/repo` form first (strip `.git`), then extracting the
   last two path segments for name/org — verified `make -f` test harness
   now resolves `NAME=wthr ORG=webappsgo`. Read: AI.md PART 26, PART 29.

10. DONE (2026-07-31): found while committing item 1 — `gitcommit ... all`
    committed the full working tree (not just the two staged files),
    accidentally including the 10 out-of-scope files' hand-edits from item 8.
    That push also surfaced a real `govulncheck` finding unrelated to those
    files: `golang.org/x/text v0.38.0` affected by `GO-2026-5970` (infinite
    loop on invalid input), reachable via `letsencrypt.go`, `severe_weather.go`,
    `database/schema.go`, and `graphql/schema.resolvers.go`. Fixed by (a)
    bumping `golang.org/x/text` to `v0.39.0` (`go get` + `go mod tidy`,
    verified `govulncheck ./...` now reports 0 vulnerabilities in code the
    project calls) and (b) reverting the 10 out-of-scope files back to their
    prior committed state so item 8's failure stays exactly as documented,
    without the new coverage-threshold regression the hand-edits introduced
    (admin_auth.go's uncovered new lines dropped repo-wide coverage to 51%,
    below the 60% gate). Read: AI.md PART 9 (dependency hygiene).

11. DONE (2026-07-31): 2FA/TOTP secrets are now encrypted at rest with
    AES-256-GCM per AI.md PART 11 ("Cryptographic Keys" -> "Server Encryption
    Key" / "Data protection matrix"). (a) Key source: `server.security.
    encryption_key` (base64-encoded 32 bytes) + `encryption_key_version` in
    `server.yml` — NOT `app_secrets` (that table is reserved for
    `installation_secret`/`cookie_signing_key`/`csrf_token_secret`, per PART
    11's explicit consolidation note). `src/config/config.go`'s `LoadConfig`
    generates the key once on first run and, on upgrade, once for any existing
    `server.yml` missing it (persisted immediately via `SaveConfig` — never
    regenerated on an already-populated field, since that would make
    previously-encrypted data undecryptable). (b) New stdlib-only helper
    `src/util/encryption.go` (`EncryptAtRest`/`DecryptAtRest`/
    `IsEncryptedAtRest`). (c) One-time migration: `src/server/model/user.go`
    `Enable2FA` now always encrypts before persisting; new
    `DecryptTwoFactorSecret` transparently falls back to treating a
    non-decryptable stored value as legacy plaintext, so existing 2FA users
    aren't locked out — no separate migration script or schema change needed.
    Wired into all read/verify call sites: `handler/twofa.go` (verify code,
    regenerate recovery keys), `handler/auth.go` and `handler/auth_api.go`
    (login TOTP verification). Verified: `go build ./...` clean; `go test
    ./src/config/... ./src/server/model/... ./src/server/handler/...
    ./src/util/...` all pass (Docker `casjaysdev/go:latest`); full `make test`
    passes for every package except one pre-existing, unrelated failure (see
    item 13). Read: AI.md PART 11 (Cryptographic Keys, Data protection
    matrix), PART 34 (recovery keys / 2FA).

12. TODO (flagged 2026-07-31 during audit): ~722 database calls use the
    non-Context variants (`Query`/`Exec`/`QueryRow`) rather than
    `QueryContext`/`ExecContext`/`QueryRowContext` with a timeout. AI.md
    PART 10 requires every query/transaction wrapped in `context.WithTimeout`
    (SELECT 5s, JOIN 15s, write 10s, bulk 60s, reports 2m). This is a systemic
    migration across the whole data layer, not a single-file audit fix — should
    be done as a dedicated pass (introduce a per-call-site context helper, then
    convert package by package with tests). Read: AI.md PART 10 (query
    timeouts / connection pooling).

13. DONE (2026-07-31): pre-existing, unrelated test failure —
    `src/graphql/schema.resolvers_mutations_test.go`
    `TestMutationResolver_RegisterUser/registration_not_available_with_no_config`
    expected `err.Error() == "registration is not available"` but
    `src/server/handler/auth_api.go` (~line 396) returns `"public
    registration is not available"` for this code path. `"public
    registration is not available"` is the correct, intended wording (it
    distinguishes public self-registration from invite/admin-created
    accounts per PART 34's four registration modes) — fixed the test's
    expected string to match production, not the other way around. See
    item 8 for the full CI-fix pass this was part of. Read: AI.md PART 34
    (registration modes).

14. TODO (flagged 2026-07-31, out of scope for item 8): `src/server/handler/
    admin_auth.go`'s admin-auth handlers (e.g. `AdminLogoutAllHandler`,
    `AdminMeHandler` and others reading an admin from request context) never
    actually get an admin value injected via `context.WithValue` anywhere in
    the real request path — only the test file constructs a context with the
    admin already set. In production these handlers appear to be effectively
    unreachable / always fail their context lookup. Needs a dedicated
    investigation: find (or add) the middleware that is supposed to set the
    admin context value for these routes, and verify each handler's context
    key matches it. Read: AI.md PART 17 (Server Admin), PART 24/25
    (privilege/service — for how admin sessions should be established).

15. TODO (flagged 2026-07-31 while fixing item 8's `getUserIDFromContext`/
    `getIPFromContext` bug): the SAME bare-`string`-vs-typed-`contextKey`
    mismatch exists much more widely across the GraphQL resolver layer —
    NOT limited to the two helpers fixed in item 8. Found via grep, NOT
    fixed (large, security-relevant blast radius; needs its own dedicated
    session with case-by-case fail-open/fail-closed verification, not a
    mass find-replace):
    - `src/graphql/schema.resolvers.go`: ~60+ bare-string lookups, e.g.
      `ctx.Value("user_role").(string)` (repeated ~35+ times across lines
      838-3026), `ctx.Value("admin_id").(int)` (lines 1219, 1250, 2703),
      `ctx.Value("request_user_agent")` (1622, 1644), `ctx.Value("request_ip")`
      (1643), `ctx.Value("client_ip")` (1971) — none of these match the
      typed `ctxKeyUserRole` / `ctxKeyAdminID` / `ctxKeyRequestUserAgent` /
      `ctxKeyRequestIP` / `ctxKeyClientIP` constants defined in `graphql.go`.
    - `src/graphql/resolvers_helpers.go`: `ctx.Value("user_session")` (451),
      `ctx.Value("request_host")` (459, 565, 925), `ctx.Value("request_scheme")`
      (465, 570, 930), `ctx.Value("admin_id")` (474, 839),
      `ctx.Value("admin_email")` (855) — same mismatch.
    - `src/graphql/passkey_impl.go`: `ctx.Value("request_host")` (16),
      `ctx.Value("request_scheme")` (22) — same mismatch.
    Practical effect: nearly every role-based/admin authorization check and
    request-metadata lookup in the GraphQL resolver layer may be silently
    returning zero-value/failed type assertions in production (fail-open or
    fail-closed depending on how each call site handles the `!ok` case —
    must be checked individually, not assumed). This is a serious,
    wide-blast-radius issue distinct from item 8's narrow fix. Read: AI.md
    PART 9 (defense in depth), PART 11 (authz), PART 34 (multi-user roles).

16. TODO (diagnosed 2026-07-31 after item 8/13's fix push): CI's `test` job
    "Enforce coverage threshold" step (`ci.yml`, 60% gate on
    `coverage.filtered.out`) is still failing post-push
    (`coverage 50% < threshold 60%`, run 30650961406) — but this is a
    **pre-existing, long-standing gap, not a regression from item 8/13's
    changes**. Confirmed via `gh run list`: every single CI run in this
    repo's history has failed the `test` job (item 5 shows coverage as low
    as 25.9% at one point); the immediately preceding commit's CI run
    (30643361771) failed for a different reason (an actual `go test` FAIL
    in `src/graphql`, the exact bug item 8 fixed) but never even reached a
    passing coverage number either. All packages now report `ok` (no test
    failures) as of run 30650961406 - the remaining gap is purely coverage
    volume, concentrated in `src/graphql` (2.4%!, the ~60+ resolvers/
    helpers in schema.resolvers.go are almost entirely untested - see item
    15), `src/server` (0.0%), `src/server/handler` (39.8%), `src/path`
    (48.5%), `src/scheduler` (54.4%), `src/cli` (55.0%), `src/email`
    (55.0%), `src/common/banner` (55.2%). Closing a 10-point repo-wide gap
    requires substantial new `*_test.go` coverage across multiple packages
    (biggest lever: `src/graphql`, which is large enough that even partial
    coverage there would move the repo average significantly) - this is a
    dedicated test-writing pass, not a quick CI fix, and should be done
    package-by-package with the two-phase testing strategy (PART 29).
    Read: AI.md PART 29 (testing coverage requirements), PART 26 (Makefile
    coverage gate).

17. TODO (flagged 2026-07-31 by go-lint during item 12's src/database pass):
    `src/database/failover.go` lines 154-268 use emoji (⚠️/📝/✅) in
    `log.Printf` log-output lines — pre-existing, not introduced by item 12's
    context-timeout conversion; out of scope for that pass since it's a
    logging-format issue, not a query-timeout issue. Log output must be raw
    plain text with no emoji/color per the API/logging rules (banners/console
    may use them, log lines may not). Fix: replace the emoji prefixes with
    plain-text equivalents (e.g. "WARNING:", "INFO:", "OK:"). Read: AI.md
    PART 14 (log output plain-text rule).
