# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Add targets beyond the six core ones (build / local / release / docker / test / dev / clean)
- Use Makefile in CI/CD → explicit commands only in workflows
- Hardcode PROJECTNAME or PROJECTORG → infer from git remote
- Build on host → all builds via Docker (`casjaysdev/go:latest`)
- Use `golang:alpine` → use `casjaysdev/go:latest`
- Push in `make docker` → that is CI/CD's responsibility
- Put coverage output in project tree → use `mktemp -d "/tmp/$(PROJECTORG)/..."` temp dir
- Use `$(pwd)` in docker -v flags → use `$(PWD)` (Makefile variable, not subshell)

## CRITICAL - ALWAYS DO

- Infer PROJECTNAME and PROJECTORG from `git remote get-url origin`
- Version from `release.txt` (or `devel` fallback)
- BUILD_DATE in ISO 8601 UTC: `date -u +"%Y-%m-%dT%H:%M:%SZ"`
- Cache dirs: `GO_CACHE ?= $(HOME)/go/pkg/mod`, `GO_BUILD ?= $(HOME)/.cache/go-build`
- Docker run with `--name`, `-it`, correct volume paths to `/usr/local/share/go/`
- `-e GOFLAGS=-buildvcs=false` on all Docker runs
- `-buildvcs=false -trimpath` on all `go build` commands
- Coverage gate: fail if < 80%
- `clean` target removes only `binaries/` and `releases/`

## GO_DOCKER Pattern

```makefile
GO_DOCKER := docker run --rm -it \
    --name $(PROJECTNAME)-$$(tr -dc 'a-z0-9' </dev/urandom | head -c8) \
    -v $(PWD):/app \
    -v $(GO_CACHE):/usr/local/share/go/pkg/mod \
    -v $(GO_BUILD):/usr/local/share/go/cache \
    -w /app \
    -e CGO_ENABLED=0 \
    -e GOFLAGS=-buildvcs=false \
    casjaysdev/go:latest
```

## Six Core Targets

| Target | Purpose |
|--------|---------|
| `build` | All 8 platforms + local; runs `clean` first |
| `local` | Local OS/ARCH only |
| `release` | Build + strip + archive + GitHub release |
| `docker` | buildx multi-arch (linux/amd64,linux/arm64); local only |
| `test` | Coverage > 80% gate; output to temp dir |
| `dev` | Fast local build to `mktemp` dir |
| `clean` | Remove `binaries/` and `releases/` |

## Reference

For complete details, see AI.md PART 26
