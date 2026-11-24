package handler

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestContactAntispamFormTokenIsSingleUseAndIPIndependent(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	gate := NewContactAntispam([]byte("test installation secret"), ContactAntispamConfig{MinFill: time.Second, MaxAge: time.Hour})
	token, err := gate.NewContactFormToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, reason := gate.ConsumeContactFormToken(token, now.Add(2*time.Second)); !ok || reason != "" {
		t.Fatalf("first consume = %v, %q", ok, reason)
	}
	if ok, reason := gate.ConsumeContactFormToken(token, now.Add(3*time.Second)); ok || reason != "replay" {
		t.Fatalf("replay = %v, %q", ok, reason)
	}
}

func TestContactAntispamRejectsEarlyExpiredAndForgedTokens(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	gate := NewContactAntispam([]byte("test installation secret"), ContactAntispamConfig{MinFill: 3 * time.Second, MaxAge: time.Hour})
	token, err := gate.NewContactFormToken(now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		when        time.Time
	}{
		{"early", token, now.Add(time.Second)},
		{"expired", token, now.Add(2 * time.Hour)},
		{"forged", token[:len(token)-1] + "0", now.Add(10 * time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ok, _ := gate.ConsumeContactFormToken(tc.token, tc.when); ok {
				t.Fatal("accepted invalid token")
			}
		})
	}
}

func TestContactAntispamReviewTokenCoversNormalizedSnapshotAndDropFlag(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	gate := NewContactAntispam([]byte("test installation secret"), ContactAntispamConfig{ReviewMin: time.Second, ReviewMax: time.Minute})
	values := ContactAntispamValues{"email": "Sender@Example.com", "subject": "  Hello  ", "message": "one   two"}
	token, err := gate.NewContactReviewToken(values, now, true)
	if err != nil {
		t.Fatal(err)
	}
	changedSpacing := ContactAntispamValues{"email": "sender@example.com", "subject": "Hello", "message": "one two"}
	if valid, drop := gate.VerifyContactReviewToken(token, changedSpacing, now.Add(2*time.Second)); !valid || !drop {
		t.Fatalf("normalized review = %v, drop %v", valid, drop)
	}
	changedSpacing["message"] = "different"
	if valid, _ := gate.VerifyContactReviewToken(token, changedSpacing, now.Add(2*time.Second)); valid {
		t.Fatal("accepted changed review values")
	}
}

func TestValidateContactShapeRejectsUnknownControlAndOrigin(t *testing.T) {
	values := ContactAntispamValues{"name": "a", "email": "a@example.com", "subject": "s", "message": "m"}
	r := httptest.NewRequest("POST", "https://weather.example/server/contact", nil)
	r.Header.Set("Origin", "https://weather.example")
	allowed := []string{"name", "email", "subject", "message"}
	required := append([]string(nil), allowed...)
	if reason := ValidateContactShape(r, values, allowed, required, true, "https://weather.example"); reason != "" {
		t.Fatalf("valid shape = %q", reason)
	}
	values["extra"] = "no"
	if reason := ValidateContactShape(r, values, allowed, required, true, "https://weather.example"); reason != "shape" {
		t.Fatalf("unknown field = %q", reason)
	}
	delete(values, "extra")
	values["subject"] = "bad\r\nsubject"
	if reason := ValidateContactShape(r, values, allowed, required, true, "https://weather.example"); reason != "shape" {
		t.Fatalf("control character = %q", reason)
	}
	values["subject"] = "ok"
	r.Header.Del("Origin")
	if reason := ValidateContactShape(r, values, allowed, required, true, "https://weather.example"); reason != "origin" {
		t.Fatalf("missing origin = %q", reason)
	}
}

