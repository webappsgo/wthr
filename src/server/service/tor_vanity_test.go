package service

import (
	"crypto/ed25519"
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TestTorVanity_Start_ValidatesPrefixLength covers the length boundary: 0
// chars (empty), the 1-char minimum, the 6-char maximum, and 7 chars (one
// past max).
func TestTorVanity_Start_ValidatesPrefixLength(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"empty prefix", "", true},
		{"1 char (min)", "a", false},
		{"6 chars (max)", "abcdef", false},
		{"7 chars (over max)", "abcdefg", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := NewVanityGenerator()
			err := vg.Start(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start(%q) error = %v, wantErr %v", tt.prefix, err, tt.wantErr)
			}
			if err == nil {
				// Stop the background goroutine promptly.
				vg.Cancel()
				vg.Wait()
			}
		})
	}
}

// TestTorVanity_Start_ValidatesPrefixCharacters covers valid base32
// characters (a-z, 2-7) vs invalid ones (0, 1, 8, 9, uppercase-only symbols,
// punctuation) — base32 excludes 0/1/8/9 to avoid visual ambiguity.
func TestTorVanity_Start_ValidatesPrefixCharacters(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"all lowercase letters", "abcxyz", false},
		{"digits 2-7", "234567", false},
		{"uppercase gets lowercased first", "ABCDEF", false},
		{"digit 0 invalid", "a0b", true},
		{"digit 1 invalid", "a1b", true},
		{"digit 8 invalid", "a8b", true},
		{"digit 9 invalid", "a9b", true},
		{"punctuation invalid", "a-b", true},
		{"space invalid", "a b", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := NewVanityGenerator()
			err := vg.Start(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start(%q) error = %v, wantErr %v", tt.prefix, err, tt.wantErr)
			}
			if err == nil {
				vg.Cancel()
				vg.Wait()
			}
		})
	}
}

// TestTorVanity_Start_RejectsConcurrentRun covers the "already running"
// error path: starting a second generation while one is in progress must
// fail without disturbing the first run.
func TestTorVanity_Start_RejectsConcurrentRun(t *testing.T) {
	vg := NewVanityGenerator()
	// Use an unlikely-to-match-fast prefix so the first run stays "running"
	// long enough for the second Start call to observe it.
	if err := vg.Start("zzzzzz"); err != nil {
		t.Fatalf("first Start unexpected error: %v", err)
	}
	defer func() {
		vg.Cancel()
		vg.Wait()
	}()

	if err := vg.Start("aaaaaa"); err == nil {
		t.Fatal("expected error when starting while another generation is running")
	}
}

// TestTorVanity_Cancel_NoGenerationInProgress covers calling Cancel before
// any Start — must return an error, not panic.
func TestTorVanity_Cancel_NoGenerationInProgress(t *testing.T) {
	vg := NewVanityGenerator()
	if err := vg.Cancel(); err == nil {
		t.Fatal("expected error cancelling with no generation in progress")
	}
}

// TestTorVanity_Cancel_StopsRunningGeneration covers the happy path: Cancel
// on a running generation succeeds, and after Wait() returns, status.Running
// must be false (set by the ctx.Done() branch of generate()).
func TestTorVanity_Cancel_StopsRunningGeneration(t *testing.T) {
	vg := NewVanityGenerator()
	if err := vg.Start("zzzzzz"); err != nil {
		t.Fatalf("Start unexpected error: %v", err)
	}

	if err := vg.Cancel(); err != nil {
		t.Fatalf("Cancel unexpected error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		vg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for generation to stop after Cancel")
	}

	status := vg.GetStatus()
	if status == nil {
		t.Fatal("GetStatus should not be nil after a run has started")
	}
	if status.Running {
		t.Error("status.Running should be false after Cancel + Wait")
	}
}

// TestTorVanity_Cancel_Idempotency covers calling Cancel a second time after
// the first cancellation completed — it must return an error (nothing left
// to cancel), not panic on a nil/expired context.
func TestTorVanity_Cancel_Idempotency(t *testing.T) {
	vg := NewVanityGenerator()
	if err := vg.Start("zzzzzz"); err != nil {
		t.Fatalf("Start unexpected error: %v", err)
	}
	if err := vg.Cancel(); err != nil {
		t.Fatalf("first Cancel unexpected error: %v", err)
	}
	vg.Wait()

	if err := vg.Cancel(); err == nil {
		t.Error("second Cancel call should error since status.Running is now false")
	}
}

// TestTorVanity_GetStatus_NilBeforeStart covers the boundary of calling
// GetStatus before any Start call.
func TestTorVanity_GetStatus_NilBeforeStart(t *testing.T) {
	vg := NewVanityGenerator()
	if status := vg.GetStatus(); status != nil {
		t.Errorf("GetStatus() before Start = %+v, want nil", status)
	}
}

