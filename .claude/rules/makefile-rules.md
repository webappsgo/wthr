# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use Makefile in CI/CD pipelines — CI uses explicit commands only
- ❌ Run `go` directly on host — always via Docker (`make dev`, `make build`)
- ❌ `make install` that installs to system paths without user confirmation
- ❌ Include platform-specific commands that break cross-platform `make`
- ❌ Use Makefile for production deployment — CI/CD only

## CRITICAL - ALWAYS DO
- ✅ Makefile is for LOCAL DEV ONLY — never referenced from CI/CD YAML
- ✅ All `make` targets run inside Docker containers
- ✅ `make dev` — start development server with hot reload
- ✅ `make build` — build all 8 platform binaries via Docker
- ✅ `make test` — run all tests via Docker
- ✅ `make lint` — run go vet + staticcheck via Docker
- ✅ `make clean` — remove build artifacts
- ✅ `.PHONY` for all non-file targets

## REQUIRED MAKE TARGETS
| Target | Purpose |
|--------|---------|
| `dev` | Run development server (Docker) |
| `build` | Build all 8 platform binaries |
| `test` | Run all tests |
| `lint` | Lint and vet |
| `clean` | Remove build artifacts |
| `docker-build` | Build Docker image |
| `docker-run` | Run Docker container locally |

## DOCKER PATTERN FOR MAKE TARGETS
```makefile
build:
	docker run --rm -v $(PWD):/build -w /build \
	  -e CGO_ENABLED=0 \
	  golang:alpine \
	  go build -ldflags="-s -w" -o weather ./src
.PHONY: build
```

---
For complete details, see AI.md PART 26
