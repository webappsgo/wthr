# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Put Dockerfile or docker-compose.yml in project root → always in `docker/`
- Create `Dockerfile.build` for Go projects → use `casjaysdev/go:latest` directly
- Put LABEL blocks in Dockerfile → use OCI annotations via `docker/metadata-action` in CI
- Use `file_system/` as overlay directory name → must be `rootfs/`
- Add `DATABASE_DIR` pointing anywhere other than `/data/db/sqlite` in container
- Create mount points in Dockerfile → binary creates dirs on first run
- Use any port other than 80 as EXPOSE

## CRITICAL - ALWAYS DO

- Multi-stage build: builder (`casjaysdev/go:latest`) + runtime (`alpine:latest`)
- Build-time overlay: `docker/rootfs/` → copied to `/` in runtime image
- WORKDIR in builder: `/app`
- Binary output in builder: `/app/binary/wthr`
- Runtime binary at: `/usr/local/bin/wthr`
- Entrypoint: `["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- STOPSIGNAL: `SIGRTMIN+3`
- HEALTHCHECK: `--start-period=10m --interval=5m --timeout=15s --retries=3`
- ENV: `MODE=development TZ=America/New_York DATABASE_DIR=/data/db/sqlite`
- Required packages: `git curl bash tini tor`
- Runtime volumes: `./volumes/config:/config:z` and `./volumes/data:/data:z`

## Build Flags

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -buildvcs=false -trimpath \
    -ldflags "-s -w -X 'main.Version=${VERSION}' ..." \
    -o /app/binary/wthr ./src
```

## Container Paths

| Path | Purpose |
|------|---------|
| `/config/wthr/` | App config (server.yml, ssl/, tor/) |
| `/data/wthr/` | App data (uploads, cache, tor/) |
| `/data/db/sqlite/` | SQLite databases (server.db, users.db) |
| `/data/log/wthr/` | App logs |
| `/data/backups/wthr/` | Backup archives |
| `/usr/local/bin/wthr` | Application binary |
| `/usr/local/bin/entrypoint.sh` | Container entrypoint |

## Reference

For complete details, see AI.md PART 27
