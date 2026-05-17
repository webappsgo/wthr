# CLI Reference

Weather ships with the server binary (`weather`) and a companion client (`weather-cli`).

## Server CLI

Basic usage:

```bash
weather [flags] [command]
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
weather --status
weather maintenance
weather update
weather service
```

## Client CLI

Install the companion client:

```bash
curl -q -LSsf -O https://github.com/casapps/wthr/releases/latest/download/weather-cli-linux-amd64
chmod +x weather-cli-linux-amd64
sudo mv weather-cli-linux-amd64 /usr/local/bin/weather-cli
```

Configure it against the official server:

```bash
weather-cli --server https://wthr.top --token YOUR_API_TOKEN
```

Common usage:

```bash
weather-cli --help
weather-cli weather Brooklyn,NY
weather-cli severe-weather
weather-cli moon
```

## Output and Status

```bash
weather --status
weather-cli --help
```

The CLI honors the project color/plain-output rules, including `NO_COLOR`.

## Configuration

- Server config: `{config_dir}/server.yml`
- Client config: `~/.config/casapps/wthr/cli.yml`

## Next Steps

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
