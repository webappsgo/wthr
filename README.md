# Weather Service

[![Build](https://github.com/apimgr/weather/actions/workflows/build.yml/badge.svg)](https://github.com/apimgr/weather/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/apimgr/weather)](https://github.com/apimgr/weather/releases)
[![Documentation](https://readthedocs.org/projects/apimgr-weather/badge/?version=latest)](https://apimgr-weather.readthedocs.io)
[![License](https://img.shields.io/github/license/apimgr/weather)](LICENSE.md)

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

## Production

### Docker (Recommended)

```bash
docker run -d \
  --name weather \
  -p 64580:80 \
  -v ./volumes/config:/config:z \
  -v ./volumes/data:/data:z \
  ghcr.io/apimgr/weather:latest
```

### Docker Compose

```bash
curl -q -LSsf -O https://raw.githubusercontent.com/apimgr/weather/main/docker/docker-compose.yml
docker compose up -d
```

### Binary

```bash
# Download latest release
curl -q -LSsf -O https://github.com/apimgr/weather/releases/latest/download/weather-linux-amd64

# Make executable and run
chmod +x weather-linux-amd64
./weather-linux-amd64
```

### Systemd Service

```bash
# Download and install
sudo mv weather-linux-amd64 /usr/local/bin/weather
sudo useradd -r -s /bin/false weather
sudo mkdir -p /var/lib/weather/{data,config}
sudo chown -R weather:weather /var/lib/weather

# Create service
sudo tee /etc/systemd/system/weather.service > /dev/null <<EOF
[Unit]
Description=Weather API Service
After=network.target

[Service]
Type=simple
User=weather
ExecStart=/usr/local/bin/weather
Restart=always

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now weather
```

## CLI Client

A companion CLI client is available for interacting with the server API.

### Install

```bash
# Download latest release
curl -q -LSsf -O https://github.com/apimgr/weather/releases/latest/download/weather-cli-linux-amd64
chmod +x weather-cli-linux-amd64
sudo mv weather-cli-linux-amd64 /usr/local/bin/weather-cli
```

### Configure

```bash
# Connect to official server (creates ~/.config/apimgr/weather/cli.yml)
weather-cli --server https://wthr.top --token YOUR_API_TOKEN
```

### Usage

```bash
weather-cli --help
weather-cli weather Brooklyn,NY
weather-cli severe-weather
weather-cli moon
```

## Configuration

Configuration is auto-generated on first run. Edit via admin panel at `https://wthr.top/admin`.

Key settings:
- `server.address` - Listen address (default: :80)
- `server.mode` - production or development

Configuration file: `$CONFIG_DIR/server.yml`

```yaml
server:
  address: ":80"
  mode: production

logging:
  level: info
  format: json

geoip:
  update_interval: 168h

cache:
  weather_ttl: 15m
  severe_ttl: 5m
```

## API

Full API documentation is available at `https://wthr.top/openapi`, with OpenAPI JSON at `https://wthr.top/openapi.json` and GraphQL at `https://wthr.top/api/v1/graphql`.

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET https://wthr.top/healthz` | Frontend health/status route |
| `GET https://wthr.top/api/v1/healthz` | API health route |

### Weather Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/weather` | Current weather (auto-detect location) |
| `GET /api/v1/weather/:location` | Weather for specific location |
| `GET /api/v1/forecast` | Multi-day forecast (up to 16 days) |
| `GET /api/v1/ip` | Client IP and geolocation info |
| `GET /api/v1/location` | Auto-detect location from IP |

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
| `GET /api/v1/locations` | List saved locations |
| `POST /api/v1/locations` | Save a new location |
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

- Check logs: `docker logs weather`
- Health check: `curl -q -LSsf https://wthr.top/healthz`
- Port in use: `netstat -tulpn | grep :80`

### Support

- Documentation: https://apimgr-weather.readthedocs.io
- Issues: https://github.com/apimgr/weather/issues

## Development

**Development instructions are for contributors only.**

### Prerequisites

- Docker (for containerized builds)
- Make

### Build

```bash
# Clone
git clone https://github.com/apimgr/weather
cd weather

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
