## Project description

A unified weather information platform that aggregates global meteorological data from authoritative sources into a single, accessible service for developers, businesses, and end users. Provides weather forecasts, severe weather alerts, earthquake data, hurricane tracking, and lunar phases through a single API and web interface.

**Target Users:**
1. **Developers** — Building weather-aware applications; need a single API instead of 10+ integrations
2. **System Administrators** — Need self-hosted weather services with full data control
3. **Emergency Services** — Monitoring severe weather and seismic activity in real-time
4. **News Organizations** — Require reliable weather data feeds for broadcast/web
5. **IoT/Smart Home** — Systems needing weather integration for automation
6. **General Public** — Clean, ad-free weather interface with location detection

**Key Features:**

| Feature | Description |
|---------|-------------|
| Weather Forecasts | 16-day global forecasts with hourly/daily breakdown |
| Severe Weather Alerts | Real-time alerts from 6 countries (US, Canada, UK, Australia, Japan, Mexico) |
| Hurricane Tracking | Active tropical storm monitoring from NOAA NHC |
| Earthquake Data | Real-time seismic activity from USGS |
| Moon Phases | Lunar cycles, illumination, rise/set times |
| Location Detection | Automatic IP-based geolocation |
| Multi-Format API | JSON, text/plain, read-only GraphQL support |
| Terminal Output | ASCII/ANSI colored weather for curl/terminal |
| Bash Integration | Shell function for terminal weather (`/:bash.function`) |
| WebSocket Alerts | Real-time push notifications for severe weather |
| Contact Webhooks | Signed, role-based outbound notifications for contact and security events |
| User Locations | Save favorite locations for quick access |
| Alert Subscriptions | Subscribe to alerts for specific regions |
| API Documentation | Interactive REST and GraphQL documentation |
| i18n | English, Spanish, French, German, Arabic, Japanese, Chinese |

All weather data sources are free/public APIs and require no operator API keys or provider accounts. Live forecasts, alerts, and related lookups require outbound network access to those providers. GeoIP databases are downloaded on first run and updated weekly by the scheduler (never embedded in the binary). The service can be self-hosted, with cached-data fallback behavior described below when providers are unavailable.

## Project variables

    project_name:     wthr
    project_org:      webappsgo
    # STABLE - set at first-time setup; change only on an explicit rename or fork reset
    internal_name:    wthr
    # STABLE - set at first-time setup; change only on an explicit rename or fork reset
    internal_org:     webappsgo
    app_name:         Weather
    official_site:    https://wthr.top
    project_repo:     https://github.com/webappsgo/wthr
    maintainer_name:  webappsgo
    maintainer_email: casjay@yahoo.com
    module_path:      github.com/webappsgo/wthr
    uses_geolocation: true

## Business logic

### Product scope & non-goals

**In scope:**
- Weather forecasts (16-day global via Open-Meteo)
- Severe weather alerts (US/Canada/UK/Australia/Japan/Mexico)
- Earthquake data (USGS, real-time)
- Hurricane/tropical storm tracking (NOAA NHC)
- Moon phases (calculated locally, no external API)
- Location detection (GeoIP databases downloaded on first run and updated by the scheduler)
- Multi-format API (REST JSON, plain text, GraphQL)
- Terminal/curl-friendly output with ASCII art
- WebSocket real-time alert push
- Admin panel for all settings
- Syndication: `/feed.xml` (Atom) and `/rss.xml` (RSS) list the most recent earthquakes; `/sitemap.xml` lists the public pages
- User accounts with saved locations and alert subscriptions
- Tor hidden service (auto-enabled if Tor is present)
- Backup/restore, scheduler, metrics, email notifications, and signed contact webhooks

**Non-goals:**
- Paid features or premium tiers — all functionality free for all users
- Proprietary weather APIs requiring keys (all sources are free/public)
- Weather data storage beyond caching (no historical data warehouse)
- Mobile apps (responsive web only)
- Organization/multi-tenant features (PART 35 not adopted)
- Custom domain features (PART 36 not adopted)

### Roles & permissions

| Role | Description | Auth method |
|------|-------------|------------|
| Anonymous | No account — basic API and web access | None |
| Regular User | Registered account — saved locations, subscriptions | Password + session cookie |
| Server Admin | Manages the app configuration and admins | Password with optional TOTP or passkey (AI.md PART 17) |
| Primary Admin | First admin created — cannot be deleted | Password with optional TOTP or passkey (AI.md PART 17) |

Admins manage TOTP from their profile Security page. Enrollment requires a password check and
authenticator verification, and creates ten one-time recovery codes.

