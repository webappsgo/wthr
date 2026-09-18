# Weather Service

Welcome to the official documentation for **Weather Service**.

Weather Service is a production-grade weather API providing global weather forecasts, severe weather alerts, earthquake data, moon phase information, and hurricane tracking. Built with Go for performance and reliability.

## Features

- **Global Weather Forecasts** - 16-day forecasts for any location worldwide via Open-Meteo
- **Severe Weather Alerts** - Real-time alerts from US, Canada, UK, Australia, Japan, and Mexico
- **Earthquake Tracking** - Live USGS earthquake data with interactive maps
- **Hurricane Tracking** - Active storm tracking with NOAA advisories and forecasts
- **Moon Phases** - Detailed lunar information including phases, illumination, rise/set times
- **GeoIP Location** - Automatic location detection via IP address
- **Real-Time Notifications** - WebSocket-powered notification system
- **Mobile Responsive** - Optimized for desktop, tablet, and mobile
- **Single Static Binary** - No external dependencies, all assets embedded

## Quick Start

=== "Docker (Recommended)"

    ```bash
    docker run -d \
      --name wthr \
      -p 172.17.0.1:64580:80 \
      -v ./volumes/config:/config:z \
      -v ./volumes/data:/data:z \
      ghcr.io/webappsgo/wthr:latest
    ```

    The published port is bound to the Docker bridge address so a reverse
    proxy fronts the service instead of exposing it on every interface.

=== "Binary Installation"

    ```bash
    # Download latest release
    curl -q -LSsf -O https://github.com/webappsgo/wthr/releases/latest/download/wthr-linux-amd64
    chmod +x wthr-linux-amd64
    sudo mv wthr-linux-amd64 /usr/local/bin/wthr

    # Run the server
    wthr
    ```

=== "Docker Compose"

    ```yaml
    services:
      wthr:
        image: ghcr.io/webappsgo/wthr:latest
        ports:
          - "172.17.0.1:64580:80"
        volumes:
          - ./volumes/config:/config:z
          - ./volumes/data:/data:z
        restart: unless-stopped
    ```

## Documentation

- [Installation](installation.md) - Detailed installation instructions
- [Configuration](configuration.md) - Every configuration option
- [API Reference](api.md) - REST API, OpenAPI/Swagger UI, GraphQL
- [CLI](cli.md) - The `wthr-cli` client
- [Admin Panel](admin.md) - Manage the server via web UI
- [Security](security.md) - Authentication, public endpoints, and reporting
- [Integrations](integrations.md) - External identity and discovery endpoints
- [Development](development.md) - Building, testing, and contributing

## Links

- [GitHub Repository](https://github.com/webappsgo/wthr)
- [Docker Images](https://ghcr.io/webappsgo/wthr)
- [Report Issues](https://github.com/webappsgo/wthr/issues)
- Swagger UI - `/openapi` on your running server
- GraphQL Playground - `/graphql` on your running server

## License

MIT - see [LICENSE.md](https://github.com/webappsgo/wthr/blob/main/LICENSE.md).
