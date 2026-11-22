package format

import (
	"testing"
	"time"
)

func TestTimestampUsesConfiguredTimezone(t *testing.T) {
	t.Setenv("TZ", "America/Los_Angeles")
	value := time.Date(2026, time.January, 5, 22, 3, 7, 0, time.UTC)
	if got, want := Timestamp(value), "January 05, 2026 at 14:03:07 PST"; got != want {
		t.Errorf("Timestamp() = %q, want %q", got, want)
	}
}

func TestTimestampDefaultsToNewYork(t *testing.T) {
	t.Setenv("TZ", "")
	value := time.Date(2026, time.January, 5, 22, 3, 7, 0, time.UTC)
	if got, want := Timestamp(value), "January 05, 2026 at 17:03:07 EST"; got != want {
		t.Errorf("Timestamp() = %q, want %q", got, want)
	}
}

func TestTimestampFallsBackForInvalidTimezone(t *testing.T) {
	t.Setenv("TZ", "Invalid/Timezone")
	value := time.Date(2026, time.January, 5, 22, 3, 7, 0, time.UTC)
	if got, want := Timestamp(value), "January 05, 2026 at 17:03:07 EST"; got != want {
		t.Errorf("Timestamp() = %q, want %q", got, want)
	}
}
