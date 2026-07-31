# AI Assistant & Critical Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER guess or assume a requirement, file location, approach, or default value — STOP and ASK
- NEVER edit AI.md content (PARTS 0-36 are READ-ONLY, source of truth). Project changes belong in IDEA.md; project-specific rule overrides belong in SPEC.md
- NEVER "improve" or "optimize" the spec, or create patterns not in spec
- NEVER create report/analysis files (AUDIT.md, COMPLIANCE.md, SUMMARY.md, etc.) — fix issues directly. Temporary `AUDIT.AI.md` is allowed only for explicit audits with 5+ issues, and must be deleted when resolved
- NEVER rely on memory for spec content — read the relevant PART when needed, do not pre-load speculatively
- NEVER add unrequested features
- NEVER read an image larger than 1000x1000 directly — check dimensions and resize first
- NEVER install Go locally or run `go` commands directly on the host — ALL builds/tests use Docker (`casjaysdev/go:latest`) or Incus
- NEVER use inline comments — comments always go ABOVE the code, never on the same line or below
- NEVER use `SELECT *` in application code — name columns explicitly
- NEVER pass user input directly to shell, SQL, or eval — validate and parameterize everything
- NEVER reveal sensitive info (credentials, connection strings, internal IPs/paths, stack traces) to users, in API responses, error messages, or `/server/healthz`
- NEVER weaken security (authn/authz, TLS, CSRF/CSP/CORS, rate limiting, input validation, least privilege) to improve usability — solve usability with better defaults/UX instead
- NEVER use `cron`/Task Scheduler/external schedulers — use the built-in scheduler (PART 19)
- NEVER require SSH/CLI/manual config-file edits — every `server.yml` setting must be editable via admin WebUI
- NEVER require a restart for config changes except listen address/port/DB driver (live reload is mandatory)
- NEVER jump between half-finished features — complete one thing fully before starting the next
- NEVER treat non-conforming IDEA.md as authoritative without running the migration procedure
- NEVER let a subagent write `.git/COMMIT_MESS` or call `gitcommit` — only the parent instance commits
- NEVER use plain `git commit`/`git push` — use `gitcommit <command>` only, after writing AND re-reading `.git/COMMIT_MESS`
- NEVER include AI attribution anywhere (code, comments, commits, PRs, docs)
- NEVER leave TODO/FIXME/XXX comments in code that's called "done"
- NEVER treat "audit" as triggered by anything other than the user explicitly saying "audit" / "check compliance" / "verify project"

## CRITICAL - ALWAYS DO

- ALWAYS read the relevant AI.md PART(s) before implementing that specific task — not the whole file speculatively
- ALWAYS ask when unsure, when multiple interpretations exist, or before an architectural/destructive decision
- ALWAYS read a file before editing it; search before creating; verify/test before claiming "done"
- ALWAYS run the Mandatory Verification Steps before saying "done": read files, search patterns, test changes, verify output, confirm certainty
- ALWAYS check `.claude/rules/` on first session with a project and create/update the 14 rule files if missing or AI.md is newer
- ALWAYS read TODO.AI.md and TODO.md (if present) before starting work; never delete or empty the human-owned TODO.md, only mark items done
- ALWAYS keep IDEA.md, README.md, Swagger/GraphQL, docs/ in sync with actual code
- ALWAYS use parameterized queries, HTML-escaping, CSRF tokens, and allowlists for input handling
- ALWAYS trim whitespace on text input (`strings.TrimSpace()`); reject (never trim) leading/trailing whitespace in passwords
- ALWAYS treat server apps as internet-facing/hostile-exposed by default unless the user says otherwise
- ALWAYS use the standard `curl -q -LSsf` flag set in docs, scripts, tests, and CI
- ALWAYS use full `{fqdn}`-based URLs in embedded/runtime code and `{official_site}`-based URLs in documentation, never bare paths (except internal router registration)
- ALWAYS number/letter multiple-choice questions when asking the user for a decision
- ALWAYS verify self-work with real tools appropriate to the change type (tests, curl, build, browser, DB rollback, etc.) — never conclude from "looks right"
- ALWAYS keep AI.md PARTs 0-36 fully implemented for every project, regardless of how "simple" the project seems — no partial/lite implementations
- ALWAYS make optional PARTs (34-36) NON-NEGOTIABLE once a project has implemented them, and keep unused optional features completely absent from code (no stubs, no dead conditionals, no toggles)
- ALWAYS strip AI attribution and act as an extension of the user, not a separate contributor

## Key Rules Summary

