package service

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// setupSecurityTxtServerDB opens a fresh in-memory SQLite database with the
// real production ServerSchema applied (server_config lives there),
// uniquely named per test via t.Name().
func setupSecurityTxtServerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_securitytxt?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return db
}

// wireSecurityTxtGlobalDB wires database.SetGlobalDualDB since SettingsModel
// reads/writes via database.GetServerDB() rather than its injected DB
// field, and restores nil afterward.
func wireSecurityTxtGlobalDB(t *testing.T, serverDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// newSecurityTxtService is the standard fixture: in-memory DB with schema
// applied, global DB wired, and a real SettingsModel/SecurityTxtService.
func newSecurityTxtService(t *testing.T) (*SecurityTxtService, *sql.DB) {
	t.Helper()
	db := setupSecurityTxtServerDB(t)
	wireSecurityTxtGlobalDB(t, db)
	sm := &models.SettingsModel{DB: db}
	return NewSecurityTxtService(sm), db
}

// TestSecurityTxt_Generate_DefaultContact covers the happy path when no
// security.contact setting exists: it must fall back to the config-derived
// default address (security@wthr.top, since no global AppConfig or FQDN is
// set in tests).
func TestSecurityTxt_Generate_DefaultContact(t *testing.T) {
	svc, _ := newSecurityTxtService(t)
	out := svc.Generate("https://example.com")

	if !strings.Contains(out, "Contact: mailto:security@wthr.top") {
		t.Errorf("expected default contact line, got:\n%s", out)
	}
	if !strings.Contains(out, "# This file follows RFC 9116") {
		t.Errorf("expected RFC 9116 comment header, got:\n%s", out)
	}
	if !strings.Contains(out, "Preferred-Languages: en") {
		t.Errorf("expected default language line, got:\n%s", out)
	}
	if !strings.Contains(out, "Canonical: https://example.com/.well-known/security.txt") {
		t.Errorf("expected default canonical line, got:\n%s", out)
	}
}

// TestSecurityTxt_Generate_MultiContactAndMailtoPrefix covers comma-split
// multi-contact support and auto-mailto-prefixing only for values that
// contain "@" and are not already a mailto:/http(s) URI.
func TestSecurityTxt_Generate_MultiContactAndMailtoPrefix(t *testing.T) {
	svc, db := newSecurityTxtService(t)
	sm := &models.SettingsModel{DB: db}
	if err := sm.SetString("security.contact", "security@example.com, https://example.com/report, mailto:already@example.com"); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	out := svc.Generate("https://example.com")

	wantLines := []string{
		"Contact: mailto:security@example.com",
		"Contact: https://example.com/report",
		"Contact: mailto:already@example.com",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("expected line %q in output:\n%s", want, out)
		}
	}
}

// TestSecurityTxt_Generate_ExpiresAutoRenew_Empty covers the boundary where
// security.expires is unset: Generate must auto-renew to ~1 year from now
// AND persist it, so a subsequent call returns the SAME value (idempotency
// of the side-effecting read after the first renewal).
func TestSecurityTxt_Generate_ExpiresAutoRenew_Empty(t *testing.T) {
	svc, _ := newSecurityTxtService(t)

	out1 := svc.Generate("https://example.com")
	expires1 := extractSecurityTxtField(t, out1, "Expires")
	parsed1, err := time.Parse(time.RFC3339, expires1)
	if err != nil {
		t.Fatalf("parse first Expires value %q: %v", expires1, err)
	}
	wantApprox := time.Now().AddDate(1, 0, 0)
	if diff := parsed1.Sub(wantApprox); diff > time.Minute || diff < -time.Minute {
		t.Errorf("Expires = %v, want ~%v", parsed1, wantApprox)
	}

	// Idempotency: generating again must reuse the persisted value, not
	// silently drift forward another year.
	out2 := svc.Generate("https://example.com")
	expires2 := extractSecurityTxtField(t, out2, "Expires")
	if expires1 != expires2 {
		t.Errorf("expected persisted Expires to be stable across calls, got %q then %q", expires1, expires2)
	}
}

