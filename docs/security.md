# Security

## Authentication

### Admin Authentication

Server Admins sign in at the shared `/server/auth/login` page with their username (or email) and
password (Argon2id). An admin who has registered a passkey (WebAuthn: YubiKey, Touch ID, Windows
Hello or any FIDO2 device) must complete the passkey prompt before the session is created. If no
passkey is registered and TOTP is enabled, the admin must enter a current authenticator code or an
unused recovery code before the session is created.
Passkeys are stored in the `passkeys` table of `users.db`; the authenticator private key never
leaves the device. An unauthenticated request to `/server/administration` redirects to the login page.

| Flow | Description |
|------|-------------|
| **Sign in** | `/server/auth/login` → password → passkey prompt or TOTP/recovery challenge → session (30-day cookie) |
| **Register a passkey** | Admin panel → your profile → Security → add a passkey (the password is asked again) |
| **Enable TOTP** | Admin profile → Security → confirm password → scan QR/manual key → verify code and save ten one-time recovery codes |
| **Recovery codes** | Admin profile → Security; regenerate with a current TOTP code. Each code is stored hashed and consumed once |
| **Revoke** | Admin panel → your profile → Security → remove a passkey |

### User Authentication

Regular user accounts authenticate with **email + password (Argon2id)** and a persistent session
cookie. Users may also configure passkeys for passwordless login.

| Parameter | Value |
|-----------|-------|
| Hash algorithm | Argon2id |
| Time cost | 3 iterations |
| Memory cost | 64 MB |
| Parallelism | 4 threads |
| Hash length | 32 bytes |
| Session validity | 30 days (configurable) |

### Session Tokens

All session tokens are 32 bytes of `crypto/rand`, stored as SHA-256 in the database. The raw token
is returned to the client once (in the `Set-Cookie` header) and never stored in plaintext. Admin
sessions are prefixed `adm:` to distinguish them from user sessions.

---

## Authorization

| Role | Admin Panel | API (authenticated) | API (anonymous) |
|------|-------------|---------------------|-----------------|
| Anonymous | ✗ | ✗ | Read-only weather/public endpoints |
| Regular User | ✗ | Saved locations, subscriptions | — |
| Server Admin | ✓ full access | All endpoints + admin API | — |

Server Admins and Regular Users are stored in **separate tables** (`admins` and `users`) of
`users.db`, with admin login sessions in `server.db`. A Regular User account can never be elevated to Server Admin status through the normal
user-facing flows.

---

## Rate Limiting

All endpoints are rate-limited with a sliding window: per IP for anonymous requests, and per token
for requests carrying a valid Bearer token, so clients behind a shared IP never throttle one another.
Failed authentication is always counted per IP. The defaults below live under
`server.rate_limit` in `{config_dir}/yaml.d/security.yml` and are editable from the admin panel.

| Bucket | Default limit |
|--------|---------------|
| Read (GET and other safe methods) | 120 requests / minute |
| Write (POST, PUT, PATCH, DELETE) | 10 requests / minute |
| Health endpoints | 120 requests / minute |
| Global burst across all buckets | 240 requests / minute |
| Login | 5 attempts / 15 minutes |
| Password reset | 3 requests / hour |
| Registration | 5 requests / hour |

Exceeding the limit returns **HTTP 429 Too Many Requests** with a `Retry-After` header.

### Trusted IP allowlist

Administrators manage trusted IP addresses and CIDRs at `/server/{admin_path}/security/firewall`.
Entries are stored in `server.security.allowlist` in `{config_dir}/yaml.d/security.yml`. They bypass
IP blocklists, rate limits, and GeoIP country blocking. They never bypass admin authentication, API
token validation, CSRF protection, path security, or TLS requirements.

Administrators can add manual IP/CIDR blocks from the same page. Temporary blocks expire after one
hour, extended blocks after 24 hours, and permanent blocks have no expiry. Expired records remain
visible for review until purged.

---

## Service Account and Privilege Drop

When the server starts as root (a system service, or a container), it runs a root phase and then
runs as a dedicated account:

1. It creates the `wthr` system user and group when they do not exist. The UID and GID are the same
   value, chosen from 899 downward in the range 200-899 and never one of the reserved
   well-known IDs.
2. It gives that account ownership of the config, data, cache, log, database and backup directories
   and of the per-service PID directory `/var/run/webappsgo/wthr/`.
3. It makes every private path owner-only on every start: `server.yml`, `yaml.d/`, `ssl/`, `tor/`
   and `i2p/` below the config directory, and `security/` and `tor/` below the data directory,
   with everything inside them (directories `0700`, files `0600`).
4. It binds any port below 1024 while still root, so a service can listen on port 80 without
   running as root.
