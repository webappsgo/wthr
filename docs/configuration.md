# Configuration

wthr generates its configuration at runtime with sane defaults and keeps the live configuration in OS-specific directories, not in the repository.

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| `server.yml` | `{config_dir}/server.yml` | Server configuration |
| `yaml.d/*.yml`, `yaml.d/*.yaml` | `{config_dir}/yaml.d/` | Optional drop-in files that override `server.yml` |
| `secrets.yml` | `{config_dir}/secrets.yml` | Owner-only secret values, including the encrypted scheduled-backup password |
| `cli.yml` | `~/.config/webappsgo/wthr/cli.yml` | CLI client configuration |

Use the admin panel for routine changes:

- Server settings: `https://wthr.top/server/administration`
- User settings: `https://wthr.top/users/settings`

## Precedence

Configuration is applied in this order:

1. Command-line flags
2. Environment variables
3. Config files: `yaml.d/*` drop-ins override `server.yml`; `secrets.yml` supplies secret keys last
4. Embedded defaults

## Important Rules

- `server.yml` is generated on first run
- Do not keep runtime config files in the repository
- Do not store plaintext passwords or tokens in `server.yml`
- Single-instance settings are owned by `server.yml` or the canonical area file in `yaml.d/`; `secrets.yml` stores credentials with mode `0600`
- Remote PostgreSQL/MySQL mode shares settings through `srv_config` and `srv_config_meta`; each node keeps synced area-file copies and local `server.yml`/`secrets.yml`
- Most changes apply live without restarting the service

## Drop-in Config Directory (`yaml.d`)

A large configuration may be split into files in `{config_dir}/yaml.d/`, next to `server.yml`. The directory is optional; `server.yml` alone is always valid.

| Rule | Behavior |
|------|----------|
| Files | `*.yml` and `*.yaml`, regular files only. Hidden files, backups (`*.bak`, `*~`, `*.orig`), other extensions and subdirectories are ignored. A symlink is followed only when its target stays inside `yaml.d/` |
| Order | Built-in defaults, then `server.yml`, then every drop-in in byte-wise filename order (`10-tls.yml`, then `20-mail.yml`), then `secrets.yml` for secret keys |
| Merge | Maps merge key by key and recursively. A scalar or a list in a later file replaces the earlier value whole; lists are never appended. An explicit `null` is an error, not a reset |
| Keys | The same dotted paths as `server.yml`; any section may live in any file |
| Validation | The merged result is validated like any config. An error names the file, line and key; a duplicate key inside one file is an error |
| Reload | `server.yml` and `yaml.d/` are watched. A change, add or remove re-merges everything. If the merged result is invalid, the last good configuration stays active and the error is logged |
| Generated values | The saved random port and `server.token` are written only to `server.yml`. If a drop-in sets the same key, the drop-in wins and the startup log says which file did |
| Permissions | A drop-in that is group or world writable is refused. Use `0600` for files that hold secrets |
| Backup | `{config_dir}/yaml.d/` is included in backups whenever it exists |

The admin panel writes core server settings to `server.yml` and stores the scheduled-backup password encrypted in `secrets.yml`. A setting that a drop-in file sets is owned by that file (`set by yaml.d/20-mail.yml`); edit the drop-in instead. The startup log lists each loaded drop-in in order at info level and, with debug enabled, names the file that won for every overridden key. Secret values are never logged.

## Branding and SEO

Branding, SEO, accent color, and operator-authored page content are stored in
`{config_dir}/yaml.d/branding.yml`. The admin UI writes this area file atomically. Existing
branding values found in older `server.yml` files are migrated into the area file. Uploaded images
are stored in the private `{config_dir}/assets/` directory; remote image URLs are fetched through
the same size, type, and image validation pipeline.

Raster uploads and remote images produce responsive logo variants (200px and 50px wide), favicon
variants (16, 32, 48, 180, 192, and 512px, including the Apple touch icon), and a 1200×630 Open
Graph image variant. The header advertises the responsive logo sizes; the page head uses the
generated favicon and social preview images. The accent stylesheet is served at
`/static/css/accent.css`.

## Environment Variables

Runtime environment variables are re-read on every start. `DATABASE_URL` and
`DATABASE_DRIVER` are init-only: they seed the saved database connection on
first run, then the saved URL or fields take precedence on every later start.
URLs containing credentials are stored in `secrets.yml`; the full URL is never
shown in logs or admin responses.

### Runtime

