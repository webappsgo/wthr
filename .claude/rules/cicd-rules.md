# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use Makefile in CI/CD → explicit commands only (no `make build`)
- Install tools inline (`apk add`, `go install`) → use `casjaysdev/go:latest` container
- Create `Dockerfile.build` or `build-toolchain.yml` for Go → not needed
- Use `ensure-build-image` pre-flight for Go → not needed
- Reference host paths like `~/.local/share/go` → use CI-native cache or `/tmp/`
- Use tag aliases in Actions → pin to full commit SHA
- Use Dependabot → use Renovate only (AGPL-3.0, free)
- Cross-cancel different release refs → only cancel older runs for the EXACT same ref

## CRITICAL - ALWAYS DO

- All jobs run inside `container: image: casjaysdev/go:latest`
- Pin all third-party Actions to full commit SHA (not tags)
- `concurrency` block on every workflow to cancel in-progress runs for same ref
- Build info: set VERSION/COMMIT_ID/BUILD_DATE in "Set build info" step → `$GITHUB_ENV`
- `-buildvcs=false` on all `go build` commands in CI
- truffleHog secret scanning in `ci.yml`
- govulncheck vulnerability scan in `ci.yml`
- Jenkinsfile required for Jenkins CI support

## Workflow Files

| File | Trigger | Purpose |
|------|---------|---------|
| `ci.yml` | Push/PR to main; weekly cron (security jobs) | Lint + test + vuln scan + secret scan |
| `release.yml` | Tag push (`v*`) | Production release |
| `beta.yml` | Push to `beta` branch | Beta release |
| `daily.yml` | Daily 3am UTC + push to main | Daily build |
| `docker.yml` | Version tag, push to main/beta | Docker images |

## Build Info Pattern

```yaml
- name: Set build info
  run: |
    if [ -f release.txt ]; then echo "VERSION=$(cat release.txt)" >> $GITHUB_ENV
    else echo "VERSION=${GITHUB_REF_NAME#v}" >> $GITHUB_ENV; fi
    echo "COMMIT_ID=$(git rev-parse --short HEAD)" >> $GITHUB_ENV
    echo "BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> $GITHUB_ENV
```

## Jenkinsfile

Required on every project. Uses `casjaysdev/go:latest` via `docker run`. Builds all 8 platforms.

## Reference

For complete details, see AI.md PART 28
