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
| Multi-Format API | JSON, text/plain, GraphQL support |
| Terminal Output | ASCII/ANSI colored weather for curl/terminal |
| Bash Integration | Shell function for terminal weather (`/:bash.function`) |
| WebSocket Alerts | Real-time push notifications for severe weather |
| User Locations | Save favorite locations for quick access |
| Alert Subscriptions | Subscribe to alerts for specific regions |
| OpenAPI/Swagger | Full API documentation at `/swagger/` |
| i18n | English, Spanish, French, German, Arabic, Japanese, Chinese |

All weather data sources are free/public APIs — no API keys required. GeoIP databases are downloaded on first run and updated weekly by the scheduler (never embedded in the binary). Single binary deployment — no external dependencies.

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
- Backup/restore, scheduler, metrics, email notifications

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
| Server Admin | Manages the app configuration and admins | Passkey (WebAuthn) + session |
| Primary Admin | First admin created — cannot be deleted | Passkey (WebAuthn) + session |

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

Rate limits use the AI.md PART 11/12 defaults (reads 120/min, writes 10/min, health 120/min, global burst 240/min): per IP for anonymous callers and per token for authenticated callers. Every value is editable in the admin panel.

### Data model & sensitivity

**Databases:**
- `server.db` — app configuration, admin accounts, admin sessions, admin passkeys, audit log, scheduler state, metrics, cache state
- `users.db` — regular user accounts, user sessions, saved locations, alert subscriptions

**Sensitive data:**
- Admin passwords: Argon2id hash (time=3, mem=64MB, threads=4, keyLen=32)
- User passwords: Argon2id (same parameters)
- Session tokens: 32 bytes crypto/rand → stored as SHA-256 hash; raw token returned once
- Admin session tokens prefixed `adm:` before hashing
- SMTP password: stored encrypted in config
- No API keys (all weather sources are public)

**Cached data (non-sensitive):**
- Weather forecasts: 15-minute TTL in Valkey/memory
- Alert data: 5-minute TTL
- GeoIP lookups: in-memory for session duration

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
| Client IP address | Untrusted | Used only for GeoIP lookup; no other trust |
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
- Authenticated admin sessions (passkey-verified, `adm:`-prefixed token)
- Authenticated user sessions (password-verified token)
- Internal scheduler jobs

**Untrusted inputs:**
- All HTTP requests (headers, query params, body, path)
- All external API responses (Open-Meteo, NOAA, USGS, etc.)
- Client IP addresses
- User-supplied location strings

**Main attacker/abuser goals:**
1. Gain admin panel access (credential theft, session hijacking, passkey bypass)
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
| Credential theft | Argon2id, constant-time compare, no timing oracle |
| Session hijacking | 32-byte crypto/rand tokens, SHA-256 stored, HttpOnly Secure cookies |
| Passkey bypass | WebAuthn ceremony enforced server-side, pending token expires in 5 min |
| Account enumeration | Same message + timing for wrong-password vs no-such-user |
| Scraping | Rate limiting (AI.md defaults, per IP or per token), 429 + Retry-After |
| Malformed API response | Schema validation before use, never eval/exec |
| SSRF | Location input validated against allowed formats (city name, coordinates, ZIP); no URL input |
| Path traversal | Backup filenames sanitized; never use user input directly in file paths |
| DoS on expensive queries | Rate limit + pagination on all list endpoints; timeouts on all external calls |
| Alert spam | Subscription requires verified user account |

**Explicit non-goals (acceptable risk):**
- GeoIP is a convenience feature, not a security gate; VPN bypass is acceptable
- No MFA for regular users (passkey is admin-only)

### Security decisions & exceptions

1. **Admin auth is passkey-only** — no password fallback for admin login. This eliminates credential stuffing against the admin panel. The tradeoff (lockout if all passkeys lost) is acceptable; recovery via server console is documented.

2. **Regular user accounts use password auth** — passkey is not required for regular users because the user feature (PART 34) is optional and passkey enrollment UX adds friction for a low-stakes feature (saved locations, alert subscriptions).

3. **Anonymous weather access** — core weather/alert/earthquake/hurricane data is public with no auth required. Rate limiting is the only protection. This is intentional: weather data is public information.

4. **External API data is fetched server-side** — no client-side fetches to external APIs. This prevents CORS issues and keeps API call patterns server-controlled.

5. **Tor hidden service** — auto-enabled when `tor` binary is found. The .onion address is shown in admin panel. This is a privacy feature, not a security exception.

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
| IP Geolocation | sapics/ip-location-db | Monthly |

### Interfaces & compatibility

**Interfaces.** Every feature is available as a web page (HTML for browsers, plain
text for curl and terminals), as a versioned JSON API, as GraphQL and OpenAPI, and
through the CLI client. An MCP server (Streamable HTTP, public weather operations
as tools) lets AI assistants use the service; it can be switched off in the admin
panel. The standard server pages (about, help, MCP help, privacy, terms, contact,
security reporting, data protection officer contact, health, preferences) have
real content derived from this document.

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
