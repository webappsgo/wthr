# API Reference

Weather Service provides a RESTful JSON API for accessing weather data, managing locations, and more.

## Base URL

```
https://wthr.top/api/v1
```

The versioned API root is `GET /api/v1/`. It returns the service and current API
version plus links to the API documentation and server endpoints.

## Authentication

Most endpoints are public and don't require authentication.

Tokens are presented one way on every API route: `Authorization: Bearer {token}`. Other
headers (`X-API-Key`, `X-Auth-Token`, Basic, Digest) and a `?token=` query parameter are
ignored, so a request that uses them is anonymous. A missing, invalid, expired or revoked token
returns `401` with `WWW-Authenticate: Bearer realm="wthr"` (plus `error="invalid_token"` for an
invalid, expired or revoked token). A valid token without the needed scope returns `403`.

Browser sessions use a cookie; the API never needs or checks a CSRF token.

### Authentication Flow

1. **Login** - `POST /api/v1/server/auth/login`
2. **Access Protected Endpoints** - `Authorization: Bearer {token}` (API) or the session cookie (browser)
3. **Logout** - `POST /api/v1/server/auth/logout`

### Server information endpoints

| Route | Description |
|-------|-------------|
| `GET /api/v1/server/about`, `/privacy`, `/terms`, `/help` | Page content as JSON |
| `GET /api/v1/server/help/mcp` | How to connect an MCP client (404 when MCP is disabled) |
| `GET /api/v1/server/dpo` | Data protection officer contact: `name`, `email`, `privacy_url` |
| `GET /api/v1/server/security` | Security contacts, expiry, PGP state, policy and thanks links |
| `GET /api/v1/server/security/policy` | Disclosure policy (`policy`, Markdown) |
| `GET /api/v1/server/security/thanks` | Acknowledgments (`entries`) |
| `GET /api/v1/server/contact` | Contact form description: conditional `fields`, offered `types`, a fresh `form_token`, `captcha` (`{provider, captcha_id, question}` for the built-in challenge; provider configurations also include a built-in `fallback`), `security_url`, and `abuse_email` |
| `POST /api/v1/server/contact` | Two calls: `step=submit` sends the fields, `form_token`, and one CAPTCHA proof and returns `review: {type, email, subject}`, `review_token`, and `confirm_after_seconds` without delivery; `step=confirm` repeats the fields with that token and `confirmed: true`, then delivers. A valid `security_id` selects the coordinated-disclosure fields and follows the same review flow. |
| `GET /api/v1/server/preferences/sync` | Portable guest-preference export as JSON, text, or an HTML sync page, depending on `Accept` |
| `GET /api/v1/server/preferences/sync/import` | Import preferences from URL query parameters or a `code` |
| `POST /api/v1/server/preferences/sync` | Import preferences from submitted form/query values or a `code` |
| `GET /api/v1/server/auth/check` | Token check: `200` with `{"kind": "operator" \| "owner" \| "user"}` for a valid `Authorization: Bearer` token, `401` otherwise; no side effects, the token is never echoed |
| `POST /api/v1/server/mcp` | MCP endpoint (Streamable HTTP); aliases `/api/mcp` and `/mcp` |

### Server Admin database maintenance

`POST /api/v1/server/{admin_path}/database/vacuum` enqueues a one-off
`database_vacuum` maintenance task for local SQLite databases and returns `202 Accepted`.
The request does not run `VACUUM`; the disabled-by-default scheduler task performs it in the
background outside a transaction. The route is unavailable for remote databases.

The contact form checks the signed, single-use `form_token` and CAPTCHA on the first call only.
The built-in challenge is the default (`captcha_id` plus `captcha_answer`); with `recaptcha` or
`hcaptcha` configured, send the provider token as `captcha_response`. The second call does not
repeat CAPTCHA: the signed review token binds the confirmed values. Origin checks apply to browser
form posts; the JSON API is not subject to browser-origin checks. Content, link, rate, duplicate,
and honeypot rules run before delivery. Failed validation returns `400 VALIDATION_FAILED`; rate
limits return `429` with `Retry-After`.

Contact field limits count Unicode characters after trimming: `name` 100, `email` 254,
`subject` 200, `message` 5000.

### Fixed-format files

These routes are plain `GET` and `HEAD` files with no content negotiation, built per request with
absolute URLs for the host the client used.

