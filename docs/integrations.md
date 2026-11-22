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
| [Japan JMA](https://www.jma.go.jp/bosai/) | Japan weather alerts | `https://www.jma.go.jp/bosai/warning/data/r8/map.json` (JMA website data feed; not a documented API) |
| [Mexico CONAGUA](https://smn.conagua.gob.mx/) | Mexico weather warnings | SMN data feeds |
| [USGS Earthquake Hazards](https://earthquake.usgs.gov/earthquakes/feed/) | Real-time seismic data | GeoJSON feeds |

Data is fetched on a configurable schedule (default: weather alerts every 15 minutes, earthquake
data every 1 minute). No API keys, accounts, or registration are required for any of these sources.

## Location Reference Data

Location lookup uses maintained public reference datasets:

| Dataset | Source | Refresh and use |
|---------|--------|-----------------|
| US ZIP codes | [GeoNames postal-code data](https://download.geonames.org/export/zip/US.zip) ([license and format](https://download.geonames.org/export/zip/readme.txt)) | The server downloads the US archive at startup and during its scheduled data refresh. Postal-code coordinates are representative locations, not official USPS delivery boundaries. GeoNames requires attribution under CC BY 4.0 and provides the data without accuracy, timeliness, or completeness guarantees. |
| Airports | [OurAirports airport data](https://ourairports.com/data/airports.csv) ([data terms](https://ourairports.com/data/)) | The airport lookup service downloads the CSV when instantiated and can refresh it on demand. The dataset is public domain and community maintained, with no accuracy or fitness guarantee; use it for location lookup, not flight operations or navigation. |

The ZIP and airport loaders validate the downloaded schema, record counts, codes, and coordinate bounds before replacing the in-memory snapshot. If a refresh fails, the last validated airport cache remains available; ZIP lookup retains its previous in-memory snapshot. These directories are reference data and can lag authoritative postal or aviation records. JMA changed its warning feed in 2026; the server reads the current Reiwa 8 map feed and tracks the latest update or cancellation for each area and warning code. JMA may change this website feed without notice, so consult [JMA's warnings](https://www.jma.go.jp/bosai/warning/) directly for current official information.

---

## GeoIP

wthr uses the [sapics/ip-location-db](https://github.com/sapics/ip-location-db) databases for automatic location detection from IP address. The databases are downloaded on first
run and updated weekly (Sunday 03:00) via the built-in scheduler — never embedded in the binary.

Databases used:

- `dbip-city-ipv4` — IPv4 city-level geolocation (DB-IP, CC BY 4.0)
- `dbip-city-ipv6` — IPv6 city-level geolocation (DB-IP, CC BY 4.0)
- `iptoasn-country` — country lookup (iptoasn.com, PDDL)
- `iptoasn-asn` — autonomous system number lookup (iptoasn.com, PDDL)

Files are fetched from the sapics/ip-location-db GitHub Release assets.

GeoIP supports automatic location detection on the weather page and an optional deny-only country
policy. Country policy is only one risk signal: it never grants access or replaces authentication or
authorization. Missing databases and lookup failures fail open for GeoIP, while the rest of the
security pipeline continues. Private and internal addresses are not looked up, and configured
`server.security.allowlist` addresses bypass country blocking. Operators configure country lists and
their saved presets in `/server/{admin_path}/network/geoip`.

---

## External Authentication

### OIDC regular-user accounts

OIDC login matches returning users by the provider name and stable `sub` claim. When an
authenticated identity has no local mapping and that provider has `auto_register: true`, the user
is sent to `/server/auth/external/username` before an account is created. The server keeps the
verified identity in a short-lived, HttpOnly pre-auth cookie and in-memory record (10-minute
expiry). The page displays an editable candidate derived from the mapped preferred-username claim,
email local part, display name, or provider name, in that order. The server normalizes the candidate
and suggests a numeric suffix when it is unavailable; the user confirms the final username in a
plain POST form. The selected name is stored once, and later OIDC logins use the provider plus `sub`
mapping.

With `auto_register: false`, an unmatched OIDC identity is denied and no local account is created;
the user must contact the operator. Existing mapped OIDC accounts can still sign in.

LDAP login is a separate username/password endpoint and does not use this confirmation page; its
current handler can create a regular-user account directly from the submitted username.

### SAML authentication

SAML providers are configured in the server authentication settings. Browser-initiated login uses
`/server/auth/saml/{provider}`, publishes SP metadata at
`/server/auth/saml/{provider}/metadata`, and accepts IdP responses at
`/server/auth/saml/{provider}/acs`. IdP metadata must use HTTPS. The SP signing private key is
encrypted with the configured server encryption key before it is written to the private secrets
file. Assertions are validated by the SAML protocol library, request IDs are correlated to the
browser state, and assertion IDs are recorded in the server database for replay rejection.

New users with `auto_register: true` complete the shared plain POST username confirmation step at
`/server/auth/external/username`. Existing identities match by provider name and persistent NameID.
For transient NameID, configure a stable external ID attribute or login is rejected. With
`auto_register: false`, unmatched identities are denied.

SAML browser login also supports Server Admin provisioning when an assertion's mapped groups match
the provider's `admin_groups`. New Server Admins choose a username before provisioning, and later
logins use the stored provider/external identity mapping. During first login, the admin also sets a
local fallback password, which allows password login if the identity provider becomes unavailable;
the mapping uses provider name plus stable external ID and never links by email alone.
`role_mapping` assigns regular-user roles.
IdP-initiated login is accepted only when enabled for that provider. The admin page supports provider
testing, metadata download, key regeneration, and provider removal; generated SP certificates rotate
automatically before expiry. Provider removal revokes sessions associated with that provider and
deletes its stored signing key and certificate.

SP-initiated and IdP-initiated SLO validate signed messages and terminate mapped sessions using
Redirect or POST bindings. Native API-mode login provides a browser redirect followed by one-time
request exchange. Its exchange request is bound to the originating IP address and user-agent; API
clients can exchange the request ID alone or provide the one-time code shown on the browser completion
page. No application URI scheme is configured, so the code is presented for manual return rather than
in a custom app deep link. API SLO accepts authenticated regular-user/admin sessions and API tokens.
Browser-initiated requests and first-login confirmation remain process-local, so a load-balanced
deployment requires sticky routing through completion. Existing administrators with TOTP must verify
it after SAML; passkey-only administrators are directed to the local login flow. Operators may use
external SP certificate/key paths; private key files must be regular files with mode 0400 or 0600, and
certificate/key match and validity are checked. Generated SP certificates rotate automatically before
expiry. File upload is not implemented. MFA remains optional for newly provisioned administrators,
matching the admin policy in AI.md.

---

## API Access

### REST API

Full REST API documentation is available at `/server/docs/swagger` (OpenAPI/Swagger UI).
The JSON document is served at `/api/{api_version}/server/swagger`. Example usage:

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

GraphQL is read-only (queries only). Send queries with `POST /api/v1/server/graphql`
(unversioned alias: `/api/graphql`). The interactive playground is at `/server/docs/graphql`.
Every write operation (sign-in, settings, saved locations, passkeys) uses the REST API, which is
described in the OpenAPI document at `/api/v1/server/swagger` (Swagger UI: `/server/docs/swagger`).

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

wthr exposes Prometheus text-exposition metrics, prefixed `wthr_`. The endpoint is **internal
only** — block it at your firewall, reverse proxy, or NetworkPolicy and never expose it to the
public internet.

The same handler is mounted at every one of these paths (no redirects):

| Path | Notes |
|------|-------|
| `/metrics` | Root alias, gated by `server.metrics.root.enabled` (default `true`) |
| `/server/metrics` | Frontend route tree |
| `/api/v1/server/metrics` | Versioned API route |
| `/api/metrics` | Unversioned API alias |

Each path also accepts a trailing service segment — `/metrics/prometheus`, `/metrics/grafana`,
`/metrics/loki` — selecting which consumer the request is authenticated as.

Access is gated by a **per-service bearer token**, sent in the `Authorization` header only
(query-string tokens are forbidden). A service whose token is empty is disabled and answers `403`
with an empty body. Tokens are compared in constant time, never logged, and always displayed
masked as `xxxxx` in the admin panel and API responses.

Configure the tokens under `server.metrics.auth.tokens` in `{config_dir}/yaml.d/metrics.yml`, or in the admin panel at
`/server/{admin_path}/metrics`:

```yaml
server:
  metrics:
    enabled: true
    root:
      enabled: true
    auth:
      allow_unauthenticated: false
      tokens:
        prometheus: your-prometheus-token
        grafana: your-grafana-token
        loki: your-loki-token
    include_system: true
    include_runtime: true
    loki:
      max_entries: 1000
      max_age: 1h
```

`allow_unauthenticated: true` turns off the token check for every service. It exists only for
deployments where the endpoint is already unreachable from outside, and defaults to `false`.

Example `prometheus.yml` scrape config:

```yaml
scrape_configs:
  - job_name: wthr
    metrics_path: /metrics/prometheus
    static_configs:
      - targets: ['your-wthr-instance.example.com']
    authorization:
      credentials: your-prometheus-token
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
