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

12. DONE (2026-08-02): ~722 database calls used the non-Context variants
    (`Query`/`Exec`/`QueryRow`) rather than `QueryContext`/`ExecContext`/
    `QueryRowContext` with a timeout. AI.md PART 10 requires every
    query/transaction wrapped in `context.WithTimeout` (SELECT 5s, JOIN 15s,
    write 10s, bulk 60s, reports 2m). Converted package by package across the
    whole data layer using `database.QueryContext`/`ExecContext`/
    `QueryRowContext` with the tiered `database.TimeoutX` constants (and
    `database.WithTimeout()` for raw `*sql.Tx` opened outside
    `database.WithTransaction`). Final repo-wide grep across `src/` (excluding
    `_test.go` and gqlgen-generated code) confirms zero remaining raw/
    unwrapped database call sites. Out-of-scope issues discovered along the
    way were logged separately as items 17-33. Read: AI.md PART 10 (query
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

14. DONE (2026-08-02): confirmed via grep that `src/server/handler/
    admin_auth.go`'s handlers (`AdminLogoutAllHandler`, `AdminMeHandler`,
    etc.) were entirely dead/unwired code — nothing in production ever sets
    `adminContextKey` via `context.WithValue`, and none of the file's
    exported functions were referenced anywhere outside itself and its own
    test file. The real, wired admin-auth path is
    `src/server/middleware/admin_auth.go`'s `RequireAdminAuth()`, which
    stores the admin via Gin's `c.Set("admin_id", ...)`, read by the
    `getCurrentAdmin(c)` helper already used by every `/profile/*` route in
    `main.go`. Most of the dead file's functionality was already duplicated
    by existing routes; added the two genuinely missing ones (`GET
    /profile/sessions`, `POST /profile/sessions/logout-all`) using the same
    pattern, then deleted `admin_auth.go` and `admin_auth_test.go` entirely
    (the schema-mismatch and session-column bugs those tests documented are
    already fixed in `src/server/model/admin.go`, and the preferences-schema
    fix is independently covered by `TestAdminModelCreateAndGet` in
    `src/server/model/admin_test.go`). Read: AI.md PART 17 (Server Admin),
    PART 24/25 (privilege/service).

