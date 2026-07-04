# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Put API endpoints outside `/api/v1/` prefix
- Use verbs in route paths → nouns only
- Return different error shapes for different endpoints
- Skip `X-Request-ID` propagation
- Use self-signed certs in production → Let's Encrypt or user-provided
- Mount admin routes outside `/server/{admin_path}/` scope
- Mount auth routes outside `/server/auth/` scope
- Mount user routes outside `/users/` scope

## CRITICAL - ALWAYS DO

- Versioned API: `/api/v1/`
- Standard JSON response: `{"ok":true,"data":{...}}` (success) / `{"ok":false,"error":"..."}` (error)
- RFC 7807 error body for 4xx/5xx
- `X-Request-ID` header on every response
- Health endpoint: `GET /health` → `{"status":"ok","version":"..."}`
- Version endpoint: `GET /version` → full build info JSON
- Metrics endpoint: `GET /metrics` (Prometheus format)

## Route Scopes (PART 14)

| Scope | Prefix | Purpose |
|-------|--------|---------|
| Auth | `/server/auth/` | Login, logout, register, OAuth, passkey |
| Users | `/users/` | User profiles, settings, locations, API tokens |
| Admin web | `/server/{admin_path}/` | Admin panel pages |
| Admin API | `/api/v1/server/{admin_path}/` | Admin REST endpoints |
| Public API | `/api/v1/` | Weather, health, version |

## Auth Routes

```
GET  /server/auth/login
POST /server/auth/login
GET  /server/auth/logout
GET  /server/auth/register
POST /server/auth/register
GET  /server/auth/2fa
POST /server/auth/2fa
GET  /server/auth/oidc/...
POST /server/auth/ldap/...
GET  /server/auth/passkey/...
```

## SSL/TLS (PART 15)

- Let's Encrypt: auto-renew, ACME HTTP-01 or DNS-01
- Local certs: user-provided, paths in `server.yml`
- TLS 1.2+ minimum (TLS 1.3 preferred)
- Redirect HTTP → HTTPS in production mode

## Reference

For complete details, see AI.md PART 13, 14, 15
