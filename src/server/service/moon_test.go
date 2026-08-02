package service

import (
	"fmt"
	"math"
	"testing"
	"time"
)

const moonFloatTolerance = 1e-6

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// TestMoon_NewMoonService verifies the constructor returns a usable,
// non-nil service.
func TestMoon_NewMoonService(t *testing.T) {
	ms := NewMoonService()
	if ms == nil {
		t.Fatal("NewMoonService() = nil")
	}
}

// TestMoon_CalculateMoonAge_KnownNewMoon anchors the age calculation against
// the reference new moon (2000-01-06 18:14 UTC) baked into the source, and
// checks a range of offsets around it including boundary/wraparound cases.
func TestMoon_CalculateMoonAge_KnownNewMoon(t *testing.T) {
	ms := NewMoonService()
	knownNewMoon := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	var synodicDays float64 = 29.53058867
	// Build offsets directly in nanoseconds (days * hours-per-day *
	// ns-per-hour) rather than time.Duration(days*24)*time.Hour, which
	// truncates to whole hours before scaling and would throw off the
	// sub-hour precision this test checks for.
	fullSynodicOffset := time.Duration(synodicDays * 24 * float64(time.Hour))
	halfSynodicOffset := time.Duration((synodicDays / 2) * 24 * float64(time.Hour))

	tests := []struct {
		name    string
		t       time.Time
		wantAge float64
	}{
		{"exactly at reference new moon", knownNewMoon, 0},
		{"one day after reference", knownNewMoon.Add(24 * time.Hour), 1},
		{"one synodic month after reference (wraps to ~0)", knownNewMoon.Add(fullSynodicOffset), 0},
		{"one day before reference (wraps to end of cycle)", knownNewMoon.Add(-24 * time.Hour), synodicDays - 1},
		{"half synodic month after (full moon point)", knownNewMoon.Add(halfSynodicOffset), synodicDays / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.calculateMoonAge(tt.t)
			if got < 0 || got >= 29.53058867 {
				t.Errorf("age out of range [0, synodicMonth): got %v", got)
			}
			if !almostEqual(got, tt.wantAge, 1e-4) {
				t.Errorf("calculateMoonAge() = %v, want ~%v", got, tt.wantAge)
			}
		})
	}
}

// TestMoon_CalculateMoonAge_Boundaries exercises far past/future dates,
// Unix epoch, and a leap day to make sure the modulo arithmetic never
// panics or produces an out-of-range age.
func TestMoon_CalculateMoonAge_Boundaries(t *testing.T) {
	ms := NewMoonService()

	tests := []struct {
		name string
		t    time.Time
	}{
		{"unix epoch", time.Unix(0, 0).UTC()},
		{"far past (year 1)", time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"far future (year 9999)", time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)},
		{"leap day", time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"leap day far future", time.Date(2400, 2, 29, 0, 0, 0, 0, time.UTC)},
		{"zero time.Time", time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := ms.calculateMoonAge(tt.t)
			if math.IsNaN(age) || math.IsInf(age, 0) {
				t.Fatalf("calculateMoonAge(%v) = %v, want finite number", tt.t, age)
			}
			if age < 0 || age >= 29.53058867 {
				t.Errorf("calculateMoonAge(%v) = %v, want in [0, 29.53058867)", tt.t, age)
			}
		})
	}
}

// TestMoon_CalculateMoonAge_Idempotent ensures calling the pure function
// repeatedly with the same input always yields the same output.
func TestMoon_CalculateMoonAge_Idempotent(t *testing.T) {
	ms := NewMoonService()
	when := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	first := ms.calculateMoonAge(when)
	for i := 0; i < 10; i++ {
		got := ms.calculateMoonAge(when)
		if got != first {
			t.Fatalf("iteration %d: calculateMoonAge changed from %v to %v", i, first, got)
		}
	}
}

