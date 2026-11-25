# Development Guide

This guide describes the current Go project layout, containerized build and test workflow, and the documentation that must stay in sync with code.

## Requirements

- Docker with BuildKit/buildx for Go builds, tests, and container images
- Git
- Go is not required on the host; the Makefile uses `casjaysdev/go:latest`

The Makefile mounts the repository and the Go module/build caches into named, resource-limited containers. Do not run Go build or test commands directly on the host.

## Project Layout

```text
src/
├── backup/         # Backup and restore
├── cli/            # Server CLI commands
├── client/         # Companion API client
├── cluster/        # Cluster enrollment and node management
├── config/         # Configuration and validation
├── database/       # Database setup and schemas
├── graphql/        # GraphQL schema and resolvers
├── scheduler/      # In-process scheduled work
├── server/
│   ├── handler/    # HTTP handlers
│   ├── middleware/ # HTTP middleware
│   ├── model/      # Server and user models
│   ├── service/    # Server-side services
│   ├── static/     # Embedded CSS, JavaScript and images
│   └── template/   # Embedded Go HTML templates
└── util/           # Shared utilities
docker/             # Container definitions and compose files
docs/               # MkDocs/ReadTheDocs source only
scripts/            # Project scripts
tests/              # Test runners and test support
```

## Build

Use the existing Makefile targets. `make dev` builds the server and the companion client into a project-scoped OS temporary directory. `make local` builds the current platform into `binaries/`; `make build` builds the supported release platforms. Builds generate the OpenAPI document from the registered routes inside the Go container.

```bash
make dev
make local
make build
make docker
```

`make docker` builds the image locally. CI publishes release images.

## Beta Release

The maintainer can publish a beta from a clean `main` checkout with `make beta`. This creates and pushes a UTC `YYYYMMDDHHMMSS-beta` tag; the release and container workflows build from that tag. Beta releases are tag based and do not use a `beta` branch.

## Tests

Run both project test phases when validating a complete change:

```bash
make test
./tests/run_tests.sh
```

`make test` runs Go tests in the project Go container and enforces the project coverage threshold. `tests/run_tests.sh` runs the Docker basic binary and route suite. Run `tests/incus.sh` separately for full init-system validation when Incus is available; `tests/e2e.sh` is also a separate, on-demand browser suite.

Keep package tests beside the code they cover as `*_test.go`. Integration and binary tests belong under `tests/`. Do not bypass authentication in tests.

## Source Changes

Use the existing `src/server/handler`, `src/server/model`, `src/server/service`, `src/config`, and `src/database` packages. Keep package directories singular. HTTP handlers use the existing `net/http` and router patterns in `src/main.go`; do not introduce a second web framework.

Before changing a public API route, update the `### API routes` inventory in `IDEA.md`, then update the registered route, its tests, generated OpenAPI output, and `docs/api.md`. Generate OpenAPI from the source; do not hand-edit generated files.

For configuration changes, update the relevant config definitions, validation, defaults, tests, and `docs/configuration.md`. User-visible strings must use the existing i18n catalog, with the same keys in every locale.

For scheduled work, use the in-process scheduler and its registered task definitions. Do not add host cron or systemd timers for application tasks.

## Code Style

- Format Go with `gofmt` through the project build environment.
- Check errors and wrap them with useful operation context.
- Keep comments above the code they describe.
- Use Go `html/template` for HTML and keep page behavior usable with JavaScript disabled.
- Keep CSS mobile-first and use the shared theme tokens.
- Keep JavaScript in `src/server/static/js/app.js`; use progressive enhancement and no inline handlers.

## Contributor Workflow

Keep changes focused, update the relevant documentation, and run the project test and lint gates before submitting a GitHub pull request. Include a clear summary of the change and list the files it affects.

## Documentation

`docs/` contains only MkDocs documentation source. When user-visible behavior, API routes, configuration, CLI flags, or integrations change, update the corresponding pages and README in the same change. `IDEA.md` describes product behavior; `AI.md` is the read-only implementation specification.

Build or preview the documentation in an isolated documentation environment with the dependencies from `docs/requirements.txt` and `mkdocs.yml`.

## Next Steps

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
- [CLI Reference](cli.md)
- [Admin Panel](admin.md)