**Permission matrix:**

| Action | Anonymous | Regular User | Server Admin |
|--------|-----------|--------------|--------------|
| View weather/alerts/earthquakes/hurricanes/moon | ✅ | ✅ | ✅ |
| curl/terminal output | ✅ | ✅ | ✅ |
| GraphQL queries (read) | ✅ | ✅ | ✅ |
| Save locations (up to 10) | ❌ | ✅ | — |
| Alert subscriptions | ❌ | ✅ | — |
| Admin panel | ❌ | ❌ | ✅ |
| Manage server config | ❌ | ❌ | ✅ |
| Manage admins | ❌ | ❌ | ✅ |
| Manage regular users | ❌ | ❌ | ✅ |

Rate limiting is always on. General read/write/health limits use the AI.md PART 12 defaults per IP for anonymous callers and per token for authenticated callers; the global ceiling is 240 requests/minute. Login is limited per IP and identifier, password reset and registration are per IP, invalid/revoked/expired Bearer tokens trigger a per-IP 5-attempt lockout, and operator-token reveal/rotation and backup restore use the strict per-IP limit. Counters use Redis/Valkey when configured and persist through `server.db` otherwise. Thresholds and windows are editable in the admin panel.

### Cluster administration

The admin panel provides node enrollment with expiring join tokens, node status
details, and shared cluster settings. In shared-database cluster mode, nodes
synchronize versioned server configuration, application secrets, and PGP keys
through the shared database. Node management pages provide the join, status,
settings, and removal flows.

### Data model & sensitivity

**Stored data:** application configuration, admin accounts, regular user accounts, saved locations, alert subscriptions, sessions, the audit log and scheduler state (storage layout: AI.md PART 10).

Scheduled infrastructure alerts monitor database connectivity and disk capacity.
Database probes alert Server Admins after consecutive failures and log recovery;
disk alerts deduplicate paths by filesystem, repeat at most daily, and clear
after usage falls five percentage points below the configured threshold.

**Sensitive data:**
- Admin and user passwords and session tokens are never stored or shown in plaintext (AI.md PART 11)
- SMTP password: stored encrypted
- No third-party API keys are needed (all weather sources are public)

**Cached data (non-sensitive):**
- Weather forecasts stay cached for at most 15 minutes
- Alert data stays cached for at most 5 minutes
- GeoIP lookups are held in memory only for the session

**Data models:**

```go
// Weather represents current conditions and forecast
type Weather struct {
    Location    Location      `json:"location"`
    Current     Current       `json:"current"`
    Forecast    []DayForecast `json:"forecast"`
    LastUpdated time.Time     `json:"last_updated"`
}

// SevereWeatherAlert represents a weather alert
type SevereWeatherAlert struct {
    ID          string    `json:"id"`
    Event       string    `json:"event"`       // tornado, flood, winter_storm, etc.
    Severity    string    `json:"severity"`    // extreme, severe, moderate, minor
    Certainty   string    `json:"certainty"`   // observed, likely, possible
    Urgency     string    `json:"urgency"`     // immediate, expected, future
    Headline    string    `json:"headline"`
    Description string    `json:"description"`
    Instruction string    `json:"instruction"`
    Areas       []string  `json:"areas"`
    Effective   time.Time `json:"effective"`
    Expires     time.Time `json:"expires"`
    Source      string    `json:"source"`      // NOAA, Environment Canada, etc.
}

// Earthquake represents a seismic event
type Earthquake struct {
    ID        string    `json:"id"`
    Magnitude float64   `json:"magnitude"`
    Location  string    `json:"location"`
    Depth     float64   `json:"depth"`       // km
    Time      time.Time `json:"time"`
    Latitude  float64   `json:"lat"`
    Longitude float64   `json:"lon"`
    Tsunami   bool      `json:"tsunami"`
    URL       string    `json:"url"`
}

// Hurricane represents a tropical storm
type Hurricane struct {
    ID        string                   `json:"id"`
    Name      string                   `json:"name"`
    Category  int                      `json:"category"`    // 1-5, 0 for tropical storm
    WindSpeed int                      `json:"wind_speed"`  // mph
    Pressure  int                      `json:"pressure"`    // mb
    Movement  string                   `json:"movement"`    // direction and speed
    Location  string                   `json:"location"`
    Latitude  float64                  `json:"lat"`
    Longitude float64                  `json:"lon"`
    Forecast  []HurricaneForecastPoint `json:"forecast"`
}

// MoonPhase represents lunar data
type MoonPhase struct {
    Phase        string    `json:"phase"`        // new, waxing_crescent, first_quarter, etc.
    Illumination float64   `json:"illumination"` // 0.0-1.0
    Age          float64   `json:"age"`          // days since new moon
    Moonrise     time.Time `json:"moonrise"`
    Moonset      time.Time `json:"moonset"`
}
```

