package util

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// reservedNames is the complete reserved list from AI.md PART 16 ("Reserved
// Names (MUST block from registration)"). It is never loosened by configuration.
var reservedNames = []string{
	"api", "server", "static", "assets", "healthz", "health", "status", "metrics", "version",
	"webhook", "webhooks", "internal", "private", "public", "well-known", "cgi-bin", "tmp", "temp",
	"backup", "backups", "debug", "test", "dev", "staging", "null", "undefined", "nil", "true",
	"false", "default", "index", "home", "main",

	"admin", "administrator", "administration", "root", "superuser", "sysadmin", "sysop", "system",
	"sys", "operator", "owner", "staff", "moderator", "mod", "manager", "management", "manage",
	"console", "panel", "cpanel", "backend", "config", "configuration", "wp-admin", "wp-login",
	"phpmyadmin", "adminer", "webmaster", "postmaster", "hostmaster", "abuse", "noreply",
	"no-reply", "bot", "bots", "anonymous", "guest", "everyone", "nobody", "me", "self", "www",
	"mail", "ftp", "smtp", "imap", "pop", "ns", "ns1", "ns2", "localhost",

	"login", "logout", "register", "signup", "signin", "signout", "auth", "oauth", "callback",
	"account", "accounts", "profile", "settings", "dashboard", "user", "users", "org", "orgs",
	"organization", "organizations", "team", "teams", "group", "groups", "password", "reset",
	"verify", "invite", "invites", "preferences",

	"search", "explore", "discover", "trending", "help", "support", "docs", "documentation",
	"about", "contact", "terms", "privacy", "legal", "security", "info", "new", "edit", "delete",
	"create", "update", "all", "download", "downloads", "upload", "billing", "sales", "hello",

	"graphql", "mcp", "swagger", "rest", "rpc", "ws", "websocket", "cdn", "media", "uploads",
	"files", "images", "css", "js", "fonts", "img",

	".well-known", "robots.txt", "sitemap.xml", "feed.xml", "rss.xml", "atom.xml", "favicon.ico",
	"manifest.json", "humans.txt", "security.txt", "llms.txt", "ads.txt", "feed", "rss", "atom",
	"sitemap", "robots", "favicon", "manifest", "browserconfig.xml", "sw.js", "service-worker.js",
}

// reservedPrivilegedNames are the names under "Privileged and administrative
// names"; only these also match when followed by digits (admin1, root2).
var reservedPrivilegedNames = []string{
	"admin", "administrator", "administration", "root", "superuser", "sysadmin", "sysop", "system",
	"sys", "operator", "owner", "staff", "moderator", "mod", "manager", "management", "manage",
	"console", "panel", "cpanel", "backend", "config", "configuration", "wp-admin", "wp-login",
	"phpmyadmin", "adminer", "webmaster", "postmaster", "hostmaster", "abuse", "noreply",
	"no-reply", "bot", "bots", "anonymous", "guest", "everyone", "nobody", "me", "self", "www",
	"mail", "ftp", "smtp", "imap", "pop", "ns", "ns1", "ns2", "localhost",
}

// reservedFileExtensions are the suffixes that make a candidate look like a file.
var reservedFileExtensions = []string{
	".xml", ".txt", ".json", ".ico", ".html", ".htm", ".php", ".js", ".css", ".map",
	".webmanifest", ".png", ".jpg", ".svg", ".zip",
}

var (
	reservedNameSet       = buildNameSet(reservedNames)
	reservedPrivilegedSet = buildNameSet(reservedPrivilegedNames)
)

func buildNameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// IsReservedName reports whether a user, org, team, project slug, vanity route
// or short code is reserved (AI.md PART 16 "Reserved-name matching"). The
// candidate is trimmed, lowercased, NFKC-normalized and stripped of one
// leading "@" first, so Unicode lookalikes cannot slip past the list. Callers
// return the same validation error as a taken name so the matching rule is
// never revealed.
func IsReservedName(candidate string) bool {
	name := strings.TrimSpace(candidate)
	name = norm.NFKC.String(name)
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "@")
	if name == "" {
		return false
	}
	if reservedNameSet[name] {
		return true
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	for _, ext := range reservedFileExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	trimmed := strings.TrimRightFunc(name, unicode.IsDigit)
	if trimmed != name && reservedPrivilegedSet[trimmed] {
		return true
	}
	return false
}