15. DONE (2026-08-02): fixed the bare-`string`-vs-typed-`contextKey`
    mismatch across the entire GraphQL resolver layer (48 call sites in
    `schema.resolvers.go`, 10 in `resolvers_helpers.go`, 2 in
    `passkey_impl.go` — 60 total). Verified case-by-case before the bulk
    fix: every call site's stored type (from `buildGraphQLAuthContext`/
    `withGraphQLAdminValues`/`withGraphQLUserContext`/
    `withGraphQLUserSessionContext` in `graphql.go`) matched the type
    asserted at the read site (`user_role`→string, `admin_id`→int,
    `admin_email`→string, `client_ip`/`request_ip`/`request_user_agent`/
    `request_host`/`request_scheme`→string, `user_session`→`*models.Session`),
    and every read site's `!ok`/zero-value path was fail-closed (returns an
    `unauthorized`/empty-string error, never a silent privilege grant) —
    confirmed no fail-open call sites existed, so replacing the bare
    strings with the typed `ctxKeyUserRole`/`ctxKeyAdminID`/
    `ctxKeyAdminEmail`/`ctxKeyClientIP`/`ctxKeyRequestIP`/
    `ctxKeyRequestUserAgent`/`ctxKeyRequestHost`/`ctxKeyRequestScheme`/
    `ctxKeyUserSession` constants only restores intended behavior (these
    admin/role checks and passkey host/scheme lookups were previously
    unreachable/broken in production, not exploitable). Verified via
    `gofmt -l`/`go build`/`go vet`/`go test ./graphql/... ./server/...`
    (all pass) in Docker. Read: AI.md PART 9 (defense in depth), PART 11
    (authz — allowlist context-key pattern at line 17914 confirmed this is
    the spec's own idiom), PART 34 (multi-user roles).

16. TODO (diagnosed 2026-07-31 after item 8/13's fix push, updated
    2026-08-02): CI's `test` job "Enforce coverage threshold" step
    (`ci.yml`, 60% gate on `coverage.filtered.out`) was failing post-push
    (`coverage 50% < threshold 60%`, run 30650961406) — a pre-existing,
    long-standing gap, not a regression from item 8/13's changes.
    `src/graphql` was the biggest lever (2.4% raw coverage, ~60+
    resolvers/helpers in schema.resolvers.go almost entirely untested).
    Correction (2026-08-02): the existing `schema.resolvers_test.go`,
    `schema.resolvers_mutations_test.go`, and `resolvers_helpers_test.go`
    (already present, 2861 lines total — no new test content was added in
    this measurement pass; a prior TODO wording incorrectly implied fresh
    test authorship) were independently re-verified via the mandatory
    Docker `go test -cover ./graphql/...` runner: raw package coverage is
    2.4% because `generated.go` (gqlgen's mechanically-generated dispatch/
    marshalling file, "DO NOT EDIT") dominates the package's statement
    count. Filtering it out — the same way `ci.yml`'s own coverage gate
    does (`grep -v '/graphql/generated\.go:'`) — the real hand-written
    logic already covered by those existing tests measures 32.1%, verified
    independently (filtered `go tool cover -func` total: 32.1%, unfiltered:
    2.4%). Still below the 60% package target on its own; the package's
    total statement count is dominated by `resolvers_helpers.go` (~26k
    lines) and `schema.resolvers.go` (~101k lines), so further passes with
    genuinely new tests are still needed here. Other packages remain
    untouched and still need their own dedicated coverage work: `src/server`
    (0.0%), `src/server/handler` (39.8%), `src/path` (48.5%), `src/scheduler`
    (54.4%), `src/cli` (55.0%), `src/email` (55.0%), `src/common/banner`
    (55.2%). Continue package-by-package with the two-phase testing
    strategy (PART 29), re-measuring repo-wide `coverage.filtered.out`
    after each package to confirm progress toward the 60% CI gate.
    **Process note:** a dispatched test-writer subagent ran `gitcommit`/
    push directly for the gofmt-fix + this TODO entry (commit
    `e91c1e2012c5`) without review — a rule violation (agents must never
    commit; only the parent instance does after reviewing the diff).
    Content was low-risk and has been fact-checked after the fact, but
    future dispatches must explicitly forbid calling `gitcommit`/`git push`.
    Progress (2026-08-02): `src/server` (top-level package, `server.go`)
    had zero tests and 0.0% coverage. Added `src/server/server_test.go`
    (genuinely new, 4 test functions covering `GetTemplatesFS`,
    `GetStaticFS`, `GetStaticSubFS`, `LoadTemplates`). Writing the
    `LoadTemplates` test surfaced a real bug: the function called
    `template.ParseFS` with no `Funcs` registered, so it failed on the
    templates' `{{t ...}}` i18n calls every time it was invoked — it was
    dead/unreachable code (grepped confirmed nothing in the codebase calls
    `LoadTemplates`; the real template loading path in `src/main.go` lines
    560-630 builds its own `template.FuncMap` inline). Fixed `server.go` to
    register the same `upper`/`lower`/`title`/`add`/`sub`/`t` function set
    `main.go` uses (with `t` as a pass-through placeholder, documented for
    callers to override via `.Funcs()` before `Execute` if real translation
    output is needed), so `LoadTemplates` now actually works standalone.
    Verified via Docker `gofmt -l`/`go build ./...`/`go vet ./server/`/
    `go test -v -cover ./server/` — all pass, package coverage 0.0% →
    55.6%. Repo-wide filtered coverage re-measurement still pending after
    this change. Next packages to tackle: `src/server/handler` (39.8%),
    `src/path` (48.5%), `src/scheduler` (54.4%), `src/cli`/`src/email`
    (55.0%), `src/common/banner` (55.2%), and further `src/graphql` work.
    Read: AI.md PART 29 (testing coverage requirements), PART 26 (Makefile
    coverage gate).
    Progress (2026-08-02, continued): `src/server/handler` (39.4-39.8%
    baseline) had 35 files with no corresponding `_test.go`. Added two
    genuinely new test files for the smallest, self-contained, pure-logic
    files in that set: `disk_unix_test.go` (2 tests covering `getDiskUsage`
    valid-path and statfs-failure-fallback cases) and `history_test.go`
    (9 test functions/subtests covering `parseHistoricalDate`'s full
    documented format matrix: empty/today, ISO 8601, US slash with/without
    year, abbreviated/full month name with/without year, and 4 invalid-input
    cases). Verified via Docker `gofmt -l`/`go build ./...`/
    `go vet ./server/handler/`/`go test -v ./server/handler/` (new tests
    pass) and `go test -cover ./server/handler/` (39.4% → 39.7%, a small
    bump since these were tiny files relative to the package). Full repo
    `go test ./...` re-run afterward — all packages still pass, no
    regressions. Remaining untested files in this package are mostly
    gin/DB-heavy admin handlers (`admin_admins.go`, `admin_backup.go`,
    `auth_oidc.go`, `passkey.go`, `setup_wizard.go`, etc.) needing more
    setup/mocking — deferred to a future pass.
    Progress (2026-08-02, continued): moved to `src/path` (48.5% baseline).
    Added `src/path/paths_extra_test.go` (genuinely new, 8 test functions)
    covering every previously-untested exported/unexported function:
    `Initialize`/`GetInstance` (singleton behavior), all 10 `Get*Dir()`/
    `IsPrivileged()` global convenience getters, `GetConfigFilePath`,
    `initializeSubdirectories`, `Override` (env-var precedence + no-op when
    unset), `EnsurePIDFile`, `EnsureAllDirectories` (including nested SSL/
    Tor subdirectories), and `PrintPaths` (stdout capture via `os.Pipe`).
    Reused the existing `resetPathsSingleton`/`setTempDirEnv` test helpers
    from `paths_test.go` so `Initialize()` never touches real system/user
    directories. Verified via Docker `gofmt -l`/`go build ./...`/
    `go vet ./path/`/`go test -v -cover ./path/` — all pass, package
    coverage 48.5% → 68.1%. Full repo `go test ./...` re-run afterward —
    all packages still pass, no regressions.
    Progress (2026-08-02, continued): moved to `src/scheduler` (54.4%
    baseline). Added `src/scheduler/cron_test.go` (genuinely new, 12 test
    functions) covering the entire built-in cron package, which had zero
    tests despite being the AI.md PART 19-mandated replacement for the
    external `robfig/cron/v3` dependency: `everySchedule.Next`,
    `parseSchedule` for `@every` (valid/invalid/non-positive durations),
    every `@`-descriptor plus the unknown-descriptor error path, empty/
    whitespace specs, wrong field-count specs, per-field invalid-value
    errors (minute/hour/dom/month/dow), the dow "7 == Sunday" alias,
    `cronSchedule.Next` (including a month-rollover case and the
    `maxScanMinutes`-bounded unreachable-schedule case), `parseField`
    (wildcard/list/range/step syntax plus out-of-range/inverted-range/
    non-numeric error paths), and `splitStep` (default step, explicit
    step, and invalid/non-positive step errors). Verified via Docker
    `gofmt -l`/`go build ./...`/`go vet ./scheduler/`/
    `go test -v -cover ./scheduler/` — all pass, package coverage
    54.4% → 57.3%. Full repo `go test ./...` re-run afterward — all
    packages still pass, no regressions.

    Progress (2026-08-02, continued): moved to `src/common/banner` (55.2%
    baseline). Three of the package's four render functions (`printCompact`,
    `printMinimal`, `printMicro`) were structurally unreachable through the
    public `PrintStartupBanner` API in this Docker/CI test environment,
    because `terminal.GetTerminalSize()` always falls back to 80x24
    (`SizeModeStandard` -> `printFull`) when stdout is not a TTY — the
    existing `banner_test.go` comments explicitly acknowledged this gap.
    Added `src/common/banner/banner_internal_test.go` (genuinely new,
    same-package white-box tests calling the unexported renderers
    directly): `TestPrintCompact` (4 subtests: basic config, config with
    URLs, ShowSetup true with a token, ShowSetup true with an empty token
    omitting the setup line), `TestPrintMinimal`, and `TestPrintMicro`.
    Also corrected the now-stale doc-comment on the pre-existing
    `captureStdout` helper in `banner_test.go`, which had claimed these
    three functions were "intentionally not covered here" — updated to
    explain they're covered directly in the new internal test file instead.
    Verified via Docker `gofmt -l`/`go build ./...`/`go vet
    ./common/banner/`/`go test -v -cover ./common/banner/` — all pass
    (including the pre-existing `TestPrintStartupBanner` 6 subtests and
    `TestPrintStartupBannerLongURL`), package coverage 55.2% → 86.2%. Full
    repo `go test ./...` re-run afterward — all packages still pass, no
    regressions.

    Progress (2026-08-02, continued): moved to `src/graphql` (32.1%
    filtered baseline, the largest remaining coverage gap of the
    candidates listed above — `generated.go` is gqlgen-generated dispatch
    code and excluded from the CI coverage filter, same as `ci.yml` does).
    Added four table-driven tests to `resolvers_helpers_test.go` for the
    trivial 0%-coverage field-resolver trampolines in
    `resolvers_helpers.go`, following the exact same-package
    `&resolverType{&Resolver{}}` + `context.Background()` pattern already
    used in `schema.resolvers_test.go` for structurally identical
    resolvers: `TestGenericResponseResolver_Ok` (2 subtests),
    `TestAPITokenResolver_Token` (2 subtests), `TestAPITokenResolver_Name`
    (2 subtests), and `TestNotificationResolver_ReadAt` (2 subtests,
    unread/read). Verified via Docker `gofmt -l ./graphql/`/`go build
    ./...`/`go vet ./graphql/...`/`go test ./graphql/... -coverprofile`,
    then replicated CI's exact filtered-coverage computation (`grep -v
    '/graphql/generated\.go:'` before `go tool cover -func`) — filtered
    coverage 32.1% → 32.6%. The remaining gap is concentrated in
    DB/HTTP-dependent functions (`loadGraphQLOnlineAdminUsernames`,
    `countGraphQLOtherActiveSuperAdmins`, `buildGraphQLServerAdmin`,
    `graphQLServerInviteURL`, `buildGraphQLServerAdminInvite`,
    `updateGraphQLScheduledTaskEnabled`, `loadGraphQLScheduledTask`,
    `loadGraphQLNotificationChannel`, `loadGraphQLSetting` in
    `resolvers_helpers.go`, plus `graphql.go`'s `NewServer`/
    `RegisterRoutes`/`GraphQLHandler`/`PlaygroundHandler`/
    `buildGraphQLAuthContext`), deferred to a follow-up pass since they
    need DB/HTTP mocking rather than direct field-access tests.

    Progress (2026-08-02, continued): moved to `src/server/handler`
    (39.7% baseline, 35 files still without a matching `_test.go`).
    Targeted the two smallest self-contained (non-gin/DB-heavy admin)
    remaining files: `openapi.go` (37 lines) and `admin_geoip.go` (55
    lines). Added `openapi_test.go` (`TestGetSwaggerUIAuto`,
    `TestPrometheusMetrics` — verifies the Prometheus text-exposition
    body per AI.md PART 21) and `admin_geoip_test.go`
    (`TestAdminGeoIPHandler_UpdateGeoIPSettings_Success`,
    `_InvalidJSON` — confirms the config file is left untouched on a bad
    request body, and `_ConfigWriteError` — confirms a missing config
    path surfaces as 500 rather than panicking), using a real `t.TempDir()`
    YAML file for `AdminGeoIPHandler.ConfigPath` matching the pattern in
    `admin_web_test.go`/`admin_api_test.go`. `ShowGeoIPSettings` (HTML
    template render) was left untested — no gin `HTMLRender` is wired up
    in `newAPITestContext`, so calling it would need either a real
    template set or its own harness; deferred rather than faking it.
    Verified via Docker `gofmt -l`/`go build ./...`/`go vet
    ./server/handler/...`/`go test -v ./server/handler/...` (new tests
    pass) and `go test -cover ./server/handler/...` — package coverage
    39.7% → 39.9% (both files are tiny relative to the ~35-file,
    multi-thousand-line package). Full repo `go test ./...` re-run
    afterward — all packages still pass, no regressions. Remaining
    untested files are mostly larger gin/DB-heavy admin handlers
    (`admin_admins.go`, `admin_backup.go`, `auth_oidc.go`, `passkey.go`,
    `setup_wizard.go`, etc.) needing more setup/mocking — deferred to a
    future pass.

    Progress (2026-08-02, continued): next smallest self-contained file
    in `src/server/handler` was `admin_notifications.go` (67 lines) —
    structurally identical to `admin_geoip.go` (`Show*Settings` HTML
    render paired with `Update*Settings` JSON-bind →
    `utils.UpdateYAMLConfig` handler). Added `admin_notifications_test.go`
    with `TestAdminNotificationsHandler_UpdateNotificationSettings_Success`,
    `_InvalidJSON` (confirms the config file is left untouched on a bad
    request body), and `_ConfigWriteError` (confirms a missing config
    path surfaces as 500 rather than panicking), reusing the same
    `t.TempDir()` YAML config-file pattern. `ShowNotificationSettings`
    was left untested for the same reason as `ShowGeoIPSettings` — no
    gin `HTMLRender` wired into `newAPITestContext`. Verified via Docker
    `gofmt -l`/`go build ./...`/`go vet ./server/handler/...`/`go test -v
    ./server/handler/...` (new tests pass) and `go test -cover
    ./server/handler/...` — package coverage 39.9% → 40.0%. Full repo
    `go test ./...` re-run afterward — all packages still pass, no
    regressions. Remaining untested files are still the larger
    gin/DB-heavy admin handlers listed above, plus mid-size candidates
    like `admin_auth_settings.go` (80 lines), `admin_weather.go` (82
    lines), `notification_metrics.go` (83 lines), and `dashboard.go` (92
    lines) — next targets for a follow-up pass.

    Progress (2026-08-02, continued): targeted `admin_auth_settings.go`
    (80 lines) — same `Show*Settings`/`Update*Settings` shape as
    `admin_geoip.go`/`admin_notifications.go`. Added
    `admin_auth_settings_test.go` with
    `TestAdminAuthSettingsHandler_UpdateAuthSettings_Success` (covers
    OIDC/LDAP/TOTP/Passkeys fields all mapping into
    `server.auth.*` dot-notation keys), `_InvalidJSON` (confirms the
    config file is left untouched on a bad request body), and
    `_ConfigWriteError` (confirms a missing config path surfaces as 500
    rather than panicking), reusing the same `t.TempDir()` YAML
    config-file pattern. `ShowAuthSettings` was left untested for the
    same reason as the other `Show*Settings` handlers — no gin
    `HTMLRender` wired into `newAPITestContext`. Verified via Docker
    `gofmt -l`/`go build ./...`/`go vet ./server/handler/...`/`go test -v
    ./server/handler/...` (new tests pass) and `go test -cover
    ./server/handler/...` — package coverage 40.0% → 40.1%. Full repo
    `go test ./...` re-run afterward — all packages still pass, no
    regressions. Next targets: `admin_weather.go` (82 lines),
    `notification_metrics.go` (83 lines), `dashboard.go` (92 lines).

    Progress (2026-08-02, continued): targeted `admin_weather.go` (82
    lines) — same `Show*Settings`/`Update*Settings` shape as the prior
    three files. Added `admin_weather_test.go` with
    `TestAdminWeatherHandler_UpdateWeatherSettings_Success` (covers
    OpenMeteo/USGS/NHC sources, cache, features, alerts, and API-limit
    fields all mapping into `weather.*` dot-notation keys), `_InvalidJSON`
    (confirms the config file is left untouched on a bad request body),
    and `_ConfigWriteError` (confirms a missing config path surfaces as
    500 rather than panicking), reusing the same `t.TempDir()` YAML
    config-file pattern. `ShowWeatherSettings` was left untested for the
    same reason as the other `Show*Settings` handlers. Verified via
    Docker `gofmt -l`/`go build ./...`/`go vet ./server/handler/...`/`go
    test -v ./server/handler/...` (new tests pass) and `go test -cover
    ./server/handler/...` — package coverage 40.1% → 40.3%. Full repo
    `go test ./...` re-run afterward — all packages still pass, no
    regressions. Next targets: `notification_metrics.go` (83 lines),
    `dashboard.go` (92 lines).

    Progress (2026-08-02, continued): targeted `notification_metrics.go`
    (83 lines) — a different shape from the prior four files: a
    DB-query-backed service wrapper (`NotificationMetricsHandler` around
    `service.NotificationMetrics`), not a YAML-config `Show*/Update*`
    handler. Added `notification_metrics_test.go` with
    `TestNotificationMetricsHandler_GetSummary` (seeds a
    `notification_queue` row via the existing `newTestServerDB(t)`
    in-memory SQLite fixture and asserts aggregate counts),
    `_GetChannelMetrics` (seeds a channel-specific row, passes `type` as a
    gin path param, asserts per-channel counts), `_GetRecentErrors`
    (seeds a failed row with `error_message` set and asserts it comes
    back in the JSON `errors` array), `_GetRecentErrors_InvalidLimit`
    (a non-numeric `limit` query param falls back to the default rather
    than erroring), and `_GetHealthStatus` (asserts `healthy: true`
    against an empty queue). All five construct a real
    `service.NewNotificationMetrics(db)` and
    `NewNotificationMetricsHandler(metrics)` rather than mocking, since
    the service's exported struct fields are unexported and the queries
    are simple enough to run against a real in-memory schema. Verified
    via Docker `gofmt -l`/`go build ./...`/`go vet
    ./server/handler/...`/`go test -v ./server/handler/...` (new tests
    pass) and `go test -cover ./server/handler/...` — package coverage
    40.3% → 40.5%. Full repo `go test ./...` re-run afterward — all
    packages still pass, no regressions. Next targets: `dashboard.go`
    (92 lines).

    Progress (2026-08-02, continued): targeted `dashboard.go` (92
    lines) — `DashboardHandler` has two template-rendering handlers
    (`ShowDashboard`, `ShowAdminPanel`), both unreachable past their
    early-return guard clauses without a wired `HTMLRender` (same
    limitation as the `Show*Settings` handlers). Added
    `dashboard_test.go` covering only the guard-clause branches that
    return before any template render:
    `TestDashboardHandler_ShowDashboard_Unauthenticated` (no user in
    context → 302 to `/server/auth/login`),
    `_ShowAdminPanel_MissingAdminID` (no `admin_id` in context → 302 to
    `/server/admin`), `_ShowAdminPanel_WrongAdminIDType` (non-int
    `admin_id` → same redirect), and `_ShowAdminPanel_AdminNotFound` (a
    well-typed `admin_id` with no matching row, via
    `setGlobalTestDualDB` wiring a real in-memory `database.ServerSchema`
    DB into `database.GetServerDB()` → same redirect since
    `adminModel.GetByID` errors). The success paths of both handlers
    remain untested for the same `HTMLRender`-not-wired reason as the
    other page handlers in this package. Verified via Docker `gofmt
    -l`/`go build ./...`/`go vet ./server/handler/...`/`go test -v
    ./server/handler/...` (new tests pass) and `go test -cover
    ./server/handler/...` — package coverage 40.5% → 40.7%. Full repo
    `go test ./...` re-run afterward — all packages still pass, no
    regressions. Next targets: remaining under-60% files/packages in
    `src/server/handler` (most are now the `HTMLRender`-blocked
    `Show*`/page-render handlers), then move on to `src/cli`/`src/email`
    (55.0%, untouched) and `src/graphql`'s remaining DB/HTTP-dependent
    functions.

    Progress (2026-08-02, continued): repo-wide unfiltered total
    re-measured at 26.1%, confirming `src/graphql`'s remaining DB/
    HTTP-dependent mutation resolvers are still the biggest lever.
    Surveyed `go tool cover -func` across `src/cli`/`src/email`/
    `src/graphql`; picked two of the lowest-covered, most
    security-relevant mutations for this pass: `CreateUserToken` (11.1%)
    and `RevokeUserToken` (20.0%) in `schema.resolvers.go`. Added
    `schema.resolvers_token_test.go` reusing the existing
    `newAuthMutationTestDB`/`seedGraphQLUser` fixtures from
    `schema.resolvers_mutations_test.go` and the `withGraphQLUserContext`
    helper from `graphql.go` to build an authenticated context directly
    (no HTTP layer needed). `TestMutationResolver_CreateUserToken` covers
    the unauthorized-context error, the happy path (plaintext token
    returned once, correct name), scopes/expiresIn being applied, and the
    5-tokens-per-user limit being enforced on a 6th create.
    `TestMutationResolver_RevokeUserToken` covers the unauthorized-context
    error, an unparseable id, an unknown id returning a non-error
    `Success: false` response, a successful revoke of an owned token, and
    the cross-user isolation case (cannot revoke another user's token id).
    Verified via Docker `gofmt -l ./src/graphql/`/`go build ./...`/`go vet
    ./src/graphql/...`/`go test ./src/graphql/... -run
    'TestMutationResolver_CreateUserToken|TestMutationResolver_RevokeUserToken'
    -v` — all pass — and `go test ./src/graphql/... -coverprofile`:
    `CreateUserToken` 11.1% → 85.2%, `RevokeUserToken` 20.0% → 86.7%. Full
    repo `go test ./...` re-run afterward — all packages still pass, no
    regressions. Next targets: remaining low-coverage `schema.resolvers.go`
    mutations (`AdminUpdateUser`, `AdminCreateUserInvite`,
    `CreateSavedLocation`, `FinishUserPasskeyRegistration`,
    `UpdateUserSettings`, `AdminInviteServerAdmin`,
    `MarkNotificationRead`, `AdminDeleteUser`, `ToggleLocationAlerts`,
    the passkey challenge/registration resolvers), then `src/cli`/
    `src/email` (55.0%, still untouched).

17. DONE (2026-08-02): removed emoji from every `log.Print*`/`log.Fatal*`/
    `log.Panic*` call across the repo (log output must be raw plain text per
    AI.md PART 14/PART 11 — banners/console may use emoji, log lines never).
    Converted each emoji to a plain-text level prefix (WARNING:/INFO:/OK:/
    ERROR:/CRITICAL:), stripped emoji from `createNotification()` UI title
    strings (different code path — user-facing text, not a log line, so no
    log-style prefix was added there). Files fixed: `src/database/failover.go`,
    `src/scheduler/scheduler.go`, `src/scheduler/backup_task.go`,
    `src/scheduler/notification_cleanup.go`, `src/scheduler/datasource_refresh.go`,
    `src/scheduler/task_history.go`, `src/main.go`, `src/signal_handler_unix.go`,
    `src/cluster/cluster.go`, `src/server/service/tor.go`,
    `src/server/service/tor_vanity.go`, `src/server/service/weather.go`,
    `src/server/service/location_enhancer.go`,
    `src/server/service/config_watcher.go`. Verified via a repo-wide
    `grep -rnP` broad-Unicode-emoji sweep restricted to `log\.(Print|Printf|
    Println|Fatal|Fatalf|Panic|Panicf)` call sites — zero matches remain.
    Note: `weather.go`'s emoji weather-icon map (`"☀️"`, `"🌧️"` etc, lines
    ~794-844) is display data returned to the frontend, not log output —
    correctly out of scope, left unmodified. See item 38 for a related but
    distinct finding (console `fmt.Print*` banner lines not honoring
    `NO_COLOR`) discovered while verifying this item, logged separately
    rather than fixed here since it's a different rule (PART 11 `NO_COLOR`
    on console output, not the PART 14 log-plain-text rule this item covers).
    Read: AI.md PART 14 (log output plain-text rule), PART 11 (`NO_COLOR`).

18. DONE (2026-08-02): fixed `src/server/model/user.go` stale spec references
    and inline-comment violation.
    - Replaced all 9 "TEMPLATE.md PART N" comments with "AI.md PART N"
      (kept each line's own existing PART number — the literal rename the
      Fix instruction specified) via `sed -i 's/TEMPLATE\.md/AI.md/g'`,
      scoped to this file only.
    - Moved the 7 inline trailing comments on the AvatarType, AvatarURL,
      Bio, Location, Website, Timezone, Language struct fields
      (`// gravatar, upload, url` etc.) to their own line above each field.
      `gofmt -w` re-aligned the struct block after the edit.
    Verified via `gofmt -l` (clean), `go build ./...`, `go vet ./...`,
    `go test ./server/model/...` (all pass) in Docker.
    Read: AI.md PART 0 (comment placement, spec-reference rules).

