# TODO.AI.md

Dependency order: items are listed in the order they must be done (each depends
on the ones above it being in place first). Read the cited AI.md PART slice
before starting each item — do not rely on memory.

40. RESOLVED (2026-09-24): nil-safe DB handles. `SettingsModel` gained a
    `getDB()` accessor mirroring `user.go:164-182` (injected handle, falling
    back to `database.GetServerDB()` when nil) and its direct `m.DB` derefs
    route through it; `UserModel.CountByRole` switched to `m.getDB()`. The 12
    sibling models already used the accessor pattern. Constructing either
    model with a nil handle no longer panics; nil-handle coverage added in
    `settings_test.go` / `user_test.go`.

61. BLOCKED (upstream, 2026-09-24) - CI GOVULNCHECK FAILURE FROM STALE GO TOOLCHAIN IN
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

73. STALE (removed 2026-09-24): the premise is disproved against AI.md
    itself. AI.md PART 17 (line 30451) is the authoritative rule and permits
    exactly two direct children of `/server/{admin_path}` — `{admin_username}`
    and `config`. The route tree already follows that shape, so no migration
    is required.

74. STALE (removed 2026-09-24): admin global search is implemented in
    `src/server/handler/admin_search.go` with coverage in
    `admin_search_test.go`; the dashboard `q` form is wired to it.

75. RESOLVED (2026-09-24): the admin sidebar's expand/collapse state now
    persists across page loads. A short-lived allow-listed cookie (the same
    pattern as `flash.go`) carries the open/closed state per section;
    `AdminTemplateData` reads it and emits the `open` attribute server-side
    (zero-JS correct), and `static/js/app.js` adds a `data-action` delegation
    that writes the cookie as the JS enhancement. The six `<details>` in
    `partial/admin_chrome.tmpl` carry `data-action` instead of hardcoding
    `open`.

91. RESOLVED (2026-09-24): `DecodeAndValidate` in `validate.go` is now
    content-type aware. `application/x-www-form-urlencoded` bodies decode via
    `decodeFormBody` reflection over `form:` tags (bools through
    `config.ParseBool`, never `strconv.ParseBool`; handles slices, maps, and
    the nested `AIBots` struct); `application/json` keeps the original path.
    `form:` tags were added alongside `json:` on the affected request
    structs. A form submit branches on `wantsFormSubmission(r)` and gets a
    POST-redirect-GET flash via `redirectAdminForm`; JSON clients keep the
    canonical API shape. All 31 call sites unchanged.

93. RESOLVED (2026-09-24): the notification-channel and email-template admin
    pages previously could not be made interactive without JavaScript for a
    second reason beyond item 91 - the whole `adminAPI` group was bearer-token
    authenticated (`TokenAuthMiddleware` + `RequireAdminToken`), not
    session-cookie authenticated, so a browser form POST would 401 even on the
    no-body routes (`enable`, `disable`, `initialize`). Session-authenticated,
    form-encoded POST-redirect-GET routes now exist alongside the
    token-authenticated JSON API: channels at `src/main.go:2109-2112`
    (`adminRoutes` group, `UpdateChannel`/`EnableChannel`/`DisableChannel`/
    `TestChannel`) and email templates at `src/main.go:2158-2160`
    (`UpdateTemplate`/`TestTemplate`/`ImportTemplate`). The token-auth JSON
    routes remain at `src/main.go:3814-3822`. Form submits set a flash via
    `SetFlash` and 303 back to the config page; JSON clients keep the canonical
    API shape. Handlers branch on `wantsFormSubmission(r)`. Form-PRG coverage
    in `admin_email_templates_test.go` and `notification_channels_test.go`.
    Read: AI.md PART 16, 17.

103. STALE (removed 2026-09-24): the claimed contradiction does not exist.
    AI.md line 30451 is the authoritative rule (only `{admin_username}` and
    `config` are direct children of `/server/{admin_path}`). Lines
    31020-31050 are an ASCII invite-flow diagram with no `/help` route; the
    real help route is `/server/{admin_path}/config/pages/help`, which is
    what `src/main.go:2430` registers. The implementation is correct as-is.

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

