# TODO

Project work still needed to bring wthr into line with AI.md. AI.md errors and contradictions are logged separately in ~/Projects/github/webappsgo/TODO.SERVER.md.

- PART 10/5 table names: the SQLite names must be unprefixed (`admins`, `users`, `audit_log`, `tokens`, ...) with admins, tokens, passkeys and TOTP in `users.db`; the code uses `server_*` and `user_*` names with admins in `server.db`
- PART 5 single-instance mode has no config table: settings still live in `server_config` next to `server.yml`
- PART 11 `app_secrets`: only `installation_secret` exists; `cookie_signing_key` and `csrf_token_secret` (with rotation) are missing, CSRF uses random double-submit tokens
- PART 11 security reports: maintainer email to admins who registered a personal PGP key (admin profile setting) is not built; the maintainer mail never carries content
- PART 17 admin TOTP and recovery keys (only passkey as a second factor exists); external identity (OIDC/LDAP/SAML) first-login username step for Server Admins
- PART 12 `server.baseurl` / `X-Forwarded-Prefix` per request for the `path` template helper; GeoIP deny-only ordering
- PART 31 plural messages are not nested under CLDR categories (`*_singular` / `*_plural` pairs remain); hardcoded English in `app.js` toasts and some handlers
- PART 14 the OpenAPI document is built at runtime from the live routes with thin schemas; build-time generation with response schemas is not wired
- Template pages still concatenate some UI labels
