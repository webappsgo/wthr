# License, Structure & Paths Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER use GPL/AGPL/LGPL licensed dependencies (copyleft — forces project to be GPL)
- NEVER ship a project without `LICENSE.md` in the root
- NEVER omit third-party license attribution for MIT/Apache 2.0/BSD/ISC/MPL 2.0 deps
- NEVER hardcode `{project_name}` or `{project_org}` — always infer from git remote or path
- NEVER assume current working directory is project root — always resolve explicitly
- NEVER mix runtime directory purposes (user config in `{data_dir}`, app data in `{config_dir}`, etc.)
- NEVER use `github.com/mattn/go-sqlite3` (requires CGO) — use `modernc.org/sqlite`
- NEVER use `github.com/lib/pq`, `github.com/ooni/go-libtor`, `github.com/dgrijalva/jwt-go`, `github.com/gorilla/mux`, `github.com/go-redis/redis` (old path) — see forbidden libs table
- NEVER use any CGO-requiring library — breaks `CGO_ENABLED=0` static builds
- NEVER store plaintext passwords anywhere (config, logs, database)
- NEVER trim/allow passwords with leading/trailing whitespace — reject instead
- NEVER hardcode a specific Go version anywhere (docs, Docker, CI) — always latest stable
- NEVER use `/data/**` or `/config/**` paths outside Docker containers
- NEVER run `go` directly — always use Makefile targets (`make dev`, `make test`, etc.)

## CRITICAL - ALWAYS DO

- ALWAYS use MIT License for the project (`LICENSE.md` in root)
- ALWAYS embed third-party licenses in `LICENSE.md` (compact table for 10+ deps, full text where license requires it e.g. Apache 2.0 NOTICE, BSD-3-Clause non-endorsement clause)
- ALWAYS include a license badge in README.md: `[![License](https://img.shields.io/github/license/{project_org}/{project_name})](LICENSE.md)`
- ALWAYS apply license as OCI annotation at Docker build time (`--annotation "org.opencontainers.image.licenses=MIT"`), never a Dockerfile `LABEL`
- ALWAYS infer `{project_name}`/`{project_org}` from git remote first, fall back to directory path
- ALWAYS treat `{internal_name}` as frozen once set (survives project renames); it seeds `{config_dir}`, `{data_dir}`, `{log_dir}`, `{cache_dir}`, `{pid_file}`, systemd unit name, `{plist_name}`
- ALWAYS resolve paths relative to project root (git top-level via `git rev-parse --show-toplevel`), never CWD
- ALWAYS support all 4 OSes (Linux, BSD, macOS, Windows) and both AMD64/ARM64
- ALWAYS use `modernc.org/sqlite` (pure Go, no CGO) for SQLite; accept `sqlite`/`sqlite2`/`sqlite3` as config aliases, normalize internally
- ALWAYS hash passwords with Argon2id (OWASP 2023 params: time=3, memory=64MB, threads=4, keyLen=32, saltLen=16); bcrypt only as a legacy-verify-then-rehash fallback
- ALWAYS hash API/session tokens with SHA-256 (fast lookup; different from password hashing)
- ALWAYS keep config file named `server.yml` (never `.yaml`)
- ALWAYS use `casjaysdev/go:latest` (unpinned) for Go builds/CI, never `setup-go` or pinned tags
- ALWAYS include `.claude/rules/` cheatsheet files matching AI.md PART groupings (this file is PART 2/3/4)

## Key Rules Summary

