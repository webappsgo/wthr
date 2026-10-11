package main

import (
	"strconv"
	"time"
)

// Build info - set via -ldflags at build time (Version, CommitID, BuildEpoch, OfficialSite)
// Example: go build -ldflags="-X main.Version=1.2.3 -X main.CommitID=abc1234 -X main.BuildEpoch=1765112713 -X main.OfficialSite=https://example.com"
var (
	// Version is the semantic version of the application
	Version = "devel"

	// CommitID is the short git commit hash
	CommitID = "N/A"

	// BuildDate is derived from BuildEpoch in init(); "N/A" when BuildEpoch is unset
	BuildDate = "N/A"

	// BuildEpoch is the Unix build timestamp (seconds, UTC) set via -ldflags; "0" when unset
	BuildEpoch = "0"

	// OfficialSite is the default server URL; empty means users must use --server
	OfficialSite = ""
)

// init derives BuildDate (RFC 3339 UTC) from the embedded BuildEpoch
func init() {
	deriveBuildDate()
}

// deriveBuildDate sets BuildDate from BuildEpoch when the epoch is valid
func deriveBuildDate() {
	if n := parseBuildEpoch(BuildEpoch); n > 0 {
		BuildDate = time.Unix(n, 0).UTC().Format("2006-01-02T15:04:05Z")
	}
}

// parseBuildEpoch returns the epoch seconds, or 0 when unset or invalid
func parseBuildEpoch(v string) int64 {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// GetVersion returns the full version string
func GetVersion() string {
	return Version
}

// GetBuildInfo returns a map of build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":       Version,
		"build_date":    BuildDate,
		"build_epoch":   BuildEpoch,
		"commit_id":     CommitID,
		"official_site": OfficialSite,
	}
}

// GetVersionString returns a formatted version string with all build info
func GetVersionString() string {
	return "v" + Version + " (built: " + BuildDate + ", commit: " + CommitID + ")"
}
