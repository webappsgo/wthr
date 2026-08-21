# CLI Reference

Weather ships with the server binary (`wthr`) and a companion client (`wthr-cli`).

## Server CLI

Basic usage:

```bash
wthr [flags] [command]
```

Common flags:

| Flag | Purpose |
|------|---------|
| `--help` | Show help |
| `--version` | Show version |
| `--mode` | Set `production` or `development` |
| `--config {config_dir}` | Override config directory |
| `--data {data_dir}` | Override data directory |
| `--log {log_dir}` | Override log directory |
| `--port N` | Override listen port |
| `--address HOST` | Override listen address |
| `--debug` | Enable debug mode |

Useful commands:

```bash
wthr --status
wthr --maintenance
wthr --update
wthr --service
```

## Client CLI

Install the companion client:

```bash
curl -q -LSsf -O https://github.com/webappsgo/wthr/releases/latest/download/wthr-cli-linux-amd64
chmod +x wthr-cli-linux-amd64
sudo mv wthr-cli-linux-amd64 /usr/local/bin/wthr-cli
```

Configure it against the official server:

```bash
wthr-cli --server https://wthr.top --token YOUR_API_TOKEN
```

Common usage:

```bash
wthr-cli --help
wthr-cli weather Brooklyn,NY
wthr-cli severe-weather
wthr-cli moon
```

## Output and Status

```bash
wthr --status
wthr-cli --help
```

The CLI honors the project color/plain-output rules, including `NO_COLOR`.

## Configuration

- Server config: `{config_dir}/server.yml`
- Client config: `~/.config/webappsgo/wthr/cli.yml`

### Environment Variables

The client binary (`wthr-cli`) reads these environment variables
(command-line flags take precedence):

| Variable | Purpose |
|----------|---------|
| `WTHR_SERVER_PRIMARY` | Default server URL to connect to |
| `WTHR_TOKEN` | API token for authenticated requests |
| `WTHR_OUTPUT_FORMAT` | Default output format (e.g. `json`, `text`) |
| `WTHR_DEBUG` | Enable client debug output |
| `MYLOCATION_NAME` | Default location name for weather lookups |
| `MYLOCATION_ZIP` | Default ZIP/postal code for weather lookups |

The server binary (`wthr`) reads these directory overrides, used by its
`--maintenance` subcommands and by first-run directory resolution
(the matching `--config` / `--data` / `--log` flags take precedence):

| Variable | Purpose |
|----------|---------|
| `CONFIG_DIR` | Override the server config directory |
| `DATA_DIR` | Override the server data directory |
| `LOG_DIR` | Override the server log directory |

## Next Steps

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
