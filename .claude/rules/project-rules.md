# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use GPL/AGPL/LGPL dependencies — only MIT/Apache 2.0/BSD/ISC/MPL allowed
- ❌ Skip `LICENSE.md` or omit third-party license attributions
- ❌ Use `github.com/mattn/go-sqlite3` (CGO) — use `modernc.org/sqlite`
- ❌ Use `github.com/lib/pq` — use `github.com/jackc/pgx/v5`
- ❌ Use `github.com/dgrijalva/jwt-go` (unmaintained) — use `github.com/golang-jwt/jwt/v5`
- ❌ Use `github.com/gorilla/mux` (archived) — use `github.com/go-chi/chi/v5`
- ❌ Use any CGO-requiring library
- ❌ Pin Go to a specific version in CI (`go-version: '1.21'`) — use `'stable'`
- ❌ Place `Dockerfile` or `docker-compose.yml` in project root — they live in `docker/`
- ❌ Place `data/`, `config/`, `logs/`, `tmp/`, `temp/`, `test-data/`, `build/`, `dist/`, `out/`, `vendor/`, `node_modules/`, `lib/`, `libs/`, `utils/`, `common/` at project root
- ❌ Create `SUMMARY.md`, `COMPLIANCE.md`, `NOTES.md`, `CHANGELOG.md`, `AUDIT.md`, `REPORT.md`, `ANALYSIS.md`, `TODO.md`, `*.example.*`, `*.sample.*`, `server.yml`, `cli.yml`, `.env*` at project root
- ❌ Place `CONTRIBUTING.md` / `CODE_OF_CONDUCT.md` / `SECURITY.md` / `PULL_REQUEST_TEMPLATE.md` at project root — they live under `.github/` (or `.gitea/`)
- ❌ Hardcode dev-machine values (hostname, IP, CPU count, memory, paths) — detect at runtime
- ❌ Hardcode platform paths (`/etc/...`, `~/.config/...`) — resolve via `paths` package per OS + privilege
- ❌ Use plural directory names (`handlers/`, `models/`, `services/`) — use singular
- ❌ Use vague filenames (`utils.go`, `common.go`, `misc.go`, `stuff.go`)
- ❌ Skip platforms in releases — build all 8: `linux/darwin/windows/freebsd × amd64/arm64`
- ❌ Use `-musl` suffix in binary names (Alpine builds are NOT musl-specific)
- ❌ Mix purposes between `{config_dir}`, `{data_dir}`, `{log_dir}`, `{backup_dir}`

