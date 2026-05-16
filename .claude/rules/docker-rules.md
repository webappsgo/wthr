# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Put Dockerfile in repo root — `docker/Dockerfile` only
- ❌ Override or bypass the tini → entrypoint.sh → app startup chain
- ❌ Run as root in the container at runtime (app should drop privileges)
- ❌ Use `latest` tag for base images in production Dockerfiles
- ❌ Commit `.env` files — `docker-compose.yml` has sane hardcoded defaults
- ❌ Use `--network host` in test containers
- ❌ Leave containers running after tests — always `docker run --rm`
- ❌ Pin base images in dev Dockerfiles — rolling tags only for dev

## CRITICAL - ALWAYS DO
- ✅ Startup chain: `tini → entrypoint.sh → app` (never bypass)
- ✅ `ENTRYPOINT ["/usr/bin/tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- ✅ Multi-stage build: `golang:alpine` (builder) → `alpine` or `debian` (runtime)
- ✅ `CGO_ENABLED=0` in builder stage
- ✅ Standard image (`Dockerfile`): alpine-based, small, app-only
- ✅ AIO image (`Dockerfile.aio`): debian-based, includes PostgreSQL + Valkey + Tor + Supervisor
- ✅ `docker-compose.yml` with hardcoded sane defaults — works with zero `.env`
- ✅ Only expose app port (80/443) — DB/cache are internal-only
- ✅ HEALTHCHECK in every Dockerfile
- ✅ Build for linux/amd64 + linux/arm64

## DOCKERFILE LOCATIONS
| File | Image | Base | Purpose |
|------|-------|------|---------|
| `docker/Dockerfile` | `ghcr.io/apimgr/weather:latest` | alpine | Standard — app only |
| `docker/Dockerfile.aio` | `ghcr.io/apimgr/weather:latest-aio` | debian | All-in-one with DBs |

## STARTUP CHAIN (REQUIRED)
```
PID 1: tini (signal reaper)
  └─► entrypoint.sh (init, env setup, first-run config)
        └─► weather (the app)
```

All startup customization goes in `entrypoint.sh`. Never override `ENTRYPOINT`.

## HEALTHCHECK PATTERN
```dockerfile
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD timeout 10s bash -c ':> /dev/tcp/127.0.0.1/80' || exit 1
```
AIO: use `--start-period=90s` (PostgreSQL init takes longer)

## ENVIRONMENT VARIABLES (standard)
| Var | Default | Purpose |
|-----|---------|---------|
| `MODE` | `production` | Application mode |
| `PORT` | `80` | HTTP port |
| `DEBUG` | `false` | Debug logging |
| `TZ` | `America/New_York` | Timezone |
| `DATABASE_DIR` | `/data/db/sqlite` | SQLite location |

---
For complete details, see AI.md PART 27