### Trust boundaries & external services

| Service | Trust level | Failure mode |
|---------|-------------|-------------|
| Open-Meteo API | Untrusted (external) | Return cached data; if no cache, return 503 |
| NOAA Weather API | Untrusted (external) | Return cached alerts; if no cache, empty list with stale warning |
| Environment Canada | Untrusted (external) | Same as NOAA |
| UK Met Office | Untrusted (external) | Same as NOAA |
| Australian BOM | Untrusted (external) | Same as NOAA |
| JMA (Japan) | Untrusted (external) | Same as NOAA |
| CONAGUA (Mexico) | Untrusted (external) | Same as NOAA |
| USGS Earthquake API | Untrusted (external) | Return cached; if no cache, empty list |
| NOAA NHC (Hurricanes) | Untrusted (external) | Return cached; if no cache, empty list |
| sapics/ip-location-db | Trusted (downloaded on first run, updated weekly by the scheduler, never embedded) | Fall back to default location on lookup failure |
| Client IP address | Untrusted | Used for GeoIP lookup, security logs, rate limiting, and IP blocklist checks; never trusted as identity |
| User-supplied location | Untrusted | Validate and sanitize before use |
| Admin input (config) | Trusted (authenticated admin) | Validate schema; reject invalid values |

All external API responses are validated before use. Network calls have 30-second timeout. Failed external calls never crash the server.

### Threat model & abuse cases

**Primary assets being protected:**
- Admin credentials and session tokens
- Regular user account data and saved locations
- Server configuration (sensitive SMTP credentials)
- Service availability (DoS protection)

**Trusted inputs:**
- Authenticated admin sessions
- Authenticated user sessions
- Internal scheduler jobs

**Untrusted inputs:**
- All HTTP requests (headers, query params, body, path)
- All external API responses (Open-Meteo, NOAA, USGS, etc.)
- Client IP addresses
- User-supplied location strings

**Main attacker/abuser goals:**
1. Gain admin panel access (credential theft, session hijacking)
2. Scrape weather data to avoid rate limits
3. Exploit external API response parsing (malformed data)
4. Enumerate user accounts
5. Abuse alert subscription system for spam
6. DoS via unbounded queries or expensive lookups
7. SSRF via location parameter
8. Path traversal via location strings or backup filenames

**Required defenses:**
| Threat | Defense |
|--------|---------|
| Credential theft | Passwords are never recoverable and login comparison leaks no timing (AI.md PART 11) |
| Session hijacking | Unguessable session tokens that scripts cannot read (AI.md PART 11) |
| Account enumeration | Same message + timing for wrong-password vs no-such-user |
| Scraping | Rate limiting (AI.md defaults, per IP or per token), 429 + Retry-After |
| Malformed API response | Schema validation before use, never eval/exec |
| SSRF | Location input validated against allowed formats (city name, coordinates, ZIP); no URL input |
| Path traversal | Backup filenames sanitized; never use user input directly in file paths |
| DoS on expensive queries | Rate limit + pagination on all list endpoints; timeouts on all external calls |
| Alert spam | Subscription requires verified user account |

**Explicit non-goals (acceptable risk):**
- GeoIP can apply a configured deny-only country rule, but it is never the sole access-control gate or a substitute for authentication; VPN and proxy bypass are expected
- Regular users are not required to enable two-factor authentication

### Security decisions & exceptions

1. **Admin sign-in follows AI.md PART 17** — password login with optional TOTP or passkey; recovery of a lost admin login is through the server console.

2. **Regular user accounts use password sign-in** — two-factor authentication and passkeys are optional for users, because saved locations and alert subscriptions are a low-stakes feature.

3. **Anonymous weather access** — core weather/alert/earthquake/hurricane data is public with no auth required. Rate limiting is the only protection. This is intentional: weather data is public information.

4. **External API data is fetched server-side** — no client-side fetches to external APIs. This prevents CORS issues and keeps API call patterns server-controlled.

5. **Tor hidden service** — auto-enabled when `tor` binary is found. The .onion address is shown in admin panel. This is a privacy feature, not a security exception.