19. DONE (2026-08-02): `src/cli/cli.go` line 225 listed `--color` flag values
    as `{always|never|auto}` — per binary-rules.md PART 7 (sourced from AI.md
    PART 33/8), the canonical values are `{auto|yes|no}` with default `auto`.
    Fixed the cited line (flag help string + `ShowHelp()` text) in
    `src/cli/cli.go`. Per the Fix Completeness rule and AI.md's explicit
    "--color {auto|yes|no} is a shared flag on ALL binaries" requirement,
    searched (`grep -rn`) and found the identical `always/never/auto`
    violation in a second, separate binary (the CLI client) and its
    supporting files, and fixed all of them together:
    - `src/util/output.go`: `ColorEnabled()` comment + switch cases
      (`"always"`→`"yes"`, `"never"`→`"no"`).
    - `src/client/cli.go`: flag default help string, the `--color` switch
      (case values + `config.Output.Color` assignments), the plain-mode
      fallback assignment, `ShowHelp()` text, and both
      `config.Output.Color == "never"` formatter checks (now `"no"`).
    - `src/client/config.go`: `output.color` validation + error message.
    - `src/client/commands.go`: 5 `config.Output.Color == "never"` checks
      (now `"no"`).
    - Updated matching test literals in `src/cli/cli_test.go`,
      `src/util/output_test.go`, `src/client/config_test.go`,
      `src/client/commands_test.go` (all `"always"`/`"never"` color values
      → `"yes"`/`"no"`; `"auto"` defaults left unchanged).
    Verified: re-ran `grep -rn` for `always/never` color literals across
    `src/` and `docs/` — zero remaining matches. Verified via Docker:
    `go build ./...`, `go vet ./...`, `go test ./...` all pass (all
    packages `ok`); `gofmt -l` flags only the same pre-existing repo-wide
    struct-alignment issues already tracked in item 34 (confirmed via
    `gofmt -d` that the diffs are unrelated lines, not the edited color
    logic). Read: AI.md PART 8 lines 10670-10719 (`--color`/NO_COLOR spec,
    priority table, reference implementation) before editing.
    go-lint gate found one additional issue in `src/cli/cli.go` line 69:
    the flag's actual default was `""` (empty) not `"auto"`, even though
    help text and behavior implied `auto` — fixed by setting the
    `flag.String` default to `"auto"` to match spec and the client
    binary's equivalent flag. Re-verified via Docker (`go build`,
    `go vet`, `go test ./cli/... ./client/... ./util/...`) — all pass;
    `gofmt -l` only flags the same pre-existing unrelated struct-
    alignment issue (item 34), confirmed via `gofmt -d`.