**AI.md structure & spec compliance (PART 0):** AI.md (PARTs 0-36) is the read-only implementation spec (HOW); IDEA.md is the project's business logic (WHAT) and the only file that changes as features evolve; SPEC.md holds project-specific overrides and wins over AI.md when they conflict. Read only the PART(s) needed for the current task — never load the whole 38,000+ line spec speculatively. Every 3-5 changes, stop and check for drift against what you already read.

**Session initialization:** on first session with a project's AI.md, check/migrate CLAUDE.md content into IDEA.md, ensure `.claude/rules/` exists with all 14 rule files (ai-rules, project-rules, config-rules, binary-rules, backend-rules, api-rules, frontend-rules, features-rules, service-rules, makefile-rules, docker-rules, cicd-rules, testing-rules, optional-rules), check TODO.AI.md/TODO.md, and commit COMMIT/NEVER/MUST rules to memory.

**Never guess (the math):** asking costs ~100 tokens; a wrong guess plus explanation plus redo costs ~5000+ tokens. Asking is roughly 50x cheaper than guessing wrong. Red-flag thoughts ("this is probably what they meant", "I'll just assume", "this should work") are a signal to stop and ask/test/verify, not proceed.

**Verification discipline:** before claiming any task complete, confirm files were read (not assumed), existing patterns were searched, changes were tested, output was verified, and nothing was guessed or rushed. Priority order is Correct > Verified > Fast — a fast wrong answer is worse than a slow correct one.

**Audit process:** only runs on the explicit words "audit" / "check compliance" / "verify project" (never triggered by normal development, migration, or discovery). A full audit checks code-vs-spec compliance, cross-file sync (README, Swagger, GraphQL, docs/, CLI help), infrastructure file accuracy (Dockerfile, compose, CI/CD, Makefile), the 14 `.claude/rules/*.md` files, and documentation sync — then fixes issues directly rather than just reporting them. More than 5 issues get tracked temporarily in `AUDIT.AI.md`, deleted once fully resolved.

**Architecture (PART 1):** every project is a full web application — browser (HTML), PWA, JSON API, and a required CLI client (PART 33) — with a parallel web-route/API-route pattern for every feature (`/x` ↔ `/api/{api_version}/x`).

**Container-only development:** the local machine has no Go installed; all builds and unit tests run in Docker (`casjaysdev/go:latest`), full-OS/integration tests prefer Incus (`debian:latest`). Never run `go build`/`go test` directly on the host — use `make dev`/`make local`/`make build`/`make test` or `tests/run_tests.sh`/`tests/incus.sh`.

**Security-first design:** never-trust-input, defense-in-depth, least-privilege, fail-secure, secure-by-default; treat servers as internet-facing/hostile by default. Security is suggested (MFA, security score) not forced on users. Usability problems are solved by better UX/automation, never by weakening authn/authz/TLS/CSRF/CSP/CORS/rate-limiting/validation. Rate-limit defaults: reads 120/min, writes 10/min, health 120/min, global burst 240/min; auth-specific: 5 logins/15min, 3 password resets/hour, 5 registrations/hour, 10 uploads/hour. Error detail is layered by audience (user: minimal; admin: actionable; console/log: full); never reveal whether a user/email exists, DB internals, IPs, stack traces, or dependency versions to end users.

**Naming conventions:** files `lowercase_snake.go`; packages lowercase; public `PascalCase`; private `camelCase`; interfaces `PascalCase`+`-er`. Names must reveal intent — generic names like `Mode`, `Type`, `Status`, `Config`, `Get()`, `Init()` are banned in favor of specific ones (`AppMode`, `TokenType`, `UserStatus`, `ServerConfig`, `GetUserByID()`, `InitDatabase()`). Boolean functions/vars must say what condition they check (`IsAppModeDev()` not `IsDevelopment()`).

**Documentation:** README.md has a fixed section order (Title/Badges → About → Official Site → Features → Production → Client → Configuration → API → Other → Development(last) → Disclaimer → License), badges must be linked and platform-correct (GitHub/Gitea/GitLab/Jenkins), and a Disclaimer section is mandatory. `{official_site}` is used for documentation URLs, `{fqdn}` (via `BuildURL(r, ...)`, proxy-aware) for embedded/runtime code — never bare paths outside internal router registration.

**Development principles:** validate everything, only persist valid data, mobile-first, sane defaults, no AI/ML logic, every config setting must be editable in the admin WebUI with live reload (except listen address/port/DB driver), tokens/passwords shown only once at generation and otherwise masked. Target audience includes non-tech-savvy self-hosted/SMB users — assume low technical sophistication in UX and docs.

For complete details, see AI.md PART 0, 1
