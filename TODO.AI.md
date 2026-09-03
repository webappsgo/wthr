# TODO.AI.md

Dependency order: items are listed in the order they must be done (each depends
on the ones above it being in place first). Read the cited AI.md PART slice
before starting each item — do not rely on memory.

40. TODO (flagged 2026-08-02; DIAGNOSIS CORRECTED 2026-08-21 after the full
    audit the item asked for — the fix is real but it is NOT where the
    original report placed it, so re-read this before starting).
    Corrected findings:
    - `src/server/middleware/admin_auth.go` `AdminLoginHandler(db)` and
      `src/server/middleware/audit.go` `AuditLogger(db)` genuinely USE their
      `db` param (`database.QueryRowContext(..., db, ...)` /
      `database.ExecContext(..., db, ...)`). Leave both alone.
    - The dead parameter is one level deeper: every model struct carries a
      `DB *sql.DB` field that its own methods ignore, querying
      `database.GetServerDB()` / `database.GetUsersDB()` instead. Verified
      across `src/server/model/{user,admin,session,settings,token,
      recovery_keys,passkey,admin_passkey}.go` — roughly 95 call sites, and
      zero methods that actually read the field.
    - That is why `auth.go`'s `AuthMiddleware(db, required)` and
      `server_context.go`'s `InjectServerContext(db, version)` "use" their
      param: they only use it to fill a field nobody reads
      (`&model.SessionModel{DB: db}`, `&model.SettingsModel{DB: db}`), so
      `RequireAuth(db)` / `OptionalAuth(db)` are dead by transitivity.
    Recommended approach: DELETE the `DB` field rather than wire it through.
    A single `*sql.DB` cannot express the server.db-vs-users.db routing the
    models actually perform, which is precisely why the field went unused —
    wiring it through would require threading a `*database.DualDB` and would
    change nothing behaviorally. Then drop the now-unused params from
    `AuthMiddleware`/`RequireAuth`/`OptionalAuth`/`InjectServerContext` and
    fix every call site (`src/main.go` included) in one pass.
    Also fix the four now-inaccurate test comments describing the old shape:
    `src/server/middleware/{setup_test.go:62, admin_auth_test.go:24,
    server_context_test.go:21, token_auth_test.go:47}`.
    Blocked while other agents hold `src/main.go`. Read: AI.md PART 10
    (Database & Cluster) before starting.
    Original report (superseded, kept for provenance): the same dead
    `db *sql.DB` parameter pattern was believed to exist in
    middleware/auth.go, admin_auth.go, audit.go, server_context.go, and in
    src/server/model's GetByAPIToken — per the test
      comments.
    Fix, once addressed: either remove the unused parameter (as done for
    SetupTokenRequired/BlockSetupAfterComplete/BlockSetupAfterAdminExists
    in item 29) or wire the passed `db` through instead of the global
    accessor — pick one approach and apply it consistently across all
    call sites in one pass, updating every caller and test. Read: AI.md
    PART 10 (Database & Cluster) before starting.

