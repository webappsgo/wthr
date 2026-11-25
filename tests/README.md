# Weather Service Tests

Comprehensive test suite for the Weather Service including unit tests, integration tests, and end-to-end tests.

## Quick Start

```bash
# Run all tests
./tests/run_tests.sh

# Start test server for manual testing
./tests/test-server.sh
```

## Test Structure

```
tests/
├── README.md                           # This file
├── run_tests.sh                        # Docker basic-suite entrypoint
├── incus.sh                            # Incus containers ([systemd|openrc|all]: Debian trixie + latest stable Alpine, service lifecycle, Tor, I2P)
├── docker.sh                           # Phase 2 in Docker (alpine:latest)
├── e2e.sh                              # On-demand Docker-run chromedp E2E suite
├── test-server.sh                      # Isolated manual dev server
├── lib/
│   └── matrix.sh                       # Shared in-container route/header/auth/CLI matrix
├── unit/                               # Unit tests
│   ├── services/
│   │   ├── location_enhancer_test.go  # Location service tests
│   │   └── weather_service_test.go    # Weather service tests
│   └── handlers/
│       └── auth_test.go               # Authentication tests
├── integration/                        # Integration tests
│   ├── api_test.go                    # API endpoint tests
│   └── notification_api_test.go       # Notification API tests
└── e2e/                               # End-to-end tests
    └── browser_test.go                 # SSR, no-JS, and browser interaction tiers

```

## Test Scripts

### `run_tests.sh` - Docker Basic-Suite Entrypoint

Runs the Docker basic binary-validation suite. Incus full-app validation and
browser E2E are separate, explicit script invocations. The runner takes no
options and exits with status 69 when Docker is unavailable.

**Usage:**
```bash
./tests/run_tests.sh
```

Phase 1 (`go test` plus the 60% coverage gate) is a separate concern and runs
via `make test`, not through this script.

### `lib/matrix.sh` - Shared Container Matrix

Both `incus.sh` and `docker.sh` build through the Makefile's `build` target in
`casjaysdev/go:latest`, push this one script into the test container, and run it with
`matrix.sh {project_name} {project_org}`. Keeping the matrix in a single file is
what stops the Incus and Docker suites from drifting apart. It covers version
and `--help` output, binary-rename behaviour for the server and the CLI, the
setup-token → verify-token → create-admin → API-token flow, unauthenticated
rejection, invalid-credential rejection, the public/user/admin frontend and API
route matrices with every applicable `Accept` header, the `.txt` extension
endpoints, and CLI commands run against the live server. It counts failures and
exits non-zero if any check fails.

The Docker basic suite uses the platform-suffixed Linux amd64 server and CLI
artifacts from the Makefile build, then gives the CLI the unsuffixed name the
matrix expects inside Alpine.

The API route matrix follows the endpoints listed in `IDEA.md`, including
`/api/v1/severe-weather`; it does not add a standalone hurricanes API or a
hurricanes `.txt` route that the API inventory does not define.

The user-auth flow enables multi-user open registration and disables email
verification in its isolated fixture, then fetches the login form as HTML and
submits its CSRF token for both valid and invalid credentials.

Admin frontend routes must return HTML even for `Accept: text/plain`; the
shared matrix checks this content negotiation contract alongside the API's
plain-text responses.

It also runs the two PART 31 matrices:

- **Accessibility** — for one representative page per layout family (public,
  auth, user, admin) it verifies that a skip link exists and is the first
  focusable element, that every `<img>` carries an `alt` attribute, that every
  visible form control is labelled (`<label for>`, `aria-label`, or
  `aria-labelledby`; hidden and `aria-hidden` controls are excluded), that the
  page has exactly one `<h1>` and skips no heading level, and that the banner,
  navigation, main, and contentinfo landmarks are all present.
- **Language and direction** — every supported language (`en`, `es`, `zh`,
  `fr`, `ar`, `de`, `ja`) is requested via `?lang=`, `<html lang>` must match,
  Arabic must render `dir="rtl"` and no other language may, and an unsupported
  `?lang=` must silently fall back to English instead of erroring.

### `test-server.sh` - Isolated Test Server

Runs the weather service in an isolated temporary directory.

**Features:**
- Isolated temp directory per run (`/tmp/webappsgo/wthr-XXXXXX/`)
- Auto-cleanup on exit (Ctrl+C)
- No repo pollution
- Real-time log following

**Usage:**
```bash
# Basic usage (auto port)
./tests/test-server.sh

# Custom port
PORT=3053 ./tests/test-server.sh

# Keep temp directory for debugging
KEEP_TEMP=1 PORT=3053 ./tests/test-server.sh
```

