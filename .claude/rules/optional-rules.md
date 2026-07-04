# Optional Rules (PART 34, 35, 36)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Status for This Project

| PART | Feature | Status |
|------|---------|--------|
| 34 | Multi-User | **IMPLEMENTED** — NON-NEGOTIABLE |
| 35 | Organizations | Not implemented — must not appear in code |
| 36 | Custom Domains | Not implemented — must not appear in code |

## CRITICAL - NEVER DO (PART 34 — Multi-User)

- Use `bcrypt` → Argon2id for passwords
- Store raw session tokens → SHA-256 hash in `token_hash` column
- Use `public`/`private` as registration modes → `open`/`invite`/`admin_only`/`disabled`
- Mix admin accounts with regular user accounts → separate DB tables
- Expose session token in API response body → return raw token in cookie only (set once at login)
- Allow concurrent sessions beyond `auth.max_sessions_per_user` config value
- Skip email verification when `users.registration.require_email_verification` is true

## CRITICAL - ALWAYS DO (PART 34)

- Registration modes: `open` | `invite` | `admin_only` | `disabled` (default: `invite`)
- `users.db` for all user data (separate from `server.db`)
- `user_sessions` table: `token_hash TEXT NOT NULL` (SHA-256 of raw bearer token)
- Session cookie: `weather_session`, HttpOnly, Secure, SameSite=Lax
- User settings at `/users/settings/`
- User locations at `/users/locations/`
- User API tokens at `/users/api-tokens/`
- Invite flow: admin creates invite → unique token → email → user registers
- List sessions endpoint shows IsCurrent flag (compare SHA-256 of current cookie)
- Revoke session by row ID (not by raw token)

## CRITICAL - NEVER DO (PART 35/36 — Not Implemented)

- Reference `organizations` table
- Reference `custom_domains` table
- Add `/orgs/*` routes
- Add org membership or ownership concepts
- Add custom domain verification or SSL for custom domains

## User Route Scope

```
GET  /users/settings
POST /users/settings
GET  /users/locations
POST /users/locations
GET  /users/locations/new
GET  /users/locations/:id/edit
PUT  /users/locations/:id
DELETE /users/locations/:id
GET  /users/api-tokens
POST /users/api-tokens
DELETE /users/api-tokens/:id
GET  /users/sessions
DELETE /users/sessions/:id
```

## Reference

For complete details, see AI.md PART 34, 35, 36