61. TODO - CI GOVULNCHECK FAILURE FROM STALE GO TOOLCHAIN IN
    `casjaysdev/go:latest` (flagged 2026-08-13, pre-existing, not caused
    by any recent commit - confirmed identical failure on commit
    `2ea3c8dffee8` (unrelated "Spec: Updated the SPEC for Servers"
    commit) and on `017bbe16dd78` (script-lint compliance commit); both
    fail the `ci.yml` `vuln-scan` job with `govulncheck ./...` exit code
    3. The CI job container image is running Go 1.26.5, which carries 7
    known stdlib vulnerabilities (GO-2026-6218, GO-2026-6091,
    GO-2026-6090, GO-2026-6089, GO-2026-6088, GO-2026-5972, GO-2026-5026)
    all fixed in Go 1.26.6. This is an environment/toolchain-version
    issue, not an application code issue - project code does not use
    `github.com/mattn/go-sqlite3` or other CGO/vulnerable deps directly;
    the flagged call sites (net/url, html/template, crypto/tls,
    net/http, encoding/xml, encoding/asn1, x/net/idna) are all reached
    via ordinary stdlib usage (HTTP client/server, TLS, templates, XML
    parsing) that will be safe as soon as the toolchain image updates.
    Per CLAUDE.md, `casjaysdev/go:latest` must stay unpinned/floating -
    the fix is for the image maintainer to rebuild `casjaysdev/go:latest`
    against a current Go patch release (1.26.6+), not a workaround in
    this repo. Action: confirm whether a newer `casjaysdev/go:latest`
    has been published; if not, this blocks `ci.yml`'s `vuln-scan` job
    project-wide until it is - re-run `gh run list` after any future
    push to check if it has cleared on its own once the image updates.

73. TODO (flagged 2026-08-21 during the admin panel review): the admin route
    tree in `src/main.go` does not match PART 17's required shape — all
    server management must live under `/server/{admin_path}/config/*`, with
    only the admin's own account under
    `/server/{admin_path}/{admin_username}/*`. Migrating the tree also
    changes every href in `admin_chrome.tmpl`, so this must be done as one
    self-contained pass (route registration + template hrefs + any
    hardcoded admin URLs in handlers/tests together). Read: AI.md PART 17.

74. TODO (flagged 2026-08-21 during the admin panel review): the admin
    header's global search form GETs `q` to the dashboard, which ignores the
    parameter entirely — there is no global admin search route or handler.
    Either implement the search route PART 17 describes or remove the form;
    a control that silently does nothing is a defect either way.
    Read: AI.md PART 17.

75. TODO (flagged 2026-08-21 during the admin panel review): the admin
    sidebar's expand/collapse state does not persist across page loads.
    Needs either a cookie written by a small handler or the state stored and
    restored from `static/js/app.js` (no inline JS, no `on*` attributes —
    PART 16 requires `data-action` delegation and a CSS-first solution where
    one exists). Read: AI.md PART 16, 17.

91. TODO (flagged 2026-08-21 during the admin panel review): nearly every
    admin mutation endpoint binds its request body with `ShouldBindJSON`, so
    it accepts only `application/json`. PART 16 requires the frontend to be
    fully functional with JavaScript disabled, and a plain HTML form submits
    `application/x-www-form-urlencoded` - so admin CRUD cannot work JS-free
    today no matter how the templates are written. Fix on the Go side: accept
    form-encoded input as well as JSON on every admin mutation route
    (content-type-aware binding), and respond with a POST-redirect-GET flash
    for non-AJAX submits while keeping the canonical JSON shape for API
    clients. Read: AI.md PART 16, 14.

93. TODO (flagged 2026-08-21 while creating the two root admin templates): the
    notification-channel and email-template admin pages cannot be made
    interactive without JavaScript for a second reason beyond item 91 - the whole
    `adminAPI` group is bearer-token authenticated (`src/main.go:2463-2467`,
    `TokenAuthMiddleware` + `RequireAdminToken`), not session-cookie
    authenticated, so a browser form POST would 401 even on the no-body routes
    (`enable`, `disable`, `initialize`). Fixing item 91 alone is not enough:
    session-authenticated, form-encoded `POST {admin_path}/server/channels/...`
    and `.../templates/...` routes that redirect back (POST-redirect-GET) must
    exist alongside the token-authenticated JSON API. Read: AI.md PART 16, 17.

103. TODO (flagged 2026-08-21 while verifying the admin route migration):
    AI.md contradicts itself about what may sit directly under the admin
    path. Line 30017 states that the admin's own account is the only direct
    child and that everything else lives under
    `/server/{admin_path}/config/*`, but lines 31022-31049 enumerate
    `/server/{admin_path}/help` as a direct child. The implementation
    currently follows line 30017. AI.md is read-only, so this cannot be
    fixed here — it needs a user decision, and the resolution belongs in
    SPEC.md, which outranks AI.md. Read: AI.md PART 17.

106. TODO (flagged 2026-08-21 by the notification DB-handle fix): three
    services store an injected `*sql.DB` that is never read —
    `DeliverySystem.db` (`src/server/service/delivery_system.go:50`),
    `SMTPService.db` (`src/server/service/smtp.go:54`) and
    `WeatherNotificationService.db` (`src/server/service/weather_notifications.go:15`).
    All three query exclusively through `database.GetServerDB()` /
    `database.GetUsersDB()`, so the handle their constructors accept has no
    effect and can silently disagree with the schema they actually read.
    Either use the field or drop it from the struct and the constructor
    signature. Same class as item 40, which covers the model structs. Read:
    AI.md PART 10.
    DONE (2026-08-21). RESOLVED by using the field rather than dropping it:
    each service gained one private accessor (`serverDB()` / `usersDB()`)
    that returns the injected handle when non-nil and falls back to the
    process-global accessor otherwise, documented in its doc-comment.
    Constructor signatures are unchanged. All 15 server.db sites in
    `delivery_system.go`, all 3 in `smtp.go` and all 8 users.db sites in
    `weather_notifications.go` now route through it. Two sites in
    `delivery_system.go` deliberately keep `database.GetUsersDB()` - they are
    genuine cross-database reads (`user_notification_channel_preferences`,
    `user_accounts`) that the injected server handle cannot serve; each
    carries a comment naming the database and why.
    Every table each service touches was inventoried against
    `database.ServerSchema`/`UsersSchema`; none was orphaned from both.
    Anti-regression tests wire the process-global handle to a SECOND,
    different database seeded with contradicting values, so ignoring the
    injected field again fails the test rather than passing by luck:
    `TestDeliverySystem_UsesInjectedServerDB`,
    `TestSMTPService_UsesInjectedServerDB`,
    `TestWeatherNotifications_UsesInjectedUsersDB`, plus three
    `*_NilInjectedDBFallsBackToGlobal` companions pinning the documented
    fallback. Both databases in each test are built from the real schema
    constants verbatim. The 12 `NewWeatherNotificationService` calls in
    `weather_notifications_test.go` were switched from the server handle to
    the users handle, which is what that service actually reads.
    An APP-BREAKING call site this exposed was fixed immediately:
    `src/server/handler/auth_api.go:598` built its SMTP service from the
    `db` in scope, which is the USERS handle for every caller
    (`schema.resolvers.go` passes `r.UsersDB`, `auth_api.go:1117` passes
    `h.DB`, and the same handle writes the `user_password_resets` row just
    above). The SMTP service reads `server_config` and
    `server_notification_channels`, both `ServerSchema` tables, so it was
    looking for its configuration in the wrong database and finding no SMTP
    settings - no password-reset email could be sent. It now takes
    `database.GetServerDB()`, with a comment recording why. The other two
    call sites the sweep flagged were verified correct and left alone:
    `src/main.go:774` passes `dualDB.Users` to
    `NewWeatherNotificationService`, whose tables are all in `UsersSchema`,
    and `notification_channels.go:28` is only ever called with
    `dualDB.Server`.

113. TODO (flagged 2026-08-21 by item 97, verify before the next release):
    package `database` lost fifteen legacy-only test cases when `InitDB`,
    `InitDBFromConnectionString`, `InitDBWithConfig` and the migration
    helpers were deleted. The code they covered was deleted with them, so
    both numerator and denominator shrink, but the package's absolute test
    count drops noticeably. Confirm the repo still clears the 60% coverage
    gate the next time `make test` runs — and note that per SPEC.md the gate
    filters `src/graphql/generated.go` out of `coverage.out` first. Read:
    AI.md PART 26, 29.

124. TODO (flagged 2026-08-21 by the session-expiry conversion):
    `src/server/middleware/admin_auth_test.go:58` and `:156` still seed
    fixtures with `CURRENT_TIMESTAMP` / `datetime(?, 'unixepoch')`. Harmless
    today (both yield canonical text that parses fine) but inconsistent with
    the `dbtime.FormatSQLTimestamp` convention the rest of the suite now
    follows. Low priority. Read: AI.md PART 10.

125. TODO (flagged 2026-08-21 by the CLI legacy-table fix): `adminRecoverySetup`
    in `src/cli/maintenance.go` diverges from AI.md PART 22 - the spec says
    `{project_name} --maintenance setup` clears the admin credentials and
    prints a one-time setup token for re-authentication, leaving all user
    data untouched. The implementation instead prompts for a new username and
    password and writes them directly, which is a different (and weaker)
    recovery model. Feature-level fix. Read: AI.md PART 22.

147. TODO (found 2026-08-21 while closing item 133):
    `src/scheduler/scheduler.go` writes `server_cve_alerts.published_at`
    straight from the NVD API's `published` JSON string (RFC-3339-ish with
    fractional seconds, e.g. `2024-01-15T10:30:00.000`), not canonical
    `dbtime` text. Nothing in Go reads that column today, so it is latent
    rather than broken - but it is the exact mixed-layout condition dbtime
    exists to prevent, and the first consumer to sort or compare it will get
    silently wrong results. Parse it best-effort and store
    `dbtime.FormatSQLTimestamp` output, keeping the row (with a NULL
    `published_at`) when the upstream value will not parse rather than
    dropping the CVE alert. Read: AI.md PART 10.
    DONE (2026-08-21). RESOLVED as specified: `parseNVDPublished` accepts the
    documented millisecond form, the second-precision form and RFC 3339,
    normalizes to UTC and is stored via `dbtime.FormatSQLTimestamp`; an
    unreadable value is bound as a nil `interface{}` (SQL NULL) and logged at
    WARN, and the CVE row is still written. `published_at = excluded.published_at`
    was added to the upsert, which had silently never updated the column.
    `TestParseNVDPublished` in `src/scheduler/scheduler_test.go` covers all
    three layouts, offset normalization, and the error path.
    TWO APP-BREAKING BUGS were found in the same function and fixed
    immediately rather than deferred, since `UpdateCVEDatabase` could not
    have worked at all:
    (a) the task created `server_cve_alerts` itself at task time, and the
    statement named a column `references` - a reserved word that is a hard
    syntax error as a bare identifier (verified directly against SQLite). The
    task therefore aborted at its first statement on every run where CVE
    monitoring was enabled, and no CVE was ever stored. The table now lives
    in `database.ServerSchema` (column renamed `reference_urls`, plus
    severity/published/acknowledged indexes, `NOT NULL` on the defaulted
    columns), `ServerSchemaVersion` is bumped 7 -> 8, and the task-time DDL is
    replaced with the same presence check used in items 123 and 148.
    (b) the NVD response was decoded with `cve.descriptions` modelled as an
    object wrapping a `description_data` array - the shape of the retired 1.0
    feed. API 2.0 returns a bare `[{lang, value}]` array, so the field never
    bound and every stored CVE would have had an empty description. The
    struct now matches the 2.0 shape.
    Nothing else in the repo references `server_cve_alerts`, so no reader
    needed updating alongside the rename.

