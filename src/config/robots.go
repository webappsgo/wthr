package config

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	// DefaultSitemapMaxURLs is the sitemap protocol limit per file.
	DefaultSitemapMaxURLs = 50000
	// DefaultFeedMaxItems is the default number of entries per feed.
	DefaultFeedMaxItems = 50
	// MaxFeedMaxItems is the largest allowed feed size.
	MaxFeedMaxItems = 200
	// maxRobotsPathLength bounds one robots path.
	maxRobotsPathLength = 255
	// maxRobotsPaths bounds how many paths one list may hold.
	maxRobotsPaths = 100
)

// RobotsConfig holds the robots.txt allow and deny paths (AI.md PART 16). The
// `{admin_path}` placeholder is replaced by the configured admin path.
type RobotsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// DefaultRobotsConfig returns the AI.md PART 16 default robots rules.
func DefaultRobotsConfig() RobotsConfig {
	return RobotsConfig{
		Allow: []string{"/", "/api"},
		Deny:  []string{"/server/{admin_path}"},
	}
}

// NormalizeRobotsPaths trims, validates and de-duplicates robots paths. Every
// path must start with "/", hold no whitespace or control characters and stay
// within the length and count limits; offending entries are dropped and
// described in the returned messages.
func NormalizeRobotsPaths(paths []string) ([]string, []string) {
	cleaned := make([]string, 0, len(paths))
	var problems []string
	seen := map[string]bool{}
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		switch {
		case !strings.HasPrefix(path, "/"):
			problems = append(problems, fmt.Sprintf("path %q must start with /", path))
		case len(path) > maxRobotsPathLength:
			problems = append(problems, fmt.Sprintf("path %q is longer than %d characters", path[:32]+"...", maxRobotsPathLength))
		case strings.IndexFunc(path, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0:
			problems = append(problems, fmt.Sprintf("path %q contains whitespace or control characters", path))
		case seen[path]:
		case len(cleaned) >= maxRobotsPaths:
			problems = append(problems, fmt.Sprintf("more than %d paths, %q ignored", maxRobotsPaths, path))
		default:
			seen[path] = true
			cleaned = append(cleaned, path)
		}
	}
	return cleaned, problems
}

// expandRobotsPath replaces the {admin_path} placeholder.
func expandRobotsPath(path, adminPath string) string {
	return strings.ReplaceAll(path, "{admin_path}", adminPath)
}

// RobotsTxt renders the robots.txt body: the allow and deny rules for every
// crawler and the absolute sitemap URL. AI crawlers get no special treatment.
func (r RobotsConfig) RobotsTxt(adminPath, appURL string) string {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for _, path := range r.Allow {
		b.WriteString("Allow: " + expandRobotsPath(path, adminPath) + "\n")
	}
	for _, path := range r.Deny {
		b.WriteString("Disallow: " + expandRobotsPath(path, adminPath) + "\n")
	}
	b.WriteString("Sitemap: " + strings.TrimRight(appURL, "/") + "/sitemap.xml\n")
	return b.String()
}

// Allows reports whether crawlers may fetch path under these rules. The most
// specific (longest) matching rule wins and Allow wins a tie; a path no rule
// matches is allowed, as in the robots exclusion protocol (RFC 9309).
func (r RobotsConfig) Allows(path, adminPath string) bool {
	type rule struct {
		prefix string
		allow  bool
	}
	rules := make([]rule, 0, len(r.Allow)+len(r.Deny))
	for _, p := range r.Allow {
		rules = append(rules, rule{expandRobotsPath(p, adminPath), true})
	}
	for _, p := range r.Deny {
		rules = append(rules, rule{expandRobotsPath(p, adminPath), false})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if len(rules[i].prefix) != len(rules[j].prefix) {
			return len(rules[i].prefix) > len(rules[j].prefix)
		}
		return rules[i].allow && !rules[j].allow
	})
	for _, candidate := range rules {
		if strings.HasPrefix(path, candidate.prefix) {
			return candidate.allow
		}
	}
	return true
}

// UpdateWebRobots validates and persists the robots allow and deny paths.
func UpdateWebRobots(allow, deny []string) []ValidationError {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return []ValidationError{{Field: "web.robots", Message: "global config not initialized"}}
	}
	cleanAllow, allowProblems := NormalizeRobotsPaths(allow)
	cleanDeny, denyProblems := NormalizeRobotsPaths(deny)
	var errs []ValidationError
	for _, message := range allowProblems {
		errs = append(errs, ValidationError{Field: "web.robots.allow", Message: message})
	}
	for _, message := range denyProblems {
		errs = append(errs, ValidationError{Field: "web.robots.deny", Message: message})
	}
	if len(errs) > 0 {
		return errs
	}
	cfg.Web.Robots = RobotsConfig{Allow: cleanAllow, Deny: cleanDeny}
	if err := SaveConfig(cfg); err != nil {
		return []ValidationError{{Field: "web.robots", Message: err.Error()}}
	}
	return nil
}
