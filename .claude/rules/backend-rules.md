# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use bcrypt for new passwords (only to verify legacy, then rehash to Argon2id)
- ❌ Store plaintext passwords ANYWHERE
- ❌ Store passwords (even hashed) in `server.yml` — DB only
- ❌ Log passwords, API tokens, session tokens, secrets, GPG private keys, recovery codes
- ❌ Allow passwords with leading/trailing whitespace (REJECT, don't trim)
- ❌ Concatenate strings into SQL — only parameterized queries
- ❌ Skip CSRF tokens on cookie-authenticated state-changing forms (Bearer/API-token requests are exempt)
- ❌ Reveal whether a username/email exists ("If account exists, email sent")
- ❌ Reveal stack traces, internal paths, dependency versions, internal hostnames in user-facing errors
- ❌ Embed cryptographic keys, GPG keypair, or `app_secrets` in the binary — generate on first run, store in DB
- ❌ Skip backup of `app_secrets` table and PGP private key — they're required for restore
- ❌ Skip cluster anti-split-brain rotation when nodes drop out
- ❌ Leave a removed cluster node's records (sessions, state, scheduler ownership) — clean up
- ❌ Run Tor as a separate service — the server binary controls Tor startup (PART 32)
- ❌ Expose Tor private keys, onion address private material, or hidden-service descriptors in any output
- ❌ Skip security report generation on critical events
- ❌ Encrypt-at-rest with hardcoded keys — use `server.security.encryption_key` (32 bytes, generated first-run)

## CRITICAL - ALWAYS DO
- ✅ Argon2id (RFC 9106) for passwords; SHA-256 for API/session tokens
- ✅ PHC-format password storage: `$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`
- ✅ Generate `installation_secret` (32B), `cookie_signing_key` (32B HMAC-SHA256), `csrf_token_secret` (32B), `server.security.encryption_key` (32B AES) on first start, BEFORE the setup token
- ✅ Generate GPG keypair on first start (server-controlled), store private key encrypted with `installation_secret`-derived KDF
- ✅ Audit log every admin action and config change to `srv_audit_log` (who/what/when/from-where)
- ✅ Public Endpoint Safety Principle: never reveal internal infrastructure / pre-auth identity / config values
- ✅ Cluster join: new node fetches DB connection from operator, runs join handshake, gets node ID, syncs config from DB → `server.yml` cache
- ✅ Cluster split-brain prevention: rotate primary node lease every 30s; on drop, secondary takes over after 2 missed leases
- ✅ Removed-node cleanup: scheduler task purges sessions / cluster_state / scheduler_history rows owned by removed nodes
- ✅ Public-facing logs: structured JSON, no secrets, redact tokens to last 4 chars
- ✅ Tor: spawn `tor` as subprocess via `github.com/cretz/bine`; the server binary IS the controller; gracefully shut down on SIGTERM
- ✅ Tor onion address: persist per cluster node in `cluster_state` table
- ✅ Security report generated on auth-failure spikes, blocked countries hits, blocklist hits, repeated 4xx/5xx clusters

## CRYPTOGRAPHIC KEYS (PART 11)
| Name | Length | Storage | Used by |
|------|--------|---------|---------|
| `installation_secret` | 32B | `app_secrets` table | HMAC root, KDF for GPG private key, `{security_id}` |
| `cookie_signing_key` | 32B | `app_secrets` | Session cookie integrity |
| `csrf_token_secret` | 32B | `app_secrets` | CSRF token HMAC |
| `server.security.encryption_key` | 32B AES | `server.yml` | All at-rest encryption (2FA secrets, security reports) |
| GPG keypair | RSA 4096 | Private encrypted in DB; public served at `/server/pubkey.asc` | Encrypted security reports, signed releases (optional) |

## DATABASE & CLUSTER (PART 10)
- SQLite default: `{db_dir}/server.db` (server-side tables: `srv_*`) and `{db_dir}/users.db` (user-side: `usr_*`)
- Remote DB (Postgres / MySQL / MSSQL / MongoDB / libSQL): single-DB with prefixed tables (`srv_*` + `usr_*`)
- Auto-migrate via `PRAGMA table_info` + idempotent ALTERs (no migration framework)
- Cluster: shared DB + Valkey/Redis cache → all nodes see same config in real time
- Single instance: SQLite local; no Valkey/Redis required

## TOR HIDDEN SERVICE (PART 32) — REQUIRED, auto-enabled if `tor` binary is found
- Server binary spawns Tor (`github.com/cretz/bine`)
- HiddenService dir: `{data_dir}/tor/hs/`
- Onion address persisted per node in `cluster_state`
- Backup includes Tor keys (per-node) — restore on the same node ID rebuilds the same .onion

---
For complete details, see AI.md PART 9, 10, 11, 32
