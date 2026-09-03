# Weather Service

[![Build](https://github.com/webappsgo/wthr/actions/workflows/ci.yml/badge.svg)](https://github.com/webappsgo/wthr/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/webappsgo/wthr)](https://github.com/webappsgo/wthr/releases)
[![Documentation](https://readthedocs.org/projects/webappsgo-wthr/badge/?version=latest)](https://webappsgo-wthr.readthedocs.io)
[![License](https://img.shields.io/github/license/webappsgo/wthr)](LICENSE.md)

## About

A unified weather information platform that aggregates global meteorological data from authoritative sources into a single, accessible service for developers, businesses, and end users.

Weather Service combines forecasts, severe weather alerts, earthquakes, hurricanes, and lunar data from 10+ government agencies worldwide through one consistent API and web interface.

## Official Site

https://wthr.top

## Features

- Global weather forecasts (16-day) via Open-Meteo
- Severe weather alerts from 6 countries (NOAA, Environment Canada, UK Met Office, BOM, JMA, CONAGUA)
- Real-time earthquake monitoring via USGS
- Hurricane/tropical storm tracking via NOAA NHC
- Moon phase calculations
- Automatic IP-based location detection
- WebSocket real-time notifications
- Single static binary with all assets embedded
- Multi-platform (Linux, macOS, Windows, FreeBSD - amd64/arm64)
- Production grade with rate limiting, caching, health checks
- Tor hidden service (always-on when the Tor binary is found)
- I2P eepsite support (opt-in, admin-configurable)
- User accounts with saved locations and alert subscriptions (password auth, optional TOTP/passkey 2FA)
- Guest and per-user theme/language preferences (`/server/preferences`)

## Production

### Docker (Recommended)

```bash
docker run -d \
  --name wthr \
  -p 64580:80 \
  -v ./volumes/config:/config:z \
  -v ./volumes/data:/data:z \
  ghcr.io/webappsgo/wthr:latest
```

### Docker Compose

```bash
curl -q -LSsf -O https://raw.githubusercontent.com/webappsgo/wthr/main/docker/docker-compose.yml
docker compose up -d
```

### Binary

```bash
# Download latest release
curl -q -LSsf -O https://github.com/webappsgo/wthr/releases/latest/download/wthr-linux-amd64

# Make executable and run
chmod +x wthr-linux-amd64
./wthr-linux-amd64
```

### Systemd Service

```bash
# Download and install
sudo mv wthr-linux-amd64 /usr/local/bin/wthr
sudo useradd -r -s /bin/false wthr
sudo mkdir -p /var/lib/webappsgo/wthr/{data,config}
sudo chown -R wthr:wthr /var/lib/webappsgo/wthr

# Create service
sudo tee /etc/systemd/system/wthr.service > /dev/null <<EOF
[Unit]
Description=Wthr API Service
After=network.target

[Service]
Type=simple
User=wthr
ExecStart=/usr/local/bin/wthr
Restart=always

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now wthr
```

## CLI Client

A companion CLI client is available for interacting with the server API.

### Install

```bash
# Download latest release
curl -q -LSsf -O https://github.com/webappsgo/wthr/releases/latest/download/wthr-cli-linux-amd64
chmod +x wthr-cli-linux-amd64
sudo mv wthr-cli-linux-amd64 /usr/local/bin/wthr-cli
```

### Configure

```bash
# Connect to official server (creates ~/.config/webappsgo/wthr/cli.yml)
wthr-cli --server https://wthr.top --token YOUR_API_TOKEN
```

### Usage

```bash
wthr-cli --help
wthr-cli weather Brooklyn,NY
wthr-cli severe-weather
wthr-cli moon
```

## Configuration

Configuration is auto-generated on first run. Edit via admin panel at `https://wthr.top/admin`.

Key settings:
- `server.port` - Listen port (auto-selected in the 64000-64999 range on first run, then persisted; set explicitly to override)
- `server.address` - Listen address (default: `[::]`)
- `server.mode` - production or development

Configuration file: `$CONFIG_DIR/server.yml`

```yaml
server:
  port: 0
  address: "[::]"
  mode: production
  admin_path: admin
  api_version: v1

weather:
  forecast_days: 16
  update_interval: 900
  location_search_enabled: true
```

## API

Full API documentation is available at `https://wthr.top/openapi`, with OpenAPI JSON at `https://wthr.top/openapi.json` and GraphQL at `https://wthr.top/api/v1/graphql`.

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET https://wthr.top/server/healthz` | Frontend health/status route |
| `GET https://wthr.top/api/v1/server/healthz` | API health route (unversioned alias: `/api/healthz`) |

### Weather Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/weather` | Current weather (auto-detect location) |
| `GET /api/v1/weather/:location` | Weather for specific location |
| `GET /api/v1/weather/forecast` | Multi-day forecast (up to 16 days) |
| `GET /api/v1/ip` | Client IP and geolocation info |
| `GET /api/v1/weather/locations` | Auto-detect location from IP |

### Severe Weather & Natural Events

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/severe-weather` | Active severe weather alerts |
| `GET /api/v1/severe-weather/:id` | Get specific alert by ID |
| `GET /api/v1/earthquakes` | Recent earthquake data (USGS) |
| `GET /api/v1/earthquakes/:id` | Get specific earthquake by ID |
| `GET /api/v1/hurricanes` | Active hurricanes (deprecated) |

### Astronomical Data

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/moon` | Moon phase, illumination, rise/set |
| `GET /api/v1/moon/calendar` | Moon phases for a month |
| `GET /api/v1/sun` | Sunrise, sunset, day length |

### Location Management (authenticated)

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/users/locations` | List saved locations |
| `POST /api/v1/users/locations` | Save a new location |
| `GET /api/v1/locations/search` | Search for locations |

### Examples

```bash
# Weather for location
curl -q -LSsf https://wthr.top/api/v1/weather/Brooklyn,NY

# Severe weather
curl -q -LSsf https://wthr.top/api/v1/severe-weather

# Moon phase
curl -q -LSsf https://wthr.top/api/v1/moon
```

## Other

### Data Sources

- **Weather**: [Open-Meteo](https://open-meteo.com/)
- **US Alerts**: [NOAA NWS](https://www.weather.gov/)
- **Hurricanes**: [NOAA NHC](https://www.nhc.noaa.gov/)
- **Earthquakes**: [USGS](https://earthquake.usgs.gov/)
- **GeoIP**: [sapics/ip-location-db](https://github.com/sapics/ip-location-db)

### Troubleshooting

- Check logs: `docker logs wthr`
- Health check: `curl -q -LSsf https://wthr.top/server/healthz`
- Port in use: `netstat -tulpn | grep :80`

### Support

- Documentation: https://webappsgo-wthr.readthedocs.io
- Issues: https://github.com/webappsgo/wthr/issues

## Development

**Development instructions are for contributors only.**

### Prerequisites

- Docker (for containerized builds)
- Make

### Build

```bash
# Clone
git clone https://github.com/webappsgo/wthr
cd wthr

# Quick dev build (outputs to binaries/)
make dev

# Full build (all platforms, outputs to binaries/)
make build

# Test
make test
```

### Project Structure

```
src/           # Source code
tests/         # Test files
docker/        # Docker files
binaries/      # Built binaries (gitignored)
```

## Disclaimer

This software is provided "as is" without warranty of any kind. Use at your own risk.

- **No Warranty**: The authors are not responsible for any damages, data loss, or issues arising from use of this software
- **Not Professional Advice**: Weather data is provided for informational purposes only and should not be used as the sole basis for safety decisions during severe weather
- **Third-Party Services**: This software aggregates data from external APIs (Open-Meteo, NOAA, USGS, etc.) - their terms of service and data accuracy apply separately
- **Security**: While we strive to follow security best practices, no software is guaranteed to be free of vulnerabilities
- **Production Use**: Evaluate thoroughly before deploying in production environments

By using this software, you acknowledge that you have read and understood this disclaimer.

## License

MIT - See [LICENSE.md](LICENSE.md)
