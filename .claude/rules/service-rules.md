# Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Require root permanently → drop privileges after port binding
- Skip privilege drop when running as root
- Hardcode service user UID/GID
- Skip service file for any supported service manager
- Use `os.Exit(1)` after port binding without cleanup

## CRITICAL - ALWAYS DO

- Detect running privilege level at startup (root vs user)
- Drop privileges after port binding (Unix): `syscall.Setuid` / `syscall.Setgid`
- Create service user `wthr:wthr` if it does not exist
- Support ALL service managers: systemd, OpenRC, SysVinit, launchd (macOS), Windows Service
- Service files installed to PART 4 paths
- `--install-service` flag generates and installs service file
- `--uninstall-service` flag removes service file and disables
- Windows: run as `NT SERVICE\wthr` (Virtual Service Account)

## systemd Unit

```ini
[Unit]
Description=wthr service
Documentation=https://casapps.github.io/wthr
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/wthr
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/casapps/wthr
ReadWritePaths=/var/lib/casapps/wthr
ReadWritePaths=/var/cache/casapps/wthr
ReadWritePaths=/var/log/casapps/wthr

[Install]
WantedBy=multi-user.target
```

## Privilege Escalation (PART 24)

Binary detects context:
- Root → bind any port, then drop to `wthr` user
- Non-root → bind only ports > 1024
- Docker → no privilege drop (already isolated)

## Reference

For complete details, see AI.md PART 24, 25
