package buildstamp

import "testing"

func TestStampAndAsset(t *testing.T) {
	Set("1.2.3", "abcdef1234567")
	if got := Stamp(); got != "1.2.3-abcdef1" {
		t.Fatalf("Stamp = %q", got)
	}
	if got := Asset("/static/js/app.js"); got != "/static/js/app.js?v=1.2.3-abcdef1" {
		t.Fatalf("Asset = %q", got)
	}
	if got := Asset("/a?x=1"); got != "/a?x=1&v=1.2.3-abcdef1" {
		t.Fatalf("Asset with query = %q", got)
	}
	if ETag() != `"1.2.3-abcdef1"` {
		t.Fatalf("ETag = %q", ETag())
	}
	Set("devel", "N/A")
	if Stamp() != "devel" {
		t.Fatalf("Stamp without commit = %q", Stamp())
	}
}
