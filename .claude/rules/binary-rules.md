# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ `CGO_ENABLED=1` — pure Go only, all 8 platforms
- ❌ Skip platforms in releases — build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
- ❌ Use `-musl` suffix in binary names (Alpine builds are NOT musl-specific)
- ❌ Build from anywhere except `./src`
- ❌ Use short flags besides `-h` and `-v`
- ❌ Skip `--help`, `--version`, `--config`, `--data`, `--log`, `--pid`, `--address`, `--port`, `--baseurl`, `--debug`, `--status`, `--service`, `--daemon`, `--maintenance`, `--update` flags
- ❌ Skip the client binary `weather-cli` — REQUIRED for all projects (PART 33)
- ❌ Add the agent binary unless this is a monitoring/management project (PART 33 agent is OPTIONAL)
- ❌ Print color/emoji when `NO_COLOR` is set or `TERM=dumb`
- ❌ Hardcode display mode — auto-detect (CLI / TUI / GUI / Headless) at runtime
- ❌ Embed dev-machine data (hostnames, IPs, paths)
- ❌ Have AI run binaries on the host — execute inside Docker / Incus only

## CRITICAL - ALWAYS DO
- ✅ Single static binary per platform with all assets embedded via `embed`
- ✅ Binary naming: `weather-{os}-{arch}` (Windows adds `.exe`)
- ✅ Server binary `weather`, client `weather-cli`, optional agent `weather-agent`
- ✅ Honor `NO_COLOR` (no-color.org spec) — disable ALL ANSI escapes when set
- ✅ Honor `TERM=dumb` — disable ALL ANSI escapes AND force CLI mode
- ✅ `--color=auto|always|never` flag overrides env vars
- ✅ `--lang=<code>` flag for i18n; default from `LC_ALL` / `LANG` / `en`
- ✅ Auto-detect display: `runtime.GOOS` + TTY check + `$DISPLAY` / `$WAYLAND_DISPLAY` for GUI
- ✅ Detect own name via `os.Args[0]` / `filepath.Base(os.Executable())` for renamed binaries (`{project_name}` may change; `{internal_name}=weather` is locked)
- ✅ `--status` exits 0 (healthy) or 1 (unhealthy)
- ✅ `--maintenance setup` requires authorization (first-run, root, or valid setup token)
- ✅ `--maintenance restore` requires admin auth OR root OR empty DB (destructive)
- ✅ `--update` checks `release.txt` then verifies SHA-256 + TLS before replacing self (PART 23)
- ✅ Client (`weather-cli`): CLI/TUI/GUI auto-detected; first-run wizard prompts for server URL + token + tests connection; saves config

## CLI FLAGS (canonical, NON-NEGOTIABLE)
```
--help                        (-h)
--version                     (-v)
--mode {production|development}
--config {dir}
--data {dir}
--log {dir}
--pid {file}
--address {listen}
--port {port}
--baseurl {path}              # default: /
--debug
--status
--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}
--daemon
--maintenance {backup,restore,update,mode,setup,--help}
--update [check|yes|branch {stable|beta|daily}]
```

## CLIENT (weather-cli) — REQUIRED
- First-run wizard: prompt for server URL, test connection, ask for API token (or generate via login), save config
- Three modes auto-detected: CLI (pipe/non-TTY), TUI (interactive TTY), GUI (when `$DISPLAY` available)
- Smart context: detects what subcommand the operator is in and offers context-appropriate help
- Cluster failover: tries each cluster member from saved config until one responds
- Auto-update from official `release.txt` URL with SHA-256 + TLS verification
- Token revocation: on 401 with token-revoked code, drops cached token and re-prompts

## AGENT (weather-agent) — OPTIONAL
- Only built if this project monitors/manages external resources
- Cluster-style bootstrap registration: registers via API, receives scoped agent token
- Scoped tokens: `adm_agt_*` / `usr_agt_*` / `org_agt_*` per owner
- Auto-update with SHA-256 + TLS verification
- Token lifecycle: rotate on revocation; never log token

---
For complete details, see AI.md PART 7, 8, 33
