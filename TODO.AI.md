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
- PART 11: app_secrets rotation, HMAC CSRF (HttpOnly=false per spec line 29887, Origin/SameSite bypass), cookie_signing_key, operator token regenerate
- PART 5/10: SettingsModel still backs ~100 legacy keys in `server_config`; move to server.yml (cluster-only `srv_config`)
- PART 12: `server.session.*` max_age/idle_timeout are validated but never parsed or applied
- Legacy stubs: `admin_geoip.tmpl` has hard-coded values and is not wired; `CheckSSLRenewal` only logs and does not renew; `CreateSystemBackup`/hourly use `AppVersion "1.0.0"` and an empty password
- Admin UI uses JS-built lists and hard-coded English strings in `app.js` (PART 16/31)
- Pre-existing gofmt fixes applied to cli/maintenance.go, auth_check.go, setup_test.go
- CI: `.github/workflows/ci.yml` has failed with "workflow file issue" (no jobs run) on every push since a66f937; the file parses as valid YAML, so find the GitHub-side rejection (check the Actions UI annotation) and fix it
- ORDER (dependencies decide, number is only a tiebreaker): 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7 -> 8 -> 9 -> 10 -> 11. Item 1 blocks the PID directory ownership and Tor/I2P child credentials; item 2 blocks every setting that moves into server.yml (3, 4)
- 1. PART 8 / 24 / 25 root phase: no in-process service user creation, chown, privilege drop after port binding or Windows Virtual Service Account (Windows uses NSSM today); the PID directory `/var/run/{org}/{name}/` must be created and owned in step 8b/8c
- 2. PART 5 / 10 config storage (see the PART 5/10 item above): server.yml source of truth, `srv_config` only in cluster mode
- 3. PART 12 CORS: hard-coded `*` with a short header list in main.go; needs `web.cors` resolution (explicit config, DOMAIN, learned proxy hosts, `*` fallback), the spec header list, credentials only for explicit origins, admin `/web` preview; CSRF must mount on frontend routes only, never `/api/**`
- 4. PART 11 (see the PART 11 items above), including HMAC CSRF and the `web.csrf` config block
- 5. PART 15 DNS-01 credential contract: `dns_provider` plus a flat `dns_credentials` map of lego env names, `enc:v1:` AES-256-GCM at rest, constructor-only passing to lego, `app_secrets.dns_validated_at`; nothing is implemented yet
- 6. PART 17 / 34 (see the admin TOTP item above)
- 7. PART 14 (see the OpenAPI item above)
- 8. PART 31 (see the plural/JS strings item above)
- 9. PART 10 DDL alignment (see the PART 12 schema item above); `admins.api_token_hash` still exists in `legacy_tables.go` while AI.md removed it
- 10. Legacy stubs listed above (admin_geoip.tmpl, CheckSSLRenewal, backup version/password)
- 11. Not yet re-audited against AI.md: PARTS 7, 9, 13, 16, 18, 20-23, 26-30, 32, 33
- AI.md PART 29 line 48167 reads the PID file as `/var/run/${INTERNAL_ORG}/${INTERNAL_NAME}.pid`, which contradicts the per-service PID directory in PARTS 3, 4, 8 and 25; follow PART 8 and report it in the template inbox
