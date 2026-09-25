# Fully implement remaining AI.md work (open items in TODO.AI.md)

## Context

`TODO.AI.md` still carries 10 open items. The user asked for all of AI.md
to be fully implemented and verified. This plan covers the real, still-open
defects found by reading the code, and records which items are stale
(premise disproved against AI.md itself) and must be removed rather than
"fixed".

Verified findings that drive the work:

- `SettingsModel` (6 direct `m.DB` derefs, no accessor) and
  `UserModel.CountByRole` panic on a nil DB handle; 12 sibling models
  already use a `getDB()` accessor (the pattern item 106 established).
- `DecodeAndValidate` is JSON-only, and 54 request-struct fields carry only
  `json:` + `binding:` tags — so admin mutation routes cannot be driven
  JS-free. AI.md PART 16 requires full JS-free operation.
- ~40 hardcoded English strings and ~20 raw `err.Error()` passthroughs in
  `validate.go`, `server_pages.go`, `admin_auth_settings.go`,
  `admin_passkey.go`, `passkey.go`.
- `page/about.tmpl` references 12 `about_source_*` i18n keys that do not
  exist; `en.json` has differently-named `about_datasource_*` keys. The
  Data Sources card currently renders raw key names.
- `LICENSE.md` has no DB-IP / NRO attribution, which AI.md PART 20 marks
  NON-NEGOTIABLE and requires verbatim.
- Six admin nav `<details>` elements hardcode `open` with no persistence.

## Stale items — remove from TODO.AI.md, do not "fix"

- **73** — admin route placement already follows AI.md:30451 (only
  `{admin_username}` and `config` are direct children of
  `/server/{admin_path}`).
- **74** — admin search is already implemented
  (`admin_search.go` + `admin_search_test.go`).
- **103** — the claimed contradiction does not exist. AI.md:30451 is the
  authoritative rule; AI.md:31020-31050 is an ASCII invite diagram with no
  `/help` route. The real help route is
  `/server/{admin_path}/config/pages/help`, which is what `main.go:2430`
  registers. Note the fix in TODO.AI.md before removing.
- **113** — verify the coverage gate on the final `make test` run and
  remove the item once the number is recorded.
- **125** — already implemented and committed (`71a613184466`); remove.
- **61** — BLOCKED upstream: CI govulncheck needs `casjaysdev/go:latest`
  rebuilt with Go 1.26.6+ for 7 stdlib CVEs. The Go version must never be
  hardcoded in this repo. Mark BLOCKED with that reason; do not attempt a
  workaround.

## Implementation order (dependency-sorted)

### 1. Item 40 — nil-safe DB handles
`src/server/model/settings.go`: add a `getDB()` accessor mirroring
`user.go:164-182` (injected handle, fall back to `database.GetServerDB()`
when nil) and route the 6 derefs (lines 38, 95, 106, 145, 151, 171) through
it. `src/server/model/user.go` `CountByRole` (line ~830): switch `m.DB`
to `m.getDB()`. Tests: `settings_test.go` / `user_test.go` — construct
with a nil handle and assert no panic.

### 2. Item 174 — i18n for admin/contact/passkey errors
Add `errors.passkey.*` and `errors.contact.*` keys (plus the two generic
validate keys) to all 7 locale files with identical key sets. Replace the
literals in `validate.go`, `server_pages.go`, `admin_auth_settings.go`,
`admin_passkey.go`, `passkey.go` with `Translate(r, "...")` following the
existing `twofa.go:217` precedent. Replace every `err.Error()` passed into
a response with a translated key; log the raw error instead. Leave
`passkey.go:124` `RPDisplayName: "Weather"` alone — it is a WebAuthn
relying-party name, not user-facing text.

### 3. Item 179 — data-source attribution
- Reconcile `page/about.tmpl` to the `about_datasource_*` keys that
  actually exist (or rename the en.json keys consistently; template edit is
  the smaller change).
- Add a single Data Sources block to `ShowAboutPage` + `GetAboutAPI`
  (`server_pages.go`) carrying the PART 20 verbatim text:
  `<a href="https://db-ip.com/">IP Geolocation by DB-IP</a>` and
  `Country and ASN data licensed CC BY 4.0 by the Number Resource
  Organization (NRO).`
- Add the same attribution to `LICENSE.md` Acknowledgments.
- Complete `GetPrivacyAPI`'s `third_parties` list (currently missing OSM
  Nominatim, GeoNames, DB-IP, NRO, and the severe-alert agencies) and
  make it a translated key rather than a hardcoded slice.

