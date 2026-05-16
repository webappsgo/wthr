package utils

import (
	"strings"
)

// UsernameBlocklist contains reserved usernames per AI.md PART 22
// This list prevents users from registering common system usernames, admin terms,
// and potentially confusing identifiers
var UsernameBlocklist = []string{
	// System & Administrative
	"admin", "administrator", "root", "system", "sysadmin", "superuser",
	"master", "owner", "operator", "manager", "moderator", "mod",
	"staff", "support", "helpdesk", "help", "service", "daemon",

	// Server & Technical
	"server", "host", "node", "cluster", "api", "www", "web", "mail",
	"email", "smtp", "ftp", "ssh", "dns", "proxy", "gateway", "router",
	"firewall", "localhost", "local", "internal", "external", "public",
	"private", "network", "database", "db", "cache", "redis", "mysql",
	"postgres", "mongodb", "elastic", "nginx", "apache", "docker",

	// Application & Service Names
	"app", "application", "bot", "robot", "crawler", "spider", "scraper",
	"webhook", "callback", "cron", "scheduler", "worker", "queue", "job",
	"task", "process", "service", "microservice", "lambda", "function",

	// Authentication & Security
	"auth", "authentication", "login", "logout", "signin", "signout",
	"signup", "register", "password", "passwd", "token", "oauth", "sso",
	"saml", "ldap", "kerberos", "security", "secure", "ssl", "tls",
	"certificate", "cert", "key", "secret", "credential", "session",

	// Roles & Permissions
	"guest", "anonymous", "anon", "user", "users", "member", "members",
	"subscriber", "editor", "author", "contributor", "reviewer", "auditor",
	"analyst", "developer", "dev", "devops", "engineer", "architect",
	"designer", "tester", "qa", "billing", "finance", "legal", "hr",
	"sales", "marketing", "ceo", "cto", "cfo", "coo", "founder", "cofounder",

	// Common Reserved
	"account", "accounts", "profile", "profiles", "settings", "config",
	"configuration", "dashboard", "panel", "console", "portal", "home",
	"index", "main", "default", "null", "nil", "undefined", "void",
	"true", "false", "test", "debug", "demo", "example",
	"sample", "temp", "temporary", "tmp", "backup", "archive", "log",
	"logs", "audit", "report", "reports", "analytics", "stats", "status",

	// API & Endpoints
	"api", "rest", "graphql", "grpc", "websocket", "ws", "wss", "http",
	"https", "endpoint", "endpoints", "route", "routes", "path", "url",
	"uri", "callback", "hook", "hooks", "event", "events", "stream",

	// Content & Media
	"blog", "news", "article", "articles", "post", "posts", "page", "pages",
	"feed", "rss", "atom", "sitemap", "robots", "favicon", "static",
	"assets", "images", "image", "img", "media", "upload", "uploads",
	"download", "downloads", "file", "files", "document", "documents",

	// Communication
	"contact", "message", "messages", "chat", "notification", "notifications",
	"alert", "alerts", "inbox", "outbox", "sent", "draft", "drafts",
	"spam", "abuse", "report", "flag", "block", "mute", "ban",

	// Commerce & Billing
	"shop", "store", "cart", "checkout", "order", "orders", "invoice",
	"invoices", "payment", "payments", "subscription", "subscriptions",
	"plan", "plans", "pricing", "billing", "refund", "coupon", "discount",

	// Social Features
	"follow", "follower", "followers", "following", "friend", "friends",
	"like", "likes", "share", "shares", "comment", "comments", "reply",
	"mention", "mentions", "tag", "tags", "group", "groups", "team", "teams",
	"community", "communities", "forum", "forums", "channel", "channels",

	// Brand & Legal
	"official", "verified", "trusted", "partner", "affiliate", "sponsor",
	"brand", "trademark", "copyright", "legal", "terms", "privacy",
	"policy", "policies", "tos", "eula", "gdpr", "dmca", "abuse",

	// Offensive / Impersonation Prevention
	"fuck", "shit", "ass", "bitch", "bastard", "damn", "cunt", "dick",
	"penis", "vagina", "sex", "porn", "xxx", "nude", "naked", "nsfw",
	"kill", "murder", "death", "die", "suicide", "hate", "nazi", "hitler",
	"racist", "racism", "terrorist", "terrorism", "isis", "alqaeda",

	// Numbers & Special
	"0", "1", "123", "1234", "12345", "000", "111", "666", "911", "420", "69",

	// Common Spam Patterns
	"info", "noreply", "no-reply", "donotreply", "mailer", "postmaster",
	"webmaster", "hostmaster", "abuse", "spam", "junk", "trash",

	// Project-specific
	"weather", "apimgr",
}

// Critical terms that block substrings per AI.md PART 22
// These terms are blocked even if they appear anywhere in the username
var criticalSubstringTerms = []string{
	"admin", "root", "system", "mod", "official", "verified",
}

// IsUsernameBlocked checks if a username (email local part) is on the blocklist per AI.md PART 22
// Returns true if the username is blocked, false otherwise
func IsUsernameBlocked(email string) bool {
	// Extract username from email (everything before @)
	parts := strings.Split(email, "@")
	if len(parts) < 2 || parts[0] == "" {
		// Invalid email format or empty username
		return true
	}

	username := strings.ToLower(strings.TrimSpace(parts[0]))

	// Empty username check
	if username == "" {
		return true
	}

	// Check exact matches against full blocklist
	for _, blocked := range UsernameBlocklist {
		if username == strings.ToLower(blocked) {
			return true
		}
	}

	// Check for critical terms as substrings per AI.md PART 22
	// Block usernames containing "admin", "root", "system", "mod", "official", "verified"
	for _, critical := range criticalSubstringTerms {
		if strings.Contains(username, critical) {
			return true
		}
	}

	// Check for variations with numbers/special chars at the end
	// e.g., "test123", "user_1", "guest-test"
	// But NOT "testing" or "username" (where blocked word is part of a larger word)
	for _, blocked := range UsernameBlocklist {
		blockedLower := strings.ToLower(blocked)

		// Skip critical terms (already checked as substrings above)
		isCritical := false
		for _, critical := range criticalSubstringTerms {
			if blockedLower == critical {
				isCritical = true
				break
			}
		}
		if isCritical {
			continue
		}

		// Only check if username starts with the blocked term
		if !strings.HasPrefix(username, blockedLower) {
			continue
		}

		// Get what comes after the blocked term
		suffix := username[len(blockedLower):]

		// If there's a suffix, only block if it's "simple" (numbers/punctuation only)
		// This allows "testing" and "username" but blocks "test123" and "user_1"
		if suffix != "" && !isSimpleSuffix(suffix) {
			// Has complex suffix, allow it
			continue
		}

		// If we get here, it's either exact match or blocked term + simple suffix
		if suffix == "" || isSimpleSuffix(suffix) {
			return true
		}
	}

	return false
}

// isSimpleSuffix checks if a string contains only numbers, hyphens, and underscores
func isSimpleSuffix(s string) bool {
	if s == "" {
		return true
	}

	for _, c := range s {
		if !((c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// GetBlocklistSize returns the number of blocked usernames
func GetBlocklistSize() int {
	return len(UsernameBlocklist)
}

// IsBlocklistPublic always returns true - this blocklist is intentionally public
// to allow users to understand why their chosen username was rejected
func IsBlocklistPublic() bool {
	return true
}
