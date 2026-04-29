# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use the Makefile in CI/CD workflows — workflows have explicit commands with all env vars
- ❌ Pin a specific Go version (`go-version: '1.21'`) — use `'stable'`
- ❌ Diverge GitHub / Gitea / Jenkins workflows — same platforms, same env vars, same logic
- ❌ Skip Docker builds for branches — Docker MUST build on EVERY push to ANY branch
- ❌ Push to a registry from a fork's PR — use `pull_request_target` only with explicit allowlist or use untrusted-PR build (no push)
- ❌ Skip `actions/checkout@v6` (or current major) — pin to a major version
- ❌ Hardcode build credentials in workflows — use `secrets.GITHUB_TOKEN` / `vars.PROJECT_ORG` / repository secrets
- ❌ Strip `v` prefix from non-semver tags — only strip from semver (`v1.2.3` → `1.2.3`); leave `dev`, `beta`, branch names as-is
- ❌ Use `actions/cache@v4` without scoping the key to the workflow + OS — cache poisoning risk
- ❌ Skip license check (`go-licenses csv ./...` must pass with no GPL/AGPL/LGPL)
- ❌ Skip cross-compilation for any of the 8 platforms in release workflows

## CRITICAL - ALWAYS DO
- ✅ Workflows live under `.github/workflows/` (GitHub) and/or `.gitea/workflows/` (Gitea/Forgejo)
- ✅ Standard workflow files:
  - `release.yml` — stable releases on tag push
  - `beta.yml` — beta releases (branch or tag)
  - `daily.yml` — daily build (cron schedule)
  - `docker.yml` — Docker image build/push on every push
  - `licenses.yml` — license compatibility check on push/PR
  - `tests.yml` — unit + integration tests on PR
- ✅ LDFLAGS: `-s -w -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}' -X 'main.OfficialSite=https://wthr.top'`
- ✅ `${VERSION}` from tag (strip `v` prefix from semver only)
- ✅ Cross-compile for all 8 platforms in release workflow
- ✅ Docker tag scheme:
  - Push to any branch → `devel`, `{commit_short}`
  - Push to `beta` branch → adds `beta` tag
  - Push of semver tag → `{version}`, `latest`, `YYMM` (e.g., `2604` for April 2026), `{commit_short}`
- ✅ Generate SHA-256 checksums for every released binary; sign with the project's PGP key (when available)
- ✅ Upload binaries + checksums + signatures to release page
- ✅ License-check job in CI fails on any GPL/AGPL/LGPL match
- ✅ Cache `~/go/pkg/mod` and `~/.cache/go-build` keyed by `hashFiles('go.sum')` + OS + workflow
- ✅ Jenkins (`Jenkinsfile`) mirrors the same logic when used

## STANDARD WORKFLOW SHAPE (GitHub example)
```yaml
name: Release
on:
  push:
    tags: ['v*.*.*']

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        os: [linux, darwin, windows, freebsd]
        arch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v5
        with:
          go-version: 'stable'
      - name: Build
        env:
          GOOS: ${{ matrix.os }}
          GOARCH: ${{ matrix.arch }}
          CGO_ENABLED: '0'
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          COMMIT="$(git rev-parse --short HEAD)"
          BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
          go build -ldflags "-s -w -X 'main.Version=$VERSION' -X 'main.CommitID=$COMMIT' -X 'main.BuildDate=$BUILD_DATE' -X 'main.OfficialSite=https://wthr.top'" \
            -o "binaries/weather-${{ matrix.os }}-${{ matrix.arch }}${{ matrix.os == 'windows' && '.exe' || '' }}" ./src
      - name: Checksums
        run: cd binaries && sha256sum * > SHA256SUMS
      - uses: softprops/action-gh-release@v2
        with:
          files: |
            binaries/*
            binaries/SHA256SUMS
```

## DOCKER WORKFLOW
```yaml
name: Docker
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      packages: write
    steps:
      - uses: actions/checkout@v6
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ghcr.io/apimgr/weather
          tags: |
            type=ref,event=branch
            type=sha,prefix={{branch}}-
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest,enable={{is_default_branch}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: docker/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

---
For complete details, see AI.md PART 28
