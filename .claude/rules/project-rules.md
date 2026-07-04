# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use any license other than MIT
- Put Dockerfile or docker-compose.yml in project root (use `docker/`)
- Put source code outside `src/`
- Hardcode `{project_name}`, `{project_org}`, or `{internal_name}` — derive from git remote
- Use a path not in the PART 4 table for the current OS and privilege level
- Create `Dockerfile.build` for Go or Rust projects
- Put SQLite databases anywhere except `/var/lib/{project_org}/{internal_name}/db/` (Linux root) or the equivalent per PART 4

## CRITICAL - ALWAYS DO

- License: MIT, in `LICENSE.md` at project root
- Attribution: all third-party deps in LICENSE.md third-party section
- Directory layout exactly as specified in PART 3
- Source in `src/`, Docker in `docker/`, tests in `tests/`
- `docker/rootfs/` for build-time overlay files (NOT `docker/file_system/`)
- `release.txt` as version source of truth
- `CLAUDE.md` as short loader only (content in `.claude/rules/`)
- `IDEA.md` with all three required sections

## Project Layout (PART 3)

```
project/
├── src/           # All Go source
│   ├── main.go
│   ├── client/    # CLI/TUI binary
│   ├── agent/     # Agent binary (optional)
│   ├── common/    # Shared packages
│   ├── config/
│   ├── database/
│   ├── server/
│   └── util/
├── docker/
│   ├── Dockerfile
│   ├── Dockerfile.dev
│   ├── docker-compose.yml
│   ├── docker-compose.dev.yml
│   ├── docker-compose.test.yml
│   └── rootfs/
│       └── usr/local/bin/entrypoint.sh
├── tests/         # Integration tests
├── scripts/       # Helper scripts
├── docs/          # Documentation
├── .github/workflows/
├── AI.md          # READ-ONLY spec
├── IDEA.md        # Project business logic (WHAT)
├── CLAUDE.md      # Short loader
├── Makefile
├── release.txt
└── go.mod
```

## Key Placeholders

| Placeholder | Value |
|-------------|-------|
| `{project_name}` | wthr |
| `{project_org}` | casapps |
| `{internal_name}` | wthr (frozen at project creation, never changes) |
| `{admin_path}` | admin |

## Reference

For complete details, see AI.md PART 2, 3, 4
