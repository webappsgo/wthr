package util

// validAdminRootPaths is the closed set of first path segments allowed under
// /server/{admin_path}/ and /api/{api_version}/server/{admin_path}/ (AI.md PART 17,
// "Route Conflict Prevention"). The empty name is the dashboard.
var validAdminRootPaths = map[string]bool{
	"":              true,
	"admins":        true,
	"agents":        true,
	"announcements": true,
	"backup":        true,
	"branding":      true,
	"breaches":      true,
	"cluster":       true,
	"compliance":    true,
	"database":      true,
	"domains":       true,
	"email":         true,
	"footer":        true,
	"info":          true,
	"logs":          true,
	"maintenance":   true,
	"metrics":       true,
	"moderation":    true,
	"network":       true,
	"nodes":         true,
	"notifications": true,
	"notify":        true,
	"orgs":          true,
	"pages":         true,
	"privacy":       true,
	"reports":       true,
	"restart":       true,
	"roles":         true,
	"scheduler":     true,
	"security":      true,
	"seo":           true,
	"settings":      true,
	"setup":         true,
	"ssl":           true,
	"stats":         true,
	"status":        true,
	"tracking":      true,
	"update":        true,
	"updates":       true,
	"url":           true,
	"users":         true,
	"web":           true,
}

// IsAdminSectionName reports whether name is a fixed admin section. A Server
// Admin username may never equal one, because /server/{admin_path}/{name}/*
// would then be ambiguous between the section and the admin's own area.
func IsAdminSectionName(name string) bool {
	return name != "" && validAdminRootPaths[name]
}

// IsValidAdminRootPath reports whether segment is allowed as the first path
// segment under the admin root: the dashboard (empty) or a fixed section.
func IsValidAdminRootPath(segment string) bool {
	return validAdminRootPaths[segment]
}

// AdminSectionNames returns every non-empty admin section name; tests and the
// route validator use it to cross-check registered routes.
func AdminSectionNames() []string {
	names := make([]string, 0, len(validAdminRootPaths))
	for name := range validAdminRootPaths {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