113. RESOLVED (2026-09-24): the coverage gate holds. The most recent
    `make test` run (after the item 40/91/93/124 test additions) reported
    60.2% coverage, above the 60% gate, with `src/graphql/generated.go`
    filtered out per SPEC.md. The legacy-deletion concern did not drop the
    package below threshold.

124. RESOLVED (2026-09-24): the admin_auth_test.go fixtures at the original
    line refs already bind via `dbtime.FormatSQLTimestamp` (the refs had
    drifted). The one remaining raw instance, `setup_test.go:76`
    (`CURRENT_TIMESTAMP` in the `seedSetupAdmin` INSERT), was converted to a
    bound `dbtime.FormatSQLTimestamp(time.Now())` parameter. No SQL VALUES
    clause in `src/server/middleware/` uses a raw time function anymore.

125. RESOLVED (committed `71a613184466`): `--maintenance setup` now follows
    AI.md PART 22 — clears the admin credentials and prints a one-time setup
    token for re-authentication, leaving all user data untouched.

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

174. RESOLVED (2026-09-24): the hardcoded-English JSON error strings in
    `validate.go`, `server_pages.go`, `admin_auth_settings.go`,
    `admin_passkey.go`, and `passkey.go` now go through the
    `RespondError`-family helpers with `Translate(r, "errors.*")`. New
    `errors.passkey.*` and `errors.contact.*` keys (plus the generic
    validate keys) were added to all 7 locale files with identical key sets;
    `scripts/i18n-validate.sh` and `TestLocaleKeyParity` pass. Every
    `err.Error()` previously passed into a response was replaced with a
    translated key and the raw error logged instead. `passkey.go:124`
    `RPDisplayName: "Weather"` is a WebAuthn relying-party name, not
    user-facing text, and was left as-is.

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

177. RESOLVED (2026-09-24, README portion; originally flagged 2026-09-03 from a
    user-reported compliance sweep covering
    README.md/CI-CD/TODO.AI.md/tools.go/renovate.json). README.md Features now
    covers registration modes, 2FA/passkey enrollment, passkey-only admin
    auth, recovery keys and account recovery, public
    profiles/visibility/avatars, the admin panel feature list, LDAP/OIDC,
    user self-service, explicit "not adopted" notes for PART 35
    (organizations) and PART 36 (custom domains), and GraphQL. The
    AI.md-vs-IDEA.md registration-mode conflict is resolved in favor of
    AI.md (the source of truth): the config, tests, IDEA.md, README.md, and
    `.claude/rules/optional-rules.md` all now declare exactly two modes,
    `open` (default) and `private`, and `LoadConfig` defaults to `open`.
    Remaining triage results for
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
    - `.github/workflows/*.yml` / `docker/*`: DONE this pass (corrected
      twice - two earlier passes in this same triage each wrongly called
      this compliant). First gap found: `docker/Dockerfile.dev` (a required
      root Docker file per AI.md PART 27's directory listing and
      `.claude/rules/docker-rules.md`) did not exist, and `docker.yml` had
      no job building/pushing the `:devel` tag. Fixed: created
      `docker/Dockerfile.dev` (structurally identical to `docker/Dockerfile`,
      `MODE=devel` baked in via `ENV`, never in the production Dockerfile);
      added a `schedule: 0 4 * * *` trigger to `docker.yml` plus a new
      `build-devel` job (builds `docker/Dockerfile.dev`, tags `:devel` only,
      runs on schedule + workflow_dispatch + every non-tag push); scoped
      `build-standard` to `if: github.event_name != 'schedule'`; removed the
      `:devel` tag from `build-standard`'s tag list (now owned by
      `build-devel`); fixed `docker/docker-compose.dev.yml` to pull `:devel`
      instead of `:latest`. Second gap (found only after the user reported
      the CI/CD files were STILL missing): AI.md PART 28 explicitly requires
      `docker-aio.yml` as its own standalone workflow file - "the only image
      type with its own dedicated workflow file" - but the AIO build had
      been left as a `build-aio` job inside `docker.yml` instead. Fixed:
      extracted it into a new `.github/workflows/docker-aio.yml` matching
      spec (own triggers - push + workflow_dispatch, no schedule since no
      `:devel-aio` tag exists; own `concurrency: group: docker-aio-${{
      github.ref }}`; no cross-workflow `needs:`), and removed the
      `build-aio` job from `docker.yml` (now exactly 2 jobs: `build-standard`,
      `build-devel`). Updated `.claude/rules/cicd-rules.md`'s Required
      Workflows table to list `docker.yml` and `docker-aio.yml` as separate
      rows. Verified with `act --list -W docker.yml` (2 jobs, push/schedule/
      workflow_dispatch) and `act --list -W docker-aio.yml` (1 job, push/
      workflow_dispatch only).
    Read: AI.md PART 27, PART 28, PART 30 (README source-of-truth), PART 34.