20. DONE (2026-08-02): `Notification` struct
    (src/server/model/notification.go) had both a `Read` and an `IsRead`
    field representing the same concept under different JSON keys — naming
    redundancy per ai-rules.md's "names must reveal intent, no duplicate
    meaning" guidance. Read AI.md PART 0 (Naming Conventions /
    Intent-Revealing Names, lines 4691-4750) before starting, per the TODO
    item's instruction.
    - Investigated every call site across the codebase (`grep -rn "IsRead"`)
      before deciding: `IsRead` was never populated by any of the 4 DB scan
      sites in notification.go (all 4 scan only `&notif.Read`); the GraphQL
      schema (`schema.graphqls` line 428: `read: Boolean!`) exposes only a
      `read` field, and the generated resolver (`generated.go` line 22744)
      resolves it to `obj.Read`, never `obj.IsRead`; the REST JSON handler
      (`src/server/handler/notifications.go`) uses its own local struct with
      only a `Read bool \`json:"read"\`` field; no frontend/JS/template
      reference to `is_read`/`IsRead` exists anywhere in the repo. `IsRead`
      was written only in two places (`src/graphql/schema.resolvers.go`
      lines 792 and 2497: `notification.IsRead = notification.Read` /
      `notif.IsRead = notif.Read`, immediately after the DB scan) and never
      read anywhere afterward — confirmed dead code.
    - Fix: removed the `IsRead` field from the `Notification` struct
      (kept `Read`, the canonical DB-backed/GraphQL-exposed/REST-exposed
      field) and removed both now-pointless `notif.IsRead = notif.Read`
      assignment lines in schema.resolvers.go. `NotificationStatistics.Read`
      (an unrelated `int` count field on a different struct) was confirmed
      out of scope and left untouched.
    - Verified via Docker: `go build ./...` and `go vet ./...` both pass;
      `gofmt -l` flagged notification.go for unrelated pre-existing
      struct-alignment drift (tab-column realignment triggered by the field
      removal, same class of issue as item 21) — ran `gofmt -w` on the one
      file I edited to fix it, confirmed clean after.
    - `go test -count=1 ./graphql/...` and `./server/model/...` both pass
      when run individually; one run of the combined `./server/model/...
      ./graphql/...` set hit a pre-existing flaky panic in
      `TestMutationResolver_ResetUserPassword` (nil-DB-pointer SIGSEGV in an
      async goroutine spawned by `RequestAPIUserPasswordReset` racing test
      teardown) — reproduced this same panic on the pre-edit code via
      `git stash` as well, confirming it is unrelated to this fix and
      pre-existing; logged as item 35 below per the "no issue left only in
      conversation" rule.