func TestContactAntispamContentAndDuplicateRules(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	gate := NewContactAntispam([]byte("test secret"), ContactAntispamConfig{RejectMarkup: true, MaxLinks: 2, BlockedWords: []string{"scam"}, BlockedEmailDomains: []string{"mail.invalid"}, DuplicateFor: time.Hour})
	base := ContactAntispamValues{"name": "Alice", "email": "alice@example.com", "subject": "Report", "message": "A report at https://weather.example/report"}
	if reason := gate.CheckContactContent(base, []string{"weather.example"}, false); reason != "" {
		t.Fatalf("accepted content = %q", reason)
	}
	bad := ContactAntispamValues{"name": "Alice", "email": "alice@example.com", "subject": "Report", "message": "<a href=\"https://weather.example\">spam</a>"}
	if reason := gate.CheckContactContent(bad, []string{"weather.example"}, false); reason != "markup" {
		t.Fatalf("markup = %q", reason)
	}
	bad["message"] = "scam"
	if reason := gate.CheckContactContent(bad, nil, false); reason != "content" {
		t.Fatalf("blocked word = %q", reason)
	}
	bad["message"] = "ordinary content"
	bad["email"] = "alice@mail.invalid"
	if reason := gate.CheckContactContent(bad, nil, false); reason != "content" {
		t.Fatalf("blocked domain = %q", reason)
	}
	if gate.IsContactDuplicate(base, now) {
		t.Fatal("first message marked duplicate")
	}
	duplicate := ContactAntispamValues{"email": " ALICE@example.com ", "subject": " report ", "message": "A   report at https://weather.example/report"}
	if !gate.IsContactDuplicate(duplicate, now.Add(time.Second)) {
		t.Fatal("normalized repeat not marked duplicate")
	}
}

func TestContactAntispamRateLimitsAndStrikes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	gate := NewContactAntispam([]byte("test secret"), ContactAntispamConfig{Window: time.Hour, PerIP: 1, PerEmail: 5, Global: 10, Strikes: 3, BanDuration: time.Hour})
	if reason, _ := gate.CheckRateLimit("192.0.2.1", "a@example.com", now, false, false); reason != "" {
		t.Fatalf("preflight = %q", reason)
	}
	if reason, _ := gate.CheckRateLimit("192.0.2.1", "a@example.com", now, true, false); reason != "" {
		t.Fatalf("first count = %q", reason)
	}
	if reason, _ := gate.CheckRateLimit("192.0.2.1", "a@example.com", now, false, false); reason != "rate_ip" {
		t.Fatalf("over limit = %q", reason)
	}
	for i := 0; i < 2; i++ {
		if d := gate.RecordContactStrike("192.0.2.1", now.Add(time.Duration(i)*time.Second)); d != 0 {
			t.Fatalf("early ban = %v", d)
		}
	}
	if d := gate.RecordContactStrike("192.0.2.1", now.Add(3*time.Second)); d != time.Hour {
		t.Fatalf("ban duration = %v", d)
	}
	if reason, _ := gate.CheckRateLimit("192.0.2.1", "b@example.com", now.Add(4*time.Second), false, false); reason != "banned" {
		t.Fatalf("banned request = %q", reason)
	}
	if reason, _ := gate.CheckRateLimit("192.0.2.1", "b@example.com", now.Add(4*time.Second), true, true); reason != "" {
		t.Fatalf("overlay IP exemption = %q", reason)
	}
}

func TestContactAntispamEvictsOldestBoundedState(t *testing.T) {
	old := time.Unix(1_800_000_000, 0)
	newer := old.Add(time.Second)
	gate := NewContactAntispam([]byte("test secret"), ContactAntispamConfig{})
	gate.replays = map[string]time.Time{"old": old, "new": newer}
	gate.evictOldestReplayLocked()
	if _, exists := gate.replays["old"]; exists {
		t.Fatal("oldest replay was retained")
	}
	gate.duplicates = map[string]time.Time{"old": old, "new": newer}
	gate.evictOldestDuplicateLocked()
	if _, exists := gate.duplicates["old"]; exists {
		t.Fatal("oldest duplicate was retained")
	}
	expiry := map[string]time.Time{"old": old, "new": newer}
	evictOldestExpiry(expiry)
	if _, exists := expiry["old"]; exists {
		t.Fatal("oldest expiry was retained")
	}
	times := map[string][]time.Time{"old": {old}, "new": {newer}}
	evictOldestTimes(times)
	if _, exists := times["old"]; exists {
		t.Fatal("oldest time bucket was retained")
	}
}
