package scheduler

import (
	"testing"
	"time"
)

// CalculateNextRunTime is the only directly-testable pure logic in
// datasource_refresh.go. RefreshAllDataSources() (real network/file I/O via
// LocationEnhancer/ZipcodeService/AirportService/GeoIPService, no interface
// seam) and ScheduleDataSourceRefresh() (unexported goroutine driven by a real
// time.Sleep + time.Ticker, no injectable clock) are not exercised here — see
// the coverage-gap notes in the final report.
func TestCalculateNextRunTime(t *testing.T) {
	t.Run("time already passed today rolls over to tomorrow", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour)
		target := past.Format("15:04")

		got := CalculateNextRunTime(target)

		// Should be roughly 23 hours away (24h - 1h already elapsed), never negative.
		if got <= 0 {
			t.Fatalf("CalculateNextRunTime(%q) = %v, want a positive duration (rolled to tomorrow)", target, got)
		}
		if got > 24*time.Hour {
			t.Errorf("CalculateNextRunTime(%q) = %v, want <= 24h", target, got)
		}
	})

	t.Run("time still upcoming today returns a duration less than 24h", func(t *testing.T) {
		future := time.Now().Add(2 * time.Hour)
		target := future.Format("15:04")

		got := CalculateNextRunTime(target)

		if got <= 0 {
			t.Fatalf("CalculateNextRunTime(%q) = %v, want positive", target, got)
		}
		if got > 2*time.Hour+time.Minute {
			t.Errorf("CalculateNextRunTime(%q) = %v, want approximately 2h or less", target, got)
		}
	})

	t.Run("midnight boundary 00:00", func(t *testing.T) {
		got := CalculateNextRunTime("00:00")
		if got <= 0 || got > 24*time.Hour {
			t.Errorf("CalculateNextRunTime(\"00:00\") = %v, want in (0, 24h]", got)
		}
	})

	t.Run("malformed input does not panic and still returns a duration", func(t *testing.T) {
		// fmt.Sscanf silently leaves hour/minute at their zero values on parse
		// failure, so this behaves the same as target time "00:00" rather than
		// erroring - documenting the current (silent-failure) behavior.
		got := CalculateNextRunTime("not-a-time")
		if got < 0 || got > 24*time.Hour {
			t.Errorf("CalculateNextRunTime(\"not-a-time\") = %v, want in [0, 24h]", got)
		}
	})

	t.Run("empty string input does not panic", func(t *testing.T) {
		got := CalculateNextRunTime("")
		if got < 0 || got > 24*time.Hour {
			t.Errorf("CalculateNextRunTime(\"\") = %v, want in [0, 24h]", got)
		}
	})
}
