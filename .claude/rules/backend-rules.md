# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Store raw passwords → Argon2id always
- Store raw session tokens → SHA-256 hash before storing
- Use `SELECT *` queries
- Use destructive schema ops (DROP TABLE, DROP COLUMN)
- Use raw string queries with user input → parameterized queries only
- Log sensitive values (passwords, tokens, API keys)
- Use external cron → internal scheduler (PART 19)
- Use `sync.Map` without benchmarking → prefer `sync.RWMutex` + map
- Create migration files → use `CREATE TABLE IF NOT EXISTS` + idempotent `ALTER TABLE`

## CRITICAL - ALWAYS DO

- Dual-database: `server.db` (app data) + `users.db` (user accounts) — always SQLite by default
- Schema versioning: `schema_version` table in each DB; run migrations on startup
- Argon2id for passwords (`golang.org/x/crypto/argon2`)
- SHA-256 for session tokens (`crypto/sha256`)
- HttpOnly + Secure + SameSite=Lax cookies
- Parameterized queries — no string formatting with user data
- Connection pool limits: MaxOpenConns, MaxIdleConns, ConnMaxLifetime
- `defer tx.Rollback()` pattern for all transactions
- Error wrapping with `fmt.Errorf("...: %w", err)`
- Structured logging (logfmt or JSON); never ANSI in log files
- GeoIP: ip-location-db `-mmdb` npm packages; use as risk signal only, never sole gate
- Tor: binary installs + controls Tor (see PART 32); auto-enabled if Tor found

## Database Layout

```
SQLite databases:
  server.db — app configuration, sessions, admin accounts, weather cache, scheduler jobs
  users.db  — user accounts, user sessions, user preferences, API tokens
```

## Security Essentials (PART 11)

- Identical error messages for auth failures (enumeration prevention)
- Constant-time comparison for tokens (`subtle.ConstantTimeCompare`)
- Rate limiting on all auth endpoints
- CSRF protection on all mutating web endpoints
- `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options` headers
- HTTPS redirect in production mode
- Audit log every admin action

## Reference

For complete details, see AI.md PART 9, 10, 11, 32