// TestSecurityTxt_Generate_ExpiresAutoRenew_Unparsable covers a corrupted
// stored value (not RFC3339): must be treated the same as empty (auto-renew
// and persist).
func TestSecurityTxt_Generate_ExpiresAutoRenew_Unparsable(t *testing.T) {
	svc, db := newSecurityTxtService(t)
	sm := &models.SettingsModel{DB: db}
	if err := sm.SetString("security.expires", "not-a-date"); err != nil {
		t.Fatalf("seed expires: %v", err)
	}

	out := svc.Generate("https://example.com")
	expires := extractSecurityTxtField(t, out, "Expires")
	if _, err := time.Parse(time.RFC3339, expires); err != nil {
		t.Errorf("expected renewed Expires to be valid RFC3339, got %q: %v", expires, err)
	}
}

// TestSecurityTxt_Generate_ExpiresAutoRenew_Past covers a stored value that
// is a valid timestamp but already in the past: must be renewed, not kept.
func TestSecurityTxt_Generate_ExpiresAutoRenew_Past(t *testing.T) {
	svc, db := newSecurityTxtService(t)
	sm := &models.SettingsModel{DB: db}
	past := time.Now().AddDate(0, 0, -1).Format(time.RFC3339)
	if err := sm.SetString("security.expires", past); err != nil {
		t.Fatalf("seed expires: %v", err)
	}

	out := svc.Generate("https://example.com")
	expires := extractSecurityTxtField(t, out, "Expires")
	if expires == past {
		t.Error("expected past Expires value to be renewed, but it was unchanged")
	}
}

// TestSecurityTxt_Generate_ExpiresPreservedWhenValid covers the case where
// a valid future Expires is already stored: Generate must NOT overwrite it.
func TestSecurityTxt_Generate_ExpiresPreservedWhenValid(t *testing.T) {
	svc, db := newSecurityTxtService(t)
	sm := &models.SettingsModel{DB: db}
	future := time.Now().AddDate(0, 6, 0).Format(time.RFC3339)
	if err := sm.SetString("security.expires", future); err != nil {
		t.Fatalf("seed expires: %v", err)
	}

	out := svc.Generate("https://example.com")
	expires := extractSecurityTxtField(t, out, "Expires")
	if expires != future {
		t.Errorf("expected preserved Expires %q, got %q", future, expires)
	}
}

// TestSecurityTxt_Generate_OptionalFields covers that Encryption,
// Acknowledgments, Policy, and Hiring lines only appear when non-empty, and
// do appear (with correct values) when set.
func TestSecurityTxt_Generate_OptionalFields(t *testing.T) {
	svc, db := newSecurityTxtService(t)
	sm := &models.SettingsModel{DB: db}

	// Boundary: none set, none of the optional lines should appear.
	out := svc.Generate("https://example.com")
	for _, prefix := range []string{"Encryption:", "Acknowledgments:", "Policy:", "Hiring:"} {
		if strings.Contains(out, prefix) {
			t.Errorf("did not expect %q line when unset, got:\n%s", prefix, out)
		}
	}

	// Happy path: all set, all lines should appear with the right values.
	seed := map[string]string{
		"security.encryption":      "https://example.com/pgp-key.txt",
		"security.acknowledgments": "https://example.com/hall-of-fame",
		"security.policy":          "https://example.com/policy",
		"security.hiring":          "https://example.com/jobs",
	}
	for k, v := range seed {
		if err := sm.SetString(k, v); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
	out = svc.Generate("https://example.com")
	wantLines := []string{
		"Encryption: https://example.com/pgp-key.txt",
		"Acknowledgments: https://example.com/hall-of-fame",
		"Policy: https://example.com/policy",
		"Hiring: https://example.com/jobs",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("expected line %q in output:\n%s", want, out)
		}
	}
}

