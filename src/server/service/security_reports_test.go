package service

import (
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

func newSecurityTestStore(t *testing.T) *SecurityReportStore {
	t.Helper()
	ddb, err := database.InitDualDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDualDB: %v", err)
	}
	t.Cleanup(func() { ddb.Close() })
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	return &SecurityReportStore{DB: ddb.Server, EncryptionKey: key, PGP: NewPGPKeyManager(ddb.Server, t.TempDir())}
}

func sampleReport() SecurityReport {
	return SecurityReport{ResearcherName: "Ada", Component: "api", Severity: "high", Summary: "xss", Steps: "1. do", Impact: "bad",
		CreditPreference: "handle", CreditHandle: "ada77", CVERequested: true}
}

func TestSecurityIDWindows(t *testing.T) {
	store := newSecurityTestStore(t)
	id, err := CurrentSecurityID(store.DB)
	if err != nil || len(id) != 16 {
		t.Fatalf("CurrentSecurityID = %q, %v", id, err)
	}
	if !ValidSecurityID(store.DB, id) {
		t.Error("current id must validate")
	}
	secret, _ := InstallationSecret(store.DB)
	if !ValidSecurityID(store.DB, securityIDFor(secret, time.Now().Add(-securityIDWindow*time.Second))) {
		t.Error("previous window id must validate")
	}
	if ValidSecurityID(store.DB, securityIDFor(secret, time.Now().Add(-3*securityIDWindow*time.Second))) {
		t.Error("an id three windows old must not validate")
	}
	for _, bad := range []string{"", "short", strings.Repeat("z", 16)} {
		if ValidSecurityID(store.DB, bad) {
			t.Errorf("%q must not validate", bad)
		}
	}
	again, _ := InstallationSecret(store.DB)
	if string(again) != string(secret) {
		t.Error("installation secret must be stable")
	}
	if fp := SecurityIDFingerprint(id); len(fp) != 8 || strings.Contains(id, fp) && fp == id[:8] {
		t.Errorf("fingerprint %q must be 8 chars and not the raw id prefix", fp)
	}
}

