# Binary & CLI Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never link CGO — build MUST be `CGO_ENABLED=0`, pure Go only, single
  static binary, assets embedded via `embed`.
- Never embed security databases (GeoIP, blocklists, CVE, Trivy) in the
  binary — they're downloaded on first run and kept updated by the scheduler.
- Never hand-roll `os.Args` flag parsing for the server's primary flag set
  — use stdlib `flag` (server is single-command, no subcommands).
- Never implement UI-mode flags (`--tui`, `--cli`, `--gui`,
  `--mode tui/cli/gui`, a `tui` subcommand) — display mode is
  auto-detected; `--mode` is ONLY `production`/`development`.
- Never launch a TUI/GUI for `-h`/`--help`/`-v`/`--version` — always
  plain text, immediate exit, no escalation, no privilege check.
- Never use short flags except `-h`/`-v` — everything else is long-form only.
- Never use `strconv.ParseBool()` for boolean flags — use
  `config.ParseBool()`/`config.IsTruthy()`.
- Never resolve `~`/`$HOME` after the privilege drop — service account
  HOME points at `{data_dir}`; resolve once at startup (EUID-based mode
  decision), cache for process lifetime.
- Never fall back the system-mode backup dir to `$HOME` — fall back to
  `{data_dir}/backup/` instead.
- Never skip stale-PID detection, and never use a PID file inside a
  container (`isContainer()` → skip PID file entirely).
- Never substring-match process identity for PID reuse checks — exact
  match on binary name (substring would also match `{project_name}-cli`).
- Never show `0.0.0.0`, `127.0.0.1`, or `localhost` in user-facing URL
  display — always a valid FQDN/host/IP, one address only.
- Never hardcode host/IP/port in code — always detect dynamically.
- Never build an agent unless the project genuinely needs one (server
  must reach INTO remote machines) — client is required for every
  project, agent is a per-project decision.
- Never run an agent inside a container — agents run directly on the
  target system (systemd/launchd/Windows service).
- Never auto-retry CLI re-authentication after `401 TOKEN_REVOKED` — must
  be a deliberate user action.
- Never let the CLI add cluster URLs that weren't in the autodiscover
  response.

## CRITICAL - ALWAYS DO

- ALWAYS detect display environment (GUI/TUI/CLI/Headless) in every
  binary via `display.DetectDisplayEnv()` — Wayland > X11 > platform
  checks; `TERM=dumb` forces CLI mode (no ANSI, no emoji, no TUI, ASCII
  tables, text spinners/progress).
- ALWAYS respect `NO_COLOR` (disables colors + emojis, not bold/underline
  or box-drawing) in ALL binaries; priority: CLI flag > config >
  `NO_COLOR` > auto-detect (TTY + `TERM`).
- ALWAYS show the ACTUAL renamed binary name (`filepath.Base(os.Args[0])`)
  in `--help`/`--version`/error messages, but hardcode `{project_name}`
  for User-Agent, default paths, config keys, DB tables, API identifiers.
- ALWAYS create directories referenced by `--config/--data/--cache/--log
  /--backup/--pid` if missing (root: `0755`/`0644`; user: `0700`/`0600`),
  and verify writable.
- ALWAYS bind privileged ports (<1024) while still root, THEN drop
  privileges — never bind after dropping.
- ALWAYS follow the immediate-exit-flags-first startup order: help/version
  /status/shell/service-help/maintenance-help/update-help → service
  subcommands → maintenance subcommands → update subcommands → actual
  server startup.
- ALWAYS support `--shell completions [SHELL]` and `--shell init [SHELL]`
  in every binary (server, client, agent), auto-detecting `$SHELL` when
  omitted.
- ALWAYS accept flag values via both `--flag=value` and `--flag value`.
- ALWAYS give every flag a config-file equivalent (precedence: CLI flag >
  env var > config file > hardcoded default).
- ALWAYS allow anyone (no privilege) to run `--help`, `--version`,
  `--status`, `--update check`.
- ALWAYS refuse to load `cli.yml`/`token` if perms are too loose (Unix:
  must be `0600`; Windows: ACL restricted to the running user) — warn and
  bail, don't silently proceed.
- ALWAYS delete the cached token on `401 TOKEN_REVOKED`/`TOKEN_EXPIRED` so
  the next run prompts fresh; exit code `4` for non-interactive/streaming.
- ALWAYS verify SHA-256 before an atomic binary swap on self-update (CLI,
  server, agent all follow the same download → verify → atomic-replace →
  restart/re-exec pattern).
- ALWAYS use the standard exit codes (0 success, 1 general, 2 config, 3
  connection, 4 auth, 5 not found, 64 usage).

## Key Reference: Binary Types & Naming

| Binary | Default Name | Key Flags | Privileges |
|---|---|---|---|
| Server | `{project_name}` | `--config --data --port --mode` | Root optional (privileged ports) |
| Client | `{project_name}-cli` | `--server --token --output` | None |
| Agent | `{project_name}-agent-{os}-{arch}` | `--server --token --config` | Root/Admin required |

Shared flags (ALL binaries): `--help -h`, `--version -v`, `--shell`,
`--debug`, `--color {auto\|yes\|no}`, `--lang`.

## Key Reference: Display Mode by Binary

| Binary | GUI | TUI | CLI | Headless |
|---|---|---|---|---|
| Server | Status window | Status banner | Commands | Default (daemon) |
| CLI | Full app | Full app (default) | Commands | Error |
| Agent | Status window | Status banner | Commands | Default (service) |

CLI is the only binary with a full TUI/GUI + setup wizard; server/agent
only show status banners.

## Key Reference: Server Directory Flags

`--config --data --cache --log --backup --pid` each have OS-specific
system (root) vs user paths and an env-var fallback (`CONFIG_DIR`,
`DATA_DIR`, `LOG_DIR`, `PID_FILE`, `{PROJECT_NAME}_PORT`, `LISTEN`,
`MODE`, plus `DATABASE_DIR`/`BACKUP_DIR` with no CLI flag). Mode
(system vs user) is decided ONCE from EUID at startup and locked for
the process lifetime.

## Key Reference: Agent Decision

Build an agent only if: data originates on remote machines you manage,
the server needs to reach INTO them, a background daemon collects/
executes there, and/or machines push data to the server. Communication
pattern is one of Send Only (metrics/logs, low risk), Receive Only
(config/commands pulled, medium risk), or Bidirectional (most common,
high risk — requires mTLS/tokens, authz, audit logging, sandboxing).

For complete details, see AI.md PART 7, 8, 33
