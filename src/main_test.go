package main

import "testing"

// TestGetDefaultListenAddress covers the IPv6-detection / fallback logic in
// getDefaultListenAddress. It only ever binds to an ephemeral local port
// ("[::]:0") and closes it immediately - no external network access, no
// fixed port binding - so it is safe to exercise in a unit test.
func TestGetDefaultListenAddress(t *testing.T) {
	t.Run("returns a valid listen address", func(t *testing.T) {
		got := getDefaultListenAddress()

		switch got {
		case "::", "0.0.0.0":
			// both are valid depending on whether the test environment
			// supports IPv6 dual-stack listening
		default:
			t.Fatalf("getDefaultListenAddress() = %q, want %q or %q", got, "::", "0.0.0.0")
		}
	})

	t.Run("does not leak the probe listener", func(t *testing.T) {
		// Calling it twice in a row must not fail or hang - if the first
		// call leaked its ephemeral listener, a second dual-stack bind on
		// "[::]:0" would still succeed (different ephemeral port), so this
		// mainly guards against a panic/deadlock regression rather than a
		// specific leaked-socket assertion.
		first := getDefaultListenAddress()
		second := getDefaultListenAddress()

		if first != second {
			t.Fatalf("getDefaultListenAddress() not stable across calls: first=%q second=%q", first, second)
		}
	})
}