6. **SAML assertion parser** — login responses are validated by the maintained [`github.com/supatype/saml`](https://github.com/supatype/saml) fork at v0.7.0. The fork binds assertion identity and attribute fields to the SAML namespace and addresses Go advisory GO-2026-6909. The existing `crewjam/saml` dependency remains for SP metadata and SLO operations; its vulnerable response parser is not called. `govulncheck` verifies the reachable call graph.

### Weather data rules

- Cache weather data for 15 minutes (reduce external API calls)
- Accept city name, ZIP/postal code, coordinates, or auto-detect from IP
- ZIP code lookup service for US postal codes
- Return metric or imperial units based on user preference or region
- Fall back to cached data if external API unavailable
- Include data attribution for all external sources

### Severe weather alert rules

- Poll alert sources every 5 minutes
- Deduplicate alerts by ID across sources
- Filter alerts by severity threshold (configurable: extreme, severe, moderate, minor)
- Alerts expire automatically based on expiration timestamp
- WebSocket push for new alerts matching user subscriptions

### User account rules (Multi-User — PART 34 REQUIRED)

- Registration: configurable (open/private); default open. Per AI.md PART 34 the
  mode controls only whether the public self-registration form is reachable —
  admin invite and direct admin create work in both modes
- Email verification required before account activation
- Save up to 10 locations per user
- Subscribe to alerts for saved locations
- Email notifications for severe alerts (optional, per-user setting)
- No authentication required for basic API access (anonymous)
- User self-service: change password, change email, delete account, manage sessions

### Data sources

| Data Type | Source | Update Frequency |
|-----------|--------|------------------|
| Weather Forecasts | Open-Meteo API | 15 minutes |
| US Severe Weather | NOAA Weather API | 5 minutes |
| Canada Alerts | Environment Canada | 5 minutes |
| UK Alerts | UK Met Office | 5 minutes |
| Australia Alerts | Australian BOM | 5 minutes |
| Japan Alerts | JMA (Japan Meteorological Agency) | 5 minutes |
| Mexico Alerts | CONAGUA | 5 minutes |
| Earthquakes | USGS Earthquake API | 1 minute |
| Hurricanes | NOAA National Hurricane Center | 15 minutes |
| Moon Phases | Calculated (no external API) | N/A |
| IP Geolocation | sapics/ip-location-db | Weekly |

### Interfaces & compatibility

**Interfaces.** Every feature is available as a web page (HTML for browsers, plain
text for curl and terminals), as a versioned JSON API, as GraphQL and OpenAPI, and
through the CLI client. An MCP server (Streamable HTTP, public weather operations
as tools) lets AI assistants use the service and is always enabled. The standard server pages (about, help, MCP help, privacy, terms, contact,
security reporting, data protection officer contact, health, preferences) have
real content derived from this document.

### API routes

The versioned JSON API and public API aliases are part of the shipped product contract.
This inventory is checked against the generated OpenAPI document and router registrations.

| Method | Route |
|--------|-------|
| `GET` | `/api/autodiscover` |
| `POST` | `/api/graphql` |
| `GET` | `/api/healthz` |
| `POST` | `/api/mcp` |
| `GET` | `/api/metrics` |
| `GET` | `/api/metrics/{service}` |
| `GET` | `/api/swagger` |
| `GET` | `/api/{api_version}/` |
| `GET` | `/api/{api_version}/blocklist` |
| `GET` | `/api/{api_version}/docs` |
| `GET` | `/api/{api_version}/earthquakes` |
| `GET` | `/api/{api_version}/earthquakes/{id}` |
| `GET` | `/api/{api_version}/forecasts` |
| `GET` | `/api/{api_version}/forecasts/{location}` |
| `GET` | `/api/{api_version}/history` |
| `GET` | `/api/{api_version}/ip` |
| `GET` | `/api/{api_version}/locations/lookup/coords` |
| `GET` | `/api/{api_version}/locations/lookup/zip/{code}` |
| `GET` | `/api/{api_version}/locations/search` |
| `GET` | `/api/{api_version}/moon` |
| `GET` | `/api/{api_version}/moon/calendar` |
| `GET` | `/api/{api_version}/public/users/{username}` |
| `GET` | `/api/{api_version}/server/about` |
| `POST` | `/api/{api_version}/server/auth/2fa` |
| `POST` | `/api/{api_version}/server/auth/admin/passkey/challenge` |
| `POST` | `/api/{api_version}/server/auth/admin/passkey/verify` |
| `GET` | `/api/{api_version}/server/auth/check` |
| `GET` | `/api/{api_version}/server/auth/invite/server/{token}` |
| `POST` | `/api/{api_version}/server/auth/invite/server/{token}` |
| `GET` | `/api/{api_version}/server/auth/invite/user/{token}` |
| `POST` | `/api/{api_version}/server/auth/invite/user/{token}` |
| `POST` | `/api/{api_version}/server/auth/login` |
| `POST` | `/api/{api_version}/server/auth/logout` |
| `GET` | `/api/{api_version}/server/auth/saml` |
| `GET` | `/api/{api_version}/server/auth/saml/{provider}` |
| `POST` | `/api/{api_version}/server/auth/saml/{provider}/exchange` |
| `POST` | `/api/{api_version}/server/auth/saml/{provider}/slo` |
| `POST` | `/api/{api_version}/server/auth/passkey/challenge` |
| `POST` | `/api/{api_version}/server/auth/passkey/verify` |
| `POST` | `/api/{api_version}/server/auth/password/forgot` |
| `POST` | `/api/{api_version}/server/auth/password/reset` |
| `POST` | `/api/{api_version}/server/auth/recovery/use` |
| `POST` | `/api/{api_version}/server/auth/refresh` |
| `POST` | `/api/{api_version}/server/auth/register` |
| `POST` | `/api/{api_version}/server/auth/verify` |
| `GET` | `/api/{api_version}/server/contact` |
| `POST` | `/api/{api_version}/server/contact` |
| `GET` | `/api/{api_version}/server/dpo` |
| `POST` | `/api/{api_version}/server/graphql` |
| `GET` | `/api/{api_version}/server/healthz` |
| `GET` | `/api/{api_version}/server/help` |
| `GET` | `/api/{api_version}/server/help/mcp` |
| `POST` | `/api/{api_version}/server/mcp` |
| `GET` | `/api/{api_version}/server/metrics` |
| `GET` | `/api/{api_version}/server/metrics/{service}` |
| `GET` | `/api/{api_version}/server/preferences` |
| `POST` | `/api/{api_version}/server/preferences` |
| `GET` | `/api/{api_version}/server/preferences/sync` |
| `GET` | `/api/{api_version}/server/preferences/sync/import` |
| `GET` | `/api/{api_version}/server/preferences/export` |
| `GET` | `/api/{api_version}/server/preferences/import` |
| `GET` | `/api/{api_version}/server/privacy` |
| `GET` | `/api/{api_version}/server/security` |
| `GET` | `/api/{api_version}/server/security/policy` |
| `GET` | `/api/{api_version}/server/security/thanks` |
| `GET` | `/api/{api_version}/server/swagger` |
| `GET` | `/api/{api_version}/server/terms` |
| `GET` | `/api/{api_version}/server/{admin_path}/admins` |
| `POST` | `/api/{api_version}/server/{admin_path}/admins/invite` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/admins/{id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/admins/{id}` |
| `POST` | `/api/{api_version}/server/{admin_path}/admins/{id}/disable` |
| `POST` | `/api/{api_version}/server/{admin_path}/admins/{id}/enable` |
| `GET` | `/api/{api_version}/server/{admin_path}/backup` |
| `POST` | `/api/{api_version}/server/{admin_path}/backup` |
| `POST` | `/api/{api_version}/server/{admin_path}/backup/restore` |
| `GET` | `/api/{api_version}/server/{admin_path}/backup/schedule` |
| `POST` | `/api/{api_version}/server/{admin_path}/backup/schedule` |
| `GET` | `/api/{api_version}/server/{admin_path}/backup/stats` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/backup/{filename}` |
| `GET` | `/api/{api_version}/server/{admin_path}/backup/{filename}` |
| `GET` | `/api/{api_version}/server/{admin_path}/backup/{filename}/download` |
| `GET` | `/api/{api_version}/server/{admin_path}/branding` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/branding` |
| `POST` | `/api/{api_version}/server/{admin_path}/database/optimize` |
| `POST` | `/api/{api_version}/server/{admin_path}/database/test` |
| `POST` | `/api/{api_version}/server/{admin_path}/database/test-config` |
| `POST` | `/api/{api_version}/server/{admin_path}/database/vacuum` |
| `GET` | `/api/{api_version}/server/{admin_path}/email` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/email` |
| `POST` | `/api/{api_version}/server/{admin_path}/email/smtp/autodetect` |
| `GET` | `/api/{api_version}/server/{admin_path}/email/smtp/providers` |
| `GET` | `/api/{api_version}/server/{admin_path}/email/templates` |
| `GET` | `/api/{api_version}/server/{admin_path}/email/templates/{name}` |
| `PUT` | `/api/{api_version}/server/{admin_path}/email/templates/{name}` |
| `POST` | `/api/{api_version}/server/{admin_path}/email/templates/{name}/preview` |
| `POST` | `/api/{api_version}/server/{admin_path}/email/templates/{name}/reset` |
| `POST` | `/api/{api_version}/server/{admin_path}/email/test` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/logs` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/archives` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/audit` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/logs/audit-logs` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/audit-logs` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/audit/download` |
| `POST` | `/api/{api_version}/server/{admin_path}/logs/audit/search` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/audit/stats` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/cef/config` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/export` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/fail2ban/config` |
| `POST` | `/api/{api_version}/server/{admin_path}/logs/logging/fail2ban/configure` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/formats` |
| `PUT` | `/api/{api_version}/server/{admin_path}/logs/logging/formats` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/syslog/config` |
| `POST` | `/api/{api_version}/server/{admin_path}/logs/logging/syslog/configure` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/logging/test` |
| `POST` | `/api/{api_version}/server/{admin_path}/logs/rotate` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/stats` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/stream` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/{type}` |
| `GET` | `/api/{api_version}/server/{admin_path}/logs/{type}/download` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/config` |
| `PUT` | `/api/{api_version}/server/{admin_path}/metrics/config` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/export` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/list` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/notifications/channels/{type}` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/notifications/errors` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/notifications/health` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/notifications/summary` |
| `GET` | `/api/{api_version}/server/{admin_path}/metrics/stats` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/geoip` |
| `GET` | `/api/{api_version}/server/{admin_path}/network/i2p` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/network/i2p` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/i2p/regenerate` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/i2p/restart` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/i2p/validate` |
| `GET` | `/api/{api_version}/server/{admin_path}/network/tor` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/network/tor` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/tor/import` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/tor/regenerate` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/network/tor/vanity` |
| `GET` | `/api/{api_version}/server/{admin_path}/network/tor/vanity` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/tor/vanity` |
| `POST` | `/api/{api_version}/server/{admin_path}/network/tor/vanity/apply` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels/definitions` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels/history` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/channels/initialize` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels/queue/stats` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}` |
| `PUT` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}/disable` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}/enable` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}/stats` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/channels/{type}/test` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/templates` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/templates` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/templates/initialize` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/templates/preview` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/templates/variables` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/notifications/templates/{id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/notifications/templates/{id}` |
| `PUT` | `/api/{api_version}/server/{admin_path}/notifications/templates/{id}` |
| `POST` | `/api/{api_version}/server/{admin_path}/notifications/templates/{id}/clone` |
| `GET` | `/api/{api_version}/server/{admin_path}/pages` |
| `GET` | `/api/{api_version}/server/{admin_path}/pages/{name}` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/pages/{name}` |
| `POST` | `/api/{api_version}/server/{admin_path}/restart` |
| `GET` | `/api/{api_version}/server/{admin_path}/scheduler` |
| `GET` | `/api/{api_version}/server/{admin_path}/scheduler/{name}` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/scheduler/{name}` |
| `POST` | `/api/{api_version}/server/{admin_path}/scheduler/{name}/disable` |
| `POST` | `/api/{api_version}/server/{admin_path}/scheduler/{name}/enable` |
| `GET` | `/api/{api_version}/server/{admin_path}/scheduler/{name}/history` |
| `POST` | `/api/{api_version}/server/{admin_path}/scheduler/{name}/run` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/allowlist` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/allowlist` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/allowlist/check/{ip}` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/security/allowlist/{id}` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/auth` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}/test` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}/metadata` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/auth/saml/providers/{provider}/certificate` |
| `POST` | `/api/{api_version}/server/{admin_path}/nodes/join` |
| `POST` | `/api/{api_version}/server/{admin_path}/nodes/leave` |
| `POST` | `/api/{api_version}/server/{admin_path}/nodes/token` |
| `GET` | `/api/{api_version}/server/{admin_path}/nodes` |
| `GET` | `/api/{api_version}/server/{admin_path}/nodes/{node}` |
| `GET` | `/api/{api_version}/server/{admin_path}/nodes/settings` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/nodes/settings` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/blocked-ips` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/blocked-ips` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/security/blocked-ips/expired` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/security/blocked-ips/{ip}` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/blocked-ips/{ip}` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/encryption` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/encryption` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/operator-token` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/operator-token` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/tokens` |
| `POST` | `/api/{api_version}/server/{admin_path}/security/tokens` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/security/tokens/{id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/settings` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/settings` |
| `GET` | `/api/{api_version}/server/{admin_path}/settings/all` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/bulk` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/cache` |
| `POST` | `/api/{api_version}/server/{admin_path}/settings/cache/clear` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/database` |
| `GET` | `/api/{api_version}/server/{admin_path}/settings/export` |
| `POST` | `/api/{api_version}/server/{admin_path}/settings/import` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/rate-limit` |
| `GET` | `/api/{api_version}/server/{admin_path}/security/ratelimit` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/security/ratelimit` |
| `POST` | `/api/{api_version}/server/{admin_path}/settings/reload` |
| `POST` | `/api/{api_version}/server/{admin_path}/settings/reset` |
| `GET` | `/api/{api_version}/server/{admin_path}/settings/search` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/security` |
| `POST` | `/api/{api_version}/server/{admin_path}/settings/weather` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/web` |
| `GET` | `/api/{api_version}/server/{admin_path}/settings/{key}` |
| `PUT` | `/api/{api_version}/server/{admin_path}/settings/{key}` |
| `GET` | `/api/{api_version}/server/{admin_path}/setup` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/account` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/complete` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/security` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/services` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/token` |
| `POST` | `/api/{api_version}/server/{admin_path}/setup/verify` |
| `GET` | `/api/{api_version}/server/{admin_path}/ssl` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/auto-renew` |
| `GET` | `/api/{api_version}/server/{admin_path}/ssl/dns` |
| `PUT` | `/api/{api_version}/server/{admin_path}/ssl/dns` |
| `GET` | `/api/{api_version}/server/{admin_path}/ssl/dns-records` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/dns/check` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/obtain` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/renew` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/revoke` |
| `POST` | `/api/{api_version}/server/{admin_path}/ssl/verify` |
| `GET` | `/api/{api_version}/server/{admin_path}/stats` |
| `GET` | `/api/{api_version}/server/{admin_path}/status` |
| `GET` | `/api/{api_version}/server/{admin_path}/status/health` |
| `GET` | `/api/{api_version}/server/{admin_path}/users` |
| `GET` | `/api/{api_version}/server/{admin_path}/users/invites` |
| `POST` | `/api/{api_version}/server/{admin_path}/users/invites` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/users/invites/{id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/users/invites/{id}` |
| `POST` | `/api/{api_version}/server/{admin_path}/users/settings` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/users/{id}` |
| `PUT` | `/api/{api_version}/server/{admin_path}/users/{id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/web` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/web` |
| `GET` | `/api/{api_version}/server/{admin_path}/web/cors` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/web/cors` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/web/feeds` |
| `GET` | `/api/{api_version}/server/{admin_path}/web/robots` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/web/robots` |
| `GET` | `/api/{api_version}/server/{admin_path}/web/robots/preview` |
| `GET` | `/api/{api_version}/server/{admin_path}/web/security` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/web/security` |
| `GET` | `/api/{api_version}/server/{admin_path}/web/security/preview` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/count` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/preferences` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/preferences` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/read` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/send` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/stats` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/unread` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/{id}` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/{id}/dismiss` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/notifications/{id}/read` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/preferences` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/preferences` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile` |
| `PATCH` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/password` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys` |
| `DELETE` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/passkeys/{passkey_id}` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/totp` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/totp/disable` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/totp/enable` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/totp/recovery/regenerate` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/security/totp/setup` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/sessions` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/sessions/logout-all` |
| `GET` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/token` |
| `POST` | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/token` |
| `GET` | `/api/{api_version}/severe-weather` |
| `GET` | `/api/{api_version}/severe-weather/{id}` |
| `GET` | `/api/{api_version}/sun` |
| `GET` | `/api/{api_version}/users` |
| `PATCH` | `/api/{api_version}/users` |
| `DELETE` | `/api/{api_version}/users/account` |
| `DELETE` | `/api/{api_version}/users/avatar` |
| `GET` | `/api/{api_version}/users/avatar` |
| `PATCH` | `/api/{api_version}/users/avatar` |
| `POST` | `/api/{api_version}/users/avatar` |
| `GET` | `/api/{api_version}/users/locations` |
| `POST` | `/api/{api_version}/users/locations` |
| `DELETE` | `/api/{api_version}/users/locations/{id}` |
| `GET` | `/api/{api_version}/users/locations/{id}` |
| `PUT` | `/api/{api_version}/users/locations/{id}` |
| `PUT` | `/api/{api_version}/users/locations/{id}/alerts` |
| `GET` | `/api/{api_version}/users/notifications` |
| `GET` | `/api/{api_version}/users/notifications/count` |
| `GET` | `/api/{api_version}/users/notifications/preferences` |
| `PATCH` | `/api/{api_version}/users/notifications/preferences` |
| `PATCH` | `/api/{api_version}/users/notifications/read` |
| `GET` | `/api/{api_version}/users/notifications/stats` |
| `GET` | `/api/{api_version}/users/notifications/unread` |
| `DELETE` | `/api/{api_version}/users/notifications/{id}` |
| `PATCH` | `/api/{api_version}/users/notifications/{id}/dismiss` |
| `PATCH` | `/api/{api_version}/users/notifications/{id}/read` |
| `GET` | `/api/{api_version}/users/preferences` |
| `POST` | `/api/{api_version}/users/preferences` |
| `DELETE` | `/api/{api_version}/users/preferences/{id}` |
| `PUT` | `/api/{api_version}/users/preferences/{id}` |
| `GET` | `/api/{api_version}/users/security/2fa` |
| `POST` | `/api/{api_version}/users/security/2fa/disable` |
| `POST` | `/api/{api_version}/users/security/2fa/enable` |
| `GET` | `/api/{api_version}/users/security/2fa/setup` |
| `POST` | `/api/{api_version}/users/security/2fa/verify` |
| `POST` | `/api/{api_version}/users/security/email` |
| `GET` | `/api/{api_version}/users/security/passkeys` |
| `POST` | `/api/{api_version}/users/security/passkeys` |
| `DELETE` | `/api/{api_version}/users/security/passkeys/{passkey_id}` |
| `POST` | `/api/{api_version}/users/security/password` |
| `POST` | `/api/{api_version}/users/security/recovery/regenerate` |
| `DELETE` | `/api/{api_version}/users/sessions` |
| `GET` | `/api/{api_version}/users/sessions` |
| `DELETE` | `/api/{api_version}/users/sessions/{id}` |
| `GET` | `/api/{api_version}/users/settings` |
| `PATCH` | `/api/{api_version}/users/settings` |
| `GET` | `/api/{api_version}/users/subscriptions` |
| `POST` | `/api/{api_version}/users/subscriptions` |
| `PUT` | `/api/{api_version}/users/subscriptions/{id}` |
| `GET` | `/api/{api_version}/users/tokens` |
| `POST` | `/api/{api_version}/users/tokens` |
| `DELETE` | `/api/{api_version}/users/tokens/{id}` |
| `GET` | `/api/{api_version}/weather` |
| `GET` | `/api/{api_version}/weather/alerts` |
| `GET` | `/api/{api_version}/weather/forecast` |
| `GET` | `/api/{api_version}/weather/history` |
| `GET` | `/api/{api_version}/weather/locations` |
| `GET` | `/api/{api_version}/weather/moon` |
| `GET` | `/api/{api_version}/weather/{location}` |
| `POST` | `/mcp` |

**Syndication.** The primary public resource is the earthquake list (USGS events of
magnitude 2.5 and above from the last seven days, each linked to its detail page). The
Atom and RSS feeds always serve it, newest first, with plain-text titles and summaries;
a failed upstream read yields a valid empty feed. The sitemap lists only public pages
that `robots.txt` allows.

**Compatibility.** No external weather service's API is emulated. Feature
compatibility only. No RFC-defined protocol is implemented by the application
itself.

**Guest preferences (cookies, never stored server-side).**

| Cookie | Allowed values | Default | Export/import |
|--------|----------------|---------|---------------|
| `wthr_pref_units` | `metric`, `imperial` | region based | exported and imported with `theme` and `lang` |

### Weather capabilities

- **Current weather and forecast** for a city name, ZIP/postal code, coordinates or the visitor's detected location; hourly and daily breakdown; saved locations for signed-in users
- **Severe weather** — active alerts for the resolved location or by event type, single event detail
- **Earthquakes** — recent events, single event detail
- **Hurricanes** — active tropical storms with forecast tracks, single storm detail
- **Astronomy** — current moon phase, monthly moon calendar, sunrise and sunset
- **Location tools** — search by name, ZIP/postal code lookup, reverse geocoding of coordinates, detect location from the client IP
- **History** — recent weather history for a location
- **Real-time** — WebSocket stream of notifications for subscribed alerts
- **Accounts** — saved locations (up to 10), per-location alert subscriptions, email notifications, self-service profile and session management
- **Administration** — dashboard, settings, admins and users, security, logs, backup, SSL, scheduler, notifications, updates; reachable only by typing the admin URL, never linked from a public page
