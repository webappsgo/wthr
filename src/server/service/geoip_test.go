package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedGeoIPFixtureFiles pre-creates all 4 expected mmdb files at geoipDir so
// that GeoIPService.loadDatabases takes its "already cached" branch and never
// calls out to the real network. NewGeoIPService has no injectable HTTP
// client, so pre-seeding disk state is the only network-free seam into its
// background goroutine (see NOTE at bottom of file).
func seedGeoIPFixtureFiles(t *testing.T, geoipDir string) {
	t.Helper()
	if err := os.MkdirAll(geoipDir, 0755); err != nil {
		t.Fatalf("failed to create geoip dir: %v", err)
	}
	for _, name := range []string{
		"dbip-city-ipv4.mmdb",
		"dbip-city-ipv6.mmdb",
		"geo-whois-asn-country.mmdb",
		"asn.mmdb",
	} {
		if err := os.WriteFile(filepath.Join(geoipDir, name), []byte("placeholder"), 0644); err != nil {
			t.Fatalf("failed to write fixture %s: %v", name, err)
		}
	}
}

// waitGeoIPEnabled blocks until the service's background loadDatabases
// goroutine has finished and flipped enabled=true, or fails the test after a
// bounded wait. This prevents the goroutine from leaking past test
// completion and racing with t.TempDir() cleanup.
func waitGeoIPEnabled(t *testing.T, gs *GeoIPService) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gs.IsEnabled() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("service did not become enabled in time (background loadDatabases goroutine never completed)")
}

// TestGeoIP_NewGeoIPService covers construction: paths are derived correctly
// from configDir. All 4 expected mmdb files are pre-seeded on disk so the
// background loadDatabases goroutine started by NewGeoIPService takes the
// cache-hit branch and never makes a real network call; the test waits for
// that goroutine to finish before returning so it cannot leak into later
// tests or race with temp-dir cleanup.
func TestGeoIP_NewGeoIPService(t *testing.T) {
	t.Run("normal dir", func(t *testing.T) {
		dir := t.TempDir()
		seedGeoIPFixtureFiles(t, filepath.Join(dir, "geoip"))

		gs := NewGeoIPService(dir)
		if gs == nil {
			t.Fatal("NewGeoIPService returned nil")
		}
		waitGeoIPEnabled(t, gs)

		wantDir := filepath.Join(dir, "geoip")
		if gs.cityIPv4Path != filepath.Join(wantDir, "dbip-city-ipv4.mmdb") {
			t.Errorf("cityIPv4Path = %q, want under %q", gs.cityIPv4Path, wantDir)
		}
		if gs.cityIPv6Path != filepath.Join(wantDir, "dbip-city-ipv6.mmdb") {
			t.Errorf("cityIPv6Path = %q, want under %q", gs.cityIPv6Path, wantDir)
		}
		if gs.countryPath != filepath.Join(wantDir, "geo-whois-asn-country.mmdb") {
			t.Errorf("countryPath = %q, want under %q", gs.countryPath, wantDir)
		}
		if gs.asnPath != filepath.Join(wantDir, "asn.mmdb") {
			t.Errorf("asnPath = %q, want under %q", gs.asnPath, wantDir)
		}
		if gs.cache == nil {
			t.Error("cache map should be initialized, not nil")
		}
		if gs.cacheTime == nil {
			t.Error("cacheTime map should be initialized, not nil")
		}
	})

	t.Run("nested path", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "a", "b", "c")
		seedGeoIPFixtureFiles(t, filepath.Join(dir, "geoip"))

		gs := NewGeoIPService(dir)
		if gs == nil {
			t.Fatal("NewGeoIPService returned nil")
		}
		waitGeoIPEnabled(t, gs)

		wantDir := filepath.Join(dir, "geoip")
		if gs.cityIPv4Path != filepath.Join(wantDir, "dbip-city-ipv4.mmdb") {
			t.Errorf("cityIPv4Path = %q, want under %q", gs.cityIPv4Path, wantDir)
		}
	})

	t.Run("empty configDir uses relative geoip dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		seedGeoIPFixtureFiles(t, filepath.Join(cwd, "geoip"))

		gs := NewGeoIPService("")
		if gs == nil {
			t.Fatal("NewGeoIPService returned nil")
		}
		waitGeoIPEnabled(t, gs)

		want := filepath.Join("geoip", "dbip-city-ipv4.mmdb")
		if gs.cityIPv4Path != want {
			t.Errorf("cityIPv4Path = %q, want %q", gs.cityIPv4Path, want)
		}
	})
}

