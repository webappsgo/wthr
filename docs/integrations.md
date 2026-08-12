# Integrations

## Weather Data Sources

All weather data sources used by wthr are free and public — no API keys are required.

| Source | Data | Endpoint |
|--------|------|----------|
| [Open-Meteo](https://open-meteo.com/) | Current conditions, 16-day forecasts, hourly data | `https://api.open-meteo.com/v1/forecast` |
| [NOAA/NWS](https://www.weather.gov/documentation/services-web-api) | US severe weather alerts | `https://api.weather.gov/alerts/active` |
| [NOAA NHC](https://www.nhc.noaa.gov/data/) | Hurricane/tropical storm tracking | `https://www.nhc.noaa.gov/CurrentStorms.json` |
| [Environment Canada](https://weather.gc.ca/warnings/index_e.html) | Canadian weather alerts | `https://weather.gc.ca/warnings/` (Atom feed) |
| [UK Met Office](https://www.metoffice.gov.uk/services/data/datapoint) | UK weather warnings | Met Office DataPoint API |
| [Australia BOM](http://www.bom.gov.au/catalogue/data-feeds.shtml) | Australian weather warnings | BOM data feeds |
| [Japan JMA](https://www.jma.go.jp/bosai/) | Japan weather alerts | JMA public APIs |
| [Mexico CONAGUA](https://smn.conagua.gob.mx/) | Mexico weather warnings | SMN data feeds |
| [USGS Earthquake Hazards](https://earthquake.usgs.gov/earthquakes/feed/) | Real-time seismic data | GeoJSON feeds |

Data is fetched on a configurable schedule (default: weather alerts every 15 minutes, earthquake
data every 1 minute). No API keys, accounts, or registration are required for any of these sources.

---

## GeoIP

wthr uses the [sapics/ip-location-db](https://github.com/sapics/ip-location-db) GeoLite2-compatible
databases for automatic location detection from IP address. The databases are downloaded on first
run and updated weekly (Sunday 03:00) via the built-in scheduler — never embedded in the binary.

Databases used:

- `geolite2-city-ipv4` — IPv4 city-level geolocation
- `geolite2-city-ipv6` — IPv6 city-level geolocation
- `geo-whois-asn-country` — ASN/country lookup
- `asn` — Autonomous system number lookup

GeoIP is used only for automatic location detection on the weather page. It is not used as an access
control mechanism. Users may specify a location explicitly at any time, overriding the GeoIP result.

---

## API Access

### REST API

Full REST API documentation is available at `/swagger/` (OpenAPI/Swagger UI). Example usage:

```sh
# Current weather for a location
curl https://your-wthr-instance.example.com/api/v1/weather?location=New+York

# Active severe weather alerts (US)
curl https://your-wthr-instance.example.com/api/v1/severe-weather?country=US

# Earthquake data
curl https://your-wthr-instance.example.com/api/v1/earthquakes?minmagnitude=5
```

For authenticated endpoints, pass the session token as a Bearer token:

```sh
curl -H "Authorization: Bearer your-token-here" \
  https://your-wthr-instance.example.com/api/v1/users/locations
```

### GraphQL

A GraphQL endpoint is available at `/graphql`. Interactive playground at `/graphql` (development
mode) or via `/swagger/` (production).

### Terminal / curl-friendly Output

Request plain-text output for terminal use:

```sh
# ASCII weather for current location
curl https://your-wthr-instance.example.com/

# Weather for a named location
curl https://your-wthr-instance.example.com/New+York

# Shell function (add to .bashrc / .zshrc)
curl https://your-wthr-instance.example.com/:bash.function
```

---

## WebSocket Alerts

Real-time severe weather and earthquake alerts are pushed over WebSocket at:

```
ws://your-wthr-instance.example.com/ws/notifications
wss://your-wthr-instance.example.com/ws/notifications  (TLS)
```

Connect with any WebSocket client. Messages are JSON-encoded alert objects matching the REST API
schema. Authenticated users may subscribe to alerts for specific locations; unauthenticated
connections receive global alert broadcasts.

---

## Prometheus Metrics

wthr exposes Prometheus-compatible metrics at `/metrics`. Scrape authentication can be configured
in the admin panel or `server.yml`.

Example `prometheus.yml` scrape config:

```yaml
scrape_configs:
  - job_name: wthr
    static_configs:
      - targets: ['your-wthr-instance.example.com']
    # If scrape auth is enabled:
    # bearer_token: your-scrape-token
```

---

## Tor Hidden Service

If `tor` is present in `$PATH`, wthr automatically starts a Tor hidden service and displays the
`.onion` address in the admin panel and at the `/server/status` endpoint. No manual configuration
is needed. The hidden service runs alongside the clearnet service.

To disable: set `tor.enabled: false` in `server.yml`.

---

## Email / SMTP

wthr can send email notifications for:

- User account verification
- Password reset requests
- Alert subscription notifications (severe weather, earthquakes)
- Admin notifications (backup complete, update available)

Configure SMTP in the admin panel under **Settings → Email**, or in `server.yml` under `smtp.*`.
TLS and STARTTLS are both supported.

---

## None of the Following Are Enabled

The following integrations are explicitly **not enabled** for this deployment:

- OAuth/OIDC identity provider (no SSO/federated login)
- Native app association files (`/.well-known/apple-app-site-association`, `/.well-known/assetlinks.json`)
- Autodiscovery protocols (no CalDAV, CardDAV, Autoconfig)
- Webhooks (outbound HTTP callbacks not implemented)
- Federation (no ActivityPub or other federated protocols)
- Custom domain per-user routing (PART 36 not adopted)
- Organization/multi-tenant features (PART 35 not adopted)
