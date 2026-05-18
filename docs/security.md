# Security

## Authentication

### Admin Authentication

Server Admins authenticate using **WebAuthn (passkeys)** — no passwords. A passkey is a hardware or
platform credential (YubiKey, Touch ID, Windows Hello, or any FIDO2 device). Passkeys are stored in
the `server.db` database; the authenticator private key never leaves the device.

| Flow | Description |
|------|-------------|
| **Register** | Navigate to `/admin/` → complete passkey ceremony → passkey bound to your device |
| **Login** | Navigate to `/admin/` → browser prompts for passkey → session created (30-day cookie) |
| **Revoke** | Admin panel → Administrators → Manage Passkeys → remove a passkey |

### User Authentication

Regular user accounts authenticate with **email + password (Argon2id)** and a persistent session
cookie. Users may also configure passkeys for passwordless login.

| Parameter | Value |
|-----------|-------|
| Hash algorithm | Argon2id |
| Time cost | 1 iteration |
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

All endpoints are rate-limited. Anonymous traffic is limited more strictly than authenticated
traffic.

| Tier | Limit |
|------|-------|
| Anonymous | 20 requests / minute |
| Authenticated user | 100 requests / minute |
| Server Admin | Unlimited |
| Auth endpoints (login, register, reset) | 10 requests / 15 minutes with exponential backoff |

Exceeding the limit returns **HTTP 429 Too Many Requests** with a `Retry-After` header.

---

## Transport Security

When a `server.domain` is configured (non-localhost), wthr automatically obtains and renews a
TLS certificate via Let's Encrypt (ACME HTTP-01 challenge). HTTP traffic is redirected to HTTPS.

Certificates are stored in `{data_dir}/certs/`. Manual certificate paths can be set in
`server.yml` under `ssl.cert_file` and `ssl.key_file`.

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
| `/health` | Liveness probe — returns 200 OK when the process is alive |
| `/health/ready` | Readiness probe — returns 200 when all dependencies are ready |
| `/health/full` | Full status JSON — safe for external monitoring |
| `/metrics` | Prometheus-compatible metrics (configurable auth) |

---

## Security Reporting

To report a security vulnerability, email **casjay@yahoo.com** with subject line
`[wthr] Security Report`. Include a description of the issue, reproduction steps, and your
contact information. We aim to respond within 48 hours.

Do not file public GitHub issues for security vulnerabilities until a fix has been coordinated.
