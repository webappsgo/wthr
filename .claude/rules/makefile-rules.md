# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## SCOPE
**Makefile is for LOCAL development only — NEVER used in CI/CD.** CI workflows have explicit commands with all env vars. See `.claude/rules/cicd-rules.md`.

## CRITICAL - NEVER DO
- ❌ Run `go build / go test / go run` directly on the host — every Makefile target uses Docker (`golang:alpine`)
- ❌ Use spaces for indentation (Makefile requires TABS)
- ❌ Skip the standard targets: `dev`, `local`, `build`, `test`, `clean`, `docker`, `release`
- ❌ Pollute project root with build output — `make dev` → temp dir; `make local` / `make build` → `binaries/`
- ❌ Reference the Makefile from `.github/workflows/*.yml` / `.gitea/workflows/*.yml` / `Jenkinsfile`
- ❌ Hardcode dev-machine paths in Makefile — derive `PROJECT_NAME` / `PROJECT_ORG` from git remote / `basename "$(PWD)"`
- ❌ Bake version info into `make dev` (it's for fast iteration); only `make local` and `make build` set LDFLAGS

## CRITICAL - ALWAYS DO
- ✅ All Go work uses `docker run --rm -v "$(PWD):/src" -w /src golang:alpine ...` (or shared GODIR/GOCACHE volumes for speed)
- ✅ Standard targets:

| Target | Output | When to Use |
|--------|--------|-------------|
| `make dev` | `${TMPDIR:-/tmp}/apimgr/weather-XXXXXX/weather` | Fast iteration |
| `make local` | `binaries/weather-{os}-{arch}` (with version) | Production-like local test |
| `make build` | `binaries/weather-{os}-{arch}` (all 8 platforms) | Pre-release |
| `make test` | Coverage report | After code changes |
| `make docker` | Docker image build | Local image test |
| `make release` | Tag + cross-build + sign | Cutting a release |
| `make clean` | Removes `binaries/`, `releases/`, temp dirs | Full reset |

- ✅ TABS for recipe indentation; comments use `#` ABOVE the recipe
- ✅ `make dev` produces a fast unsigned binary in a temp dir following `${TMPDIR:-/tmp}/apimgr/weather-XXXXXX/`
- ✅ `make local` / `make build` set LDFLAGS: `-s -w -X 'main.Version=$(VERSION)' -X 'main.CommitID=$(COMMIT)' -X 'main.BuildDate=$(BUILD_DATE)' -X 'main.OfficialSite=https://wthr.top'`
- ✅ `VERSION` derived from git tag (strip `v` prefix only for semver: `v1.2.3` → `1.2.3`); fallback `dev`
- ✅ `make build` cross-compiles for `linux/darwin/windows/freebsd × amd64/arm64`
- ✅ `make test` runs `go test ./...` inside the same Docker image
- ✅ Persistent caches via volume mounts (`GODIR`, `GOCACHE`) so rebuilds are fast on subsequent runs
- ✅ `make clean` removes ALL build output AND ALL temp dirs matching the project's prefix

## STANDARD MAKEFILE SHAPE
```makefile
# Project metadata derived from git/remote; never hardcoded
PROJECT_NAME := $(shell basename "$$(git remote get-url origin 2>/dev/null | sed 's/\.git$$//')" 2>/dev/null || basename "$(PWD)")
PROJECT_ORG  := $(shell basename "$$(dirname "$$(git remote get-url origin 2>/dev/null)")" 2>/dev/null || basename "$$(dirname "$(PWD)")")
VERSION      := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
COMMIT       := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

DOCKER_IMG   := golang:alpine
SRC_MOUNT    := -v "$(PWD):/src" -w /src
CACHE_MOUNT  := -v go-mod-cache:/go/pkg/mod -v go-build-cache:/root/.cache/go-build
DOCKER_RUN   := docker run --rm $(SRC_MOUNT) $(CACHE_MOUNT) -e CGO_ENABLED=0
LDFLAGS      := -s -w -X 'main.Version=$(VERSION)' -X 'main.CommitID=$(COMMIT)' -X 'main.BuildDate=$(BUILD_DATE)' -X 'main.OfficialSite=https://wthr.top'

# .PHONY guards every non-file target
.PHONY: dev local build test docker release clean

dev:
	$(DOCKER_RUN) $(DOCKER_IMG) sh -c '...'   # fast unsigned binary to temp dir

local:
	$(DOCKER_RUN) $(DOCKER_IMG) sh -c '...'   # build with LDFLAGS to binaries/

build:
	$(DOCKER_RUN) $(DOCKER_IMG) sh -c '...'   # cross-compile all 8 platforms to binaries/

test:
	$(DOCKER_RUN) $(DOCKER_IMG) sh -c 'go test ./src/...'

clean:
	rm -rf binaries/ releases/ ${TMPDIR:-/tmp}/apimgr/weather-*
```

---
For complete details, see AI.md PART 26
