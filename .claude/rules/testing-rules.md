# Testing & Docs Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Run binaries / `go test` directly on the host — always inside Docker (`golang:alpine`) or Incus
- ❌ Use the project directory for test data — temp dirs only (`${TMPDIR:-/tmp}/apimgr/weather-XXXXXX/`)
- ❌ Skip integration tests against a real DB — DO NOT mock the database
- ❌ Run `incus.sh` / `docker.sh` on shared hosts without project-scoping the container/instance name (`test-weather`)
- ❌ Run `reboot`, `systemctl restart` host services, `iptables`, package installers from inside test scripts when targeting the host (use guest-scoped invocations)
- ❌ Skip ReadTheDocs hosting setup — `mkdocs.yml` + `.readthedocs.yaml` are REQUIRED
- ❌ Skip `lang="{{.Lang}}" dir="{{.Dir}}"` on `<html>` — every template must support i18n
- ❌ Hardcode English strings in user-facing code — use `i18n.T(lang, "key")` / `i18n.Tf(lang, "key", args)` / `t(r, "errors.*")` / template `{{t .Lang "key"}}`
- ❌ Skip translation parity — when a key exists in `en.json`, it MUST exist in every other locale (es/fr/de/zh/ar/ja)
- ❌ Mix log messages and translation keys — log messages stay English (operators); user-facing strings translate
- ❌ Hardcode `lang="en"` in the client / agent help output — they support `--lang` too

## CRITICAL - ALWAYS DO
- ✅ `tests/run_tests.sh` auto-detects Incus (preferred) or Docker (fallback) and runs the suite
- ✅ `tests/incus.sh` — full-OS systemd integration tests (preferred for service install / privilege drop / Tor)
- ✅ `tests/docker.sh` — fast ephemeral integration tests
- ✅ Test temp dirs follow `${TMPDIR:-/tmp}/apimgr/weather-XXXXXX/`; cleanup on success AND failure
- ✅ Integration tests hit a real DB (SQLite for fast tests; Postgres in a sidecar container for cluster tests)
- ✅ Content negotiation tests cover EVERY route: HTML for browsers, JSON for `Accept: application/json`, plain text for `Accept: text/plain` curl
- ✅ Host Safety in test scripts: every `systemctl` / `iptables` / `apt` invocation is wrapped in `incus exec test-weather --` or `docker exec ${CTR} --`
- ✅ AI Docker Compose Rules: AI may use `docker-compose.dev.yml` and `docker-compose.test.yml`; production `docker-compose.yml` is read-only for AI (no `up`)
- ✅ ReadTheDocs:
  - `mkdocs.yml` with material theme, `site_url` matches RTD URL, nav has `index`, `installation`, `configuration`, `api`, `cli`, `admin`, `development`
  - `.readthedocs.yaml` (Python 3.x build, install `docs/requirements.txt`, run `mkdocs build`)
  - `docs/requirements.txt` lists `mkdocs`, `mkdocs-material`, plugins
  - Docs stay in sync with code (Swagger / GraphQL / CLI `--help` / config schema)
- ✅ I18N (PART 31):
  - Locales in `src/common/i18n/locales/` embedded via `embed`
  - Base `en.json` (always 100% complete)
  - Translations: `es`, `fr`, `de`, `zh`, `ar`, `ja` (more allowed)
  - `i18n.T(lang, "key")` / `i18n.Tf(lang, "key", args...)`; missing keys fall back to English
  - `--lang=<code>` flag on server, client, agent
  - `<html lang="{{.Lang}}" dir="{{.Dir}}">` (`dir` for RTL languages: `ar`, `he`, `fa`)
- ✅ A11Y (PART 31):
  - WCAG 2.1 AA
  - Touch targets ≥ 44×44 CSS pixels
  - Keyboard navigation works on EVERY page
  - ARIA labels on all interactive elements
  - Color contrast ≥ 4.5:1 (normal text), 3:1 (large text)
  - `prefers-reduced-motion` honored
  - `prefers-color-scheme` honored for default theme

## TEST CHECKLIST (PART 29)
| Layer | Tool | Where |
|-------|------|-------|
| Unit | `go test ./src/...` | inside `golang:alpine` |
| Integration | `tests/run_tests.sh` | Incus or Docker |
| E2E | `tests/incus.sh` | Incus (full systemd) |
| Content negotiation | per-route | integration tier |
| Backup roundtrip | `--maintenance backup` then `--maintenance restore` | integration tier |
| Update roundtrip | `--update yes` against a staged release | integration tier |
| Cluster | 3-node Postgres + Valkey | integration tier |

## I18N KEYS (always add when touching UI/CLI/agent)
| Trigger | Action |
|---------|--------|
| New `http.Error()` | `t(r, "errors.*")` (NOT hardcoded English) |
| `fmt.Printf` w/ user-visible text | `i18n.Tf(lang, "key", args...)` |
| New admin/user template label | Add key to all 7 locales (en + es/fr/de/zh/ar/ja) |
| New CLI flag help | Add `cli.*` key |
| New error type | Add `errors.*` key |
| HTML template text | `{{t .Lang "key"}}` |
| `<html>` tag | `lang="{{.Lang}}" dir="{{.Dir}}"` |

DO NOT translate: log messages, structured-log error fields, test assertion strings, machine-readable responses (`OK`, status codes), MIME types, header names, config keys.

---
For complete details, see AI.md PART 29, 30, 31
