# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Place `Dockerfile` or `docker-compose.yml` in project root — they live in `docker/`
- ❌ Single-stage Dockerfile — must be multi-stage (`golang:alpine` builder + `alpine:latest` runtime)
- ❌ Modify `ENTRYPOINT` or `CMD` — all customization via `docker/file_system/usr/local/bin/entrypoint.sh`
- ❌ Set `USER weather` in Dockerfile — that prevents privileged-port binding; the binary handles privilege drop itself
- ❌ Skip required runtime packages: `git`, `curl`, `bash`, `tini`, `tor`
- ❌ Skip `STOPSIGNAL SIGRTMIN+3`
- ❌ Skip `LABEL org.opencontainers.image.licenses="MIT"` and other OCI labels
- ❌ Default port other than `80` inside the container
- ❌ Default port `80` for native (non-container) mode — that's container-only; native uses random `64xxx`
- ❌ Copy or symlink the binary outside its build path — single canonical location
- ❌ Mount runtime volume data (`./rootfs/config`, `./rootfs/data`) inside the project repo for committed runs — `rootfs/` is gitignored
- ❌ Expose Tor binary directly — the server binary controls Tor lifecycle
- ❌ Bundle GeoIP / blocklist / CVE / Trivy DBs in the image — download on first run
- ❌ Use `docker-compose` (legacy v1) — use `docker compose` (v2 plugin)

## CRITICAL - ALWAYS DO
- ✅ `docker/Dockerfile` (production), `docker/Dockerfile.dev` (optional)
- ✅ `docker/docker-compose.yml` (production), `docker/docker-compose.dev.yml`, `docker/docker-compose.test.yml`
- ✅ Multi-stage build:
  - **Builder:** `FROM golang:alpine AS builder` → `apk add git ca-certificates` → `COPY src/` → `CGO_ENABLED=0 go build -ldflags '...' -o /weather ./src`
  - **Runtime:** `FROM alpine:latest` → `apk add --no-cache git curl bash tini tor` → `COPY --from=builder /weather /usr/local/bin/weather` → `COPY docker/file_system/ /` → `STOPSIGNAL SIGRTMIN+3` → `ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- ✅ Default timezone `America/New_York`, override with `TZ` env var
- ✅ Internal port `80` (container namespace is isolated)
- ✅ Host-side port: random unused, persisted in config (e.g., `-p 64580:80`)
- ✅ OCI labels: `org.opencontainers.image.title`, `description`, `source`, `licenses=MIT`, `version`, `revision`, `created`
- ✅ `docker/file_system/` is the BUILD-TIME overlay that gets `COPY`'d into the image (entrypoint.sh, etc.)
- ✅ Runtime volumes mounted from `./rootfs/config:/config:z` and `./rootfs/data:/data:z` (with `:z` for SELinux compat)
- ✅ Docker tag scheme:
  - Any push: `devel`, `{commit_short}`
  - Beta branch: adds `beta`
  - Tag push (semver): `{version}`, `latest`, `YYMM`, `{commit_short}`
- ✅ Docker builds on EVERY push to ANY branch
- ✅ `entrypoint.sh` handles: env var → CLI flag mapping, mkdir for runtime dirs, chown to `weather:weather`, exec the binary

## DOCKER-COMPOSE.YML SHAPE
```yaml
services:
  weather:
    image: ghcr.io/apimgr/weather:latest
    container_name: weather
    restart: unless-stopped
    ports:
      - "64580:80"
    volumes:
      - ./rootfs/config:/config:z
      - ./rootfs/data:/data:z
    environment:
      TZ: America/New_York
      MODE: production
    stop_signal: SIGRTMIN+3
    healthcheck:
      test: ["CMD", "curl", "-q", "-LSsf", "http://localhost/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
```

## .DOCKERIGNORE (REQUIRED)
Must exclude: `.git/`, `.gitignore`, `.gitattributes`, `.github/`, `.gitea/`, `.gitlab-ci.yml`, `Jenkinsfile`, `rootfs/`, `binaries/`, `releases/`, `tests/`, `docs/`, `*.md`, `Makefile`, `.idea/`, `.vscode/`, `*.swp`, `.claude/`, `.cursor/`, `.aider/`, `.ai/`, `.windsurf/`.

Must INCLUDE: `src/`, `go.mod`, `go.sum`, `docker/`, `docker/file_system/`.

## AI DOCKER COMPOSE RULES (PART 29 cross-ref)
AI may use:
- `docker/docker-compose.dev.yml` for local development verification
- `docker/docker-compose.test.yml` (DEBUG=true) for integration tests
- `docker/docker-compose.yml` (production) ONLY for read-only verification (`config`, `pull` — never `up` against shared infra)

---
For complete details, see AI.md PART 27