| Variable | Purpose |
|----------|---------|
| `DOMAIN` | FQDN used for displayed/embedded URLs |
| `MODE` / `APP_MODE` / `ENVIRONMENT` | Application mode (`production` or `development`) |
| `DEBUG` | Enable debug diagnostics (verbosity only, never bypasses auth). Every `/debug/*` endpoint, pprof and expvar included, also requires the operator token `server.token` as `Authorization: Bearer`; when unset a random token is printed once on the console at startup |
| `PORT` | Listen port: `8080` (HTTP), `443` (HTTPS only) or `80,443` (HTTP then HTTPS) |
| `LISTEN` / `SERVER_ADDRESS` | Listen address (legacy fallback for `PORT`) |
| `REVERSE_PROXY` | Trust reverse-proxy forwarding headers |
| `DAEMON` | Run in background/daemon mode |
| `NO_COLOR` | Disable ANSI colors and emoji in output |
| `TERM` | Terminal type (`dumb` forces plain CLI output) |

### First-run database selection

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | Seed the first-run SQLite, PostgreSQL, or MySQL connection URL; credentials are stored in `secrets.yml` |
| `DATABASE_DRIVER` | Select `sqlite`, `postgres`, or `mysql` when no URL is supplied |
| `CLUSTER_JOIN_URL` | Versioned HTTPS `/api/{api_version}/server/{admin_path}/nodes/join` endpoint used for first-start enrollment |
| `CLUSTER_JOIN_TOKEN` | One-time 15-minute token generated by an existing cluster node; consumed only during enrollment |

### Cache

| Variable | Purpose |
|----------|---------|
| `CACHE_ENABLED` | Enable the external cache backend |
| `CACHE_HOST` | Cache (Valkey/Redis) host |
| `CACHE_PORT` | Cache port |
| `CACHE_PASSWORD` | Cache password |
| `CACHE_DB` | Cache logical DB number |

### SMTP

| Variable | Purpose |
|----------|---------|
| `SMTP_HOST` | SMTP server host |
| `SMTP_PORT` | SMTP server port |
| `SMTP_USERNAME` | SMTP auth username |
| `SMTP_PASSWORD` | SMTP auth password |
| `SMTP_FROM_ADDRESS` | Envelope/from address |
| `SMTP_FROM_NAME` | Display name for outgoing mail |

### Paths (applied on every startup; flag, then variable, then platform default)

| Variable | Purpose |
|----------|---------|
| `CONFIG_DIR` | Override config directory |
| `DATA_DIR` | Override data directory |
| `LOG_DIR` | Override log directory |
| `CACHE_DIR` | Override cache directory |
| `TEMP_DIR` | Override temp directory |
| `BACKUP_DIR` | Override backup directory (default: `{data_dir}/backups`) |

## Common Settings

### Server

```yaml
server:
  mode: production
  address: 0.0.0.0
  # Port is chosen on first run from the configured/default range
  port: 64580
```

### Database

SQLite is the single-instance default and uses `{data_dir}/db/server.db` plus
`{data_dir}/db/users.db`. Cluster Mode uses one shared PostgreSQL or MySQL
database. Configure it with a URL or individual fields:

```yaml
server:
  database:
    driver: postgres
    url: postgres://db.example.com:5432/wthr?sslmode=require
```

`DATABASE_URL` seeds the URL on first run. A saved URL takes precedence over
the separate `host`, `port`, `name`, `username`, `password`, and `sslmode`
fields. Include authentication in the URL when the database requires it.
Cluster settings are stored in `srv_config`/`srv_config_meta` and
written to each node's `yaml.d/` area files. Nodes poll shared changes every
five seconds and reconcile hand-edited area files every five minutes. Uploaded
files in `assets/` are mirrored as validated, size-limited `assets.*` entries
with deletion tombstones; remotely fetched image caches stay local. `server.yml`
and `secrets.yml` remain node-local. PostgreSQL and MySQL tables use `srv_` and
`usr_` prefixes; SQLite retains the split local database files.

The first node enters cluster mode from the admin **Nodes → Add Node** wizard. It supplies a
shared PostgreSQL/MySQL URL and Valkey/Redis URL; the wizard tests the cache, migrates local
database data, and creates the first one-time join token. Restart that first node to activate the
saved shared connections. On a joining node, set `CLUSTER_JOIN_URL` and `CLUSTER_JOIN_TOKEN`
before its first start. The endpoint requires HTTPS except for loopback development. Bootstrap
credentials and connection TLS files are authenticated-encrypted with the token; the node then
replaces the supplied shared secrets and writes private TLS files before opening databases.
Environment connection values do not override the cluster values received by a joiner.

To leave, use **Nodes → Remove This Node** on the node being removed, or its authenticated
`POST /api/{api_version}/server/{admin_path}/nodes/leave` endpoint. This removes only that node's
membership, clears its remote database/cache settings, and removes stale local SQLite files so
the next restart creates fresh standalone databases. Other nodes and their shared database are
left intact.

### Weather Data

```yaml
weather:
  cache_ttl: 15m

severe_weather:
  poll_interval: 5m

earthquakes:
  min_magnitude: 2.5

hurricanes:
  update_interval: 15m
```

