# Features Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Skip the built-in scheduler (PART 19) — there is NO external cron, NO systemd timers, NO Task Scheduler
- ❌ Skip GeoIP, Metrics, Email, Backup, Update — every project has them
- ❌ Hardcode SMTP credentials — use auto-detection from common providers + admin override
- ❌ Send email via plaintext SMTP without TLS (auto / starttls / tls)
- ❌ Embed GeoIP / blocklist / CVE / Trivy DBs in the binary — download on first run
- ❌ Allow both `deny_countries` AND `allow_countries` (mutually exclusive)
- ❌ Expose `/metrics` publicly (PART 21) — internal only, optional bearer token
- ❌ Skip metric labels for project-specific operations (every backup, every scheduled task, every API endpoint MUST have a metric)
- ❌ Compress backups without verification — backups MUST round-trip restore-ably
- ❌ Skip `app_secrets` table or PGP private key in backups — restore breaks without them
- ❌ Skip backup encryption when `server.backup.encryption.enabled: true`
- ❌ Skip cluster backup awareness — primary node only does the backup; secondaries skip
- ❌ Update the binary without verifying SHA-256 over TLS (PART 23 canonical pattern)
- ❌ Skip mode rollback after a failed update

## CRITICAL - ALWAYS DO
- ✅ Built-in scheduler runs ALL background tasks: GeoIP / blocklist / CVE update, log rotation, session cleanup, backup, SSL renewal, health check, Tor health
- ✅ Each task has cron-style schedule, retry-on-fail flag, retry delay
- ✅ Task ownership: in cluster, primary node executes; secondaries skip
- ✅ SMTP auto-detection from FROM email domain (Gmail / Outlook / Fastmail / SendGrid / Mailgun / common providers)
- ✅ SMTP env vars (`SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM_NAME`, `SMTP_FROM_EMAIL`, `SMTP_TLS`) override auto-detection
- ✅ TLS modes: `auto` (default) / `starttls` / `tls` / `none`
- ✅ Email templates per locale; subject + plain + HTML body
- ✅ GeoIP DB: download from ip-location-db on first run / weekly via scheduler
- ✅ Country blocking: deny-list OR allow-list (mutually exclusive); admin UI manages both
- ✅ Metrics: Prometheus exposition at `/metrics`, internal only; cardinality-bounded labels (no per-IP, no per-user-ID)
- ✅ Metric naming: `weather_<category>_<metric>` (e.g., `weather_http_requests_total`, `weather_scheduler_task_duration_seconds`)
- ✅ Backup default daily at 2:00 AM, retention 4 (configurable)
- ✅ Backup format: `.tar.gz` containing DB dump + config + Tor keys + `app_secrets` + PGP keys
- ✅ Compliance encryption (`server.backup.encryption.{enabled,public_key}`): tar → encrypt → store
- ✅ Update: check `release.txt` URL → download → verify SHA-256 over TLS → atomic replace → restart
- ✅ Update branches: `stable` (default), `beta`, `daily`
- ✅ Update verifyChecksum() canonical pattern (PART 23) — same shape used by client and agent
- ✅ Notification system: in-app + email + webhook; per-user opt-in/out (requires PART 34)

## SCHEDULER TASKS (default schedule from PART 5)
| Task | Schedule | Purpose |
|------|----------|---------|
| `geoip_update` | `0 3 * * 0` (weekly Sunday 3am) | Refresh GeoIP DB |
| `blocklist_update` | `0 4 * * *` (daily 4am) | Refresh IP/domain blocklists |
| `cve_update` | `0 5 * * *` (daily 5am) | Refresh CVE feed |
| `log_rotation` | `0 0 * * *` (daily midnight) | Rotate logs (max_age 30d, max_size 100MB) |
| `session_cleanup` | `@hourly` | Purge expired sessions |
| `backup` | `0 2 * * *` (daily 2am) | Run backup (retention 4) |
| `ssl_renewal` | `0 3 * * *` (daily 3am) | Renew certs (renew_before 7d) |
| `health_check` | `*/5 * * * *` (every 5 min) | Internal health probe |
| `tor_health` | `*/10 * * * *` (every 10 min) | Tor connectivity check |

---
For complete details, see AI.md PART 18, 19, 20, 21, 22, 23
