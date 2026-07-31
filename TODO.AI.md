# TODO.AI.md

Dependency order: items are listed in the order they must be done (each depends
on the ones above it being in place first). Read the cited AI.md PART slice
before starting each item — do not rely on memory.

1. Generate `.claude/rules/*.md` — 14 required cheatsheet files (PART 0 § "Rule
   Files to Create/Update", directory listed as REQUIRED in PART 3 §
   "Directory Structure"). Each file needs: header `# {Topic} Rules (PART X, Y, Z)`,
   the NON-NEGOTIABLE warning line, `## CRITICAL - NEVER DO`, `## CRITICAL -
   ALWAYS DO`, a key-rules summary, and a `For complete details, see AI.md
   PART X, Y, Z` footer. Build them in this sub-order (each pass only needs
   its own PART range, no cross-dependencies among them):
   - Read: AI.md PART 0, 1 → `.claude/rules/ai-rules.md`
   - Read: AI.md PART 2, 3, 4 → `.claude/rules/project-rules.md`
   - Read: AI.md PART 5, 6, 12 → `.claude/rules/config-rules.md`
   - Read: AI.md PART 7, 8, 33 → `.claude/rules/binary-rules.md`
   - Read: AI.md PART 9, 10, 11, 32 → `.claude/rules/backend-rules.md`
   - Read: AI.md PART 13, 14, 15 → `.claude/rules/api-rules.md`
   - Read: AI.md PART 16, 17 → `.claude/rules/frontend-rules.md`
   - Read: AI.md PART 18-23 → `.claude/rules/features-rules.md`
   - Read: AI.md PART 24, 25 → `.claude/rules/service-rules.md`
   - Read: AI.md PART 26 → `.claude/rules/makefile-rules.md`
   - Read: AI.md PART 27 → `.claude/rules/docker-rules.md`
   - Read: AI.md PART 28 → `.claude/rules/cicd-rules.md`
   - Read: AI.md PART 29, 30, 31 → `.claude/rules/testing-rules.md`
   - Read: AI.md PART 34-36 → `.claude/rules/optional-rules.md`

2. Create `.claude/settings.json` (shared team settings, version-controlled)
   and `.claude/.mcp.json` if this project uses MCP servers — depends on
   item 1 existing first since PART 3's `.claude/` layout lists `rules/`
   alongside these. Read: AI.md PART 3 § "Directory Structure".

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

8. The GitHub Actions `CI` workflow has been failing on `main` since before
   this bootstrap pass (confirmed failing at commits `3f2082cd`, `094092ff`,
   and `9e27dd31` — i.e. unrelated to and not introduced by any TODO.AI.md
   item fixed above), entirely inside the item-7-adjacent "Pre-existing, out
   of scope" files above:
   - `test` job: `TestMutationResolver_RegisterUser/registration_not_available_with_no_config`
     in `src/graphql/schema.resolvers_mutations_test.go` fails —
     `err = "public registration is not available"`, want
     `"registration is not available"` (message text mismatch).
   - `lint` job (`staticcheck`): `SA1029` (built-in `string` used as a
     context key) at `src/graphql/schema.resolvers_mutations_test.go:656,725,734`
     and `src/server/handler/admin_auth_test.go:211`; `U1000` (unused func
     `newTestLogger`) at `src/server/middleware/access_log_test.go:19` and
     `src/util/logger_test.go:55`.
   These live in the same 10 files carrying unrelated uncommitted hand-edits
   noted above — fix once those files come back into scope, not as part of
   any currently-tracked item.

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
