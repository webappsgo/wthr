# Configuration

Weather generates its configuration at runtime with sane defaults and keeps the live configuration in OS-specific directories, not in the repository.

## Configuration Files

| File | Location | Purpose |
|------|----------|---------|
| `server.yml` | `{config_dir}/server.yml` | Server configuration |
| `cli.yml` | `~/.config/casapps/wthr/cli.yml` | CLI client configuration |

Use the admin panel for routine changes:

- Server settings: `https://wthr.top/admin`
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
    username: weather@example.com
```

## Paths

Weather separates configuration, data, and logs:

| Type | Location |
|------|----------|
| Config | `{config_dir}/` |
| Data | `{data_dir}/` |
| Logs | `{log_dir}/` |

Docker deployments typically mount:

```bash
-v ./rootfs/config:/config:z
-v ./rootfs/data:/data:z
```

## Language and Theme

Language selection follows AI.md PART 31:

1. `?lang=` query parameter
2. `lang` cookie
3. `Accept-Language`
4. English fallback

Theme and language preferences can be managed without JavaScript and persist through cookies/user settings.

## Validation

Use the existing project commands for validation:

```bash
make i18n-validate
make test
```

## Next Steps

- [Installation](installation.md)
- [API Reference](api.md)
- [Admin Panel](admin.md)
- [CLI Reference](cli.md)
