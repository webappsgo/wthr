# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Hardcode dev/production values → detect at runtime
- Store secrets in config file → environment variables or secret store
- Use `public`/`private` as registration modes → use `open`/`invite`/`admin_only`/`disabled`
- Skip config file migration when schema changes
- Use a port other than 80 as internal Docker port

## CRITICAL - ALWAYS DO

- Config file: `server.yml` at PART 4 path for the current OS/privilege level
- Parse config with `yaml.v3` (never JSON for config)
- Validate all config values on load; fail fast with clear error
- Default: invite-only registration (`invite`)
- Admin path: configurable, default `admin`
- API key: auto-generated on first run, stored in DB
- Support env var overrides for all config values

## Registration Modes (PART 34)

| Mode | Meaning |
|------|---------|
| `open` | Anyone can register |
| `invite` | Invite token required (DEFAULT) |
| `admin_only` | Admin creates accounts only |
| `disabled` | No new registrations |

Legacy values normalised on read: `public` → `open`, `private` → `invite`

## Application Modes (PART 6)

| Mode | Purpose |
|------|---------|
| `development` | Allows localhost, .local, .test; extra logging |
| `production` | HTTPS required; strict origin checks |
| `testing` | In-memory DBs; no external calls |

## Server Config (PART 12)

Config file sections: `server`, `auth`, `database`, `email`, `users`, `admin`, `scheduler`, `geoip`, `metrics`, `backup`

Key defaults:
- `server.port`: 8080 (host), 80 (Docker)
- `server.mode`: `development`
- `auth.session_timeout`: 24h
- `auth.max_sessions_per_user`: 10
- `admin.path`: `admin`

## Reference

For complete details, see AI.md PART 5, 6, 12