138. TODO (flagged 2026-08-21 by the timestamp conversion agents): several
    test fixtures build their "wrong zone" timestamps with
    `time.FixedZone("EST13", ...)` (or `"FARWEST"`/`"FAREAST"`). Formatted
    through a layout carrying the `MST` element, those produce text that
    `time.Parse` cannot read - it consumes only the letters and chokes on the
    trailing digits, and it rejects names longer than six characters - so
    every such fixture is silently exercising the unparseable fail-closed
    path instead of the instant-comparison path it was written to test. Three
    files were already renamed to `"EAST"`
    (`src/server/service/weather_notifications_test.go`,
    `src/graphql/graphql_test.go`, `src/graphql/schema.resolvers_test.go`);
    the same defect remains in `src/server/model/admin_timestamp_test.go`,
    `src/server/middleware/admin_auth_test.go`,
    `src/server/handler/auth_test.go`, `src/scheduler/scheduler_test.go`,
    `src/common/dbtime/dbtime_test.go`, and `src/cluster/cluster_test.go`.
    Rename to 3-5 uppercase letters, and re-check each affected assertion:
    a case that "passed" while inert may assert the wrong outcome once the
    fixture starts parsing. Read: AI.md PART 29.
    DONE (2026-08-21): every remaining fixture renamed - `WST`/`EAT`/`EAST`
    across `src/server/model/admin_timestamp_test.go`,
    `src/server/middleware/admin_auth_test.go`,
    `src/server/handler/auth_test.go`, `src/common/dbtime/dbtime_test.go`,
    `src/cluster/cluster_test.go` and `src/scheduler/scheduler_test.go`
    (`EST13` -> `EAST`). No expected value had to change anywhere: every
    previously-inert case already asserted the outcome instant comparison
    produces, so they now exercise that path instead of the fail-closed one.
    The naming rule is recorded in a comment at each site - `time.Parse`'s
    `MST` element accepts only 3-5 uppercase letters (5 must end in `T`), so
    a digit or a sixth letter makes the whole value unparseable.
    Verified by grep: the only surviving `FixedZone` name outside that set is
    `src/database/schema_test.go`'s `"fixture"`, which is formatted with a
    numeric-offset-only layout carrying no `MST` element and is therefore
    parseable as written.

