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
  -v ./volumes/config:/config:z \
  -v ./volumes/data:/data:z \
  ghcr.io/webappsgo/wthr:latest
```

### Docker Compose

```bash
curl -q -LSsf -O https://raw.githubusercontent.com/webappsgo/wthr/main/docker/docker-compose.yml
docker compose up -d
```

## Binary

### Download

```bash
curl -q -LSsf -O https://github.com/webappsgo/wthr/releases/latest/download/wthr-linux-amd64
chmod +x wthr-linux-amd64
sudo mv wthr-linux-amd64 /usr/local/bin/wthr
```

### Run

```bash
wthr
```

On first run, wthr generates `server.yml` in `{config_dir}` and creates its data/log directories automatically.

## Service Installation

### Linux

Let the binary install itself. It detects systemd, OpenRC, runit or SysVinit, writes the matching service file, creates the `wthr` service account and enables the service:

```bash
sudo wthr --service --install
sudo wthr --service start
```

The service starts as root only to bind its ports, then the binary drops privileges to the `wthr` account (no `User=`, `command_user` or `--chuid` in any service file). Use `--service --disable` to stop and disable it, and `--service --uninstall` to remove the service, its data and the account.

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

- `https://wthr.top/server/administration/setup`

After setup, use:

- Admin panel: `https://wthr.top/server/administration`
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
