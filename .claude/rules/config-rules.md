# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use `strconv.ParseBool()` — only accepts true/false/1/0/t/f. Use `config.ParseBool()` / `config.IsTruthy()` everywhere
- ❌ Place YAML comments inline (`port: 80  # server port`) — comments go ABOVE the setting
- ❌ Name the config file `server.yaml` — always `server.yml` (auto-migrate `.yaml` → `.yml` on startup)
- ❌ Store passwords, API tokens, or secrets in `server.yml` — DB-only (admins table)
- ❌ Skip path normalization/validation on configuration paths, HTTP request paths, file paths, API params
- ❌ Allow `..`, `%2e%2e`, encoded traversal, or any path with `..` in URL paths or file paths
- ❌ Apply auth/routing middleware before path security middleware — path security MUST be FIRST
- ❌ Hardcode `cfg.FQDN`/`https://` for outbound URLs — use `BuildURL(r, "/path")` so reverse-proxy headers are honored
- ❌ Use bare `/path` for emails, OAuth callbacks, webhooks, HATEOAS — they need full URLs
- ❌ Use external schedulers (cron, systemd timers, Task Scheduler) — only the built-in scheduler (PART 19)
- ❌ Embed GeoIP / blocklist / CVE / Trivy databases in the binary — download on first run to `{config_dir}/security/`
- ❌ Re-pick the listen port on every startup — pick once on first run, persist to `server.yml`, reuse forever
- ❌ Hardcode port `80` for native binary mode — that's containers-only; native uses random `64xxx`
- ❌ Display `:80` / `:443` in URLs — strip them
- ❌ Put admin credentials in config files — admins table in DB

## CRITICAL - ALWAYS DO
- ✅ `config.ParseBool(s, default)` accepts ALL truthy/falsy variants (yes/no, true/false, 1/0, on/off, enable/disable, oui/non, da/niet, sí/no, yep/nope, y/n, t/f, ok/deny, allow/block, ...)
- ✅ Comments ABOVE the setting in YAML, never inline
- ✅ Apply path security middleware FIRST in the chain (before SecurityHeaders, AllowList, BlockList, RateLimit, GeoIP, Auth, Logging)
- ✅ Detect `{proto}` from `X-Forwarded-Proto` → `X-Forwarded-Ssl` → TLS detection → `http`
- ✅ Detect `{fqdn}` from reverse-proxy headers → `DOMAIN` env → `os.Hostname()` → `$HOSTNAME` → global IP → `localhost`
- ✅ Detect `{port}` from `X-Forwarded-Port` → Host header → server port → proto default
- ✅ Detect `{base_url}` from `X-Forwarded-Prefix` → `X-Forwarded-Path` → `X-Script-Name` → `server.baseurl` → `/`
- ✅ Use `BuildURL(r, "/path")` for ALL outbound URLs (emails, callbacks, webhooks, HATEOAS, redirects)
- ✅ First-run port: pick random unused in 64000-64999, persist to `server.yml`
- ✅ Single instance: `server.yml` is source of truth, SQLite for credentials/sessions only
- ✅ Cluster mode: DB is source of truth, `server.yml` is local cache + backup; on DB unavailable, app enters READ-ONLY mode using cached config
- ✅ Maintenance mode: enter on critical errors (DB connection / file write); self-heal continuously; admin panel stays accessible with fix instructions; writes return `503` + `Retry-After: 30` + `X-Maintenance-Mode: true` + `X-Maintenance-Reason: ...`
- ✅ Sync DB → `server.yml` on every config change, on startup, and every 5 min
- ✅ Honor `NO_COLOR`, `TERM=dumb`, `DOMAIN`, `MODE` runtime env vars; init-only env vars (`CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `PORT`, `LISTEN`, `APPLICATION_NAME`, ...) used ONCE on first run

## SERVER.YML SHAPE (REQUIRED keys)
```yaml
server:
  port: 64xxx          # Random first-run, persisted
  fqdn: {auto}
  address: "[::]"      # IPv4 + IPv6
  mode: production     # or development
  admin_path: admin    # configurable
  api_version: v1

  branding:
    title: "weather"
    tagline: ""
    description: ""

  ssl:
    enabled: false
    min_version: "TLS1.2"
    letsencrypt:
      enabled: false

  scheduler:
    enabled: true
    tasks:
      geoip_update:        { enabled: true, schedule: "0 3 * * 0" }
      blocklist_update:    { enabled: true, schedule: "0 4 * * *" }
      cve_update:          { enabled: true, schedule: "0 5 * * *" }
      log_rotation:        { enabled: true, schedule: "0 0 * * *" }
      session_cleanup:     { enabled: true, schedule: "@hourly" }
      backup:              { enabled: true, schedule: "0 2 * * *", retention: 4 }
      ssl_renewal:         { enabled: true, schedule: "0 3 * * *", renew_before: 7d }
      health_check:        { enabled: true, schedule: "*/5 * * * *" }
      tor_health:          { enabled: true, schedule: "*/10 * * * *" }

  rate_limit: { enabled: true, requests: 0, window: 60 }
  database:   { driver: file }
```

## PATH SECURITY (PART 5)
- `SafePath(input)` rejects: empty, `>2048` chars, contains `..`, segment `>64` chars, non-`[a-z0-9_-]` chars, segment of `.` or `..`
- `PathSecurityMiddleware` applied FIRST: blocks `..`, `%2e%2e`, normalizes `//` → `/`, preserves trailing slash, ensures leading `/`
- File paths from user input: use `SafeFilePath(baseDir, userPath)` and verify resolved absolute path stays under `baseDir`

## PRIVILEGE & PORT BINDING (PART 5)
- Service mode: start as root → bind privileged ports → drop to `weather` user (Unix); Windows uses VSA `NT SERVICE\weather`
- User mode: ports >1024 only, user paths only
- Setup / restore / mode change require AUTHORIZATION (not just file access): admin auth OR root OR (for setup) valid setup token OR empty database

## APPLICATION MODES (PART 6 summary)
- `production` (default): full security headers, no debug endpoints, no profiling, JSON logs
- `development`: relaxed CORS, debug endpoints (`/debug/pprof/*`), pretty logs, hot-reload helpful messages

## SERVER CONFIGURATION (PART 12 summary)
- Allowlist (CIDR) bypasses blocklist + ratelimit + geoip but NOT auth
- Blocklist: IP CIDR + domain regex + autoupdate from configurable feeds
- GeoIP: deny_countries OR allow_countries (mutually exclusive)
- Permissions-Policy: explicit allow-list per directive
- Contact: `server.contact.general.*` (recipient + webhooks; falls back to `server.contact.admin.*`)

---
For complete details, see AI.md PART 5, 6, 12