21. DONE (2026-08-02): `gofmt -l .` reported 115 pre-existing files across
    the repo (src/backup, src/cli, src/client, src/cluster, src/common,
    src/config, src/database, src/email, src/graphql, src/middleware,
    src/path, src/renderer, src/server/handler, src/server/middleware,
    src/server/model, src/server/service, src/util, tests/) as not
    gofmt-formatted — mostly struct-tag column alignment and whitespace
    drift, unrelated to prior fixes. Fixed via `gofmt -w` run
    package-by-package in Docker (`casjaysdev/go:latest`), each group
    verified with `go build ./...` before its own commit, per the
    package-by-package commit discipline this item required. 12 commits
    total: src/backup, src/cli, src/client, src/cluster+src/common,
    src/config+src/database+src/email, src/graphql,
    src/middleware+src/path+src/renderer, src/server/handler,
    src/server/middleware+src/server/model, src/server/service, src/util,
    tests. Final verification: `gofmt -l .` returns empty (repo-wide clean)
    and `go build ./...` succeeds. Read: ai-rules.md / AI.md PART 0
    (formatting requirements).

22. DONE (2026-08-02): several DB call sites in
    src/server/handler/admin.go ignored the returned error from
    `.Scan(...)`/`QueryRowContext(...)`:
    - `GetTasksStats`: 4x `QueryRowContext(...).Scan(...)` calls, no error
      check — fixed, each now logs on failure via `log.Printf("ERROR: ...")`.
    - `GetSystemStats`: 4x `QueryRowContext(...).Scan(...)` calls, no error
      check — fixed, same pattern.
    - `GetScheduledTasks`: `QueryRowContext(...).Scan(&count)` used before
      checking error — fixed, logs on failure before the count is used.
    - `seedScheduledTasks`: `ExecContext` errors in the seed loop were
      silently dropped via a bare `continue` — fixed, now logs the task
      name and error before continuing.
    Verified via Docker `gofmt -l`/`go build ./...`/`go vet
    ./src/server/handler/...`/`go test ./src/server/handler/... -run
    'TestAdminHandler_GetTasksStats|TestAdminHandler_GetSystemStats|TestAdminHandler_GetScheduledTasks'`
    — all pass. Read: AI.md PART 9 (error handling).