| Route | Content-Type | Description |
|-------|--------------|-------------|
| `/robots.txt` | `text/plain` | Generated from `web.robots.allow` and `web.robots.deny`, with the `Sitemap:` line |
| `/sitemap.xml` | `application/xml` | Public frontend pages that `robots.txt` allows; always served |
| `/feed.xml` | `application/atom+xml` | Atom 1.0 feed of the most recent earthquakes (magnitude 2.5 and above); always served |
| `/rss.xml` | `application/rss+xml` | The same items as RSS 2.0; always served |

Both feeds carry `ETag` and `Last-Modified`, answer `304` to a matching conditional request and set
`Cache-Control: public, max-age=300`. The number of entries (`server.seo.feed.max_items`, 1 to 200,
default 50) and the feed title (`server.seo.feed.title`) are set in the admin panel under Web.

## API Endpoints

### Admin profile report key

These authenticated self-service routes use the admin username in the path:

| Method and route | Description |
|------------------|-------------|

The key is public material and is used to encrypt maintainer security-report notifications sent to
the registering admin.

### Weather Endpoints

#### Get Current Weather

Get current weather for client's IP location or specific location.

```http
GET /api/v1/weather
GET /api/v1/weather/:location
```

**Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `location` | string | Location name, coordinates (lat,lon), or ZIP code |

**Response:**

```json
{
  "location": {
    "name": "New York",
    "lat": 40.7128,
    "lon": -74.0060,
    "country": "US",
    "timezone": "America/New_York"
  },
  "current": {
    "temperature": 72.5,
    "feels_like": 70.2,
    "humidity": 65,
    "pressure": 1013.25,
    "wind_speed": 10.5,
    "wind_direction": 180,
    "description": "Partly cloudy",
    "icon": "partly_cloudy",
    "timestamp": "2025-01-15T14:30:00Z"
  }
}
```

#### Get Weather Forecast

Get 16-day weather forecast.

```http
GET /api/v1/forecasts
GET /api/v1/forecasts/:location
```

**Response:**

```json
{
  "location": {
    "name": "New York",
    "lat": 40.7128,
    "lon": -74.0060
  },
  "forecast": [
    {
      "date": "2025-01-15",
      "temperature_max": 75.0,
      "temperature_min": 62.0,
      "precipitation": 0.2,
      "humidity": 70,
      "wind_speed": 12.0,
      "description": "Partly cloudy"
    }
  ]
}
```

#### Get Historical Weather

Get historical weather data.

```http
GET /api/v1/history
```

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `lat` | float | Yes | Latitude |
| `lon` | float | Yes | Longitude |
| `start_date` | string | Yes | Start date (YYYY-MM-DD) |
| `end_date` | string | Yes | End date (YYYY-MM-DD) |

**Example:**

```bash
curl -q -LSsf "https://wthr.top/api/v1/history?lat=40.7128&lon=-74.0060&start_date=2025-01-01&end_date=2025-01-07"
```

### Natural Events

#### Get Earthquakes

Get recent earthquake data from USGS.

```http
GET /api/v1/earthquakes
```

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `min_magnitude` | float | 2.5 | Minimum magnitude |
| `days` | int | 7 | Days of history |

**Response:**

```json
{
  "earthquakes": [
    {
      "id": "us6000abcd",
      "magnitude": 5.2,
      "location": "10 km E of Los Angeles, CA",
      "lat": 34.05,
      "lon": -118.24,
      "depth": 15.3,
      "time": "2025-01-15T12:34:56Z",
      "url": "https://earthquake.usgs.gov/earthquakes/eventpage/us6000abcd"
    }
  ],
  "count": 15
}
```

#### Get Severe Weather Alerts

Get severe weather alerts for a location.

```http
GET /api/v1/severe-weather
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `lat` | float | Latitude |
| `lon` | float | Longitude |
| `country` | string | Country code (US, CA, UK, AU, JP, MX) |

**Response:**

```json
{
  "alerts": [
    {
      "type": "tornado_warning",
      "severity": "extreme",
      "headline": "Tornado Warning",
      "description": "A tornado warning has been issued...",
      "start": "2025-01-15T14:00:00Z",
      "end": "2025-01-15T16:00:00Z",
      "affected_areas": ["New York County", "Kings County"]
    }
  ],
  "count": 3
}
```

#### Get Moon Phase

Get moon phase and lunar information.

```http
GET /api/v1/moon
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `date` | string | Date (YYYY-MM-DD), defaults to today |
| `lat` | float | Latitude for rise/set times |
| `lon` | float | Longitude for rise/set times |

