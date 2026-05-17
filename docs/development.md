# Development Guide

This guide covers contributing to Weather Service development.

## Development Setup

### Prerequisites

- **Docker** - Required for builds and testing
- **Go** - NOT required on host (builds use Docker)
- **Git** - Version control
- **Text Editor** - VS Code, GoLand, vim, etc.

!!! important "Docker-Only Development"
    Weather Service uses Docker for ALL Go operations. Do not install Go on the host system.
    All builds, tests, and debugging use containerized Go environments.

### Clone Repository

```bash
git clone https://github.com/casapps/wthr.git
cd weather
```

### Project Structure

```
weather/
├── src/                    # Go source code
│   ├── main.go            # Application entry point
│   ├── cli/               # CLI commands
│   ├── config/            # Configuration handling
│   ├── database/          # Database layer
│   ├── handlers/          # HTTP handlers
│   ├── middleware/        # HTTP middleware
│   ├── models/            # Data models
│   ├── paths/             # OS-specific paths
│   ├── renderers/         # Output formatters
│   ├── scheduler/         # Background tasks
│   ├── server/            # HTTP server
│   │   ├── static/        # Static assets (CSS, JS, images)
│   │   └── templates/     # HTML templates
│   ├── services/          # Business logic
│   └── utils/             # Utilities
├── docker/                # Docker files
│   ├── Dockerfile         # Multi-stage build
│   ├── docker-compose.yml # Production compose
│   └── rootfs/            # Container overlay
├── docs/                  # Documentation (MkDocs)
├── .github/               # GitHub Actions workflows
├── tests/                 # Tests
│   ├── unit/             # Unit tests
│   ├── integration/      # Integration tests
│   └── e2e/              # End-to-end tests
├── Makefile              # Build automation
├── go.mod                # Go module definition
├── AI.md                 # Project specification
├── TODO.AI.md            # Task tracking
└── README.md             # Public documentation
```

## Building

### Quick Development Build

Build and run for testing:

```bash
make dev
```

This builds to a temporary directory and runs the binary.

### Full Build (All Platforms)

Build for all platforms:

```bash
make build
```

Output location: `binaries/`

Platforms built:
- `weather-linux-amd64`
- `weather-linux-arm64`
- `weather-darwin-amd64`
- `weather-darwin-arm64`
- `weather-windows-amd64.exe`
- `weather-windows-arm64.exe`
- `weather-freebsd-amd64`
- `weather-freebsd-arm64`

### Docker Build

Build Docker image:

```bash
make docker
```

Tag and push:

```bash
make docker-push TAG=v1.2.3
```

### Release Build

Build release artifacts with version info:

```bash
make release VERSION=1.2.3
```

## Testing

### Run All Tests

```bash
make test
```

### Run Unit Tests

```bash
make test-unit
```

### Run Integration Tests

```bash
make test-integration
```

### Run End-to-End Tests

```bash
make test-e2e
```

### Test Coverage

```bash
make test-coverage
```

View coverage report:

```bash
make test-coverage-html
```

Opens coverage report in browser.

## Development Workflow

### 1. Create Feature Branch

```bash
git checkout -b feature/your-feature-name
```

Branch naming:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation
- `refactor/` - Code refactoring
- `test/` - Test additions/fixes

### 2. Make Changes

Edit source files in `src/` directory.

### 3. Test Changes

```bash
# Build and run
make dev

# Run tests
make test

# Check for issues
make lint
```

### 4. Commit Changes

Follow commit message format:

```
<emoji> <type>: <description>

<detailed description>

- Change 1
- Change 2
```

Commit types:
- ✨ `feat:` - New feature
- 🐛 `fix:` - Bug fix
- 📝 `docs:` - Documentation
- ♻️ `refactor:` - Code refactoring
- ✅ `test:` - Adding tests
- 🔧 `chore:` - Maintenance

Example:

```bash
git add .
git commit -m "✨ feat: Add hurricane tracking feature

Implement real-time hurricane tracking using NOAA NHC API.

- Add hurricane data fetching service
- Create hurricane API endpoint
- Add hurricane display on web UI
- Add tests for hurricane functionality"
```

### 5. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Create pull request on GitHub:
1. Go to repository on GitHub
2. Click **Pull Requests** → **New Pull Request**
3. Select your branch
4. Fill in PR template
5. Submit for review

## Code Style

### Go Code Guidelines

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting (automatic in Docker builds)
- Comments ABOVE code, never inline
- Error handling: always check and handle errors
- Input validation: validate ALL user input
- No magic numbers: use named constants

### Example: Good Code

```go
// CalculateWindChill calculates wind chill temperature
// using the new wind chill formula (2001)
func CalculateWindChill(temp, windSpeed float64) float64 {
	const (
		// Wind chill constants
		windChillConstant = 35.74
		tempCoefficient   = 0.6215
		windCoefficient   = 35.75
		windPowerFactor   = 0.16
	)

	if windSpeed < 3 || temp > 50 {
		return temp
	}

	windChill := windChillConstant +
		(tempCoefficient * temp) -
		(windCoefficient * math.Pow(windSpeed, windPowerFactor)) +
		(0.4275 * temp * math.Pow(windSpeed, windPowerFactor))

	return windChill
}
```