func TestSecurityReportAESRoundTrip(t *testing.T) {
	store := newSecurityTestStore(t)
	tracking, token, err := store.Create("0123456789abcdef", sampleReport(), ReportContact{Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(tracking, "sec_") || len(tracking) != 20 || len(token) != 64 {
		t.Fatalf("bad ids: %q %q", tracking, token)
	}
	var stored []byte
	var alg string
	_ = store.DB.QueryRow(`SELECT body_enc, body_alg FROM security_reports WHERE tracking_id = ?`, tracking).Scan(&stored, &alg)
	if alg != "aes-256-gcm" || strings.Contains(string(stored), "xss") {
		t.Fatalf("body must be encrypted with aes-256-gcm, alg=%q", alg)
	}
	var raw string
	_ = store.DB.QueryRow(`SELECT security_id FROM security_reports WHERE tracking_id = ?`, tracking).Scan(&raw)
	if raw == "0123456789abcdef" {
		t.Error("the raw security id must never be stored")
	}
	report, contact, err := store.Body(tracking)
	if err != nil || report.Summary != "xss" || contact == nil || contact.Email != "ada@example.com" {
		t.Fatalf("Body = %+v %+v %v", report, contact, err)
	}
	if _, _, err := store.Body("sec_missing"); !errors.Is(err, ErrReportNotFound) {
		t.Errorf("missing report error = %v", err)
	}
}

func TestSecurityReportPGPRoundTrip(t *testing.T) {
	store := newSecurityTestStore(t)
	if store.PGP.Exists() {
		t.Fatal("no keypair yet")
	}
	info, err := store.PGP.Generate("Weather", "security@example.com", false)
	if err != nil || len(info.Fingerprint) != 40 || info.ExpiresAt.Before(time.Now().Add(700*24*time.Hour)) {
		t.Fatalf("Generate = %+v %v", info, err)
	}
	if !store.PGP.Exists() {
		t.Fatal("keypair must exist after Generate")
	}
	tracking, _, err := store.Create("0123456789abcdef", sampleReport(), ReportContact{Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var alg string
	var stored []byte
	_ = store.DB.QueryRow(`SELECT body_alg, body_enc FROM security_reports WHERE tracking_id = ?`, tracking).Scan(&alg, &stored)
	if alg != "pgp" || !strings.HasPrefix(string(stored), "-----BEGIN PGP MESSAGE") {
		t.Fatalf("expected a PGP body, alg=%q", alg)
	}
	report, contact, err := store.Body(tracking)
	if err != nil || report.Summary != "xss" || contact == nil || contact.Email != "ada@example.com" {
		t.Fatalf("Body = %+v %+v %v", report, contact, err)
	}
	pub, err := store.PGP.PublicKey()
	if err != nil || !strings.Contains(pub, "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Fatalf("PublicKey = %v", err)
	}
	priv, err := store.PGP.PrivateKey()
	if err != nil || !strings.Contains(priv, "BEGIN PGP PRIVATE KEY BLOCK") {
		t.Fatalf("PrivateKey = %v", err)
	}

	// researcher keys: validation re-armors the public part and drops private material
	cleaned, err := ValidatePublicKey(priv)
	if err != nil || strings.Contains(cleaned, "PRIVATE") {
		t.Fatalf("ValidatePublicKey(private) = %q %v", cleaned, err)
	}
	sealed, err := EncryptToArmoredKey(pub, []byte("hello"))
	if err != nil || !strings.Contains(string(sealed), "BEGIN PGP MESSAGE") {
		t.Fatalf("EncryptToArmoredKey = %v", err)
	}
	if plain, err := store.PGP.Decrypt(sealed); err != nil || string(plain) != "hello" {
		t.Fatalf("Decrypt = %q %v", plain, err)
	}
	if _, err := ValidatePublicKey("not a key"); err == nil {
		t.Error("garbage must not validate")
	}
	if _, err := EncryptToArmoredKey("nope", []byte("x")); err == nil {
		t.Error("EncryptToArmoredKey must reject garbage")
	}

	// rotation keeps one row and stamps last_rotated_at
	rotated, err := store.PGP.Generate("Weather", "security@example.com", true)
	if err != nil || rotated.LastRotatedAt == nil || rotated.Fingerprint == info.Fingerprint {
		t.Fatalf("rotate = %+v %v", rotated, err)
	}

	// export then import restores the same identity without a mismatch warning
	exported, _ := store.PGP.PrivateKey()
	warning, err := store.PGP.ImportPrivate(exported, "Weather Security")
	if err != nil || warning != "" {
		t.Fatalf("ImportPrivate = %q %v", warning, err)
	}
	if warning, _ := store.PGP.ImportPrivate(exported, "someone else"); warning != "identity_mismatch" {
		t.Errorf("identity mismatch warning = %q", warning)
	}
	if _, err := store.PGP.ImportPrivate("garbage", "x"); err == nil {
		t.Error("garbage private key must be rejected")
	}

	// delete revokes and removes the files
	if err := store.PGP.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.PGP.Exists() {
		t.Error("keypair must not exist after Delete")
	}
	if _, err := store.PGP.PublicKey(); !errors.Is(err, ErrNoKeypair) {
		t.Errorf("PublicKey after Delete = %v", err)
	}
}

func TestSecurityReportStatusMoves(t *testing.T) {
	store := newSecurityTestStore(t)
	tracking, _, _ := store.Create("0123456789abcdef", sampleReport(), ReportContact{})
	if _, err := store.Update(tracking, "triaged", "looking", "2026-12-01"); err != nil {
		t.Fatalf("forward move: %v", err)
	}
	if _, err := store.Update(tracking, "received", "", ""); !errors.Is(err, ErrBadStatusMove) {
		t.Errorf("backward move error = %v", err)
	}
	if _, err := store.Update(tracking, "bogus", "", ""); err == nil {
		t.Error("unknown status must be rejected")
	}
	if _, err := store.Update(tracking, "triaged", strings.Repeat("x", 2001), ""); err == nil {
		t.Error("long comment must be rejected")
	}
	if _, err := store.Update(tracking, "triaged", "", "12/01/2026"); err == nil {
		t.Error("non ISO date must be rejected")
	}
	old, err := store.Update(tracking, "disclosed", "fixed", "")
	if err != nil || old != "triaged" {
		t.Fatalf("close: %q %v", old, err)
	}
	row, _ := store.Get(tracking)
	if row.ClosedAt == nil || row.Status != "disclosed" || row.ResearcherComment != "fixed" {
		t.Fatalf("closed row = %+v", row)
	}
	if _, err := store.Update(tracking, "wont_fix", "", ""); err != nil {
		t.Errorf("wont_fix is allowed from any state: %v", err)
	}
	if rows, err := store.List("wont_fix"); err != nil || len(rows) != 1 {
		t.Errorf("List(wont_fix) = %d %v", len(rows), err)
	}
	if rows, _ := store.List(""); len(rows) != 1 {
		t.Errorf("List(all) = %d", len(rows))
	}
}

func TestSecurityReportResearcherToken(t *testing.T) {
	store := newSecurityTestStore(t)
	tracking, token, _ := store.Create("0123456789abcdef", sampleReport(), ReportContact{})
	if _, err := store.ResearcherView(tracking, "wrong"); !errors.Is(err, ErrReportNotFound) {
		t.Errorf("wrong token = %v", err)
	}
	if _, err := store.ResearcherView("sec_missing", token); !errors.Is(err, ErrReportNotFound) {
		t.Errorf("unknown report = %v", err)
	}
	if row, err := store.ResearcherView(tracking, token); err != nil || row.TrackingID != tracking {
		t.Fatalf("first view = %v", err)
	}
	if _, err := store.ResearcherView(tracking, token); !errors.Is(err, ErrReportNotFound) {
		t.Error("the token works once per day")
	}
	_, _ = store.DB.Exec(`UPDATE security_reports SET token_last_used = '2000-01-01' WHERE tracking_id = ?`, tracking)
	if _, err := store.ResearcherView(tracking, token); err != nil {
		t.Errorf("the token works again on another day: %v", err)
	}
	_, _ = store.DB.Exec(`UPDATE security_reports SET token_last_used = '2000-01-01', closed_at = ? WHERE tracking_id = ?`, time.Now().Add(-31*24*time.Hour), tracking)
	if _, err := store.ResearcherView(tracking, token); !errors.Is(err, ErrReportNotFound) {
		t.Error("the link stops working 30 days after closing")
	}
}

func TestSecurityReportPurgeAndThanks(t *testing.T) {
	store := newSecurityTestStore(t)
	open, _, _ := store.Create("0123456789abcdef", sampleReport(), ReportContact{})
	closedOld, _, _ := store.Create("0123456789abcdef", sampleReport(), ReportContact{})
	closedNew, _, _ := store.Create("0123456789abcdef", sampleReport(), ReportContact{})
	_, _ = store.DB.Exec(`UPDATE security_reports SET status = 'disclosed', closed_at = ? WHERE tracking_id = ?`, time.Now().AddDate(-2, 0, 0), closedOld)
	_, _ = store.DB.Exec(`UPDATE security_reports SET status = 'disclosed', closed_at = ? WHERE tracking_id = ?`, time.Now(), closedNew)
	n, err := store.Purge(0)
	if err != nil || n != 1 {
		t.Fatalf("Purge = %d %v", n, err)
	}
	if _, err := store.Get(open); err != nil {
		t.Error("open reports are never purged")
	}
	if _, err := store.Get(closedOld); !errors.Is(err, ErrReportNotFound) {
		t.Error("an old closed report must be purged")
	}

	if err := store.AddThanks(closedNew); err != nil {
		t.Fatalf("AddThanks: %v", err)
	}
	entries, err := store.Thanks(true)
	if err != nil || len(entries) != 1 || entries[0].Credit != "ada77" {
		t.Fatalf("Thanks = %+v %v", entries, err)
	}
	if err := store.UpdateThanks(entries[0].ID, "Ada L.", false, 2); err != nil {
		t.Fatalf("UpdateThanks: %v", err)
	}
	if visible, _ := store.Thanks(true); len(visible) != 0 {
		t.Error("a hidden entry must not be public")
	}
	if all, _ := store.Thanks(false); len(all) != 1 || all[0].Credit != "Ada L." {
		t.Errorf("admin list = %+v", all)
	}
	if err := store.UpdateThanks(entries[0].ID, " ", true, 0); err == nil {
		t.Error("blank credit must be rejected")
	}

	// preferences: anonymous is numbered, "no" adds nothing
	anon := sampleReport()
	anon.CreditPreference = "anonymous"
	t1, _, _ := store.Create("0123456789abcdef", anon, ReportContact{})
	if err := store.AddThanks(t1); err != nil {
		t.Fatal(err)
	}
	none := sampleReport()
	none.CreditPreference = "no"
	t2, _, _ := store.Create("0123456789abcdef", none, ReportContact{})
	if err := store.AddThanks(t2); err != nil {
		t.Fatal(err)
	}
	all, _ := store.Thanks(false)
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %+v", all)
	}
}

func TestBlockedAddr(t *testing.T) {
	for addr, blocked := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.0.5": true, "169.254.169.254": true, "::1": true,
		"fe80::1": true, "0.0.0.0": true, "::ffff:10.0.0.1": true, "93.184.216.34": false, "2606:2800:220:1::1": false,
	} {
		if got := blockedAddr(netip.MustParseAddr(addr)); got != blocked {
			t.Errorf("blockedAddr(%s) = %v, want %v", addr, got, blocked)
		}
	}
}

func TestFetchResearcherKeyRefusesUnsafeTargets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if key, err := FetchResearcherKey(ctx, "  "); key != "" || err != nil {
		t.Errorf("empty input = %q %v", key, err)
	}
	for _, in := range []string{"https://127.0.0.1/key.asc", "https://localhost/key.asc", "https://[::1]/k", "https://" + strings.Repeat("a", 2100)} {
		if _, err := FetchResearcherKey(ctx, in); err == nil {
			t.Errorf("%q must be refused", in)
		}
	}
	if _, err := FetchResearcherKey(ctx, "http://example.com/key.asc"); err == nil {
		t.Error("plain http is not an accepted URL form and not a key block")
	}
}
