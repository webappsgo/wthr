# CI/CD Workflow Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never use Makefile targets inside CI/CD workflows — commands must be explicit
  (`go build`, not `make build`) for visibility.
- Never reference local user paths (`~/.local/share/go`, host cache dirs) in CI —
  use CI-native caching only.
- Never depend on local Docker containers for the build step — GitHub/Gitea Actions
  use the `casjaysdev/go:latest` job container directly.
- Never `apk add` or `go install` tools inline inside a CI job — all tooling is
  pre-installed in `casjaysdev/go:latest`.
- Never use a `build-toolchain.yml` or `ensure-build-image` gate for Go CI.
- Never cross-cancel different release refs — concurrency groups must only cancel
  older runs for the *exact same* branch/tag ref (e.g. `v1.2.4` must not cancel
  `v1.2.3`).
- Never use `github.event.default_branch` for secret-scan diff range — after a push
  it equals HEAD and silently skips the scan; use `github.event.before`/`after`.
- Never include a `-musl` suffix in binary names.
- Never skip the coverage threshold check (default 60%) in `ci.yml`.

## CRITICAL - ALWAYS DO

- Always run every CI job inside `container: image: casjaysdev/go:latest`.
- Always pin third-party GitHub Actions to a full commit SHA with a trailing
  `# vX.Y.Z` comment (e.g. `actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0  # v7.0.0`) — never a floating tag.
- Always set `VERSION`, `COMMIT_ID`, `BUILD_DATE` explicitly via a "Set build info"
  step (`$GITHUB_ENV` / `$GITEA_ENV`), not as static `env:`.
- Always build the full 8-target platform matrix: linux/darwin/windows/freebsd ×
  amd64/arm64 (windows/arm64 uses `.exe` ext).
- Always add workflow concurrency with `cancel-in-progress` for branch-push
  workflows targeting `main`, `master`, `devel`, `dev`, or `beta` (`beta.yml`,
  `daily.yml`, `docker.yml`, and any project-specific branch-push workflow).
- Always scope tag-release concurrency (`release.yml`) per exact tag ref.
- Always build CLI binaries (`-cli` suffix) only when `src/client/` exists, and
  Agent binaries only when `src/agent/` exists (`if: hashFiles(...) != ''`).
- Always run secret scanning (truffleHog, Apache-2.0) on every public repo.

## Required Workflows (per platform)

| File | Trigger | Purpose |
|------|---------|---------|
| `ci.yml` | push/PR to default branch; security jobs also weekly cron | build+test+lint+coverage+secret-scan+image-scan+workflow-policy |
| `release.yml` | tag push (`v*`, `*.*.*`) | production release, 8-platform matrix |
| `beta.yml` | push to `beta` | prerelease build |
| `daily.yml` | daily 3am UTC + push to main/master | rolling `daily` tag, deletes previous release |
| `docker.yml` | any branch push + version tags + daily schedule | standard (alpine) + devel images to `ghcr.io` |
| `docker-aio.yml` | any branch push + version tags | all-in-one (debian, postgres+valkey+tor) image to `ghcr.io` |

Config locations by provider:

| Provider | Directory |
|----------|-----------|
| GitHub | `.github/workflows/*.yml` |
| Gitea | `.gitea/workflows/*.yml` |
| Forgejo | `.forgejo/workflows/*.yml` (or `.gitea/workflows/`, compatible) |
| GitLab | single `.gitlab-ci.yml` with stages |
| Jenkins | `Jenkinsfile`, `BUILD_TYPE` var selects release/beta/daily; needs amd64+arm64 agents |

## Local Dev vs CI/CD

| Aspect | Local | CI/CD |
|--------|-------|-------|
| Go toolchain | Docker `casjaysdev/go:latest`, bind-mounted host cache | same image, CI-native cache |
| Build command | `make dev`/`make local`/`make build` | explicit `go build ...` |
| Testing | Docker/Incus containers | job container or `docker run` |

## Docker Image Matrix

| Image | Dockerfile | Tag suffix | Base |
|-------|------------|------------|------|
| Standard | `docker/Dockerfile` | (none) | alpine |
| All-in-One | `docker/Dockerfile.aio` | `-aio` | debian (+ PostgreSQL + Valkey + Tor) |

Tags: any push → `devel`/`{commit}` (+ `beta` on beta branch); version tag →
`{version}`, `latest`, `YYMM`. Built for `linux/amd64,linux/arm64` via buildx,
pushed to `ghcr.io`.

For complete details, see AI.md PART 28
