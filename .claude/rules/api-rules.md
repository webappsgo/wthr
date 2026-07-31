# API, Health & TLS Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER add sub-routes to healthz (`/server/healthz/db` etc.) — just `/server/healthz`
- NEVER expose sensitive data in health responses — no DB connection strings, API keys, passwords, internal IPs, file paths, env vars, config contents, usernames, or debug/stack traces
- NEVER hardcode `v1` in code — always use `APIBasePath()` / `{api_version}`
- NEVER use singular resource names (`/user`), uppercase routes, underscores instead of hyphens, trailing slashes, or verbs in route paths (`/getUsers`)
- NEVER keep legacy (removed/changed) endpoints — delete them completely, no redirects, no deprecation shims, no parallel old/new route trees
- NEVER layer new routes on top of old code when route rules change — migrate the implementation
- NEVER redirect an unversioned `/api/<thing>` alias to its versioned target — mount the SAME handler at both paths (redirects break POST semantics, double caching, non-redirect-following clients)
- NEVER manually edit generated OpenAPI JSON or GraphQL schema — both are build-time generated from code, JSON only (no YAML) for OpenAPI
- NEVER put swagger/graphql files anywhere but `src/swagger/` and `src/graphql/` (never project root)
- NEVER replicate an external service's entire API surface for "compatibility" unless the user explicitly asked for route/API/client compatibility — default is feature compatibility only
- NEVER skip full RFC compliance when the app itself implements an RFC-defined protocol (DNS, DHCP, SMTP, HTTP, FTP, NTP, LDAP, WebDAV) — this is not optional "compatibility"
- NEVER use legacy error shapes: no `code`/`error` field swap, no `status` duplicated in body, no bare `{"error":"..."}`, no ad-hoc top-level fields (use `details` or HTTP headers like `Retry-After`)
- NEVER put icons/ASCII art/colors in log output — logs are ALWAYS raw plain text (banners/console may use them)
- NEVER set `DOMAIN` to an overlay address (`.onion`, `.i2p`, `.exit`) — those are app-generated/managed
- NEVER let the app renew certs under `/etc/letsencrypt/**` — that's system/certbot-managed; app only auto-renews its own `{config_dir}/ssl/letsencrypt/{fqdn}/` certs
- NEVER enable `/healthz` root alias by default — it requires explicit `server.healthz.root.enabled: true` and must mount the same handler as `/server/healthz` (no redirect, no forked logic)

## CRITICAL - ALWAYS DO

- ALWAYS version every API route: `/api/{api_version}/...`
- ALWAYS use plural, lowercase, hyphenated resource names
- ALWAYS keep frontend and API in the same route tree (`/users` ↔ `/api/{api_version}/users`) except for documented API-only or frontend-only exceptions
- ALWAYS make health/API responses honor content negotiation: `.txt` extension > `Accept: application/json` > `Accept: text/plain` > non-interactive client detection > default (JSON for API, HTML for frontend)
- ALWAYS keep Swagger + GraphQL in sync with each other and with the live API, auto-generated at build time, embedded in the binary
- ALWAYS provide all 3 API types (REST, Swagger, GraphQL) for every project
- ALWAYS use the canonical error shape `{ "ok": false, "error": "CODE", "message": "...", "details": {...} }` and canonical success shapes (item: bare object; action: `{ "ok": true, "data": {...} }`)
- ALWAYS default pagination to 250 items, response shape `{ "data": [], "pagination": { "page", "limit", "total", "pages" } }`
- ALWAYS support Let's Encrypt with all 3 challenge types (HTTP-01, TLS-ALPN-01, DNS-01) and all lego DNS providers via dynamic admin WebUI form
- ALWAYS resolve `{fqdn}` in priority order: reverse-proxy headers → `DOMAIN` env → `os.Hostname()` → `$HOSTNAME` → public IPv6 → public IPv4 → `localhost`
- ALWAYS strip `:80` and `:443` from displayed URLs; other ports keep `{proto}://host:{port}`
- ALWAYS check certs in order: `/etc/letsencrypt/live/domain/` → `/etc/letsencrypt/live/{fqdn}/` → `{config_dir}/ssl/letsencrypt/{fqdn}/` → `{config_dir}/ssl/local/{fqdn}/` → request new via Let's Encrypt
- ALWAYS auto-renew app-managed LE certs 7 days before expiry (daily check, 03:00)

