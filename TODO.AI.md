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

5. `make test`'s 80%-coverage gate (Makefile lines 213-226) is broken and
   found while verifying item 4: `go tool cover -func` prints one `total:`
   line per `go test` invocation across the run, so
   `awk '{print $3}' | ... total ...` captures multiple `0.0`/percentage
   values instead of a single number; `$COVERAGE` becomes multi-line, the
   `[ $(echo "$COVERAGE < 80" | bc -l) -eq 1 ]` check fails with
   `too many arguments`, and the gate silently no-ops (exits 0) instead of
   failing the build — confirmed live: actual coverage was 25.8%
   (well under the required 80%) but `make test` still exited 0. Fix the
   coverage extraction (e.g. run `go tool cover -func` once against the
   merged `$COVDIR/coverage.out` and take only the final `total:` line, or
   `grep -c` guard against multiple matches) so the gate actually enforces
   80%. Read: AI.md PART 26, PART 29-31 (testing requirements).

6. `go-lint` flagged `Makefile` line 35: `GO_BUILD` is not project-scoped —
   must be `$(HOME)/.cache/go-build/$(PROJECTNAME)`, currently
   `$(HOME)/.cache/go-build`. Read: AI.md PART 26.

7. `go-lint` flagged `src/scheduler/scheduler.go` line 23: external cron
   library `robfig/cron/v3` used instead of a built-in scheduler — verify
   against AI.md PART 19 whether this is an approved exception; if not,
   replace with the built-in scheduler pattern. Read: AI.md PART 19.

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
