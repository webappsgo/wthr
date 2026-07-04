# Features Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use external cron (cron daemon, OS scheduler) → internal scheduler only (PART 19)
- Use GeoIP as the sole auth gate → risk signal only
- Use `geoip2-golang` package → use `maxminddb-golang` (geoip2 rejects non-MaxMind type strings)
- Store backups unencrypted → encrypt with AES-256
- Send email from goroutine without queue → use internal email queue
- Skip the update pre-flight check → verify signature before applying

## CRITICAL - ALWAYS DO

- Email: SMTP with TLS; queue in DB; retry on failure; HTML + plain text
- Scheduler: built-in, cron-expression jobs, stored in `server.db`
- GeoIP: ip-location-db MMDB files in `{data_dir}/security/geoip/`; daily refresh via scheduler
- Metrics: Prometheus-format at `GET /metrics`; store in `server.db`; retention configurable
- Backup: scheduled + manual; tar.gz + AES-256; store in `{data_dir}/backups/{project_name}/`
- Update: `--update` flag; downloads from GitHub releases; verifies checksum; replaces binary atomically

## Email (PART 18)

Templates stored in DB (admin-editable); variables: `{{.Username}}`, `{{.URL}}`, etc.

Required email events: registration verification, password reset, 2FA codes, invite links, admin alerts.

## Scheduler (PART 19)

Jobs stored in `server.db.scheduler_jobs`. Built with internal ticker, not `time.AfterFunc`.

Default jobs: GeoIP refresh (daily 3am), metrics cleanup (weekly), backup (configurable), update check (weekly).

## GeoIP (PART 20)

```go
// Use maxminddb-golang, NOT geoip2-golang
import "github.com/oschwald/maxminddb-golang"

// MMDB type strings (from ip-location-db package names):
// "asn ipv4/ipv6/ipvAll", "city ipv4/ipv6", "country ipv4/ipv6/ipvAll"
```

Store in `{data_dir}/security/geoip/`. Refresh daily via scheduler.

## Reference

For complete details, see AI.md PART 18, 19, 20, 21, 22, 23
