package util

import "testing"

func TestIsReservedName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"admin", true},
		{"ADMIN", true},
		{"  Admin  ", true},
		{"@me", true},
		{"@admin", true},
		{"administration", true},
		{"feed.xml", true},
		{"sitemap.xml", true},
		{"robots.txt", true},
		{".well-known", true},
		{".hidden", true},
		{"_private", true},
		{"profile.png", true},
		{"data.json", true},
		{"admin1", true},
		{"root42", true},
		{"ns3", true},
		{"ns2", true},
		{"ａｄｍｉｎ", true},
		{"ℯ", false},
		{"graphql", true},
		{"mcp", true},
		{"alice", false},
		{"alice2026", false},
		{"support1", false},
		{"", false},
		{"@", false},
	}
	for _, tt := range tests {
		if got := IsReservedName(tt.name); got != tt.want {
			t.Errorf("IsReservedName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestReservedNamesListedInSpec(t *testing.T) {
	for _, name := range []string{"api", "server", "healthz", "login", "oauth", "search", "security", "swagger", "favicon.ico", "service-worker.js"} {
		if !reservedNameSet[name] {
			t.Errorf("reservedNames is missing %q", name)
		}
	}
}