## Key Rules Summary

### Health & Versioning (PART 13)

- Routes: `/server/healthz` (frontend, content-negotiated), optional `/healthz` root alias (config-gated), `/api/{api_version}/server/healthz` (JSON default), `/api/healthz` (unversioned direct alias, JSON)
- `HealthResponse` canonical field order: project → status → version/build → runtime → cluster → features → checks → stats → app-specific extensions
- `features.*` shows only NON-NEGOTIABLE (or project-enabled optional) features with real enabled/disabled status — never `/metrics` data
- `checks.*` is vague ok/error only, never connection details
- SemVer: start at `1.0.0`, MAJOR.MINOR.PATCH, no `v` prefix in the version string (git tags do get `v`), no leading zeros
- Version source priority: `release.txt` → git tag → `dev` fallback

### API Structure (PART 14)

- Route scopes: `/server/*` (public), `/server/auth/*`, `/users/*` (no ID, current user from session), `/orgs/*` (`{slug}` required), `/server/{admin_path}/*` (admin), `/server/{admin_path}/config/*` (admin config), `/*` (project-specific)
- Path params for resource identity, query params for pagination/sort/filter — never duplicate
- Content negotiation priority for API routes: `.txt` > `Accept: application/json` > `Accept: text/plain` > non-interactive client > default JSON
- Content negotiation priority for frontend routes: `Accept: text/html` > `Accept: text/plain` > browser User-Agent > CLI/curl > default HTML
- Client types: Our CLI (`{project_name}-cli/` UA) gets JSON and renders itself; text browsers (lynx/w3m/links/elinks) get no-JS HTML; HTTP tools (curl/wget/httpie/empty UA) get `HTML2TextConverter()` formatted text
- External API "compatibility" defaults to feature/behavior parity using our own routes — only add the external route surface when the user explicitly requests route/API/client compatibility
- Full RFC/protocol compliance (DNS, SMTP, DHCP, Matrix, ActivityPub, WebDAV, etc.) is mandatory when the app itself implements that protocol — not an optional add-on
- Root-level endpoints: `/`, `/server/healthz`, `/server/docs/swagger`, `/server/docs/graphql`, `/metrics`, `/api/autodiscover`, `/api/swagger`, `/api/graphql`, `/api/healthz` (all unversioned aliases mount the same handler as their versioned target, never redirect)
- Formatting: 2-space indent for JSON/HTML/YAML/CSS/JS, tabs for Go/Makefiles, single trailing newline everywhere, no trailing whitespace

### SSL/TLS & Let's Encrypt (PART 15)

- Port rules: single port = HTTP by default (443 exception → HTTPS-only); dual ports = first HTTP, second HTTPS; `ssl.enabled` can override HTTP→HTTPS on any port
- Overlay networks (Tor/I2P) inherit HTTPS-only mode from clearnet; otherwise stay HTTP (network-layer encryption already provided); overlay HTTPS uses self-signed certs (LE doesn't cover `.onion`/`.i2p`)
- Cert directory structure mirrors certbot: `{config_dir}/ssl/letsencrypt/{fqdn}/{fullchain,privkey}.pem` and `{config_dir}/ssl/local/{fqdn}/{cert,key}.pem`
- Cert ownership: `/etc/letsencrypt/**` = system/certbot (app never renews); `ssl/letsencrypt/{fqdn}/` = app-managed (auto-renew 7 days before expiry); `ssl/local/{fqdn}/` = user-managed (no auto-renewal)
- DNS-01 provider credentials are AES-256-GCM encrypted at rest, validated against the provider's API before storage
- Dev TLDs (`.local`, `.test`, `.example`, `.invalid`, `{project_name}` variants, etc.) fall back to global IP for display URLs since they can't get real LE certs
- Startup banner is responsive to terminal width (full ASCII ≥80 cols, compact 60-79, minimal 40-59, micro <40, plain text for `NO_COLOR`/`TERM=dumb`) — but log output is ALWAYS plain text regardless of banner style

For complete details, see AI.md PART 13, 14, 15
