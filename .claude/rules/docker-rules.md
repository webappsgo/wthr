# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER place Dockerfile or docker-compose.yml in project root — always `docker/`
- NEVER modify ENTRYPOINT or CMD — all customization goes in `entrypoint.sh`
- NEVER bake `MODE`/`DEBUG` into the image — binary defaults to production; only `docker-compose.dev.yml`/`docker-compose.test.yml` set them
- NEVER use `LABEL` blocks in the Dockerfile — all OCI metadata applied at build time by CI (`labels:`/`annotations:`)
- NEVER include `build:` or `version:` in docker-compose.yml
- NEVER use list-style env vars (`- KEY=value`) — always map style (`KEY: value`)
- NEVER create `.env`, `.env.example`, `.env.sample` files — hardcode sane defaults with `${VAR:-default}` fallbacks
- NEVER run `docker compose` in the project directory — always via a temp dir (`mktemp -d "${TMPDIR:-/tmp}/{project_org}/{internal_name}-XXXXXX"`)
- NEVER commit runtime `./volumes/` content from local runs
- AI assistants must NEVER use `docker-compose.dev.yml` or `docker-compose.yml` (production) directly — human use only. AI uses `docker-compose.test.yml` (prefer `tests/run_tests.sh` / `tests/docker.sh`)
- NEVER expose database/cache ports externally in AIO images — only port 80

## CRITICAL - ALWAYS DO
- ALWAYS multi-stage build: builder (`casjaysdev/go:latest`) + runtime (`alpine:latest`, standard) or `debian:latest` (AIO — needs glibc for postgres/valkey/tor)
- ALWAYS use `docker/rootfs/` for build-time container overlay (entrypoint.sh, service configs) — committed to git
- ALWAYS use `entrypoint.sh` for container startup, kept MINIMAL (set env, start services, exec binary, handle signals) — binary handles dirs/perms/user/Tor
- ALWAYS use `tini` as init: `ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]`
- ALWAYS set `STOPSIGNAL SIGRTMIN+3` (systemd-compatible graceful shutdown)
- ALWAYS `HEALTHCHECK --start-period=90s --interval=10s --timeout=5s --retries=3 CMD {binary} --status`
- ALWAYS expose internal port 80 only
- ALWAYS mount exactly 2 volumes: `./volumes/config:/config:z` and `./volumes/data:/data:z` (`:z` in production only, omit in dev temp dirs)
- ALWAYS include `x-logging: &default-logging` anchor (json-file, max-size 5m, max-file 1) and reference it (`logging: *default-logging`) on every service
- ALWAYS name things consistently: compose `name: {project_name}`; main service `{project_name}` / container `{project_name}-app`; db `{project_name}-db`; cache `{project_name}-cache`
- ALWAYS bind production ports to Docker bridge (`172.17.0.1:{port}:80`); dev binds all interfaces (`{port}:80`)
- ALWAYS build for `linux/amd64` and `linux/arm64`

## Key Rules Summary

**Dockerfile requirements**
| Item | Value |
|------|-------|
| Location | `docker/Dockerfile` |
| Build | Multi-stage: builder + runtime |
| Builder | `casjaysdev/go:latest` |
| Runtime (standard) | `alpine:latest` |
| Runtime (AIO) | `debian:latest` (glibc needed for postgres/valkey/tor) |
| Packages | git, curl, bash, tini, tor |
| Binary path | `/usr/local/bin/{project_name}` |
| Init | tini |
| Port | 80 (internal, always) |

**Multi-stage build pattern**
1. Builder stage: `FROM casjaysdev/go:latest AS builder`, copy `go.mod`/`go.sum` first (cache), `go mod download`, copy source, `CGO_ENABLED=0 go build` with `-ldflags "-s -w -X ..."` using `$TARGETARCH`.
2. Runtime stage: `FROM alpine:latest` (or `debian:latest` for AIO), install runtime packages only, `COPY --from=builder`, `COPY docker/rootfs/ /`, `chmod 755`, set `ENV`, `EXPOSE 80`, `STOPSIGNAL`, `HEALTHCHECK`, `ENTRYPOINT`.
3. No `LABEL` in Dockerfile — CI applies `labels:`/`annotations:` at build time (per-platform config vs manifest index for multiarch).

**Toolchain vs runtime image rules**
- Toolchain (builder) image is always `casjaysdev/go:latest` — never swap for a custom base.
- Runtime image is the project's choice: `alpine:latest` (standard, musl, small) or `debian:latest` (AIO only, glibc required by postgres/valkey/tor — alpine/musl is insufficient there).
- Standard image = app only. AIO image (`Dockerfile.aio`, tag `:latest-aio`) = app + embedded postgres + valkey + tor via supervisord, single container.

**Directory structure**
```
docker/
├── Dockerfile              # production
├── Dockerfile.dev          # devel, tag :devel
├── Dockerfile.aio          # all-in-one (optional)
├── docker-compose.yml      # production — HUMAN USE ONLY
├── docker-compose.dev.yml  # dev — HUMAN USE ONLY
├── docker-compose.test.yml # test — AI/AUTOMATED TESTING ONLY
└── rootfs/                 # build-time container overlay
    └── usr/local/bin/entrypoint.sh
```

**Container paths**: `/config/{project_name}/` (config, ssl/, tor/), `/data/{project_name}/` (data, security/, tor/), `/data/db/{sqlite,postgres,valkey}/`, `/data/log/{project_name}/`, `/data/backups/{project_name}/`. Compose only ever mounts `/config` and `/data` as wholes, never subdirs.

**Testing workflow**: prefer `tests/run_tests.sh` / `tests/docker.sh` over invoking `docker-compose.test.yml` directly; fallback is copy-to-temp-dir-and-run.

For complete details, see AI.md PART 27
