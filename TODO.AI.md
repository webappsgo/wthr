# TODO.AI.md

## Pre-existing CI failures on main (unrelated to src/cli test coverage work)

Discovered while verifying post-push CI status for commit 442bb7cabfa8
("Raised src/cli coverage from 41.8% to 55.0%"). These failures predate
that commit — the same CI workflow has been failing on `main` since at
least 2026-07-24 (runs 30070423899, 30253960215, 30561446862,
30588147542), none of which touch `src/cli`. Not fixed here because they
are outside this task's scope (src/cli coverage only) and touch unrelated
packages.

- [ ] `src/scheduler`: `TestBackupHourlyTask`/related test panics with a
      nil-pointer SIGSEGV in `Scheduler.logTaskExecution` ->
      `database/sql.(*DB).QueryRow` — `src/scheduler/scheduler.go:275`,
      called from `executeTask` (line 268) via `TriggerTask` (line
      1070). The `*sql.DB` passed to the scheduler in that test path is
      nil. CI run 30588147542, job "test", step "Run tests with
      coverage".
- [ ] `src/graphql`: package test run reports `FAIL` with `coverage:
      2.4% of statements` in the same CI run — investigate whether this
      is caused by the same nil-DB scheduler panic aborting the test
      binary early, or a separate failure.
- [ ] `CI` "test" job: "Enforce coverage threshold" step fails
      (exit code 3) on prior runs (30561446862, 30253960215,
      30070423899) even when "Run tests with coverage" itself passes —
      overall coverage is below whatever threshold `ci.yml` enforces.
- [ ] `staticcheck` lint failures (CI run 30588147542, job "lint"):
  - `src/graphql/schema.resolvers_mutations_test.go:656,725,734` — SA1029,
    context key should not be a built-in `string` type.
  - `src/server/handler/admin_auth_test.go:211` — SA1029, same issue.
  - `src/server/middleware/access_log_test.go:19` — U1000, unused func
    `newTestLogger`.
  - `src/util/logger_test.go:55` — U1000, unused func `newTestLogger`.