### License & Attribution (PART 2)
- License: MIT only. File: `LICENSE.md` in project root.
- Attribution required for MIT, Apache 2.0, BSD 2/3-clause, ISC, MPL 2.0. Optional for Public Domain/Unlicense/CC0/WTFPL.
- Use `go-licenses` (Go) or `license-checker` (Node) to scan deps; automate GPL/AGPL/LGPL detection in CI (`.github/workflows/licenses.yml`).
- Update `LICENSE.md` whenever a dependency is added/removed/upgraded (verify license didn't change).
- `go.mod` has no license field — document via `LICENSE.md`, README badge, and package doc comments instead.

### Project Structure (PART 3)
- Variables: `{project_name}` (mutable, lowercase, e.g. `jokes`), `{PROJECT_NAME}` (UPPERCASE env/Makefile), `{project_org}` (lowercase), `{PROJECT_ORG}` (UPPERCASE), `{internal_name}` (frozen, lowercase, all on-disk identifiers), `{INTERNAL_NAME}` (frozen UPPERCASE), `{plist_name}` = `io.github.{project_org}.{internal_name}`. Anything in `{}` is a variable; anything not in `{}` is literal.
- Recommended local path: `~/Projects/{gitprovider}/{project_org}/{internal_name}` — but project root can live anywhere; always resolve via git top-level, not assumed CWD.
- Required root layout: `.github/` or `.gitea/` workflows, `CLAUDE.md`, `.claude/` (settings, agents, `rules/` cheatsheets), `docs/` (MkDocs), `src/`, `scripts/`, `tests/` (`run_tests.sh`, `docker.sh`, `incus.sh`), `docker/` (`Dockerfile`, `Dockerfile.dev`, compose files, `rootfs/` — committed), `volumes/` (gitignored), `binaries/`/`releases/` (gitignored), `README.md`, `LICENSE.md`, `AI.md`, `TODO.AI.md`, `Jenkinsfile`, `release.txt`.
- `.gitignore` MUST start with `# gitignore created on MM/DD/YY at HH:MM` then literal `ignoredirmessage`; base from `gitignore --dir . default` plus project entries (`binaries/`, `releases/`, `volumes/`, `.claude/`, `.cursor/`, `.aider/`, `.ai/`, `.windsurf/`, `CLAUDE.local.md`).
- `.dockerignore` excludes `.git/`, CI configs, `volumes/`, `binaries/`, `releases/`, `tests/`, `docs/`, `*.md`, `Makefile`, IDE files, AI config dirs — but keeps `src/`, `go.mod`/`go.sum`, `docker/` (including `rootfs/`).
- Runtime dirs: `{config_dir}` = user-editable, `{data_dir}` = app-managed, `{log_dir}` = logs, `{backup_dir}` = archives. Never mix purposes.
- Platform support required: Linux, BSD, macOS (Intel+ARM), Windows — AMD64 and ARM64.
- Go: always latest stable, build-only (static binary), `casjaysdev/go:latest` for builds/CI, never pinned versions.
- Required pure-Go libraries (CGO_ENABLED=0 compatible): DB drivers (`modernc.org/sqlite`, `libsql-client-go`, `pgx/v5`, `go-sql-driver/mysql`, `go-mssqldb`, `mongo-driver`), cache (`go-redis/v9`, `gomemcache`), core (`yaml.v3`, `google/uuid`, `x/crypto` argon2/bcrypt), auth (`pquerna/otp`, `go-webauthn/webauthn`, `golang-jwt/jwt/v5`, `go-oidc/v3`, `x/oauth2`, `go-ldap/ldap/v3`, `crewjam/saml`, `gorilla/sessions`), network (`go-chi/chi/v5`, `cretz/bine`, `gorilla/websocket`, `rs/cors`), utilities (`embed`, `gocron/v2`, `x/time/rate`, `go-playground/validator/v10`).
- Passwords: Argon2id always; tokens: SHA-256. Never plaintext, never in config files or logs.

### OS-Specific Paths (PART 4)
All paths follow `{internal_org}/{internal_name}` pattern; config file is always `server.yml`.

| OS | Privileged Config | User Config | Service |
|----|-------------------|--------------|---------|
| Linux | `/etc/{internal_org}/{internal_name}/` | `~/.config/{internal_org}/{internal_name}/` | `/etc/systemd/system/{internal_name}.service` |
| macOS | `/Library/Application Support/{internal_org}/{internal_name}/` | `~/Library/Application Support/{internal_org}/{internal_name}/` | LaunchDaemon/LaunchAgent `.plist` |
| BSD | `/usr/local/etc/{internal_org}/{internal_name}/` | `~/.config/{internal_org}/{internal_name}/` | `/usr/local/etc/rc.d/{internal_name}` |
| Windows | `%ProgramData%\{internal_org}\{internal_name}\` | `%AppData%\{internal_org}\{internal_name}\` | Windows Service Manager |

- Linux privileged data/cache/logs: `/var/lib`, `/var/cache`, `/var/log`; user: `~/.local/share`, `~/.cache`, `~/.local/log`.
- macOS privileged data: `/Library/Application Support/.../data/`; user: `~/Library/Application Support/...`.
- BSD privileged data: `/var/db`; user: `~/.local/share`.
- Windows privileged data: `%ProgramData%\...\data\`; user: `%LocalAppData%\...`.
- Docker/container ONLY: `/config/{project_name}/` and `/data/{project_name}/` (with `db/`, `cache/`, `security/`, `log/`, `backups/` subpaths) — never use these simplified paths on native OS installs. Internal port `80`.
- SQLite DBs live under `db/` subdirectory of the data path on every platform (`server.db`, `users.db`).
- Backups always under a dedicated `Backups`/`backups` path per OS, never mixed into data or config dirs.

For complete details, see AI.md PART 2, 3, 4