// TestMoon_CalculateIllumination checks the illumination formula at known
// reference ages: new moon (0%), first quarter (~50%), full moon (~100%),
// last quarter (~50%), and confirms the value never leaves [0, 100].
func TestMoon_CalculateIllumination(t *testing.T) {
	ms := NewMoonService()
	synodic := 29.53058867

	tests := []struct {
		name  string
		age   float64
		want  float64
		delta float64
	}{
		{"new moon", 0, 0, 1e-6},
		{"first quarter", synodic / 4, 50, 0.5},
		{"full moon", synodic / 2, 100, 1e-6},
		{"last quarter", synodic * 3 / 4, 50, 0.5},
		{"end of cycle approaches new moon", synodic, 0, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.calculateIllumination(tt.age)
			if got < 0 || got > 100 {
				t.Fatalf("calculateIllumination(%v) = %v, out of [0,100]", tt.age, got)
			}
			if !almostEqual(got, tt.want, tt.delta) {
				t.Errorf("calculateIllumination(%v) = %v, want ~%v", tt.age, got, tt.want)
			}
		})
	}
}

// TestMoon_CalculateIllumination_NegativeAge documents behavior for an
// out-of-domain negative age (should not panic; cosine is well-defined for
// any real input).
func TestMoon_CalculateIllumination_NegativeAge(t *testing.T) {
	ms := NewMoonService()
	got := ms.calculateIllumination(-5)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("calculateIllumination(-5) = %v, want finite", got)
	}
}

// TestMoon_GetPhaseName covers every phase boundary listed in the source,
// using values just inside and (where meaningful) just outside each bucket.
func TestMoon_GetPhaseName(t *testing.T) {
	tests := []struct {
		name string
		age  float64
		want string
	}{
		{"age 0 -> New Moon", 0, "New Moon"},
		{"just under first boundary -> New Moon", 1.84565, "New Moon"},
		{"at first boundary -> Waxing Crescent", 1.84566, "Waxing Crescent"},
		{"mid waxing crescent", 4, "Waxing Crescent"},
		{"at first quarter boundary", 7.38264, "First Quarter"},
		{"mid first quarter", 8, "First Quarter"},
		{"at waxing gibbous boundary", 9.22831, "Waxing Gibbous"},
		{"mid waxing gibbous", 12, "Waxing Gibbous"},
		{"at full moon boundary", 14.76529, "Full Moon"},
		{"mid full moon", 15.5, "Full Moon"},
		{"at waning gibbous boundary", 16.61096, "Waning Gibbous"},
		{"mid waning gibbous", 19, "Waning Gibbous"},
		{"at last quarter boundary", 22.14794, "Last Quarter"},
		{"mid last quarter", 23.5, "Last Quarter"},
		{"at waning crescent boundary", 24.99361, "Waning Crescent"},
		{"mid waning crescent", 27, "Waning Crescent"},
		{"at end of synodic month -> New Moon (fallthrough)", 29.53059, "New Moon"},
		{"beyond synodic month -> New Moon (fallthrough)", 40, "New Moon"},
	}

	ms := NewMoonService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.getPhaseName(tt.age)
			if got != tt.want {
				t.Errorf("getPhaseName(%v) = %q, want %q", tt.age, got, tt.want)
			}
		})
	}
}

// TestMoon_GetMoonIcon mirrors the phase-name boundaries but checks the
// emoji returned for each bucket, including the final else branch which
// (unlike getPhaseName) has no explicit upper-bound fallthrough.
func TestMoon_GetMoonIcon(t *testing.T) {
	tests := []struct {
		name string
		age  float64
		want string
	}{
		{"new moon", 0, "🌑"},
		{"waxing crescent", 4, "🌒"},
		{"first quarter", 8, "🌓"},
		{"waxing gibbous", 12, "🌔"},
		{"full moon", 15.5, "🌕"},
		{"waning gibbous", 19, "🌖"},
		{"last quarter", 23.5, "🌗"},
		{"waning crescent", 27, "🌘"},
		{"beyond synodic month falls into waning crescent (no upper guard)", 40, "🌘"},
	}

	ms := NewMoonService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.getMoonIcon(tt.age)
			if got != tt.want {
				t.Errorf("getMoonIcon(%v) = %q, want %q", tt.age, got, tt.want)
			}
		})
	}
}

// TestMoon_CalculateRiseSet checks the rise/set time strings are always
// well-formed HH:MM (00-23 hour, 00-59 minute) regardless of input, and
// that the function is deterministic for a fixed time.
func TestMoon_CalculateRiseSet(t *testing.T) {
	ms := NewMoonService()

	tests := []struct {
		name string
		lat  float64
		lon  float64
		t    time.Time
	}{
		{"equator/prime meridian", 0, 0, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"north pole", 90, 0, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)},
		{"south pole", -90, 0, time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)},
		{"negative longitude", 40.7128, -74.0060, time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rise, set := ms.calculateRiseSet(tt.lat, tt.lon, tt.t)
			assertHHMM(t, "rise", rise)
			assertHHMM(t, "set", set)

			rise2, set2 := ms.calculateRiseSet(tt.lat, tt.lon, tt.t)
			if rise != rise2 || set != set2 {
				t.Errorf("calculateRiseSet not idempotent: (%s,%s) vs (%s,%s)", rise, set, rise2, set2)
			}
		})
	}
}

