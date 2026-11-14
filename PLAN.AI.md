# PLAN.AI.md

Remaining work to bring wthr into line with AI.md (the source of truth). Delete this file once every item is done and committed. Verified against AI.md and the code on 2026-11-14; earlier lists in this file were partly stale.

## Done

- Admin routes without a `config` segment, `validAdminRootPaths`, one `/server/auth/logout`, unauthenticated admin requests redirect to `/server/auth/login`
- `yaml.d` drop-in config, `web.robots.*`, sitemap and feeds, `GET /api/v1/server/contact`, `GET /api/v1/server/auth/check`
- Tor and I2P health follow the four-word contract (PART 13); OpenRC detection (PART 8/24); `tests/incus.sh [systemd|openrc|all]`; `i2pd` in the images
- Contact field descriptors (`required_when`, `max_length_when`)
- GraphQL is query-only; help, terms and about JSON come from the locale files; rate-limit text matches the AI.md defaults
- PART 11 security reports: `security_id`, security mode of `/server/contact` (page and API), AES-256-GCM or PGP encrypted storage, researcher status page, hall of fame, policy editor, PGP keypair management, `/.well-known/pgp-key.asc`, retention task, backup of `{config_dir}/security/`
- Canonical error shape in the Tor admin, admin web and scheduler handlers

## Open (verified missing or divergent)

- PART 10/5 table names: the SQLite names must be unprefixed (`admins`, `users`, `audit_log`, `tokens`, ...) with admins, tokens, passkeys and TOTP in `users.db`; the code uses `server_*` and `user_*` names with admins in `server.db`
- PART 5 single-instance mode has no config table: settings still live in `server_config` next to `server.yml`
- PART 11 `app_secrets`: only `installation_secret` exists; `cookie_signing_key` and `csrf_token_secret` (with rotation) are missing, CSRF uses random double-submit tokens
- PART 11 security reports: maintainer email to admins who registered a personal PGP key (admin profile setting) is not built; the maintainer mail never carries content
- PART 17 admin TOTP and recovery keys (only passkey as a second factor exists); external identity (OIDC/LDAP/SAML) first-login username step for Server Admins
- PART 9 version-change purge: `{project_name}_build` cookie, `Clear-Site-Data: "cache", "storage"` on mismatch, client watchdog in `app.js`
- PART 12 `server.baseurl` / `X-Forwarded-Prefix` per request for the `path` template helper; GeoIP deny-only ordering
- `config.LoadConfig()` is called per request in 24 places; use the live global config
- `smtp_password` is passed to the settings template (never expose it)
- `POST /server/administration/restart` only logs; it does not restart
- PART 31 plural messages are not nested under CLDR categories (`*_singular` / `*_plural` pairs remain); hardcoded English in `app.js` toasts and some handlers
- PART 14 the OpenAPI document is built at runtime from the live routes with thin schemas; build-time generation with response schemas is not wired
- Template pages still concatenate some UI labels
