package util

import (
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestOriginRelativePath(t *testing.T) {
	cases := []struct {
		base, in, want string
	}{
		{"/", "/server/about", "/server/about"},
		{"", "/server/about", "/server/about"},
		{"/wthr", "/server/about", "/wthr/server/about"},
		{"/wthr/", "server/about", "/wthr/server/about"},
	}
	for _, c := range cases {
		cfg := &config.AppConfig{}
		cfg.Server.BaseURL = c.base
		if got := OriginRelativePath(cfg, c.in); got != c.want {
			t.Errorf("base %q path %q = %q, want %q", c.base, c.in, got, c.want)
		}
	}
	if got := OriginRelativePath(nil, "/x"); got != "/x" {
		t.Errorf("nil config = %q", got)
	}
}