### GeoIP

```yaml
server:
  geoip:
    enabled: true
    deny_countries: []
    allow_countries: []
    presets: {}
```

These controls belong in `{config_dir}/yaml.d/security.yml` and are also available at
`/server/{admin_path}/network/geoip`. GeoIP database paths and weekly updates are managed by the
built-in scheduler; they are not configuration keys.

### Trusted Proxies

```yaml
server:
  trusted_proxies:
    additional: []
    client_ip_header: auto
```

Private ranges, link-local addresses and the same /24 as the listen address are always trusted. `additional` adds public upstream proxies (IP, CIDR or DNS name). Proxy headers are read only when the TCP peer is trusted. The client IP is found by walking the forwarded chain from the right: the first hop that is not a trusted proxy is the client. `client_ip_header` is `auto` (first present of `X-Forwarded-For`, `Forwarded`, `CF-Connecting-IP`, `True-Client-IP`, `X-Real-IP`, `X-Client-IP`) or one header name that is the only header read. Pin it to the header your front proxy sets or appends to.

### Notifications

```yaml
notifications:
  websocket:
    enabled: true

email:
  enabled: false
  smtp:
    host: smtp.example.com
    port: 587
    username: wthr@example.com
```

### Metrics

The metrics endpoint is internal only and every service is gated by its own bearer token. An empty
token disables that service. See [Integrations](integrations.md) for the full route list and a
Prometheus scrape example.

```yaml
server:
  metrics:
    enabled: true
    root:
      enabled: true
    auth:
      allow_unauthenticated: false
      tokens:
        prometheus: ""
        grafana: ""
        loki: ""
    include_system: true
    include_runtime: true
    loki:
      max_entries: 1000
      max_age: 1h
```

### Scheduler

Editable task settings are available on the **Scheduler** admin page and apply without a restart. A
task that the file omits uses its built-in default; a critical task is always enabled. Durations
accept `s`, `m`, `h`, `d` and `w`.

```yaml
server:
  scheduler:
    timezone: America/New_York
    catch_up_window: 1h
    tasks:
      geoip_update:
        enabled: true
        schedule: "0 3 * * 0"
      blocklist_update:
        enabled: true
        schedule: "0 4 * * *"
        retry_on_fail: true
        retry_delay: 1h
      log_rotation:
        enabled: true
        schedule: "0 0 * * *"
        max_age: 30d
        max_size: 100MB
        compress: true
      backup_daily:
        enabled: true
        schedule: "0 2 * * *"
      backup_hourly:
        enabled: false
        schedule: "@hourly"
      tor_health:
        enabled: true
        schedule: "@every 10m"
```

Scheduler state is stored in `server.db`. `scheduler_tasks` uses `id` and `name`, with `last_run`,
`next_run`, and `locked_at` stored as Unix-second integers. `scheduler_history` stores `task_id`,
`started_at`, and nullable `finished_at` as Unix-second integers; its `duration_ms` is nullable.
Older scheduler tables are migrated during database initialization.

### CORS and CSRF

Both are editable on the **Web** admin page (CORS) and apply without a restart.

```yaml
web:
  cors: "*"
  csrf:
    enabled: true
    token_length: 32
    cookie_name: csrf_token
    header_name: X-CSRF-Token
    secure: auto
    exempt_paths: []
```

`cors` is `*` (unset: the `DOMAIN` hostnames and trusted-proxy hosts are used when present), an empty
string (CORS off) or a comma-separated list of origins such as `https://app.example.com`. `csrf`
applies to the frontend only; `secure` is `auto`, `true` or `false`, and `exempt_paths` lists
frontend paths that skip the check. An invalid value is replaced by its default with a warning.

## HTTPS and Certificates

| Port setting | Result |
|--------------|--------|
| `8080` | HTTP only |
| `443` | HTTPS only |
| `80,443` / `8080,8443` | HTTP then HTTPS; the HTTP port redirects visitors to HTTPS |
| `8080` with `ssl.enabled: true` | HTTPS on that port |

The HTTP port in dual-port mode never redirects the Let's Encrypt HTTP-01 challenge, Tor and
I2P requests (an onion address has no HTTPS endpoint) or requests addressed to `localhost`
(the container health check). With HTTPS only, Tor and I2P reach the site through a
loopback-only plain listener.

On start the server looks for a certificate for its host name in this order and uses the first
valid one (readable, not expired, naming the host):

| Order | Location | Renewed by |
|-------|----------|------------|
| 1 | `/etc/letsencrypt/live/domain/` | certbot (the app never renews it) |
| 2 | `/etc/letsencrypt/live/{fqdn}/` | certbot |
| 3 | `{config_dir}/ssl/letsencrypt/{fqdn}/` (`fullchain.pem`, `privkey.pem`) | the app, 7 days before expiry |
| 4 | `{config_dir}/ssl/local/{fqdn}/` (`cert.pem`, `key.pem`) | you (never renewed) |

