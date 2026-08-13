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
├── run_tests.sh                        # Main test runner
├── test-server.sh                      # Isolated test server
├── unit/                               # Unit tests
│   ├── services/
│   │   ├── location_enhancer_test.go  # Location service tests
│   │   └── weather_service_test.go    # Weather service tests
│   └── handlers/
│       └── auth_test.go               # Authentication tests
├── integration/                        # Integration tests
│   └── api_test.go                    # API endpoint tests
└── e2e/                               # End-to-end tests
    └── setup_flow_test.go             # Complete setup flow

```

## Test Scripts

### `run_tests.sh` - Main Test Runner

Runs the complete test suite with options for coverage and verbosity.

**Usage:**
```bash
# Run all tests
./tests/run_tests.sh

# With coverage report (generates coverage.html)
./tests/run_tests.sh --coverage

# With verbose output
./tests/run_tests.sh -v

# Run benchmarks
./tests/run_tests.sh --bench

# Combine options
./tests/run_tests.sh -c -v
```

**Options:**
- `-c, --coverage` - Generate coverage report
- `-v, --verbose` - Verbose test output
- `-b, --bench` - Run benchmarks

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

```bash
# All unit tests
make test

# Quick containerized unit test pass
make test

# With coverage
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
curl -q -LSsf http://localhost:3053/healthz
curl -q -LSsf "http://localhost:3053/api/v1/weather?lat=40.7128&lon=-74.0060"
curl -q -LSsf "http://localhost:3053/api/v1/weather?city_id=5128581"
curl -q -LSsf "http://localhost:3053/api/v1/weather?lat=40.7128&lon=-74.0060&nearest=true"

# Stop server (Ctrl+C in first terminal)
```

## Coverage Reports

```bash
# Generate coverage report
./tests/run_tests.sh --coverage

# View in browser
open coverage.html  # macOS
xdg-open coverage.html  # Linux
start coverage.html  # Windows
```

## Benchmarking

```bash
# Run benchmarks
./tests/run_tests.sh --bench

# Use the scripted test entrypoint
./tests/run_tests.sh --bench
```

## Continuous Integration

The test suite is designed to work with CI/CD pipelines:

```yaml
# Example: GitHub Actions
- name: Run tests
  run: ./tests/run_tests.sh --coverage

- name: Upload coverage
  uses: codecov/codecov-action@v3
  with:
    files: ./coverage.out
```

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

### Coverage report not generating
```bash
# Ensure you have write permissions
./tests/run_tests.sh --coverage
ls -la coverage.out coverage.html
```

## Notes

- Test server uses isolated temp directories
- No pollution of your working directory
- All tests run in-memory databases
- Coverage reports saved to `coverage.html`
- Tests require Go 1.21 or higher