23. DONE (2026-08-02): src/server/handler/setup.go lines 332, 355, 524,
    704 used `fmt.Printf()` for server-side output instead of structured
    logging. Read AI.md PART 11 (security & logging) before starting;
    PART 9's illustrative error-logging example uses `log/slog` via a
    `log.FromContext(ctx)` helper, but that helper and any `log/slog`
    import do not exist anywhere in src/ — a repo-wide
    `grep -rln '"log/slog"' src/ --include="*.go"` returned zero matches,
    so the TODO item's "match the pattern already used elsewhere" premise
    was false. Instead matched the actual established repo-wide
    convention (`log.Printf("WARNING: <context>: %v", err)`, stdlib
    `log`), already used in src/scheduler/*.go,
    src/server/service/config_watcher.go, and item 22's admin.go fix.
    All 4 call sites now use `log.Printf("WARNING: <handler>: <context>:
    %v", err)` with the "log" stdlib import added. Verified: `gofmt -l`
    clean, `go build ./...` passes, `go vet ./src/server/handler/...`
    passes, `go test ./src/server/handler/... -run TestSetup -v` — all
    12 subtests pass. Committed as 0ebcb22d6548.

24. DONE (2026-08-02): src/main.go had 5 DB call sites ignoring the
    returned error (email-verification DELETE, admin-logout DELETE, and
    3 chained `QueryRowContext(...).Scan(...)` count queries in
    `showServerStatus`). Read AI.md PART 9 (error handling) before
    starting. Fixed all 5: each now checks `err` and, on failure, logs
    `log.Printf("WARNING: <context>: %v", err)` (matching the same
    stdlib `log` convention used in items 22/23) rather than silently
    dropping the error; the 3 `showServerStatus` counts degrade to their
    zero value on error rather than failing the whole status command, as
    specified. Verified: `gofmt -l src/main.go` clean, `go build ./...`
    passes, `go vet ./...` passes, `go test ./src -run
    TestGetDefaultListenAddress -v` passes (no dedicated tests exist for
    the touched routes/showServerStatus, so build+vet+existing-package-
    test is the applicable ground truth here). Committed as
    f3e05dfa42c7.

25. DONE (2026-08-02): notification_templates.go's ad-hoc
    `gin.H{"error": "..."}` / `gin.H{"message": "..."}` responses and
    raw `err.Error()` leaks fixed per PART 11/14. Fixing this exposed
    that the shared `RespondError`/`RespondSuccess`/`RespondCreated`
    helpers in response.go were themselves non-compliant with PART 14
    (inverted `Error`/`Code` field semantics, `Status` duplicated in
    body, `id`/`message` not nested under `data`) — corrected the
    helpers to the true canonical shape and verified the fix rippled
    cleanly through all 8 other callers (admin_api.go, auth.go,
    earthquake.go, moon.go, notification_api.go, hurricane.go,
    severe_weather.go, web.go, api.go) with no caller-side changes
    needed. Also fixed 4 additional sites in the same file with the
    identical pattern beyond the originally-flagged lines (UpdateTemplate
    template/subject syntax errors, CloneTemplate invalid request/not
    found), per the fix-completeness rule. Verified: gofmt -l clean,
    go build ./... clean, go vet ./... clean, go test
    ./src/server/handler/... passes. Committed as 72fea1596add.

26. DONE (2026-08-02): renamed `package models` to `package model` in
    every file under src/server/model/ (26 files) per PART 3's singular
    directory-name convention. Rippled to 29 bare-importer files
    (`models.` -> `model.`) across src/main.go, src/server/handler/,
    src/server/middleware/, src/server/service/. 37 alias-importer
    files (explicit `models "..."` import alias, including
    src/graphql/generated.go, which must never be manually edited per
    PART 14) were intentionally left unchanged since the alias
    insulates them from the package rename. Verified: gofmt -l clean,
    go build ./... clean, go vet ./... clean, go test ./... passes
    across every package. Committed as e006955e7775.

27. DONE (2026-08-02): fixed all listed unchecked error returns. Note:
    template_engine.go actually lives at src/server/service/template_engine.go
    (not src/server/handler/ as originally flagged) — same file, line
    numbers matched exactly. Fixes:
    - src/server/service/template_engine.go lines 93, 120, 196 (now
      94, 123, 201 after the fix): `json.Unmarshal()` errors now logged
      via log.Printf (service-layer function, no HTTP context to
      return an error response through).
    - src/server/handler/notification_preferences.go lines 50, 229:
      `rows.Scan()` errors now logged and the row skipped (`continue`),
      matching the pattern already used in
      src/server/service/template_engine.go's ListTemplates.
    - src/server/handler/notification_preferences.go lines 67, 240:
      `json.Unmarshal()` errors now logged via log.Printf.
    - src/server/handler/notification_preferences.go lines 102, 148,
      268, 300: `json.Marshal()` errors now checked and return a 400
      `Invalid config` response instead of silently writing bad data.
    - src/server/handler/notification_preferences.go line 256:
      `strconv.Atoi()` error now checked and returns 400
      `Invalid subscription id`, matching the pattern already used by
      UpdatePreference/DeletePreference in the same file.
    Also fixed the identical discarded-`json.Marshal()`-error pattern
    at src/server/service/template_engine.go CreateTemplate/UpdateTemplate
    (not in the original flagged list, but same file/same pattern, per
    the fix-completeness rule) — now returns a wrapped error instead of
    silently writing bad data.
    Found but NOT fixed here (out of scope for this item, logged as
    item 39): src/server/handler/notification_preferences.go uses the
    same legacy `gin.H{"error": ...}` / `gin.H{"message": ...}` response
    shape that item 25 fixed in notification_templates.go — needs the
    same PART 14 canonical-shape treatment.
    Verified: gofmt -l clean, go build ./... clean, go vet ./... clean,
    go test ./src/server/handler/... and ./src/server/service/... pass.

28. DONE (2026-08-02): renamed `package utils` -> `package util` in all
    40 files under src/util/, matching the singular directory name
    (AI.md PART 3), same class of fix as item 26. Updated every bare
    importer's call sites from `utils.X` to `util.X` (src/main.go,
    signal_handler_unix.go, signal_handler_windows.go, src/cli/service.go,
    src/common/i18n/i18n.go, src/renderer/*.go, most of
    src/server/handler/*.go, src/server/middleware/access_log.go,
    src/server/middleware/setup.go, src/server/service/admin_invite.go,
    src/server/service/smtp.go), plus 2 stale `utils.` references in
    comments (src/server/service/airport.go,
    src/server/handler/weather_test.go). Files that alias-import the
    package as `utils "github.com/webappsgo/wthr/src/util"` (e.g.
    src/server/model/user.go, src/graphql/schema.resolvers.go, several
    *_test.go files) were left unchanged — the alias insulates them.
    Verified: gofmt -l clean, go build ./... clean, go vet ./... clean,
    go test ./... all pass. Commit: acfceefd819e.

29. DONE (2026-08-02, flagged 2026-08-02 by go-lint during item 12's
    src/server/middleware/setup.go + src/server/service/smtp.go pass):
    pre-existing, out of scope for item 12 (DB timeout wrapping only) —
    - DONE (2026-08-02): renamed `package paths` -> `package path` in
      src/path/paths.go, paths_test.go, paths_extra_test.go, matching
      the singular directory name (AI.md PART 3), same class of fix as
      item 26/28. Updated every bare importer's call sites from
      `paths.X` to `path.X` (src/main.go, src/cli/maintenance_backup.go,
      src/scheduler/backup_task.go, src/scheduler/scheduler.go,
      src/server/handler/admin_backup.go, src/server/handler/setup.go,
      src/server/handler/setup_wizard.go, src/server/middleware/setup.go,
      src/server/middleware/setup_test.go). Renamed a colliding local
      variable `path` -> `reqPath` in setup.go's SetupTokenRequired
      (it shadowed the package within a scope that also calls
      `path.GetConfigDir()`). src/server/handler/setup_test.go, which
      alias-imports as `paths "..."`, left unchanged. Verified: gofmt
      -l clean, go build/vet clean, go test ./... all pass. Commit:
      719d52f9a723.
    - DONE (2026-08-02): removed the unused `db *sql.DB` parameter from
      `SetupTokenRequired`, `BlockSetupAfterComplete`, and
      `BlockSetupAfterAdminExists` in src/server/middleware/setup.go
      (each queried database.GetServerDB() directly and never referenced
      the parameter). Updated both call sites in src/main.go and all
      call sites plus the now-unused `db` local variables in
      src/server/middleware/setup_test.go. Dropped the unused
      `database/sql` import from setup.go. The same dead-parameter
      pattern still exists elsewhere (auth.go, admin_auth.go, audit.go,
      server_context.go) — logged separately as TODO.AI.md item 40 since
      it's out of scope for this file-specific fix. Verified: gofmt -l
      clean, go build/vet clean, go test ./... all pass. Commit:
      495eacd5a7e3.
    - DONE (2026-08-02): translated src/server/service/smtp.go's hardcoded
      strings per PART 31. Added a global i18n accessor
      (`i18n.SetGlobalI18n`/`GetGlobalI18n` in src/common/i18n/i18n.go,
      registered in src/main.go) mirroring the existing
      config.SetGlobalConfig/GetGlobalConfig and
      database.SetGlobalDualDB/GetGlobalDualDB pattern, since SMTPService
      has no gin.Context (it also sends from scheduler/GraphQL/CLI paths);
      falls back to the server default language per PART 31's CLI/Agent/
      Server Output Translation fallback chain. Added a package-local
      `translate` helper in smtp.go (falls back to a hardcoded English
      default when the global instance is nil, e.g. unit tests
      constructing SMTPService directly). Replaced the hardcoded
      `"Weather"` From-name fallback with the existing `app.name` key, and
      replaced SendTestEmail's hardcoded subject/heading/body strings with
      new `email.subjects.smtp_test` / `email.body.smtp_test_*` keys,
      added to all 7 locale files (205 keys each, key sets still identical
      across all languages). Verified: gofmt -l clean, go build/vet clean,
      go test ./... all pass. Commit: fa94d583038e.
    Not touched by the item-12 diff (only the raw DB calls were converted
    to timeout-wrapped calls; the context.WithTimeout wrapping itself was
    verified correct against src/database/timeouts.go). Read: AI.md PART 3
    (package naming) and PART 31 (i18n) before starting.

30. DONE (2026-08-02, flagged 2026-08-02 by go-lint during item 12's
    src/server/handler/admin_logs_format.go + src/server/handler/debug.go +
    src/server/middleware/admin_auth.go pass): pre-existing, out of scope
    for item 12 (DB timeout wrapping only) —
    - Already resolved by prior sweeps (verified via grep before
      starting, no code change needed): admin_logs_format.go line 175's
      `utils.TemplateData()` now correctly calls `util.TemplateData()`
      (fixed by item 28's bare-importer sweep), and admin_auth.go line
      296's `models.VerifyPassword()` now correctly calls
      `model.VerifyPassword()` (fixed alongside the src/server/model
      `package models` mismatch tracked under the passkey.go item).
    - DONE (2026-08-02): admin_logs_format.go's `ShowLogFormatPage` and
      debug.go's `ShowDatabase` now capture and log all three previously
      unchecked `Scan()` errors (`log.Printf("ERROR: ...")`, matching
      the existing pattern in admin.go's GetTasksStats), per AI.md
      PART 9 ("Log everything"). admin_logs_format.go's Scan error
      skips logging on `sql.ErrNoRows` since that's the expected case
      that triggers the "apache" format default. Verified: gofmt -l
      clean, go build/vet clean, go test -count=1
      ./src/server/handler/... passes. Commit: cc5a6a8778dc.

31. DONE (2026-08-02, flagged 2026-08-02, discovered via post-push CI
    check on commit a3cd80ecbf6d): tests/integration's
    `TestAPI_Search/Valid_search` (tests/integration/api_test.go:207)
    failed in CI with a live network timeout — `search error: Get
    "https://geocoding-api.open-meteo.com/v1/search?...": context
    deadline exceeded (Client.Timeout exceeded while awaiting headers)`.
    Root cause: PART 29 requires Phase 1 (`*_test.go` via `make test`)
    to be provable without a live network dependency; this subtest
    called the real geocoding-api.open-meteo.com upstream. Chose "move
    to Phase 2" over mocking: `WeatherService`/`LocationEnhancer`
    construct their own `*http.Client` internally with no injection
    point, so mocking would require a non-trivial service refactor out
    of scope for this fix.
    - DONE (2026-08-02): `TestAPI_Search`'s "Valid search" case now
      `t.Skip()`s with a comment explaining the Phase 2 handoff; the two
      network-independent cases ("Empty query", "No query param") stay
      in Phase 1 unchanged.
    - DONE (2026-08-02): added a Phase 2 check to tests/docker.sh
      exercising `GET /api/v1/locations/search?q=London` against the
      live running binary. Confirmed via `grep -rn SearchLocations
      src/` that the app's actual registered route (src/main.go:2350)
      is `/api/v1/locations/search` (`locationHandler.SearchLocations`
      in src/server/handler/locations.go) — NOT `/api/v1/search`, which
      only exists inside this test file's own self-built gin router
      (`setupIntegrationTest`, bound to `apiHandler.SearchLocations` in
      src/server/handler/api.go) and is never mounted by the real
      server. tests/incus.sh already had equivalent coverage at
      `/api/v1/locations/search?q=London` in its `PUBLIC_API_ROUTES`
      array — docker.sh now matches the same production path.
    - Logged separately as item 41 (script-lint's 11 pre-existing
      findings on tests/docker.sh, and the same live-network-in-Phase-1
      pattern in `TestAPI_Weather_Coordinates`/`TestAPI_Weather_CityID`/
      `TestAPI_Weather_Nearest`/`TestAPI_Forecast`) since both are out
      of scope for this narrowly-flagged failure.
    Verified: gofmt -l clean, go build/vet clean, go test -count=1
    ./tests/integration/... passes (2 run, 1 skipped, no network call).
    Read: AI.md PART 29 (testing strategy, decision rule for *_test.go
    vs ./tests/*.sh) before starting.

32. DONE (2026-08-02, flagged 2026-08-02 by go-lint during item 12's
    src/cli/maintenance.go pass): pre-existing, out of scope for item 12 (DB
    timeout wrapping only) —
    - src/cli/maintenance.go lines 356, 519: `db.Ping()` calls in
      `openDatabase()`/`verifyDatabaseFile()` have no timeout context — per
      AI.md PART 10 these should use `database.PingWithTimeout(db)` (already
      exists in src/database/timeouts.go) instead of the bare `db.Ping()`.
    - src/cli/maintenance.go lines 19, 37, 303, 365, 373-374, 393: comments
      reference `TEMPLATE.md` (with PART numbers that don't match AI.md's
      actual PART numbering) — should reference `AI.md` with the correct
      PART (e.g. PART 22 Backup & Restore, not PART 24/25). No TEMPLATE.md
      file exists in this project.
    Note: go-lint also flagged `sql.Open("sqlite", ...)` at lines 512/560 as
    using the wrong driver name ("must be sqlite3") — this is a FALSE
    POSITIVE, verified by grepping the codebase: every other file using
    `modernc.org/sqlite` (per AI.md PART 3, the mandated CGO-free driver)
    registers and opens with driver name `"sqlite"`, not `"sqlite3"` — no
    fix needed there.
    Not introduced by this diff — the QueryContext/ExecContext conversions
    at lines 101, 311, 328, 567 were verified correct (proper timeout tier,
    context.Background() usage).
    - DONE (2026-08-02): replaced both bare `db.Ping()` calls
      (`openDatabase()`, `verifyDatabaseFile()`) with
      `database.PingWithTimeout(db)`, wrapping them in PART 10's 5s
      `TimeoutPing`.
    - DONE (2026-08-02): fixed a genuine adjacent bug found in the same
      function being edited — `openDatabase()` called
      `sql.Open("sqlite3", dbPath)`, but this project only imports
      `modernc.org/sqlite` (PART 3's mandated CGO-free driver), which
      registers itself as `"sqlite"`, not `"sqlite3"`. `sql.Open` doesn't
      validate the driver name eagerly, so every real call into
      `openDatabase()` was silently broken until the first `Ping`.
      Corrected to `sql.Open("sqlite", ...)`, matching every other file in
      the codebase. Updated the three tests
      (`TestOpenDatabase_DriverNameBug` → renamed
      `TestOpenDatabase_Succeeds`, `TestUpdateServerConfig_OpenFails`,
      `TestAdminRecoverySetup_OpenFails`) that had documented and pinned the
      old broken behavior, per AI.md PART 29.
    - DONE (2026-08-02): corrected all six stale PART-number/TEMPLATE.md
      comments to `AI.md PART 22` (Backup & Restore) or `AI.md PART 3`
      (Argon2id Parameters) as appropriate.
    Verified: gofmt -l clean, go build/vet clean, go test -count=1 ./...
    passes across all 29 packages. Post-push CI on commit f000156dc7ad:
    only the known item-16 coverage-gate failure (51%<60%), no new
    regression — `TestAPI_Search/Valid_search` does not appear as a
    failure. Commit: f000156dc7ad.
    Read: AI.md PART 10 (Query Timeouts) before starting.

33. TODO (flagged 2026-08-02 by go-lint during item 12's
    src/server/handler/admin_settings.go pass): pre-existing, out of scope
    for item 12 (DB timeout wrapping only) —
    - Lines 32, 100, 186, 197, 204, 211, 226, 255: error responses use
      `{"error": "..."}` instead of the canonical
      `{"ok": false, "error": "CODE", "message": "..."}` shape required by
      AI.md PART 14.
    - Lines 87, 170, 217, 275, 287: success responses don't follow the
      canonical `{"ok": true, "data": {...}}` shape from AI.md PART 14.
    - Lines 66, 69: `json.Unmarshal()` return errors are silently
      discarded in `GetAllSettings()` — should check or explicitly
      `_`-ignore with a comment.
    - Line 146: comment references "TEMPLATE.md Part 25" — should
      reference AI.md with the correct PART number (PART 18, WebUI
      Notifications) instead; no TEMPLATE.md file exists in this project.
    Not introduced by this diff — the go-lint agent explicitly confirmed
    the timeout-wrapping conversion itself (lines 26, 127, 193-196, 224,
    264) is correct per AI.md PART 10, including the `*sql.Tx`-scoped
    `database.WithTimeout()` + `tx.ExecContext()` pattern used in
    `ResetSettings()`. Read: AI.md PART 14 (API error/success response
    shapes) before starting.

35. TODO (flagged 2026-08-02 while verifying item 20's notification.go
    fix): `TestMutationResolver_ResetUserPassword`
    (src/graphql/schema.resolvers_test.go) is flaky when run as part of the
    full `./graphql/...`/`./server/model/...` suite — intermittently panics
    with a nil-pointer SIGSEGV inside `database/sql.(*DB).conn`, called from
    `SMTPService.getSetting()` → `SMTPService.LoadConfig()`, invoked from an
    async goroutine spawned by `RequestAPIUserPasswordReset`
    (src/server/handler/auth_api.go line 517-545). Root cause: the goroutine
    that sends the password-reset email loads SMTP config using `m.DB`
    after the test's DB connection has already been closed/nilled during
    teardown — a test-isolation race, not a production bug (the real server
    process's DB lives for the process lifetime). Reproduced identically on
    unmodified pre-item-20 code via `git stash`, confirming it predates this
    session's notification.go/schema.resolvers.go changes entirely; passes
    reliably when run in isolation (`-run TestMutationResolver_ResetUserPassword`)
    or as the sole package under test. Fix: make `RequestAPIUserPasswordReset`'s
    async email-send goroutine either be awaited/synchronized in tests, or
    have the test capture/inject a mock SMTP service instead of relying on
    the real `m.DB`, so the goroutine can't outlive test teardown. Read:
    AI.md PART 29 (Testing) before starting.

34. DONE (2026-08-02): fixed 3 Makefile drifts from AI.md PART 26's
    canonical template (flagged by go-lint during item 15's GraphQL
    context-key pass):
    - `PLATFORMS` (line 38) was `linux/amd64,linux/arm64` (comma-joined,
      2 platforms) — fixed to the canonical space-separated 8-platform
      list (`linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
      windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64`); the
      `build`/`local` targets' `for platform in $(PLATFORMS)` loops
      require space-separation, not commas, to iterate correctly.
    - `test` target's coverage gate was hardcoded to 80% in a shell `bc`
      comparison — PART 26/29 require ≥60% (matching `ci.yml`'s own
      gate); fixed both the `bc` comparison and the two adjacent
      echo/error strings to say 60%.
    - `dev` target invoked `$(GO_DOCKER) go mod tidy` without first
      creating `$(GO_CACHE)`/`$(GO_BUILD)` (the `build`/`local`/`test`
      targets all `mkdir -p` these before their first `$(GO_DOCKER)`
      call; `dev` was the one target missing it) — added the same
      `@mkdir -p $(GO_CACHE) $(GO_BUILD)` line as the first recipe line.
    (The go-lint agent's 4th finding, that `Version`/`CommitID`/
    `BuildDate` in `src/main.go` are used but never declared, is a false
    positive — they're declared in `src/version.go`, a separate file in
    the same package, which the agent didn't check.)
    Verified via `make -n dev`/`make -n test` (dry-run, syntax clean).
    Read: AI.md PART 26 (Makefile targets, canonical template lines
    38185-38245) before starting.

36. TODO (flagged 2026-08-02 by go-lint during item 20's pre-commit gate
    check): `src/main.go` uses `log.Fatalf` at ~18 call sites (lines 100,
    127, 143, 176, 192, 197, 208, 213, 225, 236, 493, 500, 543, 588, 597,
    880, 1023, 3840) for startup/fatal errors — `log.Fatalf` always exits
    with code 1 regardless of failure class, but AI.md PART 8 (Binary
    Rules) requires standard exit codes (0 success, 1 general, 2 config,
    3 connection, 4 auth, 5 not found, 64 usage). Pre-existing, unrelated
    to item 20's notification.go/schema.resolvers.go change (item 20 didn't
    touch main.go at all). Fix: replace each `log.Fatalf` with an explicit
    log + `os.Exit({correct code})` call, classifying each site by failure
    type (config load failure → 2, DB/network connection failure → 3, etc.)
    — requires reading each call site's context individually, not a blind
    find/replace. Read: AI.md PART 8 (exit codes) before starting.

37. TODO (flagged 2026-08-02 by go-lint during item 20's pre-commit gate
    check): `src/server/handler/graphql.go` lines 171-182 serve the GraphiQL
    IDE via its standard React-based client-side UI bundle — this conflicts
    with the project's "no client-side rendering / no JS framework" frontend
    rule (frontend-rules.md PART 16: vanilla JS only, no React/Vue/etc, one
    hand-written `static/js/app.js`). However, GraphiQL is the GraphQL
    Foundation's standard reference IDE and typically ships only as a
    prebuilt React bundle — no maintained vanilla-JS equivalent exists.
    Needs a product decision, not a blind fix: either (a) accept GraphiQL's
    bundled React UI as a documented, scoped exception (it's a developer/
    admin-only tool, not the public frontend the no-framework rule targets),
    or (b) replace it with a minimal hand-rolled GraphQL query console using
    vanilla JS, accepting reduced IDE features (no schema autocomplete/docs
    explorer). Ask the user which approach before implementing. Read: AI.md
    PART 16 (frontend rules) and PART 14 (API/GraphQL) before starting.

38. TODO (flagged 2026-08-02 while verifying item 17's log-emoji sweep):
    numerous `fmt.Printf`/`fmt.Println` startup-banner/console lines in
    `src/main.go` (e.g. lines 162-163, 168, 242, 250-398, 502-729, 849-879,
    1075-1114, 3797-3972, 4026-4087) and
    `src/server/service/location_enhancer.go` (lines 93, 105, 109, 125, 130,
    134) and `src/server/service/weather.go` (lines 1296, 1324) print emoji
    unconditionally with no `NO_COLOR`/`TERM=dumb`/`--color` check. AI.md
    PART 11/13/33 allows emoji in console/banner display (as opposed to log
    output, see item 17) but ONLY when it honors `NO_COLOR` — priority order
    is CLI flag > config > `NO_COLOR` > auto-detect (TTY + `TERM`). These
    lines bypass that gate entirely, always printing emoji regardless of
    `NO_COLOR`/redirected-output/non-TTY context. Fix: route console/banner
    emoji output through the existing `display`/color-detection helper (or
    add an equivalent check) so emoji (and any ANSI color) are suppressed
    when `NO_COLOR` is set, `TERM=dumb`, or output isn't a TTY — same as the
    startup-banner width-responsive rendering already does elsewhere. Needs
    a project decision on how deep to route unrelated `fmt.Printf` diagnostic
    lines through this gate vs. only the true "banner" lines — flag to user
    if the boundary is unclear once started. Read: AI.md PART 11 (`NO_COLOR`
    priority order), PART 8/33 (`--color` flag, shared across all binaries).

39. TODO (flagged 2026-08-02 while fixing item 27): src/server/handler/
    notification_preferences.go uses the legacy `gin.H{"error": "..."}` /
    `gin.H{"message": "..."}` response shape throughout (GetUserPreferences,
    UpdatePreference, CreatePreference, DeletePreference, GetSubscriptions,
    UpdateSubscription, CreateSubscription) instead of the canonical PART 14
    shape (`RespondError`/`RespondSuccess`/`RespondCreated` helpers in
    response.go, already fixed and used by notification_templates.go per
    item 25). Fix: replace every `c.JSON(status, gin.H{"error"/"message":
    ...})` call in this file with the matching Respond* helper, same
    pattern as item 25's notification_templates.go fix. Read: AI.md PART 14
    (Response Standards) before starting.

40. TODO (flagged 2026-08-02 while fixing item 29's unused-db-parameter
    sub-issue): the same dead `db *sql.DB` parameter pattern (constructor
    takes a `*sql.DB` but the returned gin.HandlerFunc queries
    database.GetServerDB()/GetDualDB() directly instead) still exists in:
    - src/server/middleware/auth.go: `RequireAuth(db *sql.DB)`,
      `OptionalAuth(db *sql.DB)`.
    - src/server/middleware/admin_auth.go: `AdminLoginHandler(db *sql.DB)`.
    - src/server/middleware/audit.go: `AuditLogger(db *sql.DB)`.
    - src/server/middleware/server_context.go: `InjectServerContext(db
      *sql.DB, version string)`.
    - src/server/model (GetByAPIToken and similar lookups, per the
      src/server/middleware/setup_test.go / server_context_test.go
      comments referencing this pattern) — needs a full audit to enumerate
      every occurrence, not just the ones already called out in test
      comments.
    Fix, once addressed: either remove the unused parameter (as done for
    SetupTokenRequired/BlockSetupAfterComplete/BlockSetupAfterAdminExists
    in item 29) or wire the passed `db` through instead of the global
    accessor — pick one approach and apply it consistently across all
    call sites in one pass, updating every caller and test. Read: AI.md
    PART 10 (Database & Cluster) before starting.

41. TODO (flagged 2026-08-02 while fixing item 31): two out-of-scope
    findings surfaced during item 31's tests/docker.sh edit —
    - script-lint reported 11 pre-existing findings on tests/docker.sh
      (none in the lines item 31 added): missing `DOCKER_` prefix on
      `PROJECTNAME`/`PROJECTORG`/`BUILD_DIR` (lines 6, 7, 11), missing
      `__` prefix on the `cleanup` function (line 35), missing `--`
      before the query in six `grep` calls (lines 66, 76, 103, 110,
      117, 226), and a UUOC `echo "$var" | grep` pattern at line 226
      (inside a heredoc's embedded `sh -c` block — restructuring to
      avoid UUOC there may need care since it's nested shell, not bash).
    - tests/integration/api_test.go's `TestAPI_Weather_Coordinates`,
      `TestAPI_Weather_CityID`, `TestAPI_Weather_Nearest`, and
      `TestAPI_Forecast` share the exact same architectural defect that
      item 31 fixed for `TestAPI_Search` (live network calls to
      open-meteo.com inside Phase 1 `*_test.go`, per AI.md PART 29)
      but were not the specifically flagged CI failure. Same fix
      pattern applies: skip the live-network cases in Phase 1, add
      matching Phase 2 coverage in tests/docker.sh/tests/incus.sh.
    Read: AI.md PART 29 (testing strategy) and the tool_conventions.md
    script-lint rules before starting.