// TestGeoIP_LookupIP_NotEnabled verifies LookupIP fails fast (no panic, clear
// error) when the databases have not been loaded/enabled yet — this is the
// state immediately after construction in a test environment with no real
// network access and no fixture MMDB files.
func TestGeoIP_LookupIP_NotEnabled(t *testing.T) {
	gs := &GeoIPService{
		cache:     make(map[string]*GeoIPData),
		cacheTime: make(map[string]int64),
	}

	data, err := gs.LookupIP("8.8.8.8")
	if err == nil {
		t.Fatal("expected error when databases not loaded, got nil")
	}
	if data != nil {
		t.Errorf("expected nil data on error, got %+v", data)
	}
}

// TestGeoIP_LookupIP_InvalidIP covers the invalid-IP-address error path,
// which is reached even when enabled since it is checked before any file I/O.
func TestGeoIP_LookupIP_InvalidIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"empty string", ""},
		{"garbage text", "not-an-ip"},
		{"truncated ipv4", "1.2.3"},
		{"out of range octet", "999.999.999.999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := &GeoIPService{
				enabled:   true,
				cache:     make(map[string]*GeoIPData),
				cacheTime: make(map[string]int64),
			}

			data, err := gs.LookupIP(tt.ip)
			if err == nil {
				t.Fatalf("expected error for invalid IP %q, got nil", tt.ip)
			}
			if data != nil {
				t.Errorf("expected nil data for invalid IP %q, got %+v", tt.ip, data)
			}
		})
	}
}

// TestGeoIP_LookupIP_PrivateAndLoopback covers the private/loopback rejection
// path for both IPv4 and IPv6, a boundary that must never leak to file I/O.
func TestGeoIP_LookupIP_PrivateAndLoopback(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"ipv4 loopback", "127.0.0.1"},
		{"ipv6 loopback", "::1"},
		{"private class A", "10.0.0.1"},
		{"private class B", "172.16.0.1"},
		{"private class C", "192.168.1.1"},
		{"ipv6 unique local", "fd00::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := &GeoIPService{
				enabled:   true,
				cache:     make(map[string]*GeoIPData),
				cacheTime: make(map[string]int64),
			}

			data, err := gs.LookupIP(tt.ip)
			if err == nil {
				t.Fatalf("expected error for private/local IP %q, got nil", tt.ip)
			}
			if data != nil {
				t.Errorf("expected nil data for private/local IP %q, got %+v", tt.ip, data)
			}
		})
	}
}

// TestGeoIP_LookupIP_CacheHit verifies the cache short-circuits before any
// file access, and does so idempotently across repeated calls.
func TestGeoIP_LookupIP_CacheHit(t *testing.T) {
	gs := &GeoIPService{
		enabled:   true,
		cache:     make(map[string]*GeoIPData),
		cacheTime: make(map[string]int64),
	}

	want := &GeoIPData{IP: "8.8.8.8", City: "Mountain View", CountryCode: "US"}
	gs.cache["8.8.8.8"] = want

	for i := 0; i < 3; i++ {
		got, err := gs.LookupIP("8.8.8.8")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if got != want {
			t.Errorf("call %d: got %+v, want same pointer %+v", i, got, want)
		}
	}
}

// TestGeoIP_LookupIP_MissingDBFallsBackToCountry_AlsoMissing exercises the
// "no city DB present" seam: LookupIP falls back to lookupCountry, which
// itself fails to open a missing country DB, returning a wrapped error
// rather than panicking.
func TestGeoIP_LookupIP_MissingDBFallsBackToCountry_AlsoMissing(t *testing.T) {
	dir := t.TempDir()
	gs := &GeoIPService{
		enabled:      true,
		cityIPv4Path: filepath.Join(dir, "missing-city-v4.mmdb"),
		cityIPv6Path: filepath.Join(dir, "missing-city-v6.mmdb"),
		countryPath:  filepath.Join(dir, "missing-country.mmdb"),
		cache:        make(map[string]*GeoIPData),
		cacheTime:    make(map[string]int64),
	}

	data, err := gs.LookupIP("8.8.8.8")
	if err == nil {
		t.Fatal("expected error when both city and country DBs are missing, got nil")
	}
	if data != nil {
		t.Errorf("expected nil data, got %+v", data)
	}
}