With none of them the server starts on a self-signed certificate in `ssl/local/{fqdn}/`, so a
first run works with no configuration. `ssl.cert` and `ssl.key` (absolute paths) override the
lookup. With `ssl.letsencrypt.enabled: true` and a public domain name, the server then requests
a certificate with the configured `challenge` (`http-01`, `tls-alpn-01` or `dns-01`) and serves
it without a restart. The `ssl_renewal` task checks daily
at 03:00 and renews an app-managed certificate 7 days before it expires. `ssl.min_version` is
`TLS1.2` or `TLS1.3`.

### DNS-01 provider

`ssl.letsencrypt.dns_provider` is a lego provider name (for example `cloudflare`) and
`ssl.letsencrypt.dns_credentials` is one flat map of that provider's lego variable names to values:

```yaml
ssl:
  letsencrypt:
    challenge: dns-01
    dns_provider: cloudflare
    dns_credentials:
      CF_DNS_API_TOKEN: "your-token"
```

A plaintext value is accepted at load and rewritten in `server.yml` as `enc:v1:...`
(AES-256-GCM under `server.security.encryption_key`, file mode `0600`). Values are never logged,
never returned by the API and shown as `********` in the admin SSL page. The provider is built
at startup and before every certificate request without touching DNS; a missing or invalid
variable fails the request and the error names the variable only. `dns_validated_at` is stored
in `server.db` after a certificate is issued or renewed through the provider and is cleared when
the provider or a credential changes. Rotating the encryption key re-encrypts the credentials.
With no provider set, the TXT records are listed on the admin SSL page for you to publish.

## Paths

wthr separates configuration, data, and logs:

| Type | Location |
|------|----------|
| Config | `{config_dir}/` |
| Data | `{data_dir}/` |
| Logs | `{log_dir}/` |

Docker deployments typically mount:

```bash
-v ./volumes/config:/config:z
-v ./volumes/data:/data:z
```

## Pages, Contact and Tracking

```yaml
server:
  admin_path: administration
  contact:
    # Shown on /server/dpo; empty values are never invented
    dpo:
      name: ""
      email: ""
  pages:
    help:
      # Markdown shown as the project section of /server/help/mcp
      mcp: ""
    contact:
      # simple (built-in, default), recaptcha, hcaptcha
      captcha: simple
      captcha_site_key: ""
      captcha_secret_key: ""
  tracking:
    # google, matomo, piwik, owa, fathom, plausible, umami, simple, cloudflare, or empty
    type: ""
    id: ""
    url: ""
  mcp:
    enabled: true
  i18n:
    default_language: en
    available_languages: [en, es, fr, de, it, ar, zh, ja]
```

- `admin_path` is one segment (`[a-z0-9-]`, 2-32 characters). An invalid value reverts to `administration`.
- `/server/dpo` shows `contact.dpo.email`, then `contact.general.email`, and never the admin address.
- The contact form needs a captcha on every submission. `recaptcha` and `hcaptcha` need both keys; without them the built-in challenge is used, and requests through Tor or I2P always use the built-in challenge.
- Analytics run only after the visitor opts in, never over Tor or I2P, and never inline: the page loads `/server/tracking.js`. Only the configured provider's hosts are added to the Content-Security-Policy.
- `available_languages` may be narrowed but always keeps `en`, `es`, `fr`, `de`, `it`, `ar`.

### Security reports

Editable in the admin panel (Settings and Security sections); stored with the other live settings.

| Setting | Default | Purpose |
|---------|---------|---------|
| `web.security.report_url` | empty | Primary `Contact` line of security.txt, usually the repository's private vulnerability reporting URL |
| `web.security.keyservers` | `https://keys.openpgp.org` | Comma-separated keyservers the project public key is published to |
| `server.security.reports.retention_days` | `365` | Days after closing before a closed security report is deleted; open reports are never deleted |
| `server.security.encryption_key` | generated on first run | Base64 32-byte AES-256-GCM key; encrypts report bodies when no PGP keypair exists |

## Language and Theme

Language selection follows AI.md PART 31:

1. `?lang=` query parameter
2. `lang` cookie
3. `Accept-Language`
4. English fallback

Theme and language preferences can be managed without JavaScript and persist through cookies/user settings.

## Validation

Validation runs as part of the test gate: `TestKeyConsistency` and `TestLocalesFS` fail when a language file's keys differ from `en.json`.

```bash
make test
```

## Next Steps

- [Installation](installation.md)
- [API Reference](api.md)
- [Admin Panel](admin.md)
- [CLI Reference](cli.md)
