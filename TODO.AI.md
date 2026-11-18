# TODO

Project work still needed to bring wthr into line with AI.md. AI.md errors and contradictions are logged separately in ~/Projects/github/webappsgo/TODO.SERVER.md.

- PART 12 schema: tables and database placement follow AI.md, but column names and types still follow the project (DATETIME text instead of unix integers, `password_hash`, JSON blob `admin_preferences`, `user_sessions.id` integer); align the DDL examples column by column
- PART 5 single-instance mode has no config table: settings still live in `server_config` next to `server.yml`; move them into `server.yml` and keep a config table only for cluster mode (`srv_config`, `srv_config_meta`)
- PART 11 `app_secrets`: the store, the three generated secrets, the previous-value rows, age-based rotation, retirement, HMAC CSRF tokens and cookie signing are done; left: rotation of `installation_secret` and `server.security.encryption_key` with eager re-encryption of dependents (PGP private key, 2FA secrets, report bodies) and the admin pages `/security/keypair` and `/security/encryption` with the password re-prompt and audit events `security.installation_secret_rotated` / `security.encryption_key_rotated`; cluster propagation of the secrets and heartbeat versions
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
- ORDER (dependencies decide, number is only a tiebreaker): 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7 -> 8 -> 9 -> 10 -> 11. Item 1 blocks the PID directory ownership and Tor/I2P child credentials; item 2 blocks every setting that moves into server.yml (3, 4)
- 1. PART 8 / 24 / 25 remaining after the root phase (service user, ownership, early ports, privileged pre-bind, drop are done): step 8g spawns Tor with service-user credentials before the drop (Tor is started after the drop today, still not as root); Windows uses NSSM instead of a Virtual Service Account service; the launchd plist and the rc.d script need the PART 25 form; the HTTPS port is not pre-bound because the server does not serve TLS yet (PART 15 item)
- 2. PART 5 / 10 config storage (see the PART 5/10 item above): server.yml source of truth, `srv_config` only in cluster mode
- 3. PART 12 CORS and CSRF are done (web.cors resolution, web.csrf config, admin preview, CSP connect-src shares the list); left: the legacy `cors.*` / `security.cors_origins` rows in `server_config` and the dashboard keys in admin.go are dead and go with item 2
- 4. PART 11 (see the PART 11 items above), including HMAC CSRF and the `web.csrf` config block
- 5. PART 15 DNS-01 credential contract: `dns_provider` plus a flat `dns_credentials` map of lego env names, `enc:v1:` AES-256-GCM at rest, constructor-only passing to lego, `app_secrets.dns_validated_at`; nothing is implemented yet
- 6. PART 17 / 34 (see the admin TOTP item above)
- 7. PART 14 (see the OpenAPI item above)
- 8. PART 31 (see the plural/JS strings item above)
- 9. PART 10 DDL alignment (see the PART 12 schema item above); `admins.api_token_hash` still exists in `legacy_tables.go` while AI.md removed it
- 10. Legacy stubs listed above (admin_geoip.tmpl, CheckSSLRenewal, backup version/password)
- 11. Not yet re-audited against AI.md: PARTS 7, 9, 13, 16, 18, 20-23, 26-30, 32, 33
- AI.md PART 29 line 48167 reads the PID file as `/var/run/${INTERNAL_ORG}/${INTERNAL_NAME}.pid`, which contradicts the per-service PID directory in PARTS 3, 4, 8 and 25; follow PART 8 and report it in the template inbox
- PART 15: the server never serves TLS (no ListenAndServeTLS, no certificate loading in main.go), so `--port 80,443` and `ssl.enabled` do nothing; the Let's Encrypt service exists but is not wired to a listener
- PART 5: `cli.GenerateServerYML` writes a hand-written YAML with inline comments, `# Last updated: AUTO_GENERATED` and DB-era wording; generate server.yml from the config defaults with comments above each key
- AI.md PART 26 `dev` target writes the binary to `$BUILD_DIR` inside the container, which is not mounted, so the file never reaches the host; follow the spec as written and report it in the template inbox (`make local` builds into the mounted `binaries/`)
- AI.md names a `learned_origins` table and a `learned_origins_version` heartbeat field (PART 10) but never defines the table, so learned CORS/CSP hosts are kept in memory per node; report the missing schema in the template inbox and add the table once it is defined
- PART 12 session cookies are set with SameSite=Lax in handler/auth.go while AI.md defaults `same_site` to Strict (OAuth callback cookies are the only Lax ones); the CSRF same-origin bypass of PART 16 depends on Strict, and `server.session.*` max_age/idle_timeout are still not applied
- tests/incus.sh: the script it sends into the container fails with `bash: line 185: syntax error: unexpected end of file`, so Phase 2 (`./tests/run_tests.sh`) never reaches the binary; fix the embedded script (likely an unterminated heredoc, quote or block) and rerun Phase 2