### 4. Item 75 — persistent admin nav sections
Mirror the allow-listed-cookie pattern in `flash.go`: a second short-lived
cookie carrying the open/closed state per section, validated against an
allow-list of section names. `AdminTemplateData` (`admin_context.go`)
reads it and emits the `open` attribute server-side (zero-JS correct);
`app.js` adds a `data-action` delegation that writes the cookie as the JS
enhancement. Update the six `<details>` in
`partial/admin_chrome.tmpl` to carry `data-action` instead of hardcoding
`open`.

### 5. Item 91 — form-encoded binding
Rewrite `DecodeAndValidate` in `validate.go` to be content-type aware:
`application/json` keeps the current path; `application/x-www-form-urlencoded`
uses `r.ParseForm` + a tag-name-`form` validator, decoding bools/ints/floats
through `config.ParseBool` (never `strconv.ParseBool`) and handling slices
(`AltNames`), maps (`Config`), and the nested `AIBots` struct. Add
`form:"..."` tags alongside the existing `json:` tags on the affected
request structs (`admin_ssl.go`, `locations.go`, `admin_web.go`,
`notification_preferences.go`, `auth_api.go`, `user_public.go`).

### 6. Item 93 — session-auth admin form routes
Add POST routes under the existing session-auth `adminRoutes` group
(main.go:1927-1934) for the channels and templates mutations, alongside —
not replacing — the token-auth `adminAPI` group (main.go:2679-2687).
Form submits set a flash via `SetFlash` and POST-redirect-GET back; JSON
clients keep the canonical API shape on the token-auth routes. The stale
`src/main.go:2463-2467` line reference in item 93 gets corrected.
Note the TODO line reference is wrong; record the real one.

### 7. Item 124 — test timestamp fixtures
`src/server/middleware/admin_auth_test.go:58,156`: replace
`CURRENT_TIMESTAMP` / `datetime(?, 'unixepoch')` with the
`dbtime.FormatSQLTimestamp` convention used by the rest of the suite.

### 8. Item 177 — README currency
Section order is already correct. Add the missing coverage: registration
modes, 2FA/passkey enrollment and passkey-only admin auth, recovery keys
and account recovery, public profiles/visibility/avatars, the admin panel
feature list, LDAP/OIDC, user self-service, explicit "not adopted" notes
for PART 35 (organizations) and PART 36 (custom domains), and GraphQL in
Features. The AI.md-vs-IDEA.md registration-mode conflict is resolved in
favor of AI.md PART 34 (two modes, `open` default / `private`); the README,
`optional-rules.md`, `config.go`, and the config tests now all agree.

## Verification

1. `python3 scripts/i18n-validate.sh` — 7 locales, identical key sets.
   (There is no `make i18n-validate` target; the script plus
   `TestLocaleKeyParity` in `i18n_test.go` are the real gates.)
2. `grep -rn 'TODO\|FIXME\|HACK' src/` — zero.
3. `grep -rn 'about_source_' src/` — zero.
4. `grep -n 'DB-IP\|Number Resource' LICENSE.md src/server/handler/server_pages.go` — both present.
5. `make test` — must pass and report ≥60% (currently exactly 60.0%, so
   every new handler needs matching tests or coverage will drop below the
   gate).
6. `./tests/run_tests.sh` — Phase 2 binary/endpoint coverage.
7. `gitcommit --dir /root/Projects/github/webappsgo/wthr all` after writing
   and re-reading `.git/COMMIT_MESS`; then check the triggered CI run.

## Risks

- **Coverage is the binding constraint.** At exactly 60.0%, any new
  handler code without a matching `_test.go` fails the pre-commit gate.
  Tests are written in the same work pass as each item, never deferred.
- **Item 91 is the largest change** — nested struct + map + slice
  form-decoding touches 31 `DecodeAndValidate` call sites. A
  content-type branch inside the existing helper keeps all 31 call sites
  unchanged; only the destination structs gain `form:` tags.
- **Item 174 spans 5 files and 7 locales**; a key-set mismatch fails
  `i18n-validate`, so add every key to all 7 files in the same edit.
- `rm`-class and Makefile-target changes are out of scope; the missing
  `i18n-validate` target is a rules-file inaccuracy, not a code defect, and
  is not fixed here (Makefile rules forbid adding targets beyond the six).