## CRITICAL - ALWAYS DO
- ✅ MIT license; `LICENSE.md` in repo root with embedded third-party licenses (compact table for 10+ deps)
- ✅ Argon2id for passwords (RFC 9106, OWASP 2023 params: time=3, memory=64MB, threads=4, keylen=32, saltlen=16)
- ✅ SHA-256 for API/session token hashing (high-entropy tokens don't need slow hashing)
- ✅ `CGO_ENABLED=0` everywhere
- ✅ Latest stable Go (`go-version: 'stable'` in CI, `golang:alpine` in Docker)
- ✅ Build all 8 platforms: linux/darwin/windows/freebsd × amd64/arm64
- ✅ Binary naming `weather-{os}-{arch}` (Windows adds `.exe`)
- ✅ Build source ALWAYS from `./src`
- ✅ All Go code under `src/`; Docker files under `docker/`; scripts under `scripts/` & `tests/`; MkDocs under `docs/`
- ✅ Project root determined via `git rev-parse --show-toplevel`, never assumed from cwd
- ✅ License-check in CI (no GPL/AGPL/LGPL in `go-licenses csv ./...`)
- ✅ License badge in README using `https://img.shields.io/github/license/apimgr/weather` (so GitHub auto-detects)
- ✅ Dockerfile `LABEL org.opencontainers.image.licenses="MIT"`
- ✅ Files end with exactly ONE newline; no trailing whitespace; UTF-8

## ALLOWED ROOT FILES (exhaustive — anything else MUST NOT exist)
| File | Required | Gitignored |
|------|:--------:|:----------:|
| `AI.md` | ✓ | No |
| `IDEA.md` | ✓ | No |
| `CLAUDE.md` | ✓ | No |
| `CLAUDE.local.md` | - | **Yes** |
| `README.md` | ✓ | No |
| `LICENSE.md` | ✓ | No |
| `go.mod`, `go.sum` | ✓ | No |
| `Makefile` | ✓ | No |
| `mkdocs.yml`, `.readthedocs.yaml` | ✓ | No |
| `release.txt` | ✓ | No |
| `site.txt` | - | No |
| `.gitignore`, `.dockerignore`, `.gitattributes` | ✓ | No |
| `.editorconfig` | - | No |
| `Jenkinsfile` | - | No |
| `TODO.AI.md`, `PLAN.AI.md` | - | No |

## ALLOWED ROOT DIRECTORIES (exhaustive)
| Directory | Required | Gitignored |
|-----------|:--------:|:----------:|
| `src/` | ✓ | No |
| `docker/` | ✓ | No |
| `docs/` | ✓ | No |
| `scripts/` | ✓ | No |
| `tests/` | ✓ | No |
| `.github/` (or `.gitea/`) | If used | No |
| `binaries/` | Auto | **Yes** |
| `releases/` | Auto | **Yes** |
| `rootfs/` | Auto | **Yes** |
| `.claude/`, `.cursor/`, `.aider/`, `.ai/`, `.windsurf/` | Optional | Personal/state ignored, shared rules tracked |

## OS-SPECIFIC PATHS (PART 4 summary)
Always use `{config_dir}`, `{data_dir}`, `{db_dir}`, `{log_dir}`, `{cache_dir}`, `{backup_dir}`, `{pid_file}` placeholders — resolve at runtime.

| OS | Privileged config | Privileged data | Privileged logs |
|----|-------------------|-----------------|-----------------|
| Linux | `/etc/apimgr/weather/` | `/var/lib/apimgr/weather/` | `/var/log/apimgr/weather/` |
| macOS | `/Library/Application Support/apimgr/weather/` | `/Library/Application Support/apimgr/weather/data/` | `/Library/Logs/apimgr/weather/` |
| BSD | `/usr/local/etc/apimgr/weather/` | `/var/db/apimgr/weather/` | `/var/log/apimgr/weather/` |
| Windows | `%ProgramData%\apimgr\weather\` | `%ProgramData%\apimgr\weather\data\` | `%ProgramData%\apimgr\weather\logs\` |
| Docker | `/config/weather/` | `/data/weather/` | `/data/log/weather/` |

User-mode (non-privileged) paths follow XDG (`~/.config/`, `~/.local/share/`, `~/.cache/`, `~/.local/log/`).

**Config file is ALWAYS `server.yml`** (auto-migrate from `server.yaml` on startup).

**Internal-name vs project-name:** filesystem paths and system identifiers use `{internal_name}=weather` (locked at first install). User-visible strings use `{project_name}` (the binary basename, may change on rename).

## REQUIRED LIBRARIES (pure Go, CGO_ENABLED=0)
- DB: `modernc.org/sqlite`, `github.com/jackc/pgx/v5`, `github.com/go-sql-driver/mysql`, `github.com/microsoft/go-mssqldb`, `github.com/tursodatabase/libsql-client-go`, `go.mongodb.org/mongo-driver`
- Cache/cluster: `github.com/redis/go-redis/v9`, `github.com/bradfitz/gomemcache`
- Core: `gopkg.in/yaml.v3`, `github.com/google/uuid`, `golang.org/x/crypto`
- Auth: `github.com/pquerna/otp`, `github.com/go-webauthn/webauthn`, `github.com/golang-jwt/jwt/v5`, `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2`, `github.com/go-ldap/ldap/v3`, `github.com/gorilla/sessions`
- Network: `github.com/go-chi/chi/v5`, `github.com/cretz/bine`, `github.com/gorilla/websocket`, `github.com/rs/cors`
- Utilities: `github.com/robfig/cron/v3`, `golang.org/x/time/rate`, `github.com/go-playground/validator/v10`

## TRACKED-VS-IGNORED CONSISTENCY (PART 3 first-session check)
On first AI session in this project:
```bash
# Find paths both tracked AND matched by .gitignore
git ls-files -i -c --exclude-standard
# For each offender, untrack (working-tree copy stays):
git rm --cached -r <path>
```

---
For complete details, see AI.md PART 2, 3, 4
