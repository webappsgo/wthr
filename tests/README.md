# Weather Service Tests

Comprehensive test suite for the Weather Service including unit tests, integration tests, and end-to-end tests.

## Quick Start

```bash
# Run all tests
./tests/run_tests.sh

# Run with coverage report
./tests/run_tests.sh --coverage

# Run with verbose output
./tests/run_tests.sh --verbose

# Run benchmarks
./tests/run_tests.sh --bench

# Start test server for manual testing
./tests/test-server.sh
```

## Test Structure

```
tests/
├── README.md                           # This file
├── run_tests.sh                        # Phase 2 entrypoint (Incus preferred, Docker fallback)
├── incus.sh                            # Phase 2 in Incus (debian/trixie, full systemd)
├── docker.sh                           # Phase 2 in Docker (alpine:latest)
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
    └── setup_flow_test.go             # Complete setup flow

```

## Test Scripts

### `run_tests.sh` - Phase 2 Entrypoint

Dispatches the Phase 2 binary-validation suite: Incus when `incus` is available
(preferred, full systemd), otherwise Docker. It takes no options and exits
non-zero when neither runtime is present.

**Usage:**
```bash
./tests/run_tests.sh
```

Phase 1 (`go test` plus the 60% coverage gate) is a separate concern and runs
via `make test`, not through this script.

### `lib/matrix.sh` - Shared Container Matrix

Both `incus.sh` and `docker.sh` build the binaries in `casjaysdev/go:latest`,
push this one script into the test container, and run it with
`matrix.sh {project_name} {project_org}`. Keeping the matrix in a single file is
what stops the Incus and Docker suites from drifting apart. It covers version
and `--help` output, binary-rename behaviour for the server and the CLI, the
setup-token → verify-token → create-admin → API-token flow, unauthenticated
rejection, invalid-credential rejection, the public/user/admin frontend and API
route matrices with every applicable `Accept` header, the `.txt` extension
endpoints, and CLI commands run against the live server. It counts failures and
exits non-zero if any check fails.

It also runs the two PART 31 matrices:

- **Accessibility** — for one representative page per layout family (public,
  auth, user, admin) it verifies that a skip link exists and is the first
  focusable element, that every `<img>` carries an `alt` attribute, that every
  visible form control is labelled (`<label for>`, `aria-label`, or
  `aria-labelledby`), that the page has exactly one `<h1>` and skips no heading
  level, and that the banner, navigation, main, and contentinfo landmarks are
  all present.
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
```

### End-to-End Tests

```bash
# All e2e tests
./tests/run_tests.sh

# Full Incus-backed end-to-end matrix
./tests/incus.sh

# Docker-backed end-to-end matrix
./tests/docker.sh
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

- All test data stored in `/tmp/webappsgo/wthr-XXXXXX/`
- Automatically cleaned up on exit
- Use `KEEP_TEMP=1` to inspect after test run
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

### Neither Incus nor Docker available
`run_tests.sh` exits non-zero when neither runtime is installed. Install
`incus` (preferred) or `docker` — there is no host-toolchain fallback, because
the host is never expected to have Go installed.

## Notes

- Test server uses isolated temp directories
- No pollution of your working directory
- Every build and test runs in a container, never on the host
- Coverage artifacts are written outside the project tree