### Example: Bad Code

```go
// Don't do this
func calc(t, w float64) float64 {
	if w < 3 || t > 50 { return t }
	return 35.74 + (0.6215*t) - (35.75*math.Pow(w,0.16)) + (0.4275*t*math.Pow(w,0.16)) // calculate wind chill
}
```

### Template Guidelines

- Use semantic HTML5
- Mobile-first responsive design
- Accessible (ARIA labels, keyboard navigation)
- Escape all user content (XSS prevention)
- Use Dracula theme colors
- No inline CSS or JavaScript

### CSS Guidelines

- Use external CSS files
- Mobile-first media queries
- Use CSS variables for theming
- Follow BEM naming convention
- Minimize specificity

### JavaScript Guidelines

- Vanilla JS (no jQuery)
- Use ES6+ features
- No inline event handlers
- Progressive enhancement
- Handle errors gracefully

## Adding Features

### Adding a New API Endpoint

1. **Create handler** in `src/handlers/`

```go
// src/handlers/feature.go
package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// GetFeature handles GET /api/v1/feature
func GetFeature(c *gin.Context) {
	// Implementation
	c.JSON(http.StatusOK, gin.H{
		"feature": "data",
	})
}
```

2. **Register route** in `src/main.go`

```go
// In setupRoutes function
apiV1.GET("/feature", handlers.GetFeature)
```

3. **Add tests** in `tests/unit/handlers/`

```go
func TestGetFeature(t *testing.T) {
	// Test implementation
}
```

4. **Update documentation** in `docs/api.md`

### Adding a Database Table

1. **Update schema** in `src/database/server_schema.go` or `users_schema.go`

```go
const ServerSchema = `
-- Existing tables...

-- New table
CREATE TABLE IF NOT EXISTS new_table (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_new_table_name ON new_table(name);
`
```

2. **Create model** in `src/models/`

```go
// src/models/feature.go
package models

import "database/sql"

// Feature represents a feature record
type Feature struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// FeatureModel handles database operations
type FeatureModel struct {
	DB *sql.DB
}

// Create creates a new feature
func (m *FeatureModel) Create(name string) (*Feature, error) {
	// Implementation
}
```

3. **Increment schema version** in `src/database/`

```go
const ServerSchemaVersion = 6  // Increment
```

### Adding a Scheduled Task

1. **Create task** in `src/scheduler/`

```go
// src/scheduler/mytask.go
package scheduler

import "time"

// MyTask is a scheduled task
type MyTask struct {
	// Fields
}

// Run executes the task
func (t *MyTask) Run() error {
	// Implementation
	return nil
}

// Schedule returns the task schedule
func (t *MyTask) Schedule() string {
	return "0 */6 * * *"  // Every 6 hours
}
```

2. **Register task** in `src/scheduler/scheduler.go`

```go
// Add to scheduler initialization
s.AddTask("my_task", &MyTask{})
```

## Debugging

### Debug Logging

Enable debug mode:

```bash
weather --mode development
```

Or set log level:

```bash
weather --log-level debug
```

### Attach Debugger

Use Delve debugger in Docker:

```bash
docker run -it --rm \
  -v $(pwd):/app \
  -w /app \
  -p 8080:8080 \
  -p 2345:2345 \
  golang:alpine \
  sh -c "go install github.com/go-delve/delve/cmd/dlv@latest && \
         dlv debug ./src --headless --listen=:2345 --api-version=2"
```

Connect from IDE on port 2345.

### View Logs

```bash
# Real-time logs
tail -f /var/log/weather/weather.log

# With Docker
docker logs -f weather

# With systemd
journalctl -u weather -f
```

## Documentation

### Update API Docs

Edit `docs/api.md` when adding/changing endpoints.

### Update Configuration Docs

Edit `docs/configuration.md` when adding settings.

### Build Documentation Locally

```bash
# Install dependencies
pip install -r docs/requirements.txt

# Serve documentation
mkdocs serve
```

View at `http://localhost:8000`

## Pull Request Guidelines

### Before Submitting

- [ ] Code compiles without errors
- [ ] All tests pass
- [ ] No linter warnings
- [ ] Documentation updated
- [ ] Commit messages follow format
- [ ] No merge conflicts

### PR Description

Include:
- **What** - What changes were made
- **Why** - Why the changes were needed
- **How** - How the changes work
- **Testing** - How changes were tested
- **Screenshots** - For UI changes

### Review Process

1. Automated checks run (build, test, lint)
2. Maintainer reviews code
3. Requested changes addressed
4. Approved and merged

## Release Process

Releases are automated via GitHub Actions.

### Version Tags

```bash
# Create and push tag
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

Triggers:
- Build for all platforms
- Run all tests
- Create GitHub release
- Build and push Docker image
- Tag Docker image as `latest`, `v1.2.3`, `2501` (YYMM)

## Getting Help

- **GitHub Issues** - Report bugs, request features
- **GitHub Discussions** - Ask questions, discuss ideas
- **Documentation** - Read the docs
- **AI.md** - Full project specification

## Next Steps

- [Installation](installation.md) - Install development environment
- [Configuration](configuration.md) - Configure development server
- [API Reference](api.md) - API endpoint documentation
- [CLI Reference](cli.md) - Command-line tools
