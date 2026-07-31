# Features Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER send any email without a valid, working SMTP connection — no SMTP = no attempt, no queueing "for later", no "would have sent" log lines
- NEVER show email-dependent UI (password reset link, etc.) when SMTP is not configured — hide it, show "Email features require SMTP configuration" instead
- NEVER use an external scheduler (cron, systemd timers, Task Scheduler, launchd, Kubernetes CronJob, cloud schedulers) for ANY scheduled task — the built-in scheduler is the only mechanism, with zero exceptions
- NEVER embed GeoIP MMDB databases in the binary — always downloaded on first run and updated via scheduler (`geoip_update`, weekly)
- NEVER use `geoip2-golang` for ip-location-db files — use `github.com/oschwald/maxminddb-golang` (ip-location-db's custom `database_type` strings make `geoip2.Open()` fail)
- NEVER expose `/metrics` publicly — internal only; block at firewall/proxy/NetworkPolicy, never proxy it to the public internet
- NEVER use raw client IP as a metrics label (unbounded cardinality = memory-DoS) — aggregate or use a fixed-size LRU; log per-IP detail to structured logs instead
- NEVER delete old backups before the new backup passes ALL verification checks (file exists, size>0, checksum, decrypt test, manifest, content extraction, DB integrity)
- NEVER restore without authorization (PART 5 Sensitive Operations) — first-run/root allowed, service user needs admin creds, random user denied
- NEVER skip checksum verification on self-update — refuse to update if no `.sha256` asset is found for the platform binary
- NEVER roll out `auto_install: true` updates to all cluster nodes simultaneously — node-by-node only, so the cluster stays available
- NEVER surface "update available" on public pages — running version/update status is Tier 3 info (PART 11); only Server Admin routes see it (PWA "new version" banner is the sole exception, since it discloses no server version)
- NEVER let compliance mode (`server.compliance.enabled: true`) run backups without an encryption password set — block with error/dialog, log audit warning, retry next scheduled run
- NEVER store the backup encryption password anywhere — admin must remember it; no recovery if lost

## CRITICAL - ALWAYS DO

- ALWAYS auto-detect local SMTP on first run (127.0.0.1 → docker bridge → gateway → fqdn → global IPv4 → mail./smtp. subdomains, ports 25/465/587) and re-test the configured connection on every startup
- ALWAYS provide sane, working-out-of-the-box defaults for every email template; allow full customization via admin panel with custom files in `{config_dir}/template/email/` overriding embedded defaults
- ALWAYS include in account-related emails: why sent, recipient address, app name + FQDN, a visible plaintext link (not just a button), and an "ignore if not you" disclaimer where applicable
- ALWAYS suppress `scheduler_error` when a more specific failure event fires for the same execution (`backup_failed` for backup tasks, `ssl_renewal_failed` for SSL tasks) — one notification, not two
- ALWAYS keep the scheduler always-running, with persistent state in `server.db`, automatic catch-up of missed tasks within `catch_up_window`, and cluster-aware locking (global tasks run on one node, local tasks on every node)
- ALWAYS run the required built-in tasks: `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`, `update_check`, `session_cleanup`, `token_cleanup`, `log_rotation`, `backup_daily` (+ optional `backup_hourly`), `healthcheck_self`, `tor_health`, `cluster_heartbeat`
- ALWAYS respect country blocking mode exclusivity — if both `deny_countries` and `allow_countries` are set, `allow_countries` (allowlist mode) wins; allowlisted IPs and private/RFC1918 IPs always bypass country blocking
- ALWAYS prefix metrics with `{project_name}_`, use snake_case, base units (seconds/bytes), and `_total` suffix on counters; keep labels low-cardinality
- ALWAYS expose the required metric families: app info/uptime, HTTP (requests/duration/size/active), DB (if used), auth attempts/sessions — plus category-specific metrics (cache, scheduler, system, runtime, cluster, Tor) when those subsystems are enabled
- ALWAYS verify every backup immediately after creation (file exists, size>0, checksum, decrypt test if encrypted, manifest parse, content extraction, DB integrity) before applying retention/pruning
- ALWAYS require Primary Admin re-authentication via one-time setup token after restoring to a new server (existing password/settings preserved, just re-verified); additional local/OIDC/LDAP/SAML admins can log in immediately
- ALWAYS verify SHA256 checksum before installing a downloaded update binary, and use the platform-specific replace strategy (atomic rename on Unix; rename-to-`.old` + `MOVEFILE_DELAY_UNTIL_REBOOT` on Windows)
- ALWAYS treat update channels as cumulative — `beta` = beta + stable, `daily` = daily + beta + stable — and never propose anything older than or equal to the running version
- ALWAYS gate the scheduled `update_check` task by `defer_days` (release eligible only once `now - published_at >= defer_days`), but let manual `--update check`/`--update yes` bypass the defer window entirely

## Key Rules Summary

### Email & Notifications (PART 18)

- SMTP config lives at `server.notifications.email.smtp.*`; `SMTP_*` env vars override the config file (useful for containers)
- No SMTP → password reset hidden, email verification skipped (auto-verified), no login/welcome/security alerts sent or attempted
- ~20 templates ship with sane defaults (`welcome`, `password_reset`, `email_verify`, `login_alert`, `security_alert`, `mfa_reminder`, `2fa_enabled/disabled`, `password_changed`, `token_regenerated`, `backup_complete/failed`, `ssl_expiring/renewed/renewal_failed`, `scheduler_error`, `startup/shutdown`, `update_available/installed`, `breach_notification`, `breach_admin_alert`, `test`); templates use `{variable}` syntax with a `Subject: ... --- body` format
- Breach notification content auto-adjusts per enabled compliance standard (GDPR, HIPAA, CCPA, LGPD, PIPEDA, APPI, PDPA)
- Two notification systems: WebUI (toast/banner/notification center, always available) and email (only when SMTP works). Decision matrix: critical/security/permanent-record events get both; routine confirmations get WebUI only; routine successes get nothing
- WebUI notifications persist in `admin_notifications`/`user_notifications` tables, 30-day retention, 100 max per user/admin, real-time via WebSocket
- Security notification categories (login alerts, password/2FA changes) cannot be disabled by admins or users

### Scheduler (PART 19)

- Built-in scheduler only — never cron/systemd timers/Task Scheduler/launchd/K8s CronJob/cloud schedulers, no exceptions, even for "I already have cron" or "complex schedules" (built-in supports full cron syntax)
- State persists in `server.db` (task_id, schedule, last_run, last_status, next_run, run_count, fail_count, locked_by/locked_at for cluster)
- Cluster mode: global tasks (ssl_renewal, geoip_update, blocklist_update, cve_update, backup_daily, update_check) run on one elected node with a 5-minute lock timeout; local tasks (session_cleanup, token_cleanup, healthcheck_self, cluster_heartbeat) run on every node
- Retry policy: 3 max retries, 5m initial delay, exponential backoff (5m/10m/20m)
- Shutdown waits up to 30s for running tasks, then force-releases locks and marks interrupted tasks for retry on next start

### GeoIP (PART 20)

- Uses sapics/ip-location-db databases (no API key), downloaded to `{data_dir}/security/geoip`, updated weekly (Sunday 03:00) via scheduler
- Three lookup categories toggleable independently: ASN, Country, City (city ships separate IPv4/IPv6 MMDB files, no combined file)
- Country blocking: `deny_countries` (blocklist) vs `allow_countries` (allowlist) — mutually exclusive by convention, `allow_countries` wins if both set; missing `country.mmdb` skips blocking with a warning, not a hard failure

### Metrics (PART 21)

- Prometheus text exposition format at `/metrics` (configurable path), internal-only access, optional bearer token auth
- Required metric families: `app_info`/`app_uptime_seconds`/`app_start_timestamp`, HTTP request/duration/size/active, DB queries/duration/connections/errors (if DB used), auth attempts/sessions
- Optional families gated by feature flags: cache, scheduler, system (`include_system`), runtime (`include_runtime`), business (multi-user), cluster, Tor, rate limiting
- Path normalization required for HTTP labels — replace UUIDs/numeric IDs with `:id` to keep cardinality bounded

### Backup & Restore (PART 22)

- `{project_name} --maintenance backup [filename]`; always includes `server.yml`, `server.db`, `users.db`; templates/themes if present; SSL/data optional via flags
- Encryption optional unless compliance mode is enabled (then mandatory); AES-256-GCM with Argon2id key derivation from password; password is never stored
- Retention priority: yearly > monthly > weekly > daily (max_backups), oldest deleted first within each tier; `max_total_size` (percent or absolute) is a hard cap that overrides count limits after count-based pruning
- Default footprint is 2 files (yesterday's full + daily incremental); every backup file matching the app's naming convention is subject to pruning — nothing is exempt
- Cluster mode: every node keeps its own local backups with the same retention/encryption/schedule (staggered 5min/node); shared NFS/S3 storage is optional, not required
- Restore requires PART 5 authorization; encrypted backups need a separate backup password (not admin auth); restoring to a new server forces Primary Admin re-authentication via one-time setup token, but preserves existing credentials
- `{project_name} --maintenance setup` is the sole recovery path when an admin has lost password + API token + recovery keys — clears only admin credentials, generates a new one-time setup token, leaves all user data/accounts/config untouched

### Update Command (PART 23)

- `--update [yes|check|branch {stable|beta|daily}]`; `--maintenance update` is an alias for `--update yes`; exit 0 = success/no update, exit 1 = error; GitHub API 404 means already current
- Channels are cumulative and always pick the newest eligible release — `beta` includes stable, `daily` includes beta+stable
- `defer_days` (0-365) gates only the scheduled `update_check` task (both notify and auto-install); manual `--update check`/`yes` always bypass it and see the true latest
- `update_check` runs daily at 06:00; `auto_install: false` (default) notifies only, never touches the binary; `true` runs the full update flow, with cluster rollout staggered node-by-node
- Self-update is platform-specific: Unix does an atomic rename over the running binary; Windows renames current to `.old` (scheduled for delete-on-reboot) then moves the new binary into place, since it can't touch a running executable directly
- Checksum verification (SHA256, from the release's `.sha256` asset) is mandatory before replacing the binary — refuse the update if the checksum asset is missing
- "Update available" is Server-Admin-only surfacing (banner + notification center + optional email); never shown on public pages (Tier 3 info, PART 11); PWA "new version" banner is the one exception since it reveals no server version

For complete details, see AI.md PART 18-23
