# Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Run as root in production after binding ports — drop to `weather` system user
- ❌ Skip privilege escalation checks before service install / privileged port binding / binary update
- ❌ Auto-escalate without prompting (`Y/n`) and without falling back when user declines
- ❌ Force escalation when the user has no sudo/wheel/admin group access — fall back gracefully
- ❌ Use `--service-install`, `--install-service`, or any other naming — the canonical flags are `--service --install` / `--service --uninstall` / `--service --disable`
- ❌ Skip service templates for any supported init: systemd, OpenRC, SysVinit, runit, rc.d (FreeBSD), launchd (macOS), Windows SCM
- ❌ Hardcode `weather.service` paths — use `{internal_name}.service` so renames don't move filesystem identity
- ❌ Run binary on host for service testing — use Incus (preferred) or Docker
- ❌ Modify host service files (`/etc/systemd/system/...`) outside the AI's project test container

## CRITICAL - ALWAYS DO
- ✅ Detect privilege at runtime (`isElevated()`), platform-independent (`privilege_unix.go` + `privilege_windows.go`)
- ✅ Check escalation ability (`canEscalate()`): sudo -n / wheel / admin group / Windows Administrators
- ✅ Smart escalation flow: ask once with `Y/n`, on `n` fall back to user-mode service or unprivileged port
- ✅ Service install creates the system user (`weather` / `weather`) the first time, `nologin` shell, system UID
- ✅ Set ownership and modes on first start: `chown -R weather:weather {config_dir} {data_dir} {cache_dir} {log_dir}`; `chmod 700` on `security/`, `ssl/`, `tor/`
- ✅ Drop privileges (Unix) AFTER binding privileged ports — bind as root, then `setuid(weather)`
- ✅ Windows: use Virtual Service Account `NT SERVICE\weather` (no privilege drop needed)
- ✅ Provide service file templates for ALL supported inits (PART 25)
- ✅ User-mode service fallback when no admin group: `~/.config/systemd/user/`, `lingering` enabled if available
- ✅ Service status queries are read-only — no escalation needed
- ✅ Re-exec via `execElevated(args)` (Unix: `sudo`; Windows: ShellExecute `runas`) when user agrees to escalate
- ✅ Setup / restore / mode change: AUTHORIZATION required, not just escalation (admin auth OR root OR setup token / empty DB)

## ESCALATION DECISION MATRIX (PART 24)
| Command | Needs Escalation? | Fallback |
|---------|:-----------------:|----------|
| `--service --install` | Check (`/etc/systemd/system/` writable?) | User service |
| `--service --uninstall` | Check (which service type installed?) | User service |
| `--service start/stop/restart/reload` | Check | User service |
| `--service status` | No (read-only) | n/a |
| `--port <1024` | Check (`isElevated()`?) | Random `64xxx` |
| `--maintenance update` | Check (binary writable?) | Error |
| `--maintenance backup` | No (backup dir owned by `weather`) | n/a |
| `--maintenance restore` | AUTH (admin OR root OR empty DB) | n/a |
| `--maintenance setup` | AUTH (first-run OR root OR setup token) | n/a |
| `--maintenance mode` | AUTH (admin OR root) | n/a |

## THE `weather` SYSTEM USER/GROUP (PART 24)
| Property | Value |
|----------|-------|
| Username/group | `weather` / `weather` |
| Shell | `/usr/sbin/nologin` |
| Home | `/var/lib/apimgr/weather` |
| UID/GID | Auto-assigned (UID < 1000 on Linux = system user) |

What the user CAN do: read/write `{config_dir}` `{data_dir}` `{cache_dir}` `{log_dir}` `{backup_dir}`, bind ports >1024 (or pre-bound privileged ports), run scheduled tasks, manage SQLite + SSL certs.

What the user CANNOT do: bind new privileged ports, modify service files, create system users, write to `/usr` `/bin`, modify the binary, install updates.

## SERVICE TEMPLATES (PART 25)
Provide for every supported init system:
- `systemd` (Linux, most): `/etc/systemd/system/weather.service` (system) + `~/.config/systemd/user/weather.service` (user)
- `OpenRC` (Alpine, Gentoo): `/etc/init.d/weather`
- `SysVinit` (older Linux): `/etc/init.d/weather` (LSB-compliant)
- `runit` (Void): `/etc/sv/weather/run`
- `launchd` (macOS): `/Library/LaunchDaemons/{plist_label}.plist` (system) + `~/Library/LaunchAgents/{plist_label}.plist` (user)
- `rc.d` (FreeBSD): `/usr/local/etc/rc.d/weather`
- Windows SCM: registered via `--service --install` (Virtual Service Account)

`{plist_label}` is reverse-DNS, persisted in config (e.g., `io.github.apimgr.weather` from repo URL).

---
For complete details, see AI.md PART 24, 25
