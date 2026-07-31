# Testing, Docs & I18N Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER run `go build`/`go test`/compiled binaries directly on the local machine — local machine has NO Go, all builds/tests run in Docker or Incus
- NEVER use `docker-compose.yml` or `docker-compose.dev.yml` as an AI — human-only; AI uses `docker-compose.test.yml` (prefer `tests/` scripts)
- NEVER put runtime/test data in the project directory — always `/tmp/{project_org}/{internal_name}-XXXXXX/`
- NEVER use bare `/tmp` root or unprefixed `mktemp -d` — always org/project structured temp dirs
- NEVER bypass or skip authentication in tests (including debug mode) — tests must verify auth works, not work around it
- NEVER use `pkill -f`, `killall`, `kill -9` first, `docker rm $(docker ps -aq)`, `docker system prune`, or any broad/pattern-based kill/cleanup — target exact PID/container name only
- NEVER put non-ReadTheDocs files in `docs/` — it is ONLY for MkDocs documentation source
- NEVER hardcode user-facing strings — every string a human reads MUST use a translation key (`t()`/`{{t .Lang key}}`), including HTTP errors, API JSON messages, Swagger/GraphQL descriptions, emails, and CLI/agent/server output
- NEVER let a missing/unsupported language error or crash — always silently fall back to English (`en`)
- NEVER convey information by color alone (accessibility)

## CRITICAL - ALWAYS DO
- ALWAYS run BOTH test phases: Phase 1 `make test` (`go test`, ≥60% coverage, pre-commit gate) and Phase 2 `./tests/run_tests.sh` (compiled binary, 100% endpoint coverage, manual/developer-initiated)
- ALWAYS create/update the matching `*_test.go` in the same work pass when adding or changing package logic — never defer to the end
- ALWAYS test every route with all applicable Accept headers: frontend (`text/html`, `text/plain`), API (`application/json`, `text/plain`), plus every `.txt` endpoint
- ALWAYS have `tests/run_tests.sh`, `tests/docker.sh`, `tests/incus.sh` (executable, WTFPL-licensed) — Incus preferred (full systemd), Docker fallback
- ALWAYS build via Docker (`casjaysdev/go:latest`) with host `GO_CACHE`/`GO_BUILD` cache dirs mounted; output to `binaries/`
- ALWAYS graceful process termination: identify exact PID first, `kill {pid}` (SIGTERM), wait, `kill -9` only if still running
- ALWAYS host RTD documentation via MkDocs Material (`docs/` + `mkdocs.yml` + `.readthedocs.yaml`); required pages: index, installation, configuration, api, admin, security, integrations, development (+ cli if applicable)
- ALWAYS keep every language file's keys identical to `en.json` — enforced by `make i18n-validate` / build-time check
- ALWAYS support the fallback chain `?lang= (sets cookie) → lang cookie → Accept-Language header → en`
- ALWAYS meet WCAG 2.1 AA: keyboard nav, screen reader support, 4.5:1 text contrast, visible focus indicators, 44x44px touch targets, skip links as first focusable elements

## Testing (PART 29)

**Two-phase strategy**
| Phase | Files | Run With | Tests | Gate |
|-------|-------|----------|-------|------|
| 1 — Toolchain | `*_test.go` | `make test` | Package logic via `go test` | ≥60% coverage, pre-commit |
| 2 — Binary Validation | `./tests/*.sh` | `./tests/run_tests.sh` | Full running binary: routes, auth, CLI/agent | 100% endpoint/route coverage, manual |

- Decision rule: provable without a running app → `*_test.go`; needs a running binary/HTTP/container → `./tests/*.sh`. Neither replaces the other.
- Critical paths (auth, DB, token validation) and all reachable error paths must always be tested regardless of overall %.
- "Tested manually" / "obvious code" / "just a getter" excuses are rejected — write the test.

**Container-only execution**
- Building: Docker, `casjaysdev/go:latest`. Container testing: Docker, `alpine:latest`. Full OS/systemd testing: Incus, `debian:latest` (prefer `images:debian/trixie`).
- Host System Safety Rule (PART 0) applies to every testing/debug action — anything touching `reboot`/`systemctl`/`iptables`/`mount`/package install runs in a container/VM/namespace, never the host.

**Temp directories**
- Required pattern: `/tmp/{project_org}/{internal_name}-XXXXXX/` (and `.../volumes/config`, `.../volumes/data`). Never bare `/tmp`, never unprefixed `mktemp -d`.
- Cleanup: `rm -rf "${TMPDIR:-/tmp}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX"` (saved var from `mktemp`), never a broad sweep.

**Config files**
- Never committed to repo — always runtime-generated on first run into the OS config directory (PART 4), with flags > env > config file > embedded defaults precedence.

**docker.sh / incus.sh must**
- Build server + client (if `src/client/`) + agent (if `src/agent/`) to `binaries/`
- Install test tools (`curl bash file jq`), test `--version`/`--help`, binary-rename test, admin setup-token → login → API token flow, full CLI/agent functionality against the running server, content negotiation, all IDEA.md endpoints, `trap` cleanup, exit 0/nonzero