178. RESOLVED (2026-09-03): app-breaking bug in
    `src/server/service/location_enhancer.go` (city/country NAME search, used
    for `GET /api/v1/locations/search` - distinct from the IP-based GeoIP
    subsystem in `src/server/service/geoip.go`, unaffected) was fetching its
    country/city datasets at runtime from two GitHub repos that never
    existed (`webappsgo/countries`, `webappsgo/citylist`, and the pre-rename
    `casapps/*` equivalents - all confirmed 404), causing
    `/api/v1/locations/search` to silently fail for any query not already
    resolvable by ZIP/coordinates. User chose "vendor a real static dataset"
    over repointing to a different live source. Fix: transformed GeoNames.org
    (`cities15000.txt`, `countryInfo.txt`, `admin1CodesASCII.txt`, CC BY 4.0)
    into the project's exact `Country`/`City` schema, committed as
    `src/server/service/data/{countries,cities}.json` (252 countries, 34,133
    cities), embedded via `go:embed` (mirrors the `src/common/i18n` embed
    pattern), and rewired `loadCountriesData`/`loadCitiesData` to read the
    embedded files instead of making HTTP requests. Removed the now-dead
    `http.Client`/`http.Transport` from `NewLocationEnhancer`.
    `location_enhancer_test.go` rewritten to exercise the embedded-data path
    (`fakeUpstreamTransport`/`httptest` mocks removed). `make test` passes in
    Docker, 60.0% coverage. `LICENSE.md`'s stale/incorrect third-party entry
    (wrong dead repos, wrong licenses CC BY-SA 4.0/MPL 2.0) replaced with the
    correct GeoNames CC BY 4.0 attribution.

179. RESOLVED (2026-09-24): a single `/server/about` "Data Sources" block
    now carries the third-party attributions in both HTML and JSON. The
    `page/about.tmpl` keys were reconciled to the `about_datasource_*` keys
    that exist in `en.json` (no more missing `about_source_*` keys).
    `ShowAboutPage`/`GetAboutAPI` (`server_pages.go`) carry the PART 20
    verbatim text: `<a href="https://db-ip.com/">IP Geolocation by
    DB-IP</a>` and `Country and ASN data licensed CC BY 4.0 by the Number
    Resource Organization (NRO).`; the same attribution was added to
    `LICENSE.md` Acknowledgments. `GetPrivacyAPI`'s `third_parties` list was
    completed (OSM Nominatim, GeoNames, DB-IP, NRO, and the severe-alert
    agencies) as a translated key rather than a hardcoded slice.

180. TODO (flagged 2026-09-24 during item 93): `src/server/template/template_editor.tmpl`
    documents a `{{$apiPath}}/server/templates` REST API (list/detail/variables/
    create/update endpoints) that does not exist. The real token-auth template
    routes are mounted at `/config/templates` (`src/main.go:3759-3767`,
    `templateHandler.ListTemplates`/`GetTemplate`/`GetTemplateVariables`/
    `CreateTemplate`/`UpdateTemplate`), and there is no `/server/templates`
    route anywhere in `src/main.go`. The page's endpoint reference block is
    therefore wrong and its "empty state" never lists real templates. Either
    wire the page to the real `/config/templates` API or remove the stale
    endpoint documentation. Read: AI.md PART 14, 17.

