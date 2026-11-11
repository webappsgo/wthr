package path

import "testing"

func TestSafeDirPath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"/var/lib/wthr", "/var/lib/wthr", false},
		{"/var/lib//wthr/", "/var/lib/wthr", false},
		{"relative/dir", "relative/dir", false},
		{"./data", "data", false},
		{"", "", true},
		{"/var/../etc", "", true},
		{"..", "", true},
		{"a/../b", "", true},
		{"/var/lib/\x00wthr", "", true},
		{"/var/lib/a\nb", "", true},
		{"/var/lib/a\tb", "", true},
		{"/var/lib/a\u0085b", "", true},
		{"/var/lib/a..b", "/var/lib/a..b", false},
	}
	for _, c := range cases {
		got, err := SafeDirPath(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("SafeDirPath(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("SafeDirPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