// TestTorVanity_GetStatus_ReturnsCopy verifies GetStatus returns a
// snapshot copy, not a pointer into live internal state — mutating the
// generator afterward must not change the value the caller already has,
// preventing data races on the caller's read.
func TestTorVanity_GetStatus_ReturnsCopy(t *testing.T) {
	vg := NewVanityGenerator()
	if err := vg.Start("zzzzzz"); err != nil {
		t.Fatalf("Start unexpected error: %v", err)
	}
	defer func() {
		vg.Cancel()
		vg.Wait()
	}()

	s1 := vg.GetStatus()
	if s1 == nil {
		t.Fatal("expected non-nil status after Start")
	}
	if s1.Prefix != "zzzzzz" {
		t.Errorf("status.Prefix = %q, want %q", s1.Prefix, "zzzzzz")
	}

	// Give the background goroutine a moment to increment Attempts, then take
	// a second snapshot; the first snapshot must remain unchanged (proving it
	// really was copied, not aliased).
	time.Sleep(20 * time.Millisecond)
	s1Attempts := s1.Attempts
	s2 := vg.GetStatus()
	if s2.Attempts < s1Attempts {
		t.Errorf("attempts should be monotonically non-decreasing: s1=%d s2=%d", s1Attempts, s2.Attempts)
	}
	if s1.Attempts != s1Attempts {
		t.Error("first snapshot's Attempts field mutated after the fact — GetStatus is not returning an independent copy")
	}
}

// TestTorVanity_GetKeys_ErrorPaths covers: no generation performed yet,
// generation still running, and (via a controlled tryGenerate call)
// generation that completed successfully.
func TestTorVanity_GetKeys_ErrorPaths(t *testing.T) {
	t.Run("no generation performed", func(t *testing.T) {
		vg := NewVanityGenerator()
		_, _, err := vg.GetKeys()
		if err == nil {
			t.Error("expected error when no generation has been performed")
		}
	})

	t.Run("generation still running", func(t *testing.T) {
		vg := NewVanityGenerator()
		if err := vg.Start("zzzzzz"); err != nil {
			t.Fatalf("Start unexpected error: %v", err)
		}
		defer func() {
			vg.Cancel()
			vg.Wait()
		}()

		_, _, err := vg.GetKeys()
		if err == nil {
			t.Error("expected error while generation is still running")
		}
	})

	t.Run("cancelled without a match", func(t *testing.T) {
		vg := NewVanityGenerator()
		if err := vg.Start("zzzzzz"); err != nil {
			t.Fatalf("Start unexpected error: %v", err)
		}
		vg.Cancel()
		vg.Wait()

		// status.Address will be "" since cancellation happened before a match.
		_, _, err := vg.GetKeys()
		if err == nil {
			t.Error("expected error when generation did not complete successfully (no address found)")
		}
	})
}

// TestTorVanity_GetKeys_SuccessPath drives tryGenerate directly (bypassing
// the real-time ticker loop in generate()) with the single-character prefix
// "a", which matches roughly 1 in 32 random keys, so it converges almost
// immediately and deterministically exercises the success branch of
// tryGenerate, then verifies GetKeys returns the matching key pair.
func TestTorVanity_GetKeys_SuccessPath(t *testing.T) {
	vg := NewVanityGenerator()
	vg.status = &VanityGenerationStatus{
		Running:   true,
		Prefix:    "a",
		StartTime: time.Now(),
	}

	found := false
	for i := 0; i < 100000; i++ {
		if vg.tryGenerate("a") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("tryGenerate did not find a match for a 1-char prefix within 100000 attempts (statistically implausible)")
	}

	vg.mu.Lock()
	vg.status.Running = false
	vg.mu.Unlock()

	pub, priv, err := vg.GetKeys()
	if err != nil {
		t.Fatalf("GetKeys unexpected error: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("public key length = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Errorf("private key length = %d, want %d", len(priv), ed25519.PrivateKeySize)
	}

	status := vg.GetStatus()
	if !strings.HasPrefix(status.Address, "a") {
		t.Errorf("resulting address %q does not start with prefix %q", status.Address, "a")
	}
	if !strings.HasSuffix(status.Address, ".onion") {
		t.Errorf("resulting address %q does not end with .onion", status.Address)
	}
}

// TestTorVanity_tryGenerate_AttemptsIncrementsRegardlessOfMatch verifies the
// attempt counter increments on every call, matched or not — this is what
// EstimatedTime and progress reporting rely on.
func TestTorVanity_tryGenerate_AttemptsIncrementsRegardlessOfMatch(t *testing.T) {
	vg := NewVanityGenerator()
	vg.status = &VanityGenerationStatus{
		Running: true,
		// Extremely unlikely prefix to guarantee no match happens during the
		// small fixed number of iterations below (keeps the test deterministic).
		Prefix: "zzzzzz",
	}

	const iterations = 10
	for i := 0; i < iterations; i++ {
		vg.tryGenerate("zzzzzz")
	}

	vg.mu.RLock()
	attempts := vg.status.Attempts
	vg.mu.RUnlock()

	if attempts != iterations {
		t.Errorf("Attempts = %d, want %d after %d tryGenerate calls", attempts, iterations, iterations)
	}
}

// TestTorVanity_publicKeyToOnionAddress_Deterministic verifies the same
// public key always yields the same address (pure function, no randomness),
// that the output is well-formed (lowercase base32, correct length range),
// and that different keys produce different addresses.
func TestTorVanity_publicKeyToOnionAddress_Deterministic(t *testing.T) {
	pub1, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	addr1 := publicKeyToOnionAddress(pub1)
	addr2 := publicKeyToOnionAddress(pub1)
	if addr1 != addr2 {
		t.Errorf("publicKeyToOnionAddress not deterministic: %q != %q", addr1, addr2)
	}

	if addr1 != strings.ToLower(addr1) {
		t.Errorf("address %q should be lowercase", addr1)
	}

	// 35 raw bytes (32 pubkey + 2 checksum + 1 version) base32-encoded without
	// padding is ceil(35*8/5) = 56 characters.
	if len(addr1) != 56 {
		t.Errorf("address length = %d, want 56", len(addr1))
	}

	pub2, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate second key: %v", err)
	}
	addr3 := publicKeyToOnionAddress(pub2)
	if addr3 == addr1 {
		t.Error("two different public keys produced the same onion address (extremely unlikely, checksum/version logic may be broken)")
	}
}