181. TODO (flagged 2026-09-24 during the final compliance sweep): raw
    `err.Error()` values are still passed into HTTP responses at
    `src/server/handler/twofa.go:146,287,332,378` and
    `src/server/handler/admin_ssl.go:123,131,170,187,316`. These can leak
    internal error chains/SQL text to the client (PART 9/11). Replace each
    with a translated `errors.*` key via the `RespondError`-family helpers
    and log the raw error instead. Read: AI.md PART 9, 11, 31.

182. TODO (flagged 2026-09-24 during the final compliance sweep): direct
    `m.DB` dereferences remain outside the accessor pattern item 40
    established — `src/server/model/location.go:27`,
    `src/server/model/token_v2.go:146`,
    `src/server/model/notification_model.go:13`, and
    `src/server/model/notification.go`. Route each through a `getDB()`
    accessor (injected handle, fall back to the global dual-DB accessor)
    matching the pattern in `user.go:164-182` / `settings.go`. Read: AI.md
    PART 10.

183. TODO (flagged 2026-09-24 during the final compliance sweep):
    `LICENSE.md:208-211` still claims the project sets "No cookies", which
    is stale — flash, admin-nav, session, and theme cookies are all set.
    `LICENSE.md:256-258` also carries a stale version/date. Correct both so
    the file reflects the shipped behavior. Read: AI.md PART 2.

184. RESOLVED (2026-09-24): a background security review flagged
    `src/server/middleware/ratelimit.go` as failing open when
    `server.rate_limit.enabled` is unset. The premise is disproved by direct
    code evidence — no code change was needed. `LoadConfig` seeds
    `Server.RateLimit: DefaultRateLimitConfig()` (config.go:903), which sets
    `Enabled: true` plus every numeric bucket, and unmarshals into
    `parsed := *cfg` (config.go:999) so an absent `rate_limit:` block leaves
    the seeded values intact. An explicit `enabled: false` is a documented
    PART 12 operator setting (`rate_limit.enabled` toggle, default On) and
    must be honored, so it must NOT be coerced back to true. The numeric
    buckets are additionally defended three times:
    `DefaultRateLimitConfig` seeding → `validateServerRateLimit` /
    `validateRateLimitBucket` repairing any `Requests < 1` / `Window < 1` /
    `GlobalBurst < 1` with a warning → the `bucket()` closure and
    `GlobalBurst <= 0` check inside `resolveRateLimitBuckets` re-applying
    defaults at the use site. Read: AI.md PART 12.

185. FIXED (2026-09-24): `handler.UpdateAdminNavState` was flagged as a
    state-changing admin endpoint with no authentication, no CSRF check, and
    a form read that accepted query params. The auth/CSRF claim is
    disproved by the route placement and the global middleware chain:
    `adminRoutes` (`src/main.go:1934-1941`) mounts only
    `/server/{cfg.GetAdminPath()}` and applies `SetupTokenRequired`,
    `RequireAdminAuth`, `AdminRateLimitMiddleware`, and `AuditLogger`, and
    `middleware.CSRFProtection` wraps the whole router at `src/main.go:545`.
    Per AI.md PART 17 "Access Control on Admin Routes", every method and
    sub-path under `/server/{admin_path}/**` is gated identically, so no
    in-handler re-check is needed. The one valid part of the finding — the
    handler read `r.Form["collapsed"]`, which merges query parameters into
    the body read and let a crafted link alter the written cookie — is
    fixed: it now reads `r.PostForm["collapsed"]`, so only the submitted
    form body contributes. Regression test
    `TestUpdateAdminNavStateIgnoresQueryParams` in
    `src/server/handler/admin_nav_state_test.go` locks that in. Read:
    AI.md PART 17.
