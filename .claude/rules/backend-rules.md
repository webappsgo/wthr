# Backend, Security & Tor Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never expose stack traces, internal Go error chains, or SQL text in production responses (`_debug` field only, `DEBUG=true` only, stripped in production by the Output Sanitization Pipeline).
- Never leak Tier 1 data publicly: DB credentials/DSN, internal IPs/hostnames, tokens/session secrets, MFA/recovery secrets, other users' PII, filesystem paths, account-existence signals, exact rate-limit thresholds.
- Never use `DROP COLUMN`, `DROP TABLE`, `DELETE` in schema updates, or rename a column directly — add new, migrate in app code, deprecate old (never drop).
- Never write a migrations/version table — schema changes are idempotent `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ADD COLUMN` run on every startup.
- Never build SQL with `fmt.Sprintf`/string concat of user input — parameterized queries only.
- Never store passwords/tokens/recovery keys/TOTP secrets in plaintext — Argon2id for passwords, SHA-256 hash for tokens/recovery keys, AES-256-GCM (`server.security.encryption_key`) for 2FA secrets.
- Never log passwords, full API/session tokens, recovery keys, TOTP secrets, private keys, credit card numbers, or unmasked emails — anywhere (server.log, audit.log, security.log).
- Never run a DB query without a `context.Context` timeout.
- Never regenerate `installation_secret`, `cookie_signing_key`, or `csrf_token_secret` per cluster node — one set, distributed via encrypted cluster-join payload.
- Never rotate an `app_secrets` row without the advisory-lock + quorum flow (anti-split-brain) — a rotation without majority-node confirmation must abort.
- Never cast user-controlled content to `template.HTML`; never inline untrusted SVG/XML/HTML in templates.
- Never shell out with raw user content, filenames, refs, or repo metadata.
- Never use the default Tor ports (9050/9051) or a hardcoded control port — always `127.0.0.1:auto`.
- Never use system Tor — the server binary starts/owns/stops its own dedicated Tor process, isolated data dir.
- Never let Tor failures stop the server from starting — Tor is optional, best-effort, non-blocking (missing binary = INFO log, not error).
- Never truncate or modify audit.log entries after write — append-only, rotation is the only removal path.

## CRITICAL - ALWAYS DO

- Use the canonical API response shape: `{"ok":bool,"data":...}` / `{"ok":false,"error":"CODE","message":"..."}` (see PART 14 for authoritative spec; `details` optional).
- Log every error with context (`error_code`, `request_id`, `http_status`, internal error only in log, never in response); level `Error` for 5xx, `Warn` for 4xx.
- Implement exponential backoff with a max cap (30s) for retryable errors only (network/timeout/503 — never retry 4xx).
- Use hierarchical lowercase colon-separated cache keys (`{type}:{id}`), include a version prefix for cache-busting, and respect the TTL table (sessions 24h, profile 5m, config 1m, rate-limit counters 1m, etc.).
- Use `SET NX EX` (or SQLite `BEGIN IMMEDIATE` + sentinel row) for distributed locks; always release only if you own the lock.
- Use connection pooling on every DB connection (`max_open`/`max_idle`/`max_lifetime`/`max_idle_time`), sized per deployment tier.
- Wrap every query/transaction in `context.WithTimeout` (SELECT 5s, JOIN 15s, write 10s, bulk 60s, reports 2m).
- Apply the 3-tier Public Endpoint Safety Principle to every public surface: Tier 1 never shown (even debug); Tier 2 always shown (version, commit_hash, uptime, aggregate metrics); Tier 3 debug-only (`--debug`/`DEBUG=true`), stripped by tag in production.
- Defend in depth on every threat class (SQLi, XSS, enumeration, timing, CSRF, path traversal, credential stuffing) — validate at input, parameterize/constant-time at data access, escape/CSP at output, TLS/least-privilege at transport. Assume every other layer is broken.
- Run every public response through the Output Sanitization Pipeline: allow-list fields → redact sensitive query params → strip internal IPs/paths → truncate long strings → strip `dev_only` fields when not in debug → constant-time pad auth-sensitive responses.
- Send all mandatory security headers (`X-Content-Type-Options`, `X-Frame-Options`, CSP, Permissions-Policy, Referrer-Policy, HSTS when SSL, `Clear-Site-Data` on session revoke, etc.) — see full list below.
- Enforce CSP default `script-src 'self'` (no inline), `object-src 'none'`, `frame-ancestors 'self'`; extend per-directive via `csp.*_extra` config, never redefine the whole policy.
- Store all cryptographic project secrets (`installation_secret`, `cookie_signing_key`, `csrf_token_secret`) in `server.db` (`app_secrets` table), never returned in any API response, always in backups, rotated per their own schedule.
- Hash tokens (SHA-256) before storage; show full value once on creation only; store 8-char prefix for display.
- Log every security-relevant action to `audit.log` (JSON Lines, one entry/line) with `id`, `time` (UTC ms), `event`, `category`, `severity`, `actor`, `result`; mask emails by default.
- Honor `Sec-GPC: 1` as a binding opt-out signal (skip non-essential cookies/tracking); do NOT honor `DNT` by default.
- Enforce both IP-level blocking (brute force/credential stuffing) AND per-account lockout (soft/hard/permanent) — they cover different attack shapes.
- Use `CREATE TABLE IF NOT EXISTS` for ALL schema; on `ALTER TABLE ADD COLUMN`, ignore "already exists"/"duplicate column" errors across SQLite/PostgreSQL/MySQL.
- Support cluster mode (config sync, session sharing, distributed locks, primary election, heartbeat) as base functionality in every project — single-instance mode auto-detected when no external cache/DB configured.
- Auto-detect the Tor binary (never require a config flag) and, if found, always start the hidden service — v3 onion address via ADD_ONION, `HiddenServiceVersion 3`.
- Run the Tor process as a child of the server process, inheriting the dropped-privilege user — never a separate user/group.
- Keep Tor console output silent during normal bootstrap; show the onion address once on success; show errors always.

