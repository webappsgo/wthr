package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicHealthChecksJSONOrdersDatasetsLast(t *testing.T) {
	checks := publicHealthChecks{
		Database: "ok", Cache: "ok", Disk: "ok", Scheduler: "ok", Tor: "ok",
		Datasets: []publicDatasetCheck{
			{Key: "geoip_country", Status: "ok"},
			{Key: "data_countries", Status: "degraded"},
		},
	}
	raw, err := json.Marshal(checks)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"database":"ok","cache":"ok","disk":"ok","scheduler":"ok","tor":"ok","geoip_country":"ok","data_countries":"degraded"}`
	if string(raw) != want {
		t.Fatalf("checks JSON = %s, want %s", raw, want)
	}
	if strings.Contains(string(raw), "i2p") || strings.Contains(string(raw), "cluster") {
		t.Fatal("disabled checks must be omitted")
	}
}

func TestHumanizeCheckKey(t *testing.T) {
	cases := map[string]string{
		"blocklist_ads":  "Blocklist: ads",
		"data_countries": "Data: countries",
		"geoip_asn":      "Geoip: asn",
		"single":         "Single",
	}
	for in, want := range cases {
		if got := humanizeCheckKey(in); got != want {
			t.Errorf("humanizeCheckKey(%q) = %q, want %q", in, got, want)
		}
	}
}
