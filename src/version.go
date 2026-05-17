package main

// Version information - injected at build time via ldflags
// Example: go build -ldflags="-X main.Version=1.2.3 -X 'main.BuildDate=Thu Dec 17, 2025 at 18:19:24 EST' -X main.CommitID=abc123 -X main.OfficialSite=https://wthr.top"
var (
	// Version is the semantic version of the application
	Version = "dev"

	// BuildDate is the build timestamp (human-readable format)
	BuildDate = "unknown"

	// CommitID is the short git commit hash
	CommitID = "unknown"

	// OfficialSite is the canonical site URL (read from site.txt at build time)
	OfficialSite = "https://wthr.top"
)

// GetVersion returns the full version string
func GetVersion() string {
	return Version
}

// GetBuildInfo returns a map of build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":       Version,
		"build_date":    BuildDate,
		"commit_id":     CommitID,
		"official_site": OfficialSite,
	}
}

// GetVersionString returns a formatted version string with all build info
func GetVersionString() string {
	return "v" + Version + " (built: " + BuildDate + ", commit: " + CommitID + ")"
}
