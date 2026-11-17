# TODO

Project work still needed to bring wthr into line with AI.md. AI.md errors and contradictions are logged separately in ~/Projects/github/webappsgo/TODO.SERVER.md.

- PART 12 schema: tables and database placement follow AI.md, but column names and types still follow the project (DATETIME text instead of unix integers, `password_hash`, JSON blob `admin_preferences`, `user_sessions.id` integer); align the DDL examples column by column
- PART 5 single-instance mode has no config table: settings still live in `server_config` next to `server.yml`; move them into `server.yml` and keep a config table only for cluster mode (`srv_config`, `srv_config_meta`)
- PART 11 `app_secrets`: only `installation_secret` exists; `cookie_signing_key`, `csrf_token_secret`, the `_previous` / `_previous_until` rotation rows, installation secret and encryption key rotation (admin pages `/security/keypair`, `/security/encryption`) and the daily retirement task are missing; CSRF still uses random double-submit tokens
- PART 11 operator token: regenerate action on `/server/{admin_path}/security` with the `security.operator_token_regenerated` audit event
- PART 11 security reports: maintainer email to admins who registered a personal PGP key (admin profile setting) is not built; the maintainer mail never carries content
- PART 17 admin TOTP and recovery keys (only passkey as a second factor exists); external identity (OIDC/LDAP/SAML) first-login username step for Server Admins
- PART 12 `server.baseurl` / `X-Forwarded-Prefix` per request for the `path` template helper; GeoIP deny-only ordering
- PART 31 plural messages are not nested under CLDR categories (`*_singular` / `*_plural` pairs remain); hardcoded English in `app.js` toasts and some handlers
- PART 14 the OpenAPI document is built at runtime from the live routes with thin schemas; build-time generation with response schemas is not wired
- Template pages still concatenate some UI labels
- AI.md was updated mid-session (pairing rule, MCP CRUD tools, no-JS captcha, Tor and data contracts, private paths and 0600 server.yml, health-check log suppression); those PARTs have not been re-audited against the code yet