**Response:**

```json
{
  "phase": "waxing_gibbous",
  "illumination": 0.73,
  "age": 10.5,
  "distance": 384400,
  "next_full_moon": "2025-01-20T12:00:00Z",
  "next_new_moon": "2025-02-03T08:00:00Z",
  "moonrise": "2025-01-15T15:30:00Z",
  "moonset": "2025-01-16T04:15:00Z"
}
```

### Location Endpoints

#### Search Locations

Search for locations by name.

```http
GET /api/v1/locations/search
```

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `limit` | int | No | Result limit (default: 10) |

**Response:**

```json
{
  "results": [
    {
      "name": "New York",
      "lat": 40.7128,
      "lon": -74.0060,
      "country": "US",
      "state": "NY",
      "population": 8336817
    }
  ]
}
```

#### Lookup by ZIP Code

```http
GET /api/v1/locations/lookup/zip/:code
```

**Example:**

```bash
curl -q -LSsf https://wthr.top/api/v1/locations/lookup/zip/10001
```

#### Lookup by Coordinates

```http
GET /api/v1/locations/lookup/coords
```

**Query Parameters:**

| Parameter | Type | Required |
|-----------|------|----------|
| `lat` | float | Yes |
| `lon` | float | Yes |

### User Location Management

Requires authentication.

#### List Saved Locations

```http
GET /api/v1/users/locations
```

**Response:**

```json
{
  "locations": [
    {
      "id": 1,
      "name": "Home",
      "lat": 40.7128,
      "lon": -74.0060,
      "alerts_enabled": true,
      "created_at": "2025-01-15T10:00:00Z"
    }
  ]
}
```

#### Save Location

```http
POST /api/v1/users/locations
```

**Request Body:**

```json
{
  "name": "Home",
  "lat": 40.7128,
  "lon": -74.0060,
  "alerts_enabled": true
}
```

#### Update Location

```http
PUT /api/v1/users/locations/:id
```

#### Delete Location

```http
DELETE /api/v1/users/locations/:id
```

### Authentication Endpoints

#### Login

```http
POST /api/v1/server/auth/login
```

**Request Body:**

```json
{
  "username": "user@example.com",
  "password": "REDACTED_EXAMPLE"
}
```

**Response:**

```json
{
  "ok": true,
  "data": {
    "user": {
      "id": 1,
      "username": "user@example.com",
      "email": "user@example.com"
    }
  }
}
```

#### Logout

```http
POST /api/v1/server/auth/logout
```

#### Register

```http
POST /api/v1/server/auth/register
```

**Request Body:**

```json
{
  "username": "newuser",
  "email": "newuser@example.com",
  "password": "SecurePassword123!"
}
```

### Utility Endpoints

#### Get Client IP

```http
GET /api/v1/ip
```

**Response:**

```json
{
  "ip": "203.0.113.45",
  "country": "US",
  "city": "New York",
  "latitude": 40.7128,
  "longitude": -74.0060
}
```

#### Get GeoIP Location

```http
GET /api/v1/weather/locations
```

Returns location for client's IP address.

#### Health Check

```http
GET /server/healthz
GET /api/v1/server/healthz
GET /api/healthz
GET /healthz
```

`/server/healthz` is the canonical route; `/api/{api_version}/server/healthz`
and `/api/healthz` are its API counterparts. The root `/healthz` alias is
mounted only when `server.healthz.root.enabled` is true (default: false).

**Response (abridged):**

```json
{
  "project": {"name": "Weather", "tagline": "", "description": ""},
  "status": "healthy",
  "version": "1.0.0",
  "go_version": "go1.25.0",
  "build": {"commit": "abc1234", "date": "2025-01-15T12:00:00Z"},
  "uptime": "1d 2h 5m",
  "mode": "production",
  "timestamp": "2025-01-15T14:30:00Z",
  "cluster": {"enabled": false},
  "features": {
    "tor": {"enabled": true, "running": true, "status": "healthy", "hostname": "{56 characters}.onion"},
    "i2p": {"enabled": false, "running": false, "status": "disabled", "hostname": "", "provider": "none"},
    "geoip": true,
    "multi_user": true
  },
  "checks": {"database": "ok", "cache": "ok", "disk": "ok", "scheduler": "ok", "tor": "ok"},
  "stats": {"requests_total": 0, "requests_24h": 0, "active_connections": 0}
}
```

