# Security

## Authentication

### Admin Authentication

Server Admins sign in at the shared `/server/auth/login` page with their username (or email) and
password (Argon2id). An admin who has registered a passkey (WebAuthn: YubiKey, Touch ID, Windows
Hello or any FIDO2 device) must also complete the passkey prompt before the session is created.
Passkeys are stored in the `server.db` database; the authenticator private key never leaves the
device. An unauthenticated request to `/server/administration` redirects to the login page.

| Flow | Description |
|------|-------------|
| **Sign in** | `/server/auth/login` → password → passkey prompt when one is registered → session (30-day cookie) |
| **Register a passkey** | Admin panel → your profile → Security → add a passkey (the password is asked again) |
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

Server Admins and Regular Users are stored in **separate database files** (`server.db` and
`users.db`). A Regular User account can never be elevated to Server Admin status through the normal
user-facing flows.

---

## Rate Limiting

All endpoints are rate-limited with a sliding window: per IP for anonymous requests, and per token
for requests carrying a valid Bearer token, so clients behind a shared IP never throttle one another.
Failed authentication is always counted per IP. The defaults below live under
`server.rate_limit` in `server.yml` and are editable from the admin panel.

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

---

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

All state-changing HTML form submissions require a CSRF token (a 32-byte `crypto/rand` value,
set as a cookie and verified server-side). API endpoints using `Content-Type: application/json`
are not subject to CSRF checks (the content type itself is a CSRF mitigation for browser
cross-origin requests).

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
