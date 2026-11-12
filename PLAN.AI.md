# PLAN.AI.md

Remaining work to bring wthr into line with AI.md (the source of truth). Delete this file once
every item below is done and committed. Items are grouped by AI.md PART; each names what is
missing. Order: items marked (blocks) come first.

## Done in the current pass

- i18n: `it.json`, locale `meta` blocks, language registry with build-time check, dotted keys, 365 undefined template keys defined, template key existence test
- `WWW-Authenticate` on 401 and `/debug/*` operator token (`server.token`), `/debug/ip` removed in favor of `/api/v1/weather/locations`
- admin_path default `administration` (config, validation, docs, tests, scripts)
- AI-bot blocking removed (robots.txt has no ai_bots; admin UI, config, locale keys removed)
- robots.txt default per PART 16 (`Allow: /api`, `Disallow: /server/{admin_path}`)
- `/security.txt` no longer served; `/.well-known/change-password` per PART 16
- Tracking: `/server/tracking.js`, consent-gated tags, CSP hosts, onion/I2P never
- Contact: captcha (simple/recaptcha/hcaptcha), no-JS form POST + flash, canonical errors
- New pages and API mirrors: `/server/dpo`, `/server/help/mcp`, `/server/security`, `/server/security/policy`, `/server/security/thanks`
- `server.i18n.available_languages`, required six-language minimum, `server.contact.dpo`, `server.pages.*`
- IDEA.md, README.md, docs (api, security, configuration, admin paths), docs/translating.md, .gitignore, .dockerignore, `.claude/rules` partial refresh
- `tests/e2e.sh` and `tests/e2e/layout.mjs` (layout review matrix)

## PART 31 I18N

- plural messages are not nested under CLDR categories (`*_singular` / `*_plural` key pairs remain); `TestLocaleKeyParity` does not apply plural key parity
- several handlers still carry hardcoded English (`GetTermsAPI`, `GetHelpAPI`, `GetAboutAPI` section text)

## PART 5/6/8 configuration and CLI

- mode precedence must read `server.mode` from `server.yml` after `MODE`
- `--shell --help` instead of `--shell help`; `--version` first line `{name} {version} ({short_commit})`
- graceful shutdown order (readiness, long-lived connections with `event: shutdown`, drain, flush)
- systemd containers (Incus/LXC) detected via `INVOCATION_ID` before generic container detection
- `server.baseurl` / `X-Forwarded-Prefix` per request for the `path` template helper (helper currently reads only configured baseurl); migrate hardcoded same-origin links in the 136 templates to `{{ path "..." }}`

## PART 11/14 security and API

- GeoIP deny-only ordering (blocklist, rate limiter, GeoIP, then authentication)
- `security_id` rotating one-shot token, `/server/contact?security_id=`, security report pipeline (`/server/security/report/{tracking_id}`), PGP keypair management, admin pages for reports, policy, thanks, keypair
- `Referrer-Policy: no-referrer` and `Cache-Control: no-store` on one-shot link pages; token redaction in logs
- `Clear-Site-Data` purge limited to top-level navigations; client watchdog in `app.js`

## PART 13 health

- API text format with `features.{name}: {status}` lines; `features.i2p.state`
- blocklist dataset checks (`blocklist_{id}`) once blocklist sources are stored as files

## PART 16/17 frontend and admin

- spec CSS tokens: `--space-1..12`, `--z-raised/dropdown/overlay/drawer/modal/banner/toast`; `100dvh`; drawer containment (no negative offset, drawer and scrim as children of `<body>`, opaque, `inset-inline-end`)
- form layout and action-row rules, one `<h1>` per page, no string concatenation in UI labels
- `/server/preferences` single-form contract (export block inline, import field, Save/Cancel/Reset popover, consent and CCPA controls)
- service worker `asset_stamp`, `X-Build` header, update-and-restart client behavior
- `/server/help` and `/server/about` content sourced from IDEA.md, `GitOrg`/`GitRepo` hardcodes removed
- admin authentication: AI.md PART 17 defines password login with optional TOTP or passkey; the project (IDEA.md and code) is passkey-only for admins. Decide and align code, IDEA.md and docs/admin.md

## PART 18-23 features

- SMTP TLS verification policy (skip only for private IP literals) and RFC 5322 message standards
- backup password stored encrypted (`server.security.encryption_key`); incremental backup contract (cumulative incremental, `base_backup`, `deleted`, 3-file default, restore of base plus incremental)
- update download temp dir under `{project_org}/{internal_name}-XXXXXX`
- metrics `QueryRowContext` wrapper (`MetricsRow`)

## PART 24-28 service, Makefile, Docker, CI

- OpenRC and SysVinit templates: no `command_user` or `--chuid`
- `make release` tag and release names use `v{version}`; `clean` target
- `Dockerfile.dev` bakes `ENV MODE=development`; compose `name:`/network identical across prod/dev/test
- truffleHog blocking statuses per PART 0 (`verified` always, `unknown` only in range scans)

## PART 32/33 overlay and client

- Tor: spawn `tor` directly, poll `control_port` file, SAFECOOKIE auth, bootstrap events, `MaxClientCircuitsPending`, `startup_timeout`
- I2P: SAMv3 for both models, `i2pd.conf` generation, destination key `site-keys.dat`, `SILENT=true`, second forward socket; `server.i2p.*` config
- client display mode order (GUI, TUI, error) and `gui_unavailable`

## PART 29/30 tests and docs

- layout tests run `tests/e2e.sh` (needs a Playwright image pull; not run in this pass)
- `docs/` pages for tracking, captcha, MCP, DPO configuration
