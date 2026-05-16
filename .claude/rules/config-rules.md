# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Hardcode any default values — detect at runtime
- ❌ Require config file to start — first-run must work with zero config
- ❌ Use `strconv.ParseBool()` — use project `config.ParseBool()` (handles 40+ variations)
- ❌ Store secrets in config file in plaintext
- ❌ Break on unknown config keys — ignore/warn, don't crash
- ❌ Use `.env` files as the primary config mechanism — `server.yml` is primary
- ❌ Require restart for settings that can be applied at runtime

## CRITICAL - ALWAYS DO
- ✅ Config file: `server.yml` in `{config_dir}`
- ✅ Environment variable overrides for all config values
- ✅ Sane, secure defaults — zero-config startup must work
- ✅ Validate all config on load; log errors clearly
- ✅ `config.ParseBool()` for boolean env vars (handles true/false/yes/no/1/0/on/off etc.)
- ✅ Support production, development, and test modes
- ✅ All admin-panel settings must write back to `server.yml`

## CONFIG HIERARCHY (highest to lowest priority)
1. Environment variables (override everything)
2. `server.yml` in config dir
3. Built-in defaults (sane, secure, always work)

## KEY CONFIG SECTIONS
| Section | Purpose |
|---------|---------|
| `server.*` | Port, host, TLS, mode |
| `database.*` | SQLite/PostgreSQL settings |
| `cache.*` | Valkey/Redis settings |
| `smtp.*` | Email settings |
| `admin.*` | Admin path, session timeout |
| `logging.*` | Log level, format, output |
| `scheduler.*` | Job intervals |
| `tor.*` | Tor hidden service config |

## APPLICATION MODES
| Mode | ENV value | Behavior |
|------|-----------|---------|
| Production | `MODE=production` | Minimal logging, no debug |
| Development | `MODE=development` | Verbose logging, debug endpoints |
| Test | `MODE=test` | In-memory DB, no side effects |

---
For complete details, see AI.md PART 5, 6, 12