// TestSecurityTxt_GetConfig covers both the empty-DB default map and a
// fully-populated map, verifying all 8 keys are present.
func TestSecurityTxt_GetConfig(t *testing.T) {
	svc, db := newSecurityTxtService(t)

	cfg := svc.GetConfig()
	wantKeys := []string{"contact", "expires", "languages", "canonical", "encryption", "acknowledgments", "policy", "hiring"}
	for _, k := range wantKeys {
		if _, ok := cfg[k]; !ok {
			t.Errorf("expected key %q in GetConfig() result, got %v", k, cfg)
		}
	}
	if cfg["languages"] != "en" {
		t.Errorf("languages default = %v, want %q", cfg["languages"], "en")
	}
	if cfg["contact"] != "" {
		t.Errorf("contact default = %v, want empty string", cfg["contact"])
	}

	sm := &models.SettingsModel{DB: db}
	if err := sm.SetString("security.contact", "sec@example.com"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg = svc.GetConfig()
	if cfg["contact"] != "sec@example.com" {
		t.Errorf("contact = %v, want %q", cfg["contact"], "sec@example.com")
	}
}

// TestSecurityTxt_UpdateConfig covers writing multiple keys at once and
// verifies each is persisted and readable back via GetConfig.
func TestSecurityTxt_UpdateConfig(t *testing.T) {
	svc, _ := newSecurityTxtService(t)

	err := svc.UpdateConfig(map[string]string{
		"contact":   "new-sec@example.com",
		"languages": "en, es",
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	cfg := svc.GetConfig()
	if cfg["contact"] != "new-sec@example.com" {
		t.Errorf("contact = %v, want %q", cfg["contact"], "new-sec@example.com")
	}
	if cfg["languages"] != "en, es" {
		t.Errorf("languages = %v, want %q", cfg["languages"], "en, es")
	}
}

// TestSecurityTxt_UpdateConfig_Empty covers the boundary of an empty map:
// must be a safe no-op, not an error.
func TestSecurityTxt_UpdateConfig_Empty(t *testing.T) {
	svc, _ := newSecurityTxtService(t)
	if err := svc.UpdateConfig(map[string]string{}); err != nil {
		t.Errorf("UpdateConfig(empty) unexpected error: %v", err)
	}
}

// TestSecurityTxt_CheckExpiry is table-driven over: unset, unparsable,
// expiring soon (<30d), and far in the future.
func TestSecurityTxt_CheckExpiry(t *testing.T) {
	tests := []struct {
		name           string
		seed           string // empty means do not seed
		wantNeedsRenew bool
	}{
		{"unset", "", true},
		{"unparsable", "garbage", true},
		{"expiring in 10 days", time.Now().Add(10 * 24 * time.Hour).Format(time.RFC3339), true},
		{"already expired", time.Now().Add(-24 * time.Hour).Format(time.RFC3339), true},
		{"far future", time.Now().AddDate(2, 0, 0).Format(time.RFC3339), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := newSecurityTxtService(t)
			if tt.seed != "" {
				sm := &models.SettingsModel{DB: db}
				if err := sm.SetString("security.expires", tt.seed); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			needsRenewal, _ := svc.CheckExpiry()
			if needsRenewal != tt.wantNeedsRenew {
				t.Errorf("CheckExpiry() needsRenewal = %v, want %v", needsRenewal, tt.wantNeedsRenew)
			}
		})
	}
}

// TestSecurityTxt_AutoRenew covers the happy path (sets ~1yr from now) and
// idempotency (calling twice in a row both succeed and each leaves a valid,
// non-expiring value).
func TestSecurityTxt_AutoRenew(t *testing.T) {
	svc, db := newSecurityTxtService(t)

	if err := svc.AutoRenew(); err != nil {
		t.Fatalf("AutoRenew: %v", err)
	}
	sm := &models.SettingsModel{DB: db}
	got := sm.GetString("security.expires", "")
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parse renewed expires %q: %v", got, err)
	}
	want := time.Now().AddDate(1, 0, 0)
	if diff := parsed.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("AutoRenew set Expires = %v, want ~%v", parsed, want)
	}

	// Idempotency: calling again succeeds and still yields a non-expired value.
	if err := svc.AutoRenew(); err != nil {
		t.Fatalf("second AutoRenew: %v", err)
	}
	needsRenewal, _ := svc.CheckExpiry()
	if needsRenewal {
		t.Error("expected CheckExpiry to report no renewal needed after AutoRenew")
	}
}

// extractSecurityTxtField pulls the value portion of a "Field: value" line
// out of a generated security.txt body, failing the test if absent.
func extractSecurityTxtField(t *testing.T, content, field string) string {
	t.Helper()
	prefix := field + ": "
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("field %q not found in content:\n%s", field, content)
	return ""
}
