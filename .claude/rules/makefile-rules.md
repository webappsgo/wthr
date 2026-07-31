# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER add targets beyond the six core targets (`dev`, `local`, `build`, `test`, `release`, `docker`)
- NEVER build on the host — all Go builds/tests run inside Docker via `$(GO_DOCKER)` (`casjaysdev/go:latest`)
- NEVER hardcode `PROJECT_NAME` / `PROJECT_ORG` — always derive from `git remote get-url origin` (fallback: directory names)
- NEVER copy or symlink binaries out of `binaries/` (no `cp`/`ln -s` to `/usr/local/bin`, `~/bin`, `releases/`, etc.) — only CI/CD copies binaries, during release
- NEVER add a `v` prefix to a TEXT or timestamp version (`dev`, `beta`, `20251218060432`) — only NUMERIC semver gets `v` (`0.2.0` → `v0.2.0`)
- NEVER double the `v` prefix (`vv0.3.0` is always wrong)
- NEVER guess/assume `site.txt` / `OFFICIAL_SITE` — must be explicitly created by the user; empty is valid for self-hosted
- NEVER let `make release` / `make docker` push — `docker` builds and tags only (no push); pushing images and creating GitHub releases from tags/branches is CI/CD's job (PART 28), `make release` is for manual local releases only
- NEVER skip `clean` semantics — `build` and `local` both run `clean` first

## CRITICAL - ALWAYS DO

- ALWAYS treat the Makefile as **local dev only** — CI/CD (PART 28) is a separate, non-overlapping concern
- ALWAYS use `$(GO_DOCKER)` for every Go command (build/test/mod) — mounts `$(PWD):/app`, caches `GO_CACHE`/`GO_BUILD`, sets `CGO_ENABLED=0` and `GOFLAGS=-buildvcs=false`
- ALWAYS derive `VERSION` with precedence: `VERSION` env var > `release.txt` > `"devel"` fallback
- ALWAYS embed `Version`, `CommitID`, `BuildDate`, `OfficialSite` via `-ldflags` (`-s -w -X main.X=...`) in `local`/`build`/`release`, never in `dev`
- ALWAYS write `dev` output to an isolated temp dir: `$(mktemp -d "${TMPDIR:-/tmp}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX")`
- ALWAYS build server + CLI (`src/client/` if present) + agent (`src/agent/` if present) in `dev`, `local`, and `build`
- ALWAYS enforce ≥60% coverage in `test` (fails the build below threshold)
- ALWAYS build all 8 platforms in `build`/`release`: linux, darwin, windows, freebsd × amd64/arm64
- ALWAYS strip binaries with musl (final name has NO `-musl` suffix); `release` strips before packaging

## Key Rules Summary

### Six targets

| Target | Purpose | Output | Ldflags? |
|---|---|---|---|
| `dev` | fast local build | `${TMPDIR}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/` | no |
| `local` | prod-style test build | `binaries/` | yes |
| `build` | full 8-platform release build | `binaries/` | yes |
| `test` | unit tests + coverage (≥60%) | coverage report | n/a |
| `release` | local manual release (runs `build`) | `releases/` + `gh release create` | yes |
| `docker` | build container, no push | buildx cache | n/a |

### GO_DOCKER pattern (reuse verbatim)

```makefile
GO_CACHE  ?= $(HOME)/go/pkg/mod
GO_BUILD  ?= $(HOME)/.cache/go-build/$(PROJECT_NAME)

GO_DOCKER := docker run --rm \
	--name $(PROJECT_NAME)-$$(tr -dc 'a-z0-9' </dev/urandom | head -c8) \
	--memory=$(DOCKER_MEM) --cpus=$(DOCKER_CPUS) \
	-v $(PWD):/app \
	-v $(GO_CACHE):/usr/local/share/go/pkg/mod \
	-v $(GO_BUILD):/usr/local/share/go/cache \
	-w /app \
	-e CGO_ENABLED=0 \
	-e GOFLAGS=-buildvcs=false \
	casjaysdev/go:latest
```

### Version tag `v` prefix

- Numeric semver (`0.2.0`, `1.2.3-rc1`) → add `v` → `v0.2.0`
- Text (`dev`, `beta`, `daily`) or timestamp (`20251218060432`) → NO `v`
- Already has `v` → leave as-is (never `vv...`)

### Local dev workflow (not CI/CD)

1. `make dev` — active coding
2. `make test` — Phase 1 toolchain gate, pre-commit requirement
3. `./tests/run_tests.sh` — Phase 2 binary validation
4. `make local` — production-style test build before release
5. `make build` — full cross-platform build before tagging
6. `./tests/incus.sh` — preferred full systemd integration test

### Binary handling

- Binaries stay in `binaries/` until explicitly moved for release — never copied/symlinked to PATH, system dirs, or between `binaries/`/`releases/`
- Only exception: CI/CD release process copies stripped binaries to a GitHub release

For complete details, see AI.md PART 26
