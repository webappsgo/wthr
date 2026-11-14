package config

import (
	"strings"
	"testing"
)

func TestRobotsTxtGeneration(t *testing.T) {
	got := DefaultRobotsConfig().RobotsTxt("administration", "https://wthr.example/")
	want := "User-agent: *\nAllow: /\nAllow: /api\nDisallow: /server/administration\nSitemap: https://wthr.example/sitemap.xml\n"
	if got != want {
		t.Errorf("RobotsTxt() =\n%q\nwant\n%q", got, want)
	}
}

func TestRobotsAllowsLongestMatchWins(t *testing.T) {
	rules := RobotsConfig{
		Allow: []string{"/", "/server/docs"},
		Deny:  []string{"/server", "/earthquakes"},
	}
	tests := map[string]bool{
		"/":                     true,
		"/weather":              true,
		"/earthquakes":          false,
		"/earthquakes/detail/1": false,
		"/server/about":         false,
		"/server/docs/swagger":  true,
	}
	for path, want := range tests {
		if got := rules.Allows(path, "administration"); got != want {
			t.Errorf("Allows(%q) = %v, want %v", path, got, want)
		}
	}
	if !(RobotsConfig{}).Allows("/anything", "administration") {
		t.Error("with no rules everything is allowed")
	}
}

func TestRobotsAdminPathPlaceholder(t *testing.T) {
	rules := DefaultRobotsConfig()
	if rules.Allows("/server/hidden-admin/settings", "hidden-admin") {
		t.Error("the configured admin path must be denied")
	}
	if !rules.Allows("/server/about", "hidden-admin") {
		t.Error("other /server pages stay allowed")
	}
}

func TestNormalizeRobotsPaths(t *testing.T) {
	clean, problems := NormalizeRobotsPaths([]string{" /a ", "/a", "b", "", "/c d", "/ok\x00", "/fine"})
	if strings.Join(clean, ",") != "/a,/fine" {
		t.Errorf("clean = %v, want [/a /fine]", clean)
	}
	if len(problems) != 3 {
		t.Errorf("problems = %v, want 3 (no leading slash, space, control char)", problems)
	}
}

func TestValidateServerSEOReplacesOutOfRange(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Server.SEO.Feed.MaxItems = 500
	cfg.Server.SEO.Sitemap.MaxURLs = -3
	validateServerSEO(cfg)
	if cfg.Server.SEO.Feed.MaxItems != DefaultFeedMaxItems || cfg.Server.SEO.Sitemap.MaxURLs != DefaultSitemapMaxURLs {
		t.Errorf("seo = %+v, want defaults", cfg.Server.SEO)
	}
}

func TestValidateWebRobotsFallsBackToDefaults(t *testing.T) {
	cfg := &AppConfig{}
	cfg.Web.Robots = RobotsConfig{Allow: []string{"no-slash"}}
	validateWebRobots(cfg)
	want := DefaultRobotsConfig()
	if strings.Join(cfg.Web.Robots.Allow, ",") != strings.Join(want.Allow, ",") {
		t.Errorf("robots = %+v, want defaults", cfg.Web.Robots)
	}
}
