# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use plural directory names: `handlers/`, `models/`, `middlewares/` → use `handler/`, `model/`, `middleware/`
- ❌ Put Dockerfile in repo root → `docker/Dockerfile`
- ❌ Store credentials, tokens, or keys in source code
- ❌ Hardcode machine-specific values (hostname, IP, paths)
- ❌ Use non-MIT license without explicit approval
- ❌ Put business logic in client binary (client is display-only)
- ❌ Use OS-specific paths without the path resolution system
- ❌ Skip the tini → entrypoint.sh → app startup chain in Docker

## CRITICAL - ALWAYS DO
- ✅ Singular directory names: `handler/`, `model/`, `middleware/`, `util/`, `path/`
- ✅ License: MIT — all projects
- ✅ Single self-contained binary — first-run works with zero config
- ✅ All functionality available to all users — no feature gating
- ✅ Telemetry opt-in only — never hardcode tracking IDs
- ✅ Use the path resolution system for OS-specific config/data/log dirs
- ✅ Detect paths at runtime (not hardcoded) — different for root vs user mode
- ✅ `src/` for source, `docker/` for Docker files

## DIRECTORY LAYOUT
```
src/
├── cli/          # weather-cli binary (REQUIRED)
├── common/       # shared code
├── config/       # configuration
├── graphql/      # GraphQL schema and resolvers
├── path/         # OS-specific path resolution (singular!)
├── renderer/     # output formatters
├── scheduler/    # built-in scheduler
├── server/
│   ├── handler/  # HTTP handlers (singular!)
│   ├── middleware/ # HTTP middleware (singular!)
│   ├── model/    # data models (singular!)
│   └── service/  # business logic services
├── swagger/      # OpenAPI/Swagger annotations
└── util/         # utilities (singular!)
docker/
├── Dockerfile
├── Dockerfile.aio
└── file_system/
```

## OS-SPECIFIC PATHS
| Mode | Config | Data | Logs |
|------|--------|------|------|
| Root/system | `/etc/apimgr/weather/` | `/var/lib/apimgr/weather/` | `/var/log/apimgr/weather/` |
| User | `~/.config/apimgr/weather/` | `~/.local/share/apimgr/weather/` | `~/.local/log/apimgr/weather/` |

## MODULE PATH
`github.com/apimgr/weather`

---
For complete details, see AI.md PART 2, 3, 4
