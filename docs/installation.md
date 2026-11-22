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

### All platforms

Let the binary install itself. `--service --install` detects the service manager of the host (systemd, OpenRC, runit or SysVinit on Linux; rc.d on BSD; launchd on macOS; Windows Service), writes the service file, enables the service and starts it:

```bash
sudo wthr --service --install
```

Without root or administrator access the command offers to escalate, and falls back to a user service (`systemd --user` on Linux, a LaunchAgent on macOS) when escalation is declined or impossible. On Windows run it from an elevated prompt; the service runs as the Virtual Service Account `NT SERVICE\wthr`.

The service starts as root only to bind its ports, then the binary drops privileges to the `wthr` account (no `User=`, `command_user` or `--chuid` in any service file). Control and inspect it with:

```bash
sudo wthr --service restart
wthr --service status
sudo wthr --service --disable
sudo wthr --service --uninstall
```

`--disable` stops the service and removes it from auto-start but keeps the service file, data and account, so `--install` enables it again. `--uninstall` asks for confirmation, then stops and removes the service, deletes the config, data, cache, log and backup directories and the service account, and leaves the binary for you to remove.

Typical runtime locations follow AI.md PART 4:

- Config: `{config_dir}`
- Data: `{data_dir}`
- Logs: `{log_dir}`

## Setup

The server is functional immediately on first run. Admin setup is completed through:

- `https://wthr.top/server/administration/setup`

After setup, use:

- Admin panel: `https://wthr.top/server/administration`
- Health: `https://wthr.top/healthz`
- Swagger UI: `https://wthr.top/server/docs/swagger`
- OpenAPI JSON: `https://wthr.top/api/v1/server/swagger`

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