5. It drops to the service account and checks the drop: the process is no longer root and cannot
   become root again. If the drop fails the server exits instead of continuing as root.

A server started by a regular user skips this phase and keeps its own account. On Windows the
service runs as a Virtual Service Account instead.

## Transport Security

When a `server.domain` is configured (non-localhost), wthr automatically obtains and renews a
TLS certificate via Let's Encrypt (ACME HTTP-01 challenge). HTTP traffic is redirected to HTTPS.

Certificates obtained by wthr are stored in `{config_dir}/ssl/letsencrypt/{fqdn}/` as
`fullchain.pem` and `privkey.pem`, and are renewed automatically 7 days before expiry.
Self-signed and operator-supplied certificates live in `{config_dir}/ssl/local/{fqdn}/`
as `cert.pem` and `key.pem` and are never auto-renewed. Explicit certificate paths can be
set in `server.yml` under `ssl.cert_file` and `ssl.key_file`.

---

## Security Headers

All responses include the following security headers:

| Header | Value |
|--------|-------|
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'` |

---

## CSRF Protection

CSRF protects the browser pages and forms that authenticate with a session cookie. A state-changing
request (`POST`, `PUT`, `PATCH`, `DELETE`) to a frontend page must carry the token (a `crypto/rand`
value with an HMAC signature, set as a cookie and sent back in a hidden `csrf_token` field or the
`X-CSRF-Token` header). A token is replaced on login and logout, and a token that does not verify
(forged, or signed before a rotation) is never accepted.

CSRF never applies to the backend: no `/api/**` route issues, requires or checks a token, whatever
the authentication method. It is also skipped for `Authorization: Bearer` requests, public
endpoints, `GET`/`HEAD`/`OPTIONS`, WebSocket upgrades and paths listed in `web.csrf.exempt_paths`.
A failed check returns `403` with the error code `CSRF_FAILED`. See [Configuration](configuration.md)
for the `web.csrf` settings.

---

## Project Secrets

The server keeps three secrets in the `app_secrets` table of `server.db`. Each is 32 bytes from
`crypto/rand`, generated on first start, included in every backup and never logged or returned by
any API:

| Secret | Used for | Rotation |
|--------|----------|----------|
| `installation_secret` | The root secret the security-report ids and the PGP key encryption are derived from | Manual from the admin panel; the previous value stays valid for 7 days |
| `cookie_signing_key` | Signs the session cookies, so a forged or tampered cookie is dropped | Every 90 days; the previous value stays valid for 7 days |
| `csrf_token_secret` | Signs the CSRF tokens | Every 180 days and on every admin password change |

A rotation writes the previous value first, then the new one, then re-encrypts what depends on it,
so a crash loses nothing. The upkeep task deletes previous values once their grace period has
passed.

The optional scheduled-backup password is separate from these three application secrets. It is
AES-256-GCM encrypted with `server.security.encryption_key` and stored in `{config_dir}/secrets.yml`.
It is included in backups, never returned by an API, and re-encrypted when the server encryption key
rotates.

**Rotating from the admin panel** always asks for your password and writes an audit event:

- **Installation secret** (Security > Keypair > *Rotate installation secret*, event
  `security.installation_secret_rotated`): the PGP private key is re-encrypted under the new secret
  and security report links issued before the rotation keep working for 7 days.
- **Encryption key** (Security > Encryption, event `security.encryption_key_rotated`): the key
  `server.security.encryption_key` is replaced and its `encryption_key_version` incremented. Two-factor
  secrets, security report bodies and contacts, and the scheduled-backup password are re-encrypted immediately. The previous key is
  stored wrapped under the new one and stays valid for 30 days. A second rotation is refused while any
  row is still readable only with the previous key, and a re-encryption that a crash interrupted is
  finished at the next start. The API is `GET` and `POST
  /api/v1/server/{admin_path}/security/encryption` (the `POST` body is `{"current_password": "..."}`). After an upgrade that first enables cookie signing, every existing session cookie is
unsigned and is treated as logged out, so users and admins sign in once more.

---

## Operator Token

The operator token is one global credential for the `/debug/*` endpoints and operator-only
operations. It is a server setting, not a user account.

- It is `server.token` in `server.yml`: `tok_` followed by 32 random base62 characters. It is
  generated on first start when the key is absent or empty, saved back to `server.yml` (mode
  `0600`) and printed once in the startup output.
- Only `SHA-256(server.token)` is kept in memory. A presented token is hashed and compared in
  constant time. The raw value is never logged after that first print, never returned by an API and
  never included in a backup export.
- Send it as `Authorization: Bearer tok_...`. `--debug` turns the debug endpoints on; it never
  replaces the token.
