# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Mix public health (`/healthz`, `/api/v1/healthz`) with Prometheus `/metrics` — they have separate purposes
- ❌ Expose `/metrics` publicly (firewall it OR token auth)
- ❌ Include detailed metrics, request counts by path, DB connection counts, CPU/memory in `/healthz`
- ❌ Use healthz for alerting — alert on Prometheus
- ❌ Return non-canonical envelope: success is `{"ok": true, "data": ...}`; error is `{"ok": false, "error": "CODE", "message": "...", "details": {...}}`
- ❌ Duplicate HTTP status code inside the JSON body
- ❌ Return JSON for browsers — content-negotiate on Accept header (HTML for `text/html`, plain text for `text/plain` curl/wget, JSON for `application/json`)
- ❌ Skip Swagger / GraphQL annotations when adding routes — both surfaces MUST stay in sync with code
- ❌ Build hardcoded HTTPS URLs from `cfg.FQDN` — always `BuildURL(r, "/path")`
- ❌ Manually manage Let's Encrypt certs when the server can do it — use the built-in `letsencrypt` config
- ❌ Skip TLS 1.2 minimum (`server.ssl.min_version: "TLS1.2"`)
- ❌ Default-allow CORS — explicit allow-list only

## CRITICAL - ALWAYS DO
- ✅ Two health surfaces: `/healthz` (frontend, content-negotiated) and `/api/v1/healthz` (always JSON)
- ✅ Public-safe healthz payload: status, version, uptime, mode, cluster summary, Tor address — no internal IPs/paths/connection details
- ✅ `/metrics` (Prometheus exposition) on internal-only — full telemetry
- ✅ Canonical response envelope (success): `{"ok": true, "data": ...}` (use `data` for object, `items` + pagination for list)
- ✅ Canonical response envelope (error): `{"ok": false, "error": "ERROR_CODE", "message": "Human readable", "details": {optional}}` with HTTP status carrying the actual code
- ✅ HTTP status code lives in the response status line, NOT duplicated in body
- ✅ Pagination via `Link:` header (RFC 8288) + `X-Total-Count` header
- ✅ Rate limit response: `429` + `Retry-After: <seconds>` header + JSON envelope
- ✅ Maintenance mode: `503` + `Retry-After: 30` + `X-Maintenance-Mode: true` + `X-Maintenance-Reason: <reason>`
- ✅ Problem Details (RFC 7807) when content-negotiated to `application/problem+json`
- ✅ Health response (RFC draft `application/health+json`) when negotiated
- ✅ Default Auth by Scope (PART 14 canonical rule):
  - `/api/v1/healthz`, `/api/v1/openapi.json`, `/api/v1/graphql` (introspection only) — public
  - `/api/v1/{admin_path}/...` — requires admin session OR `adm_*` token
  - `/api/v1/users/...` — requires user session OR `usr_*` token
  - `/api/v1/auth/...` — public (login/register/recover) but rate-limited
  - `/api/v1/public/...` — explicit public scope (e.g., public user profiles)
- ✅ Both Swagger/OpenAPI 3.1 (`/api/v1/openapi.json`) and GraphQL (`/api/v1/graphql`) expose IDENTICAL functionality
- ✅ Swagger annotations on EVERY route (`@Router`, `@Param`, `@Success`, `@Failure`, `@Tags`)
- ✅ GraphQL: schema-first via gqlgen; resolvers reuse the same runtime as REST handlers (no parallel implementation)
- ✅ Non-interactive text output: when `Accept: text/plain` AND tool detected (curl/wget/httpie via UA), return pre-formatted text via HTML2TextConverter
- ✅ SSL/TLS: prefer `letsencrypt` (HTTP-01 / TLS-ALPN-01 / DNS-01); auto-renew via scheduler
- ✅ SSL cert auto-detection order: `/etc/letsencrypt/live/{domain}/` → `{config_dir}/ssl/letsencrypt/{fqdn}/` → `{config_dir}/ssl/local/{fqdn}/`

## ROUTE PATTERN (PART 1 + PART 14)
Every web route has a JSON twin:
| Web (HTML) | API (JSON) |
|-----------|-----------|
| `/` | `/api/v1/` |
| `/healthz` | `/api/v1/healthz` |
| `/{admin_path}/dashboard` | `/api/v1/{admin_path}/dashboard` |
| `/{admin_path}/users` | `/api/v1/{admin_path}/users` |
| `/server/docs/swagger` | `/api/v1/openapi.json` |
| `/server/docs/graphql` | `/api/v1/graphql` |

## SSL/TLS (PART 15)
- Let's Encrypt: `server.ssl.letsencrypt.{enabled,email,challenge:http-01|tls-alpn-01|dns-01,staging}`
- Port 80 → enables HTTP-01; port 443 → enables TLS-ALPN-01 + auto-enables SSL
- Renew 7 days before expiry via scheduler `ssl_renewal` task
- Strip `:80` / `:443` in displayed URLs

## PUBLIC REPORTS SCOPE (PART 14)
- `/server/security-report` (HTML) and `/api/v1/server/security-report` (JSON) — public; PGP-encrypted summary using server's public key
- `/server/pubkey.asc` — public; ASCII-armored PGP public key for the server

---
For complete details, see AI.md PART 13, 14, 15