// TestGeoIP_LookupIP_CorruptCityDBFallsBackToCountry covers the "city file
// exists but is not a valid MMDB" seam: maxminddb.Open should fail, and
// LookupIP should fall back to the (also invalid) country DB and surface a
// clear error instead of panicking.
func TestGeoIP_LookupIP_CorruptCityDBFallsBackToCountry(t *testing.T) {
	dir := t.TempDir()
	cityPath := filepath.Join(dir, "corrupt-city.mmdb")
	countryPath := filepath.Join(dir, "corrupt-country.mmdb")

	if err := os.WriteFile(cityPath, []byte("not a real mmdb file"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	if err := os.WriteFile(countryPath, []byte("also not real"), 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	gs := &GeoIPService{
		enabled:      true,
		cityIPv4Path: cityPath,
		cityIPv6Path: cityPath,
		countryPath:  countryPath,
		cache:        make(map[string]*GeoIPData),
		cacheTime:    make(map[string]int64),
	}

	data, err := gs.LookupIP("8.8.8.8")
	if err == nil {
		t.Fatal("expected error for corrupt MMDB files, got nil")
	}
	if data != nil {
		t.Errorf("expected nil data, got %+v", data)
	}
}

// TestGeoIP_lookupCountry_MissingDB covers the country-only lookup path in
// isolation: a missing country DB must return a wrapped "failed to open"
// error, not a panic or nil-pointer dereference.
func TestGeoIP_lookupCountry_MissingDB(t *testing.T) {
	gs := &GeoIPService{
		countryPath: filepath.Join(t.TempDir(), "does-not-exist.mmdb"),
		cache:       make(map[string]*GeoIPData),
		cacheTime:   make(map[string]int64),
	}

	data, err := gs.lookupCountry(nil, "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for missing country database, got nil")
	}
	if data != nil {
		t.Errorf("expected nil data, got %+v", data)
	}
}

// TestGeoIP_IsEnabled_ReflectsInternalState is a boundary/idempotency check:
// reading IsEnabled repeatedly must be safe and must reflect whatever the
// internal flag currently is, in both the false and true states.
func TestGeoIP_IsEnabled_ReflectsInternalState(t *testing.T) {
	gs := &GeoIPService{}

	if gs.IsEnabled() {
		t.Error("IsEnabled() should be false on zero-value service")
	}
	// Idempotent repeated reads.
	if gs.IsEnabled() {
		t.Error("IsEnabled() should still be false on repeated read")
	}

	gs.mu.Lock()
	gs.enabled = true
	gs.mu.Unlock()

	if !gs.IsEnabled() {
		t.Error("IsEnabled() should be true after setting enabled = true")
	}
	if !gs.IsEnabled() {
		t.Error("IsEnabled() should still be true on repeated read")
	}
}

// TestGeoIP_Reload_FailsWithoutDownloadableDatabases exercises Reload's error
// path deterministically and without any real network access: a plain file
// is written at the exact path where the "geoip" directory needs to exist,
// so loadDatabases' os.MkdirAll call fails (ENOTDIR) before any network code
// runs. This is the only network-independent way to force loadDatabases to
// fail — actual network reachability cannot be assumed in the test
// environment, so a test that instead relied on "no network available" would
// be flaky/wrong.
func TestGeoIP_Reload_FailsWithoutDownloadableDatabases(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "geoip")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	gs := &GeoIPService{
		cityIPv4Path: filepath.Join(blocker, "dbip-city-ipv4.mmdb"),
		cityIPv6Path: filepath.Join(blocker, "dbip-city-ipv6.mmdb"),
		countryPath:  filepath.Join(blocker, "geo-whois-asn-country.mmdb"),
		asnPath:      filepath.Join(blocker, "asn.mmdb"),
		enabled:      true,
		cache:        make(map[string]*GeoIPData),
		cacheTime:    make(map[string]int64),
	}

	err := gs.Reload()
	if err == nil {
		t.Fatal("expected Reload to fail when the geoip directory cannot be created")
	}
	if gs.IsEnabled() {
		t.Error("service should not be enabled after a failed reload")
	}
}

// TestGeoIP_Reload_SucceedsWithPreSeededDatabases exercises Reload's happy
// path (cache-warm branch, no network) and proves the operation is
// idempotent: calling it twice in a row must succeed both times and leave
// the service enabled.
func TestGeoIP_Reload_SucceedsWithPreSeededDatabases(t *testing.T) {
	dir := t.TempDir()
	geoipDir := filepath.Join(dir, "geoip")
	seedGeoIPFixtureFiles(t, geoipDir)

	gs := &GeoIPService{
		cityIPv4Path: filepath.Join(geoipDir, "dbip-city-ipv4.mmdb"),
		cityIPv6Path: filepath.Join(geoipDir, "dbip-city-ipv6.mmdb"),
		countryPath:  filepath.Join(geoipDir, "geo-whois-asn-country.mmdb"),
		asnPath:      filepath.Join(geoipDir, "asn.mmdb"),
		cache:        make(map[string]*GeoIPData),
		cacheTime:    make(map[string]int64),
	}

	if err := gs.Reload(); err != nil {
		t.Fatalf("first Reload failed: %v", err)
	}
	if !gs.IsEnabled() {
		t.Error("service should be enabled after a successful reload")
	}

	// Idempotency: calling Reload again with the same on-disk state must
	// still succeed.
	if err := gs.Reload(); err != nil {
		t.Fatalf("second Reload (idempotency check) failed: %v", err)
	}
	if !gs.IsEnabled() {
		t.Error("service should still be enabled after a second reload")
	}
}

// TestGeoIP_LoadDatabases_AllFilesPresentEnablesService is the one testable
// seam into loadDatabases' happy path: when all 4 expected files already
// exist on disk (regardless of content), loadDatabases treats the cache as
// warm and sets enabled = true without attempting any network I/O. This is
// exercised directly (not via NewGeoIPService's goroutine) for determinism.
func TestGeoIP_LoadDatabases_AllFilesPresentEnablesService(t *testing.T) {
	dir := t.TempDir()
	geoipDir := filepath.Join(dir, "geoip")
	if err := os.MkdirAll(geoipDir, 0755); err != nil {
		t.Fatalf("failed to create geoip dir: %v", err)
	}

	gs := &GeoIPService{
		cityIPv4Path: filepath.Join(geoipDir, "dbip-city-ipv4.mmdb"),
		cityIPv6Path: filepath.Join(geoipDir, "dbip-city-ipv6.mmdb"),
		countryPath:  filepath.Join(geoipDir, "geo-whois-asn-country.mmdb"),
		asnPath:      filepath.Join(geoipDir, "asn.mmdb"),
		cache:        make(map[string]*GeoIPData),
		cacheTime:    make(map[string]int64),
	}

	for _, p := range []string{gs.cityIPv4Path, gs.cityIPv6Path, gs.countryPath, gs.asnPath} {
		if err := os.WriteFile(p, []byte("placeholder"), 0644); err != nil {
			t.Fatalf("failed to write fixture %s: %v", p, err)
		}
	}

	gs.loadDatabases()

	if !gs.IsEnabled() {
		t.Error("loadDatabases should enable the service when all 4 files already exist")
	}

	// Idempotency: calling it again with the same on-disk state should be safe
	// and leave the service enabled.
	gs.loadDatabases()
	if !gs.IsEnabled() {
		t.Error("loadDatabases should remain idempotent and keep the service enabled")
	}
}

// TestGeoIP_downloadDatabase_NetworkErrorSurfaced verifies that a malformed
// URL (guaranteed to fail without touching the real network) produces an
// error instead of a panic. This does not perform any real network call —
// the scheme is intentionally unsupported so http.Get fails locally during
// request construction/transport selection.
func TestGeoIP_downloadDatabase_NetworkErrorSurfaced(t *testing.T) {
	gs := &GeoIPService{}
	dest := filepath.Join(t.TempDir(), "out.mmdb")

	err := gs.downloadDatabase("not-a-valid://url", dest)
	if err == nil {
		t.Fatal("expected error for invalid URL scheme, got nil")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("destination file should not have been created on download failure")
	}
}

// NOTE on coverage gaps:
//   - This test file makes no real network calls. NewGeoIPService's
//     background goroutine and Reload always take the network-free
//     "cache warm" branch here, because every test that triggers them
//     pre-seeds all 4 expected mmdb files via seedGeoIPFixtureFiles first.
//     UpdateDatabase/updateSingleDatabase's and loadDatabases' actual
//     *download* success paths (the branch that calls http.Get against
//     the ip-location-db release and CDN hosts) have no network-free seam — this environment has
//     real internet access, so a test cannot rely on "no network" to force
//     that branch to fail either. Only the deterministic, network-independent
//     failure seam (os.MkdirAll failing because a plain file occupies the
//     directory path) and the cache-warm success seam are covered.
//   - LookupIP's successful city/country hit paths (parsing a real MMDB
//     record) are not covered because no valid fixture MMDB binary is
//     constructed here; the surrounding seams (cache, private/invalid IP
//     rejection, missing/corrupt file fallback) are fully covered instead.
