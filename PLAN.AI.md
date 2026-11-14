# PLAN.AI.md

Remaining work to bring wthr into line with AI.md (the source of truth). Delete this file once every item is done and committed. Checked against AI.md and the code on 2026-11-13; earlier lists in this file were partly stale.

## Done in the current pass

- Admin routes without a `config` segment; `validAdminRootPaths` and admin-username collision check; `/dashboard` alias removed; one `/server/auth/logout`; unauthenticated admin requests redirect to `/server/auth/login`; admin pages stay HTML for every client
- Invite routes at `/server/auth/invite/...` with absolute emailed links
- `yaml.d` drop-in config (merge, reload, backup, startup log, never copied into server.yml)
- `web.robots.allow/deny`, generated robots.txt, always-on sitemap limited to robots-allowed public pages, `/feed.xml` and `/rss.xml`, `server.seo.feed.*`
- `GET /api/v1/server/contact`, `GET /api/v1/server/auth/check`, contact limits in characters with field-level errors
- Reserved-name list and matching rules; admin usernames exempt from the public blocklist

## Open (verified missing or divergent)

- PART 11 security reports pipeline: `security_id`, report mode of `/server/contact`, encrypted storage, tracking pages, PGP keypair, admin pages for reports, policy, thanks and keypair
- PART 14 GraphQL is query-only: the schema still has a `Mutation` type (about 70 operations); Swagger/GraphQL generation is not wired into the build
- PART 5 single-instance mode has no config table: admin settings still live in the `server_config` table next to `server.yml`; per-field read-only display for yaml.d-owned settings needs that unification
- PART 17 external identity (OIDC/LDAP/SAML) first-login username step for Server Admins; admin auth is passkey-only in IDEA.md while AI.md defines password login with optional TOTP or passkey
- Legacy error shape `{"error":{...}}` in `admin_tor.go`, `scheduler.go`, `mcp/server.go`; hardcoded English strings in some handlers and in `app.js` toasts
- `config.LoadConfig()` is called per request in several handlers and middleware; use the live global config
- Template pages still concatenate UI labels; `smtp_password` is passed to the settings template

## PART 31 I18N

- plural messages are not nested under CLDR categories (`*_singular` / `*_plural` key pairs remain); `TestLocaleKeyParity` does not apply plural key parity
- several handlers still carry hardcoded English (`GetTermsAPI`, `GetHelpAPI`, `GetAboutAPI` section text)

## PART 5/6/8 configuration and CLI

- mode precedence must read `server.mode` from `server.yml` after `MODE`
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