- The **Security** admin page shows it masked. **Regenerate operator token** asks for your password,
  records `security.operator_token_regenerated` in the audit log, shows the new value once and stops
  the previous value working at once. The same action is `POST
  /api/v1/server/{admin_path}/security/operator-token` with `{"current_password": "..."}`.
- You can also blank `server.token` and restart to get a new value.

---

## CORS

One policy covers every route, pages and API alike. The allow-list is built on each request, in
this order:

1. `web.cors`: the origins listed there. An empty value turns CORS off. The shipped default `*`
   means "unset" and lets the next steps run.
2. The `DOMAIN` environment variable: each hostname is added as an `https://` origin.
3. Hostnames seen in `X-Forwarded-Host` from a trusted proxy.
4. If none of those produced a list: `*`.

Cookies and credentials (`Access-Control-Allow-Credentials: true`) are sent only to an origin that
is on an explicit list, never with `*`. A preflight request gets the CORS headers and a `204`.
Edit the setting under **Web** in the admin panel, where a preview shows the headers it produces.

---

## Input Validation

All input — URL parameters, request bodies, form fields, headers — is validated server-side before
use. Client-side validation is for UX only; the server never trusts client-supplied data.

Database queries use parameterized statements exclusively. String concatenation in SQL is never used.

---

## Audit Logging

Security-relevant events are written to the audit log:

- Admin login / logout / failed login
- Admin passkey registration and removal
- User registration, login, logout, failed login
- Password and email changes
- Settings changes (admin panel)
- Backup and restore operations
- Rate-limit blocks

Audit logs are append-only and never contain raw credentials or tokens.

---

## Well-Known & Public Endpoints

| Path | Purpose |
|------|---------|
| `/.well-known/acme-challenge/` | Let's Encrypt HTTP-01 ACME challenge (automatic TLS) |
| `/server/healthz` | Health status — content-negotiated (HTML for browsers, JSON for `Accept: application/json`, text for curl and wget) |
| `/api/{api_version}/server/healthz` | API health status — JSON by default, text via standard API text rules |
| `/api/healthz` | Unversioned alias for machine-friendly versionless probing |
| `/metrics` | Prometheus-compatible metrics (configurable auth) |

---

## Security Reporting

Vulnerabilities in a running instance are reported through the coordinated-disclosure pipeline.
Everything is derived from live configuration; there is nothing to edit.

| Page | Purpose |
|------|---------|
| `/.well-known/security.txt` | RFC 9116 file. `Contact` lines in preference order: the repository's private vulnerability reporting URL (`web.security.report_url`), the report form `/server/contact?security_id={id}`, then the `mailto:` address. Also `Policy`, `Acknowledgments`, `Expires` and, once a keypair exists, `Encryption` |
| `/server/security` | Human-readable version of the same information with reporting instructions |
| `/server/security/policy` | Disclosure policy (editable in the admin panel, default text with safe-harbor language) |
| `/server/security/thanks` | Hall of fame; each credit follows the researcher's own preference (real name, handle, anonymous or none) |
| `/.well-known/pgp-key.asc` | Project PGP public key; `404` until a keypair is generated |
| `/server/security/report/{tracking_id}?token=...` | Researcher status page behind a one-shot token emailed to the researcher |

**The report form.** `/server/contact?security_id={id}` switches the contact form to security mode
(fields: PGP key, affected component and endpoint, severity, summary, steps to reproduce, impact,
suggested fix, CVE request, disclosure window, credit preference, agreement). The id is
`HMAC-SHA256(installation_secret, floor(unix_seconds / 172800))`, first 16 hex characters. It rotates
every 48 hours and the current and previous values are accepted. An invalid or expired id silently
falls back to the normal contact form; only the first 8 hex characters of its SHA-256 are logged.
`GET /api/v1/server/contact?security_id={id}` returns the security field list, including the
`required_when` and `max_length_when` rules the server enforces, and `POST /api/v1/server/contact`
with the same `security_id` submits a report.

**Storage.** A report body and the researcher's contact are encrypted before they are stored: with
the project PGP key when one exists, otherwise AES-256-GCM with `server.security.encryption_key`.
Plaintext is never written to disk or to a log. The status link works once per day and stops working
30 days after the report is closed; closed reports are deleted `server.security.reports.retention_days`
(default 365) after closing. The maintainer email never carries report content; the researcher email
carries only the tracking id and the status link, encrypted to the researcher's key when one was given.

**Admin panel.** `/server/administration/security/reports` lists and updates reports (status moves
forward only), `/security/policy` edits the policy, `/security/thanks` manages the hall of fame and
`/security/keypair` generates, rotates, publishes, exports, imports and deletes the keypair. Viewing a
report, exporting the private key and the other sensitive actions are written to the audit log; the
sensitive actions ask for the admin password again.
