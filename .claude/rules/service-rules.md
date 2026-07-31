# Privilege Escalation & Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never prompt for privilege escalation if the user cannot actually escalate
  (not in sudoers/wheel/admin group) — show an informative error instead.
- Never skip the "already root/admin" check — binary must check EUID/token
  first and skip the escalation prompt entirely if already privileged.
- Never run the service permanently as root/Administrator unless IDEA.md
  explicitly approves it — default is privilege drop after port binding.
- Never reuse a reserved/well-known UID/GID (65534 nobody, 999 docker,
  980-999 systemd-*/hardware services, 101-110 sshd/postfix/dovecot,
  170-179 postgres/mysql) even if it looks free on the current system.
- Never assign different values for UID and GID — they MUST match.
- Never search the full UID/GID range — stay within the safe range
  (Linux/BSD: 200-899, macOS: 200-399).
- Never give the service user a real shell or login — always
  `/sbin/nologin` (or `/usr/bin/false` on macOS), no password.
- Never uninstall the service without an explicit confirmation prompt
  (`--service --uninstall` deletes ALL data + the system user).
- Never delete the binary automatically on uninstall — only data/service
  files; print instructions for manual binary removal.
- Never use Local System / Administrator / logged-in-user accounts for the
  Windows service — default is Virtual Service Account (VSA).
- Never make `--service --install` do user/group creation — that happens
  during normal server STARTUP, not install. Install only installs+enables+starts.

## CRITICAL - ALWAYS DO

- ALWAYS support all service managers per OS: Linux (systemd, OpenRC,
  SysVinit, runit), macOS (launchd), BSD (rc.d), Windows (Windows Service).
- ALWAYS follow OS escalation method order:
  - Linux: already root → sudo → su → pkexec → doas
  - macOS: already root → sudo → osascript (GUI admin prompt)
  - BSD: already root → doas → sudo → su
  - Windows: already Administrator → UAC prompt → runas
- ALWAYS: on `--service --install`, detect init system, install as system
  service if root/admin, else fall back to user service (systemd --user,
  launchctl user agent); enable + start either way.
- ALWAYS: on `--service --uninstall`, stop → disable → remove service file
  → delete config/data/cache/log/backup dirs + PID file → delete
  system user/group → print "delete binary manually" message.
- ALWAYS: on `--service --disable`, stop + disable only — keep service
  file, data, and user/group intact (re-enable via `--install`).
- ALWAYS create a dedicated `{internal_name}` system user/group (same
  UID==GID) unless IDEA.md explicitly approves permanent root.
- ALWAYS find UID/GID by scanning from the top of the safe range downward
  (899→200 Linux/BSD, 399→200 macOS), skipping reserved IDs.
- ALWAYS bind privileged ports (<1024) as root/admin, then drop privileges
  to `{internal_name}` for Unix-like systems (start elevated → bind → drop → run).
- ALWAYS use Virtual Service Account (`NT SERVICE\{internal_name}`) for
  Windows services — auto-managed, minimal privilege, no manual password.
- ALWAYS create the home directory (config or data dir) BEFORE creating the
  service user, then set ownership after.
- ALWAYS harden systemd units: `ProtectSystem=strict`, `ProtectHome=yes`,
  `PrivateTmp=yes`, explicit `ReadWritePaths=` for config/data/cache/log dirs.
- ALWAYS document the exception explicitly (service file + docs) when
  IDEA.md requires permanent root, explaining why privilege drop isn't possible.

## Key Reference: Service File Locations

| Init system | Path |
|---|---|
| systemd | `/etc/systemd/system/{internal_name}.service` |
| OpenRC / SysVinit | `/etc/init.d/{internal_name}` |
| runit | `/etc/sv/{internal_name}/run` |
| rc.d (FreeBSD) | `/usr/local/etc/rc.d/{internal_name}` |
| launchd (macOS) | `/Library/LaunchDaemons/{plist_name}.plist` |
| Windows | Service Control Manager (`golang.org/x/sys/windows/svc`) |

## Key Reference: Reserved UID/GID (never use)

65534 (nobody) · 999-980 (systemd-*, docker, polkitd, kvm, pipewire,
avahi, rtkit, saned, cups) · 101-110 (sshd, postfix, dovecot) · 170-179
(postgres, mysql)

## Key Reference: System User Spec

Username/Group: `{internal_name}` · UID==GID, range 200-899 (Linux/BSD) or
200-399 (macOS) · Shell: `/sbin/nologin` · No password/login · Gecos:
`{internal_name} service account` · Home: config dir or data dir.

For complete details, see AI.md PART 24, 25
