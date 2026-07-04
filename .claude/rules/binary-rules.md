# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use CGO → CGO_ENABLED=0 always
- Build on host → always use Docker (`casjaysdev/go:latest`)
- Create `Dockerfile.build` for Go → use `casjaysdev/go:latest` directly
- Skip platforms → build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
- Name CLI binary anything other than `{project_name}-cli`
- Name agent binary anything other than `{project_name}-agent`
- Skip `--help` / `--version` / `--status` flags
- Use `os.Exit` inside library code

## CRITICAL - ALWAYS DO

- Three binaries: `wthr` (server), `wthr-cli` (client — REQUIRED), `wthr-agent` (optional)
- All three built from same `go.mod`
- Static binaries: `CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}`
- Embed build info via ldflags: Version, CommitID, BuildDate, OfficialSite
- `--help` output includes all flags with descriptions
- `--version` outputs JSON: `{"version":"...","commit":"...","build_date":"..."}`
- `--status` checks if server is running and returns JSON status
- CLI default command: open TUI (not print usage)
- `src/common/` packages: terminal, display, theme, banner, version

## Common Packages (PART 7)

```
src/common/
├── banner/       # Startup banner (4-tier: full/compact/minimal/micro)
├── display/      # Display environment detection (Headless/CLI/TUI/GUI)
│   ├── detect.go
│   └── mode.go
├── terminal/     # Terminal utilities
│   ├── size.go   # Terminal size detection + SizeMode
│   ├── resize.go # SIGWINCH handler
│   └── symbols.go # Unicode/ASCII symbol set
├── theme/        # Color themes (dark/light/auto)
│   └── colors.go
└── version/      # Version info package
    └── version.go
```

## Binary Naming

| Binary | Local | Distribution |
|--------|-------|-------------|
| Server | `wthr` | `wthr-{os}-{arch}` |
| CLI | `wthr-cli` | `wthr-cli-{os}-{arch}` |
| Agent | `wthr-agent` | `wthr-agent-{os}-{arch}` |

## Reference

For complete details, see AI.md PART 7, 8, 33
