# Installation

This guide covers the supported install paths for Weather.

## Requirements

- Linux, macOS, Windows, or FreeBSD
- amd64 or arm64
- Docker for containerized deployment, or a host/service manager for native installs

## Docker

### Single Container

```bash
docker run -d \
  --name wthr \
  -p 64580:80 \
  -v ./rootfs/config:/config:z \
  -v ./rootfs/data:/data:z \
  ghcr.io/casapps/wthr:latest
```

### Docker Compose

```bash
curl -q -LSsf -O https://raw.githubusercontent.com/casapps/wthr/main/docker/docker-compose.yml
docker compose up -d
```

## Binary

### Download

```bash
curl -q -LSsf -O https://github.com/casapps/wthr/releases/latest/download/wthr-linux-amd64
chmod +x wthr-linux-amd64
sudo mv wthr-linux-amd64 /usr/local/bin/wthr
```

### Run

```bash
wthr
```

On first run, Weather generates `server.yml` in `{config_dir}` and creates its data/log directories automatically.

## Service Installation

### Linux systemd

Create a service that runs the binary in the foreground and lets Weather manage its runtime directories:

```ini
[Unit]
Description=Weather Service
After=network.target

[Service]
Type=simple
User=wthr
Group=wthr
ExecStart=/usr/local/bin/wthr --mode production
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Typical runtime locations follow AI.md PART 4:

- Config: `{config_dir}`
- Data: `{data_dir}`
- Logs: `{log_dir}`

### macOS / Windows / BSD

Use the platform-specific service helpers or native service manager with the same foreground command:

```bash
wthr --mode production
```

## Setup

The server is functional immediately on first run. Admin setup is completed through:

- `https://wthr.top/admin/server/setup`

After setup, use:

- Admin panel: `https://wthr.top/admin`
- Health: `https://wthr.top/healthz`
- OpenAPI: `https://wthr.top/openapi`

## Verification

```bash
wthr --version
wthr --status
curl -q -LSsf https://wthr.top/healthz
```

## Building From Source

Use the existing project targets:

```bash
make dev
make local
make build
make test
```

## Next Steps

- [Configuration](configuration.md)
- [Admin Panel](admin.md)
- [API Reference](api.md)
