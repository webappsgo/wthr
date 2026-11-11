package health

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reset() {
	mu.Lock()
	entries = nil
	mu.Unlock()
}

func TestRegistryOrderOmitAndAggregation(t *testing.T) {
	reset()
	Register("geoip_country", false, func() Status { return StatusOK })
	Register("data_off", false, func() Status { return StatusOmit })
	Register("data_countries", true, func() Status { return StatusOK })

	results := Results()
	if len(results) != 2 || results[0].Key != "geoip_country" || results[1].Key != "data_countries" {
		t.Fatalf("results = %+v", results)
	}
	if u, d := Worst(); u || d {
		t.Fatalf("all ok must not degrade: unhealthy=%v degraded=%v", u, d)
	}

	state := StatusDegraded
	Register("blocklist_ads", false, func() Status { return state })
	if u, d := Worst(); u || !d {
		t.Fatalf("non-critical degraded must degrade only: unhealthy=%v degraded=%v", u, d)
	}

	Register("data_cities", true, func() Status { return StatusError })
	if u, _ := Worst(); !u {
		t.Fatal("critical error must be unhealthy")
	}
}

func TestRefreshUsesCachedResultsBetweenRuns(t *testing.T) {
	reset()
	calls := 0
	state := StatusOK
	Register("data_x", false, func() Status { calls++; return state })
	base := calls
	state = StatusError
	_ = Results()
	if calls != base {
		t.Fatal("Results must not run checks")
	}
	if Results()[0].Status != StatusOK {
		t.Fatal("results must stay cached until Refresh")
	}
	Refresh()
	if Results()[0].Status != StatusError {
		t.Fatal("Refresh must update the cached status")
	}
}

func TestFileCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.mmdb")
	if FileCheck(path, time.Hour, nil)() != StatusError {
		t.Fatal("missing file must be an error")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if FileCheck(path, time.Hour, nil)() != StatusError {
		t.Fatal("empty file must be an error")
	}
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if FileCheck(path, time.Hour, nil)() != StatusOK {
		t.Fatal("fresh file must be ok")
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if FileCheck(path, time.Hour, nil)() != StatusDegraded {
		t.Fatal("stale file must be degraded")
	}
	if FileCheck(path, time.Hour, func() bool { return false })() != StatusOmit {
		t.Fatal("disabled feature must be omitted")
	}
}
