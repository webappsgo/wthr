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
wthr maintenance
wthr update
wthr service
```

## Client CLI

Install the companion client:

```bash
curl -q -LSsf -O https://github.com/casapps/wthr/releases/latest/download/wthr-cli-linux-amd64
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
- Client config: `~/.config/casapps/wthr/cli.yml`

## Next Steps

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
