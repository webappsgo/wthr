# Configuration

wthr generates its configuration at runtime with sane defaults and keeps the live configuration in OS-specific directories, not in the repository.

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| `server.yml` | `{config_dir}/server.yml` | Server configuration |
| `cli.yml` | `~/.config/webappsgo/wthr/cli.yml` | CLI client configuration |

Use the admin panel for routine changes:

- Server settings: `https://wthr.top/server/administration`
- User settings: `https://wthr.top/users/settings`

## Precedence

Configuration is applied in this order:

1. Command-line flags
2. Environment variables
3. Generated config files
4. Embedded defaults

## Important Rules

- `server.yml` is generated on first run
- Do not keep runtime config files in the repository
- Do not store plaintext passwords or tokens in `server.yml`
- All server settings must also be editable from the admin WebUI
- Most changes apply live without restarting the service

## Environment Variables

Environment variables override the generated config file (see Precedence
above). Path and bootstrap variables marked *init-only* are read once on first
run to seed the config, then ignored on later restarts; runtime variables are
re-read on every start.

### Runtime

| Variable | Purpose |
|----------|---------|
| `DOMAIN` | FQDN used for displayed/embedded URLs |
| `MODE` / `APP_MODE` / `ENVIRONMENT` | Application mode (`production` or `development`) |
| `DEBUG` | Enable debug diagnostics (verbosity only, never bypasses auth). Every `/debug/*` endpoint, pprof and expvar included, also requires the operator token `server.token` as `Authorization: Bearer`; when unset a random token is printed once on the console at startup |
| `PORT` | Listen port |
| `LISTEN` / `SERVER_ADDRESS` | Listen address (legacy fallback for `PORT`) |
| `REVERSE_PROXY` | Trust reverse-proxy forwarding headers |
| `TLS_ENABLED` | Enable HTTPS/TLS on the listener |
| `DAEMON` | Run in background/daemon mode |
| `NO_COLOR` | Disable ANSI colors and emoji in output |
| `TERM` | Terminal type (`dumb` forces plain CLI output) |

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
geoip:
  enabled: true
  update_interval: 168h
```

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
