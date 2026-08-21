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
      -p 64580:80 \
      -v ./volumes/config:/config:z \
      -v ./volumes/data:/data:z \
      ghcr.io/webappsgo/wthr:latest
    ```

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
          - "64580:80"
        volumes:
          - ./volumes/config:/config:z
          - ./volumes/data:/data:z
        restart: unless-stopped
    ```

## Next Steps

- [Installation Guide](installation.md) - Detailed installation instructions
- [Configuration](configuration.md) - Configure the weather service
- [API Reference](api.md) - Use the RESTful JSON API
- [Admin Panel](admin.md) - Manage the server via web UI

## Links

- [GitHub Repository](https://github.com/webappsgo/wthr)
- [Docker Images](https://ghcr.io/webappsgo/wthr)
- [Report Issues](https://github.com/webappsgo/wthr/issues)