func assertHHMM(t *testing.T, label, s string) {
	t.Helper()
	var hh, mm int
	n, err := fmt.Sscanf(s, "%d:%d", &hh, &mm)
	if err != nil || n != 2 {
		t.Fatalf("%s = %q is not HH:MM format: %v", label, s, err)
	}
	if hh < 0 || hh > 23 {
		t.Errorf("%s hour out of range: %q", label, s)
	}
	if mm < 0 || mm > 59 {
		t.Errorf("%s minute out of range: %q", label, s)
	}
}

// TestMoon_CalculateNextNewMoon verifies the returned time is strictly
// after the input, within one synodic month, and that its own computed
// age is ~0.
func TestMoon_CalculateNextNewMoon(t *testing.T) {
	ms := NewMoonService()
	times := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC), // exactly at a new moon
		time.Unix(0, 0).UTC(),
	}

	for _, when := range times {
		t.Run(when.String(), func(t *testing.T) {
			next := ms.calculateNextNewMoon(when)
			if !next.After(when) {
				t.Fatalf("calculateNextNewMoon(%v) = %v, want strictly after input", when, next)
			}
			diff := next.Sub(when).Hours() / 24.0
			if diff > 29.53058867+1e-6 {
				t.Errorf("next new moon is more than one synodic month away: %v days", diff)
			}
			age := ms.calculateMoonAge(next)
			// calculateNextNewMoon truncates the day offset to whole hours
			// internally (time.Duration(daysUntil*24) truncates the
			// fractional hour), so allow just over an hour of slack.
			const hourInDays = 1.0 / 24.0
			if age > hourInDays*1.5 && age < 29.53058867-hourInDays*1.5 {
				t.Errorf("age at computed next new moon = %v, want ~0 (within truncation slack)", age)
			}
		})
	}
}

// TestMoon_CalculateNextFullMoon verifies the returned time is strictly
// after the input and its computed age is close to half a synodic month
// (the full-moon point).
func TestMoon_CalculateNextFullMoon(t *testing.T) {
	ms := NewMoonService()
	synodic := 29.53058867
	fullMoonAge := synodic / 2.0

	times := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC),
		time.Unix(0, 0).UTC(),
	}

	for _, when := range times {
		t.Run(when.String(), func(t *testing.T) {
			next := ms.calculateNextFullMoon(when)
			if !next.After(when) {
				t.Fatalf("calculateNextFullMoon(%v) = %v, want strictly after input", when, next)
			}
			age := ms.calculateMoonAge(next)
			// calculateNextFullMoon truncates the day offset to whole
			// hours internally, so allow just over an hour of slack.
			const hourInDays = 1.0 / 24.0
			if !almostEqual(age, fullMoonAge, hourInDays*1.5) {
				t.Errorf("age at computed next full moon = %v, want ~%v", age, fullMoonAge)
			}
		})
	}
}

// TestMoon_CalculateDistance checks the distance stays within the
// documented physical bounds (356,500 - 406,700 km, with the simplified
// sinusoidal model bounded by avgDistance +/- variation).
func TestMoon_CalculateDistance(t *testing.T) {
	ms := NewMoonService()
	const avg = 384400.0
	const variation = 25000.0

	times := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC),
		time.Unix(0, 0).UTC(),
		time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	}

	for _, when := range times {
		t.Run(when.String(), func(t *testing.T) {
			d := ms.calculateDistance(when)
			if d < avg-variation-1e-6 || d > avg+variation+1e-6 {
				t.Errorf("calculateDistance(%v) = %v, want in [%v, %v]", when, d, avg-variation, avg+variation)
			}
		})
	}
}

