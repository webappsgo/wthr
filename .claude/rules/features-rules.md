# Features Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use OS cron or systemd timers — built-in scheduler only
- ❌ Block the main thread on scheduled tasks
- ❌ Hard-delete backups without retention policy
- ❌ Store SMTP password in plaintext
- ❌ Skip email queue — always async, never block request
- ❌ Hardcode GeoIP database — update via scheduler
- ❌ Expose raw metrics without scrape authentication
- ❌ Skip update verification (checksum) when auto-updating
- ❌ Auto-update in production without explicit admin enable

## CRITICAL - ALWAYS DO
- ✅ Built-in scheduler (`robfig/cron` or ticker loop) for ALL scheduled work
- ✅ Email: async queue, retry on failure, configurable SMTP
- ✅ GeoIP: embedded sapics/ip-location-db, monthly scheduler update
- ✅ Metrics: Prometheus-compatible at `/metrics` (scrape auth configurable)
- ✅ Backup: scheduled daily backup to `{data_dir}/backups/`, configurable retention
- ✅ Restore: admin panel UI for backup restore
- ✅ Update: `GET /server/version` shows current vs latest; admin can trigger update
- ✅ Notifications: WebSocket push for real-time alerts + email for subscriptions

## SCHEDULER JOBS (weather-specific)
| Job | Interval | Purpose |
|-----|----------|---------|
| GeoIP update | Monthly | Download sapics/ip-location-db |
| Severe weather poll | 5 min | NOAA, Environment Canada, UK Met, BOM, JMA, CONAGUA |
| Earthquake poll | 1 min | USGS |
| Hurricane update | 15 min | NOAA NHC |
| Backup | Daily 3am | Full backup to data dir |
| Alert expiry | 5 min | Clean up expired alerts |

## EMAIL FEATURES
- SMTP with TLS/STARTTLS
- Queue: async worker, configurable retry count and interval
- Templates: HTML + plain text fallback
- Events that trigger email: alert subscriptions, admin actions, account changes

## BACKUP SYSTEM
- Format: compressed archive (`.tar.gz`)
- Contents: all SQLite DBs + config file
- Location: `{data_dir}/backups/weather-{timestamp}.tar.gz`
- Retention: configurable (default: keep last 30)
- Admin panel: list, download, delete, restore

## UPDATE COMMAND
- `weather update` CLI command checks GitHub releases
- Compares current version with latest tag
- Downloads, verifies checksum, replaces binary, restarts service
- Auto-update via scheduler: disabled by default, opt-in

---
For complete details, see AI.md PART 18-23