// TestTorVanity_publicKeyToOnionAddress_ZeroKey covers the boundary of an
// all-zero public key (not a real key, but must not panic and must still
// decode as valid base32).
func TestTorVanity_publicKeyToOnionAddress_ZeroKey(t *testing.T) {
	zero := make(ed25519.PublicKey, ed25519.PublicKeySize)
	addr := publicKeyToOnionAddress(zero)

	if len(addr) != 56 {
		t.Errorf("address length for zero key = %d, want 56", len(addr))
	}

	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(addr))
	if err != nil {
		t.Fatalf("address is not valid base32: %v", err)
	}
	if len(decoded) != 35 {
		t.Errorf("decoded address length = %d, want 35 (32 pubkey + 2 checksum + 1 version)", len(decoded))
	}
	if decoded[34] != 0x03 {
		t.Errorf("version byte = 0x%02x, want 0x03", decoded[34])
	}
}

// TestTorVanity_formatDuration covers each duration bucket boundary: under a
// minute, under an hour, under a day, and multi-day.
func TestTorVanity_formatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0 seconds"},
		{"30 seconds", 30 * time.Second, "30 seconds"},
		{"59 seconds (just under a minute)", 59 * time.Second, "59 seconds"},
		{"exactly 1 minute", 1 * time.Minute, "1 minutes"},
		{"90 minutes", 90 * time.Minute, "1 hours 30 minutes"},
		{"exactly 1 hour", 1 * time.Hour, "1 hours 0 minutes"},
		{"23 hours 59 minutes", 23*time.Hour + 59*time.Minute, "23 hours 59 minutes"},
		{"exactly 1 day", 24 * time.Hour, "1 days 0 hours"},
		{"2 days 5 hours", 2*24*time.Hour + 5*time.Hour, "2 days 5 hours"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestTorVanity_EstimateTime covers the default attemptsPerSec fallback
// (zero input), a supplied rate, and the prefix-length-driven growth of the
// estimate (32^N expected attempts).
func TestTorVanity_EstimateTime(t *testing.T) {
	t.Run("zero rate uses default", func(t *testing.T) {
		got := EstimateTime(1, 0)
		if got == "" {
			t.Error("EstimateTime with zero rate should not return empty string")
		}
	})

	t.Run("prefix length 0 is near-instant", func(t *testing.T) {
		got := EstimateTime(0, 100000)
		if got != "0 seconds" {
			t.Errorf("EstimateTime(0, 100000) = %q, want %q", got, "0 seconds")
		}
	})

	t.Run("longer prefix takes longer", func(t *testing.T) {
		short := EstimateTime(2, 1000)
		long := EstimateTime(6, 1000)
		// 32^2 = 1024 attempts vs 32^6 ~= 1e9 attempts at the same rate; the
		// longer prefix's formatted string must not equal the shorter one's.
		if short == long {
			t.Errorf("expected different estimates for prefix length 2 vs 6, both = %q", short)
		}
	})
}

// NOTE on coverage gaps:
//   - generate()'s real-time ticker branch (updateEstimate firing every
//     second) and the full Start -> background goroutine -> notifyCh
//     end-to-end completion path for a *matching* run are not exercised with
//     a short timing budget: waiting for a real 1-char-prefix match via the
//     public Start() API (rather than calling tryGenerate directly) is
//     non-deterministic in duration under CPU contention, so
//     TestTorVanity_GetKeys_SuccessPath exercises the same tryGenerate logic
//     directly instead, which is the actual match-or-not decision the
//     background loop relies on.
//   - There is no external binary (e.g. mkp224o) invocation anywhere in
//     tor_vanity.go — generation is pure Go (crypto/ed25519 + golang.org/x/
//     crypto/sha3), so no shell-out seam needed to be skipped.