**Admin route testing**
- Must prove auth works: unauthenticated rejected (302/401), setup-token access works, login flow works, invalid credentials rejected (401/403). Debug mode adds logging only — never bypasses auth, ever, in any mode, in any test.

**Process/container cleanup**
- Forbidden: `pkill -f`, bare `pkill`/`killall`, `kill -9` as first resort, `docker kill`, any `docker ... $(docker ... -q)` mass op, `docker system/container/image/volume/network prune`, unscoped `rm -rf`.
- Allowed: exact PID kill after verification, `pkill -x {project_name}`, `docker stop/rm {project_name}`, `docker rmi {project_org}/{internal_name}:tag`, `rm -rf $BUILD_DIR`/`$TEST_DIR` (from `mktemp`).

## ReadTheDocs Documentation (PART 30)

- Engine: MkDocs + Material theme (dark/light/auto, follows PART 16 theme rules). `docs/` = ONLY RTD source (never source code/scripts).
- Root files: `mkdocs.yml`, `.readthedocs.yaml`.
- Required `docs/` pages: `index.md`, `installation.md`, `configuration.md`, `api.md`, `admin.md`, `security.md`, `integrations.md`, `development.md`, `requirements.txt`; `cli.md` if the project has a CLI surface.
- `docs/` must reflect the shipped product as-is: browser, admin, API, config, and any public discovery/protocol surfaces (`/.well-known/**`, Swagger/GraphQL, OIDC metadata, app-association files, security reporting). Generated OpenAPI/GraphQL output does not replace these pages.
- RTD URL formats: `{project_org}-{project_name}.readthedocs.io` (org accounts) or `{project_name}.readthedocs.io` (standalone) or a custom domain — check the actual RTD dashboard.
- Theme customization allowed but must keep WCAG AA contrast (4.5:1 min) in both light and dark, and be documented in the project's AI.md.

## I18N & A11Y (PART 31)

**Scope**: every human-readable string anywhere — web frontend, admin panel, API/error responses, Swagger/GraphQL descriptions, email templates, server/CLI/agent output, health page, cookie consent, privacy/terms — must be translatable. No exceptions.

**Core rules**
| Requirement | Value |
|-------------|-------|
| Encoding | UTF-8 everywhere |
| Default language | `en` |
| Fallback chain (web) | `?lang=` (sets cookie) → `lang` cookie → `Accept-Language` header → `en` |
| Fallback chain (CLI/agent/server) | `--lang` flag → config file → `LANG`/`LC_ALL` env → auto-detect → `en` |
| Missing key | Falls back to English value |
| Unsupported language | Silently falls back to `en` — never error/crash |
| Key validation | Build-time check (`make i18n-validate`) — every language must match `en.json` key set |

- Supported languages (all binaries, no partial support): `en`, `es`, `zh`, `fr`, `ar` (RTL), `de`, `ja`.
- Translation files: single source of truth at `src/common/i18n/locales/{lang}.json`, embedded via `go:embed` and shared by server/CLI/agent — never duplicated or subsetted per binary. WebUI JS fetches `/locales/{lang}.json` served from the same embedded files.
- Key rules: no hardcoded strings anywhere; `http.Error()`/API JSON errors always use `t(r, "errors.*")`; dot-separated lowercase keys (`admin.dashboard.title`); `{variable}` interpolation; plurals nested under CLDR categories (`zero/one/two/few/many/other`).
- CLI/agent send `Accept-Language` header on API requests derived from their own `--lang`/config/env resolution.
- RTL: Arabic sets `dir="rtl"` from `meta.direction` in the locale file; CSS must use logical properties (`margin-inline-start`, not `margin-left`).
- Adding a language: copy `en.json` → translate all keys → add to `available_languages` config → `make i18n-validate` → rebuild all binaries.

**Accessibility (WCAG 2.1 AA mandatory)**
| Requirement | Standard |
|-------------|----------|
| Keyboard navigation | All functionality reachable via keyboard |
| Screen readers | Full NVDA/JAWS/VoiceOver support |
| Color contrast | 4.5:1 text, 3:1 large text/UI components/focus indicators |
| Focus indicators | Always visible |
| Touch targets | 44x44px minimum |

- Skip links (`Skip to main content`, `Skip to navigation`) must be the first focusable elements on every page.
- Use ARIA live regions for dynamic content (`role="status" aria-live="polite"` for status, `role="alert" aria-live="assertive"` for errors); modals need `role="dialog" aria-modal="true"` with focus trap and focus-return on close; use landmark roles (`banner`, `navigation`, `main`, `complementary`, `contentinfo`).
- Focus management: page load → main heading; modal open → first focusable inside; modal close → trigger element; toast notifications → do NOT move focus, use `aria-live` instead.
- Never convey meaning by color alone — pair color with an icon/text indicator.
- Test with axe DevTools, WAVE, Lighthouse, an actual screen reader, and keyboard-only navigation; automated integration tests must check skip links, alt text, label association, heading hierarchy, and landmarks.

For complete details, see AI.md PART 29, 30, 31