// TestMoon_CalculateAngularSize checks the angular size formula against a
// hand-computed value at the average distance, plus boundary behavior for
// closest/farthest distances.
func TestMoon_CalculateAngularSize(t *testing.T) {
	ms := NewMoonService()

	tests := []struct {
		name     string
		distance float64
	}{
		{"average distance", 384400.0},
		{"perigee (closest)", 356500.0},
		{"apogee (farthest)", 406700.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.calculateAngularSize(tt.distance)
			want := 2.0 * math.Atan(3474.8/(2.0*tt.distance)) * (180.0 / math.Pi)
			if !almostEqual(got, want, 1e-9) {
				t.Errorf("calculateAngularSize(%v) = %v, want %v", tt.distance, got, want)
			}
			// Sanity: real moon angular size is roughly 0.49-0.56 degrees.
			if got < 0.4 || got > 0.6 {
				t.Errorf("calculateAngularSize(%v) = %v, outside plausible physical range", tt.distance, got)
			}
		})
	}
}

// TestMoon_Calculate_Integration exercises the top-level Calculate method
// end-to-end and checks every field is populated and internally
// consistent (phase name matches icon bucket, illumination in range,
// timestamps parse as RFC3339, and repeated calls are idempotent).
func TestMoon_Calculate_Integration(t *testing.T) {
	ms := NewMoonService()
	when := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	data := ms.Calculate(40.7128, -74.0060, when)
	if data == nil {
		t.Fatal("Calculate() = nil")
	}
	if data.Phase == "" {
		t.Error("Phase is empty")
	}
	if data.Icon == "" {
		t.Error("Icon is empty")
	}
	if data.Illumination < 0 || data.Illumination > 100 {
		t.Errorf("Illumination out of range: %v", data.Illumination)
	}
	if data.Age < 0 || data.Age >= 29.53058867 {
		t.Errorf("Age out of range: %v", data.Age)
	}
	if _, err := time.Parse(time.RFC3339, data.NextNewMoon); err != nil {
		t.Errorf("NextNewMoon not RFC3339: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, data.NextFullMoon); err != nil {
		t.Errorf("NextFullMoon not RFC3339: %v", err)
	}
	if data.Distance <= 0 {
		t.Errorf("Distance must be positive: %v", data.Distance)
	}
	if data.AngularSize <= 0 {
		t.Errorf("AngularSize must be positive: %v", data.AngularSize)
	}

	// Idempotency: same inputs, same outputs.
	data2 := ms.Calculate(40.7128, -74.0060, when)
	if *data != *data2 {
		t.Errorf("Calculate not idempotent:\n%+v\nvs\n%+v", data, data2)
	}
}

// TestMoon_Calculate_ZeroCoordinates ensures lat/lon of exactly 0,0 (a
// common "unset" zero-value bug source) does not panic or produce NaN.
func TestMoon_Calculate_ZeroCoordinates(t *testing.T) {
	ms := NewMoonService()
	data := ms.Calculate(0, 0, time.Now())
	if data == nil {
		t.Fatal("Calculate(0, 0, now) = nil")
	}
	if math.IsNaN(data.Illumination) || math.IsNaN(data.Distance) || math.IsNaN(data.AngularSize) {
		t.Errorf("Calculate(0, 0, now) produced NaN field(s): %+v", data)
	}
}

// TestMoon_GetPhaseForDate_MatchesCalculate cross-checks the convenience
// wrapper against the underlying age/phase calculation.
func TestMoon_GetPhaseForDate_MatchesCalculate(t *testing.T) {
	ms := NewMoonService()
	when := time.Date(2026, 3, 3, 6, 0, 0, 0, time.UTC)

	got := ms.GetPhaseForDate(when)
	want := ms.getPhaseName(ms.calculateMoonAge(when))
	if got != want {
		t.Errorf("GetPhaseForDate(%v) = %q, want %q", when, got, want)
	}
}

// TestMoon_GetIlluminationForDate_MatchesCalculate cross-checks the
// convenience wrapper against the underlying age/illumination calculation.
func TestMoon_GetIlluminationForDate_MatchesCalculate(t *testing.T) {
	ms := NewMoonService()
	when := time.Date(2026, 3, 3, 6, 0, 0, 0, time.UTC)

	got := ms.GetIlluminationForDate(when)
	want := ms.calculateIllumination(ms.calculateMoonAge(when))
	if !almostEqual(got, want, moonFloatTolerance) {
		t.Errorf("GetIlluminationForDate(%v) = %v, want %v", when, got, want)
	}
}