The overall `status` is `healthy`, `degraded`, `restart_required`, `maintenance`, `shutting_down` or `unhealthy`. It is `unhealthy` (HTTP 503) only when the database or disk fails or the server is still initializing.

**Tor and I2P status.** `features.tor.status` and `features.i2p.status` are always exactly one of four words, never free text. No error message, path or exit code appears in a health response; the detail goes to the server log.

| `status` | `enabled` | `running` | `hostname` | `checks.tor` / `checks.i2p` | Meaning |
|----------|-----------|-----------|------------|-----------------------------|---------|
| `disabled` | `false` | `false` | empty | omitted | Not active on this instance: no `tor` binary, or I2P not opted in or no provider found |
| `starting` | `true` | `false` | empty | `ok` | The provider was launched and is bootstrapping |
| `healthy` | `true` | `true` | the address | `ok` | Serving at `hostname` |
| `unavailable` | `true` | `false` | empty | `degraded` | The provider exists but is not serving (failed to start, bootstrap timeout, provider exited) |

A missing `tor` binary is `disabled`, never an error. An `unavailable` network makes the overall status `degraded` (HTTP 200), never `unhealthy`.

### Operator Token

Admin session or admin token required. The value is never returned by a read.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/server/{admin_path}/security/operator-token` | The masked operator token and whether one is configured |
| `POST` | `/api/v1/server/{admin_path}/security/operator-token` | Regenerate it. Body `{"current_password": "..."}`; the new value is returned once in `data.token` and the previous value stops working at once |

The operator token itself is presented as `Authorization: Bearer tok_...` on the `/debug/*` endpoints.

### Rate limit settings

These server admin API routes require an authenticated admin session or an authorized server admin
API token. Rate limiting is always on; `PATCH` changes thresholds and windows only.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/server/{admin_path}/security/ratelimit` | Read current rate limit settings |
| `PATCH` | `/api/v1/server/{admin_path}/security/ratelimit` | Update all configured rate limit thresholds and windows |

The update body accepts `global_burst`, `read_requests`, `read_window`, `write_requests`,
`write_window`, `health_requests`, `health_window`, `login_requests`, `login_window`,
`password_reset_requests`, `password_reset_window`, `registration_requests`, and
`registration_window`, each as a positive integer.

### Firewall and IP blocks

These routes are under `/api/v1/server/{admin_path}/security/` and require an authenticated server
admin session or an authorized server admin API token. Manual IP/CIDR blocks support `temporary` (one hour by default), `extended` (24
hours), and `permanent` durations. A future RFC 3339 `expires_at` may replace a temporary or
extended default.

| Method and route | Description |
|------------------|-------------|
| `GET /blocked-ips` | List IP and CIDR blocks, including expired records |
| `POST /blocked-ips` | Add a block with `ip` or `cidr`, `type`, and `reason` |
| `GET /blocked-ips/{ip}` | Get one block record |
| `DELETE /blocked-ips/{ip}` | Remove one block |
| `DELETE /blocked-ips/expired` | Purge expired block records |
| `GET /allowlist` | List trusted IP/CIDR entries |
| `POST /allowlist` | Add `{ "cidr": "203.0.113.0/24", "description": "office" }` |
| `DELETE /allowlist/{id}` | Remove an entry by its stable identifier |
| `GET /allowlist/check/{ip}` | Check whether an IP matches a trusted entry |

Trusted entries bypass IP blocks, rate limits, and GeoIP country blocking. They do not bypass
authentication, API token validation, or CSRF protection.

## Content Negotiation

`/api/**` routes pick the response format in this order:

1. `.txt` extension: plain text.
2. `Accept: application/json`: JSON, even for curl or wget.
3. `Accept: text/plain`: plain text.
4. A non-interactive client (curl, wget, httpie, empty User-Agent) that sent no explicit `Accept`: plain text.
5. Everything else: JSON.

Frontend routes (`/`, `/server/*`) follow a similar order: `Accept: application/json` returns the same data as the API route, `Accept: text/html` returns HTML, `Accept: text/plain` returns formatted text, browsers get HTML and curl, wget and httpie get the page rendered as formatted terminal text. `/server/healthz`, `/healthz` and `/api/{api_version}/server/healthz` share one health result, so the JSON is identical on all three.

Text responses on API routes use one mapping. A scalar is the bare value. An object is `OK: {message}` followed by `key: value` lines in field order (nested keys joined with dots, a list of scalars on one line). A list is `OK: {n} items found` followed by one item per line, or one block of `key: value` lines per object separated by a blank line. An error is `ERROR: {code}: {message}` with the same HTTP status as the JSON error.

```bash
curl -q -LSsf -H 'Accept: application/json' https://wthr.example.com/server/healthz
curl -q -LSsf https://wthr.example.com/server/healthz
curl -q -LSsf https://wthr.example.com/api/v1/server/healthz.txt
```

## Rate Limiting

API endpoints are rate-limited:

| Endpoint Type | Default limit |
|---------------|---------------|
| Reads (GET, HEAD) | 120 requests/minute |
| Writes (POST, PUT, PATCH, DELETE) | 10 requests/minute |
| Health | 120 requests/minute |
| Global burst | 240 requests/minute |

Anonymous requests are limited per IP. Requests with a valid Bearer token are limited per
token, so clients that share an egress IP do not throttle each other. Failed authentication is
always counted per IP. Every limit is editable in the admin panel.

Rate limit headers:

```http
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1642252800
```

When rate limited, the response uses the canonical error shape with a
`RATE_LIMITED` code. Retry timing is returned in both the `Retry-After`
HTTP header and the JSON `retry_after` field:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 30
```

```json
{
  "ok": false,
  "error": "RATE_LIMITED",
  "message": "Too many requests. Please try again later.",
  "retry_after": 30
}
```

## Error Responses

Standard error format:

```json
{
  "ok": false,
  "error": "ERROR_CODE",
  "message": "Human-readable error message",
  "details": {}
}
```

HTTP status codes:

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 429 | Rate Limited |
| 500 | Server Error |

## Examples

### Using cURL

```bash
# Get current weather
curl -q -LSsf https://wthr.top/api/v1/weather

# Get weather for New York
curl -q -LSsf https://wthr.top/api/v1/weather/New%20York

# Get forecast
curl -q -LSsf "https://wthr.top/api/v1/forecasts?lat=40.7128&lon=-74.0060"

# Get earthquakes
curl -q -LSsf "https://wthr.top/api/v1/earthquakes?min_magnitude=4.0"

# Login
curl -q -LSsf -X POST https://wthr.top/api/v1/server/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"user","password":"pass"}' \
  -c cookies.txt

# Use authenticated endpoint
curl -q -LSsf https://wthr.top/api/v1/users/locations \
  -b cookies.txt
```

### Using JavaScript

```javascript
// Get current weather
fetch('https://wthr.top/api/v1/weather')
  .then(res => res.json())
  .then(data => console.log(data));

// Get forecast
fetch('https://wthr.top/api/v1/forecasts/New%20York')
  .then(res => res.json())
  .then(data => console.log(data.forecast));

// Login
fetch('https://wthr.top/api/v1/server/auth/login', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({username: 'user', password: 'pass'})
})
  .then(res => res.json())
  .then(data => console.log(data));
```

### Using Python

```python
import requests

# Get current weather
response = requests.get('https://wthr.top/api/v1/weather')
weather = response.json()
print(weather)

# Get forecast
response = requests.get('https://wthr.top/api/v1/forecasts',
                       params={'lat': 40.7128, 'lon': -74.0060})
forecast = response.json()
print(forecast)

# Login
session = requests.Session()
response = session.post('https://wthr.top/api/v1/server/auth/login',
                       json={'username': 'user', 'password': 'pass'})
print(response.json())

# Use authenticated endpoint
response = session.get('https://wthr.top/api/v1/users/locations')
locations = response.json()
print(locations)
```

## API Versioning

Current API version: `v1`

The API version is included in the URL path: `/api/v1/`

Future versions will be available at `/api/v2/`, etc.

## Next Steps

- [Admin Panel](admin.md) - Manage the service via web UI
- [CLI Reference](cli.md) - Use command-line tools
- [Configuration](configuration.md) - Configure API settings
