# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Enable CGO — `CGO_ENABLED=0` always, no exceptions
- ❌ Link against C libraries — pure Go only
- ❌ Skip the client binary (`weather-cli`) — it is REQUIRED, not optional
- ❌ Put business logic in the client — client is display-only
- ❌ Use platform-specific build tags for core logic — must compile on all 8 targets
- ❌ Use `-musl` suffix on Alpine/musl builds — omit it
- ❌ Strip build metadata required by the spec (Version, CommitID, BuildDate, OfficialSite)

## CRITICAL - ALWAYS DO
- ✅ `CGO_ENABLED=0 GOOS=linux go build` — static binary, no dynamic deps
- ✅ Build all 8 targets: linux/darwin/windows × amd64/arm64 + freebsd/amd64 + freebsd/arm64
- ✅ Embed version info via `-ldflags`: Version, CommitID, BuildDate, OfficialSite
- ✅ Strip binary: `-ldflags="-s -w"`
- ✅ Client binary `weather-cli` in `src/client/` — required for every project
- ✅ Single self-contained binary — no external runtime dependencies
- ✅ First-run works with zero config

## BUILD COMMAND (canonical)
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w \
    -X 'main.Version=${VERSION}' \
    -X 'main.CommitID=${COMMIT_ID}' \
    -X 'main.BuildDate=${BUILD_DATE}' \
    -X 'main.OfficialSite=${OFFICIALSITE}'" \
  -o weather ./src
```

## 8 REQUIRED BUILD TARGETS
| GOOS | GOARCH | Output suffix |
|------|--------|---------------|
| linux | amd64 | — |
| linux | arm64 | — |
| darwin | amd64 | — |
| darwin | arm64 | — |
| windows | amd64 | .exe |
| windows | arm64 | .exe |
| freebsd | amd64 | — |
| freebsd | arm64 | — |

## BINARY VERSION FLAGS
| Flag | Source | Example |
|------|--------|---------|
| `main.Version` | Git tag or date | `1.2.3` |
| `main.CommitID` | `git rev-parse --short HEAD` | `abc1234` |
| `main.BuildDate` | `date` | `Thu May 15, 2025 at 10:00:00 EDT` |
| `main.OfficialSite` | `site.txt` or secret | `https://wthr.top` |

---
For complete details, see AI.md PART 7, 8, 33
