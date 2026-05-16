# Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Run as root permanently — drop privileges after port binding
- ❌ Call `ulimit -u unlimited` or `setrlimit(RLIM_INFINITY)`
- ❌ Skip systemd unit file — required for Linux service support
- ❌ Skip launchd plist — required for macOS service support
- ❌ Skip OpenRC init script — required for Alpine/OpenRC support
- ❌ Skip Windows service support
- ❌ Use hardcoded service names — derive from `{internal_name}`

## CRITICAL - ALWAYS DO
- ✅ Drop privileges after binding privileged port (< 1024)
- ✅ Systemd unit: `weather.service` in `lib/systemd/system/`
- ✅ Launchd plist: `io.github.apimgr.weather.plist` in `lib/launchd/`
- ✅ OpenRC init: `weather` in `lib/openrc/`
- ✅ Windows service: via `golang.org/x/sys/windows/svc`
- ✅ `weather service install` — installs and enables the service
- ✅ `weather service uninstall` — removes the service cleanly
- ✅ `weather service start/stop/restart/status` — manages running service
- ✅ SIGTERM handler: graceful shutdown within 30s, then SIGKILL
- ✅ PID file management (single instance enforcement)

## SERVICE COMMANDS
| Command | Action |
|---------|--------|
| `weather service install` | Install OS service, enable autostart |
| `weather service uninstall` | Remove OS service |
| `weather service start` | Start service |
| `weather service stop` | Stop service gracefully |
| `weather service restart` | Restart service |
| `weather service status` | Show service status |
| `weather service logs` | Show recent logs |

## PRIVILEGE ESCALATION
- Bind to port 80/443 if needed (requires root or `CAP_NET_BIND_SERVICE`)
- Immediately drop to unprivileged user after binding
- Recommended: run as `weather` system user (created by `service install`)
- Never run main process as root past port binding

## GRACEFUL SHUTDOWN
1. Receive SIGTERM (or Windows shutdown)
2. Stop accepting new requests
3. Drain in-flight requests (30s timeout)
4. Flush queues (email, notifications)
5. Close DB connections
6. Remove PID file
7. Exit 0

---
For complete details, see AI.md PART 24, 25