174. TODO (flagged 2026-08-28 during item 116's i18n sweep): four
    `src/server/handler/` files were out of that item's declared scope and
    still return hardcoded-English JSON error strings via raw `writeJSON`
    calls, same class of PART 31 violation as item 116 fixed everywhere
    else: `server_pages.go` (contact-form save/send failures, ~3 sites),
    `admin_auth_settings.go` (~2 sites, some already reuse `err.Error()`
    directly), `admin_passkey.go` (~15 sites: "Not authenticated",
    "Invalid request body", "Failed to load passkeys", etc.), and
    `passkey.go` (~10+ sites, user-facing passkey mirror of
    `admin_passkey.go`). Fix the same way item 116 did: swap each raw
    `writeJSON(w, status, map[string]interface{}{"error": "..."})` for the
    matching `RespondError`-family helper (`Unauthorized`, `BadRequest`,
    `InternalError`, etc.) passing `Translate(r, "errors.*")`; add new keys
    to all seven locale files, keeping the key set identical everywhere.
    Read: AI.md PART 31.

176. TODO (flagged 2026-08-31 during a code-review pass on unrelated
    open-redirect-guard cleanup): `util.GetHostFromRequest`
    (`src/util/host.go` lines 71-92) honors `X-Forwarded-Host`/
    `X-Real-Host`/`X-Original-Host` from every caller unconditionally,
    with no `server.trusted_proxies` gate — unlike `GetClientIP`, which
    item 171 already wrapped with a gated `TrustedGetClientIP` per AI.md
    PART 5's "NEVER trust `X-Forwarded-*` headers... from a peer that
    isn't in `trusted_proxies`" rule. This is a pre-existing gap (not
    introduced by this session's changes), same shape as item 171's
    fix but for the Host header family instead of the client-IP header
    family. Fix: add a `TrustedGetHostFromRequest(r *http.Request)
    string` (or equivalent gated variant) in `src/util/trusted_proxies.go`
    alongside `TrustedGetClientIP`, reusing `isTrustedPeer()`; audit and
    update real call sites of `util.GetHostFromRequest(r)` to use the
    gated version wherever the result feeds a security-relevant decision
    (redirects, CORS origin checks, cookie domain, links shown back to
    the user) — direct display-only uses may be lower priority but
    should be reviewed too. Add matching subtests to
    `trusted_proxies_test.go` (header-honored vs header-dropped by peer
    trust, mirroring the existing `TrustedGetClientIP` coverage). Read:
    AI.md PART 5, PART 12.

177. TODO (flagged 2026-09-03 from a user-reported compliance sweep covering
    README.md/CI-CD/TODO.AI.md/tools.go/renovate.json). Triage results for
    each named item:
    - `tools.go`: DONE this pass — relocated to `src/tools/tools.go` (no
      code referenced its old root path). AI.md PART 3's required root
      layout lists no root-level `.go` files.
    - `TODO.AI.md`: DONE this pass — pruned 152 completed `DONE` items
      (4048 -> 314 lines) per AI.md's explicit "remove completed items...
      as each one is fully resolved and committed" rule (PART 0, "TODO.AI.md
      Completion"), which this file had stopped following. A stale
      "Pre-existing, out of scope" bootstrap-era note (10 files with
      unrelated hand-edits, dated 2026-07-30) was removed along with it —
      those files have had extensive committed work since; the note no
      longer reflects reality.
    - `README.md`: partially fixed this pass — added Tor/I2P and
      user-account/preferences bullets to Features, the two biggest
      undocumented-feature gaps found via grep. NOT yet done: a full
      section-by-section diff against IDEA.md/AI.md PART 34 (multi-user)
      for remaining currency gaps (e.g. registration modes, 2FA/passkey
      setup flow, admin panel feature list) - the user's "many many more
      issues" framing implies this Features list is not exhaustively
      verified yet.
    - `renovate.json`: reviewed against `~/.claude/memory/cicd_conventions.md`
      Dependency Update Automation section - already exceeds the minimum
      template (covers gomod, github-actions pinDigests, vulnerabilityAlerts,
      valid non-deprecated `prCreation`/`rebaseWhen` options per current
      Renovate docs). No concrete non-compliance found against the
      documented project convention; "outdated" may refer to something
      outside file content (e.g. stale/unmerged Renovate PRs, dashboard
      state) that needs checking on the actual Renovate dashboard/PR list,
      not the config file.
    - `.github/workflows/*.yml` / `docker/*`: DONE this pass (corrected -
      an earlier pass in this same triage wrongly called this compliant).
      The actual gap: `docker/Dockerfile.dev` (a required root Docker file
      per AI.md PART 27's directory listing and `.claude/rules/docker-rules.md`)
      did not exist, and `docker.yml` had no job building/pushing the
      `:devel` tag. Fixed: created `docker/Dockerfile.dev` (structurally
      identical to `docker/Dockerfile`, `MODE=devel` baked in via `ENV`,
      never in the production Dockerfile); added a `schedule: 0 4 * * *`
      trigger to `docker.yml` plus a new `build-devel` job (builds
      `docker/Dockerfile.dev`, tags `:devel` only, runs on schedule +
      workflow_dispatch + every non-tag push); scoped `build-standard`/
      `build-aio` to `if: github.event_name != 'schedule'` so they don't
      also fire on the new trigger, and removed the `:devel` tag from
      `build-standard`'s tag list (now owned by `build-devel`). Also fixed
      `docker/docker-compose.dev.yml` to pull `:devel` instead of `:latest`,
      per `dockerfile_conventions.md`'s Dev Compose section. Verified with
      `act --list -W docker.yml` (3 jobs resolve correctly across push/
      schedule/workflow_dispatch).
    Read: AI.md PART 27, PART 28, PART 30 (README source-of-truth), PART 34.
