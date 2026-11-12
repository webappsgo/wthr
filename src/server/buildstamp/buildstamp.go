// Package buildstamp holds the running build's public asset stamp,
// {project_version}-{short_commit} (AI.md PART 9 Asset Version-Busting).
package buildstamp

import (
	"net/url"
	"strings"
	"sync"
)

var (
	mu    sync.RWMutex
	stamp = "devel"
)

// Set records the running build from its version and commit id.
func Set(version, commit string) {
	version = strings.TrimSpace(version)
	commit = strings.TrimSpace(commit)
	if len(commit) > 7 {
		commit = commit[:7]
	}
	value := version
	if commit != "" && commit != "N/A" {
		value += "-" + commit
	}
	if value == "" {
		value = "devel"
	}

	mu.Lock()
	defer mu.Unlock()
	stamp = value
}

// Stamp returns {project_version}-{short_commit}.
func Stamp() string {
	mu.RLock()
	defer mu.RUnlock()

	return stamp
}

// ETag returns the quoted build-stamp ETag.
func ETag() string {
	return `"` + Stamp() + `"`
}

// Asset appends the build stamp to a static asset URL so a new build always
// gets new URLs: /static/css/common.css -> /static/css/common.css?v={stamp}.
func Asset(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}

	return path + sep + "v=" + url.QueryEscape(Stamp())
}