## Error Handling & Caching (PART 9)

- Standard error codes map to fixed HTTP statuses (`BAD_REQUEST`/`VALIDATION_FAILED`→400, `UNAUTHORIZED`/`TOKEN_EXPIRED`/`TOKEN_INVALID`/`2FA_*`→401, `FORBIDDEN`/`ACCOUNT_LOCKED`→403, `NOT_FOUND`→404, `CONFLICT`→409, `RATE_LIMITED`→429, `MAINTENANCE`→503, default→500).
- Cache drivers: `memory` (dev default), `valkey` (preferred prod), `redis` (compat). Config lives in PART 12.
- Cache invalidation strategies: time-based (TTL), event-based (delete on write), version-based (key includes version), tag-based (invalidate by tag).
- HTTP `Cache-Control`: static assets `public, max-age=31536000, immutable`; HTML/authenticated `no-store`; public API `public, max-age=60`; private API `private, no-store`.

## Database & Cluster (PART 10)

- Schema updates run on every startup, idempotently; no migration files, no version tracking table.
- Column renames = 3-step: add new column with default → app reads new/writes both → old column stays forever, unused.
- Cluster nodes share DB+cache and get `app_secrets` distributed; agents (PART 33) never do — don't confuse the two.
- Heartbeat every 30s; `degraded` at 90s, `offline` at 5min; secret-version drift >7 days marks a node `stale` and excludes it from cluster ops.
- Primary election: lowest node ID wins; no preemption when old primary returns; split-brain resolved by DB as source of truth (latest write wins).
- Optimistic locking via a `version` column + `WHERE version = $n`; serializable isolation + retry loop for contested reservations.

## Security & Logging (PART 11)

- Standard security headers required on every response: `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Permitted-Cross-Domain-Policies: none`, `Origin-Agent-Cluster: ?1`, COOP/COEP/CORP (default `unsafe-none`/`unsafe-none`/`cross-origin`, tightened per compliance), CSP, Permissions-Policy, `Reporting-Endpoints`/`Report-To`/`NEL`, `X-Request-ID`; add HSTS when SSL is on.
- Permissions-Policy defaults locked (`()`) for camera/mic/geolocation/sensors/USB/etc.; auto-enabled per `IDEA.md` feature declarations; advertising-tracking proposals always disabled.
- Token formats: `adm_`/`usr_`/`org_` + 32 random alphanumeric; agent tokens use compound `adm_agt_`/`usr_agt_`/`org_agt_` prefixes. Store SHA-256 hash + 8-char prefix only.
- Argon2id is the only allowed password hash — no bcrypt/MD5/SHA-* options.
- Data protection matrix: passwords→Argon2id, API tokens→SHA-256, 2FA secrets→AES-256-GCM (server key), recovery keys→SHA-256, backups→AES-256-GCM (user password if set), transit→TLS 1.2+. No app-level DB-at-rest encryption — that's the OS's job (LUKS/FileVault/BitLocker).
- Breach detection always on (cannot disable): brute force (10 fails/5min→block+alert), credential stuffing (50 fails/10min across accounts), mass export, privilege escalation, session anomaly, DB/config anomaly — each with auto-action + audit event.
- IP blocks: temporary (1h, auto-release) → extended (24h) → permanent (manual only), with escalating durations for repeat offenders (1h→4h→24h→7d+alert).
- Account lockout (separate from IP block): 5 fails/15min→soft lock 15min; 10/1h→hard lock 1h; 15/24h→permanent lock until reset/admin unlock.
- Compliance standards (GDPR/CCPA/HIPAA/SOC2/PCI-DSS/ISO27001/etc.) are all OFF by default, enabled individually; when multiple are enabled, the strictest rule wins per requirement (longest retention, strongest encryption, shortest breach-notification window, shortest session timeout).
- `audit.log` is JSON-only, one entry per line, append-only, `keep: none` by default (delete on rotation) to minimize retention liability.
- Archive extraction (if implemented): reject path traversal, symlinks/special files, enforce size/count limits and compression-bomb protection.
- Private file delivery (if implemented): always re-check authz per request (never rely on obscure URLs), force `Content-Disposition: attachment` for active MIME types (html/xhtml/svg/xml).

## Tor Hidden Service (PART 32)

- Uses `github.com/cretz/bine` (pure Go, `CGO_ENABLED=0` compatible) — never an embedded/CGO Tor.
- Hidden service is always-on if the Tor binary is found — no enable/disable config flag exists.
- Two independent capabilities: (1) hidden service hosting `.onion:80 → localhost:{server_port}`, always on if binary found; (2) outbound routing via Tor SOCKS, off by default (`server.tor.use_network: false`), optionally user-overridable (`allow_user_preference`).
- Directories are fixed, never configurable: config `{config_dir}/tor/` (0700), data `{data_dir}/tor/` (0700), keys `{data_dir}/tor/site/` (0700), log `{log_dir}/tor.log`.
- Control connection is always `127.0.0.1:auto` (TCP), `SafeLogging` enabled to scrub sensitive info from Tor's own logs.
- On graceful server shutdown, terminate the dedicated Tor child process; on crash, WARN and attempt restart — never treat Tor failure as fatal to the server.
- The `.onion` address, if `expose: true`, is Tier-2 public-safe info and can appear in `/api/autodiscover`.

For complete details, see AI.md PART 9, 10, 11, 32