**Environment Variables:**
- `PORT` - Server port (default: 3053)
- `KEEP_TEMP` - Set to `1` to keep temp directory

## Running Specific Tests

### Unit Tests

`make test` is the whole of Phase 1: it runs `go test` inside
`casjaysdev/go:latest`, reports coverage, and fails below the 60% gate.

```bash
make test
```

### Integration Tests

```bash
# All integration tests
./tests/run_tests.sh

# Docker integration matrix
./tests/docker.sh

# Incus integration matrix
./tests/incus.sh

# Optional browser E2E suite (AI.md PART 29)
./tests/e2e.sh
```

Run `tests/e2e.sh` on demand for beta testing, before a release, or after major
frontend changes. It requires Docker and runs the Go tests tagged `e2e` inside
`casjaysdev/go:latest`, with a `chromedp/headless-shell` sidecar on an isolated
internal network. The suite covers SSR responses, no-JavaScript forms, browser
admin and user login/logout flows, responsive layout, themes, language, and
browser JavaScript errors. It is separate from `run_tests.sh` and `make test`.
Failure artifacts are retained in the script's scoped temp directory, whose path
is printed to stderr; successful runs remove the directory.

`tests/incus.sh` accepts `systemd`, `openrc`, or `all` (the default). It runs
the selected modes one at a time in Debian trixie and/or the latest stable
Alpine release. It builds through the Makefile into a scoped temporary
directory and removes its Incus containers and build artifacts on exit.

Resource and timeout defaults can be overridden with `INCUS_CPUS` (2),
`INCUS_MEM` (`4GiB`), `INCUS_CPU_PRIORITY` (2), `INCUS_OP_TIMEOUT` (60
seconds), `INCUS_INSTALL_TIMEOUT` (900 seconds), `INCUS_RUN_TIMEOUT` (3600
seconds), and `DOCKER_TIMEOUT` (3600 seconds).

### End-to-End Tests

The browser E2E suite is an explicit, on-demand run:

```bash
./tests/e2e.sh
```

## Manual Testing

### Start Test Server

```bash
# Start server
./tests/test-server.sh

# In another terminal, test endpoints:
curl -q -LSsf http://localhost:3053/server/healthz
curl -q -LSsf "http://localhost:3053/api/v1/weather?lat=40.7128&lon=-74.0060"
curl -q -LSsf "http://localhost:3053/api/v1/weather?city_id=5128581"
curl -q -LSsf "http://localhost:3053/api/v1/weather?lat=40.7128&lon=-74.0060&nearest=true"

# Stop server (Ctrl+C in first terminal)
```

## Coverage Reports

`make test` prints the per-package coverage and the total, and fails the build
below 60%. Coverage artifacts are written to a temp directory outside the
project tree, never into the repository.

```bash
make test
```

## Continuous Integration

CI runs the same two phases with explicit commands rather than Makefile
targets, inside the `casjaysdev/go:latest` job container. See
`.github/workflows/ci.yml` for the authoritative steps.

## Writing New Tests

### Unit Test Template

```go
package mypackage_test

import (
    "testing"
)

func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "expected", false},
        {"invalid input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)

            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if got != tt.want {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Best Practices

1. **Isolation** - Each test should be independent
2. **Cleanup** - Always clean up resources (defer, t.Cleanup)
3. **Table-driven** - Use table-driven tests for multiple cases
4. **Descriptive names** - Test names should describe what they test
5. **Fast tests** - Keep unit tests fast (<100ms each)
6. **No external deps** - Unit tests should not require network/DB

## Test Data

- Test data uses `${TMPDIR:-/tmp}/webappsgo/wthr-XXXXXX/`
- Temporary artifacts are removed on success; `tests/docker.sh` retains its
  build output and diagnostic logs there on failure and prints the directory
  path to stderr
- `KEEP_TEMP=1` is for `tests/test-server.sh`; the Docker suite retains artifacts
  automatically after failure
- Never commit test databases or temp files

## Troubleshooting

### Tests failing with database errors
```bash
# Re-run the containerized test suite
./tests/run_tests.sh
```

### Port already in use
```bash
# Change port for test server
PORT=3054 ./tests/test-server.sh
```

### Docker unavailable
`run_tests.sh` exits with status 69 when Docker is unavailable. Install Docker;
the basic suite has no host-toolchain fallback.

## Notes

- Test server uses isolated temp directories
- No pollution of your working directory
- Every build and test runs in a container, never on the host
- Coverage artifacts are written outside the project tree
