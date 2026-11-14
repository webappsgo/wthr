package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/webappsgo/wthr/src/util"
)

var adminRegistrationPattern = regexp.MustCompile(`(adminRoutes|adminAPI)\.(?:Get|Post|Put|Patch|Delete|Mount|Handle)\("(/[^"]*)"`)

// TestAdminRoutesUseOnlyValidAdminRootPaths scans the admin route registrations
// in main.go and fails when a first path segment is neither a fixed section
// from validAdminRootPaths nor the admin's own {admin_username} area (AI.md
// PART 17, "Route Hierarchy Rules": no config segment, no other direct children).
func TestAdminRoutesUseOnlyValidAdminRootPaths(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	matches := adminRegistrationPattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) < 50 {
		t.Fatalf("found %d admin route registrations, expected many more; the scan pattern is stale", len(matches))
	}

	segmentPattern := regexp.MustCompile(`^/([^/]*)`)
	for _, m := range matches {
		route := m[2]
		first := ""
		if route != "/" {
			first = segmentPattern.FindStringSubmatch(route)[1]
		}
		if first == "{admin_username}" {
			continue
		}
		if !util.IsValidAdminRootPath(first) {
			t.Errorf("%s route %q starts with %q, which is not in validAdminRootPaths", m[1], route, first)
		}
	}
}
