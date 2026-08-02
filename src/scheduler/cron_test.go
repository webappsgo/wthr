package scheduler

import (
	"testing"
	"time"
)

// TestEveryScheduleNext verifies the fixed-interval schedule simply adds the
// interval to the reference time.
func TestEveryScheduleNext(t *testing.T) {
	ref := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := everySchedule{interval: 5 * time.Minute}

	got := s.Next(ref)
	want := ref.Add(5 * time.Minute)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}
}

// TestParseSchedule_Every covers the "@every <duration>" syntax, including
// invalid duration strings and non-positive durations.
func TestParseSchedule_Every(t *testing.T) {
	sched, err := parseSchedule("@every 90s")
	if err != nil {
		t.Fatalf("parseSchedule(@every 90s) returned error: %v", err)
	}
	es, ok := sched.(everySchedule)
	if !ok {
		t.Fatalf("expected everySchedule, got %T", sched)
	}
	if es.interval != 90*time.Second {
		t.Errorf("interval = %v, want %v", es.interval, 90*time.Second)
	}

	if _, err := parseSchedule("@every notaduration"); err == nil {
		t.Error("expected error for invalid @every duration, got nil")
	}
	if _, err := parseSchedule("@every -5m"); err == nil {
		t.Error("expected error for non-positive @every duration, got nil")
	}
	if _, err := parseSchedule("@every 0s"); err == nil {
		t.Error("expected error for zero @every duration, got nil")
	}
}

// TestParseSchedule_Descriptors verifies every named descriptor expands to
// its documented cron expression, and an unknown descriptor errors.
func TestParseSchedule_Descriptors(t *testing.T) {
	for desc := range descriptors {
		if _, err := parseSchedule(desc); err != nil {
			t.Errorf("parseSchedule(%q) returned error: %v", desc, err)
		}
	}

	if _, err := parseSchedule("@nonexistent"); err == nil {
		t.Error("expected error for unknown descriptor, got nil")
	}
}

// TestParseSchedule_Empty verifies an empty or whitespace-only spec errors.
func TestParseSchedule_Empty(t *testing.T) {
	if _, err := parseSchedule(""); err == nil {
		t.Error("expected error for empty schedule, got nil")
	}
	if _, err := parseSchedule("   "); err == nil {
		t.Error("expected error for whitespace-only schedule, got nil")
	}
}

// TestParseSchedule_FieldCount verifies a wrong number of cron fields errors.
func TestParseSchedule_FieldCount(t *testing.T) {
	cases := []string{
		"* * *",
		"* * * * * *",
	}
	for _, c := range cases {
		if _, err := parseSchedule(c); err == nil {
			t.Errorf("parseSchedule(%q) expected error, got nil", c)
		}
	}
}

// TestParseSchedule_InvalidFields verifies each field position surfaces a
// wrapped, field-specific error on invalid input.
func TestParseSchedule_InvalidFields(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{"minute", "99 * * * *"},
		{"hour", "* 99 * * *"},
		{"dom", "* * 99 * *"},
		{"month", "* * * 99 *"},
		{"dow", "* * * * 99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseSchedule(tc.spec); err == nil {
				t.Errorf("parseSchedule(%q) expected error, got nil", tc.spec)
			}
		})
	}
}

// TestParseSchedule_DowSevenAlias verifies "7" in the day-of-week field is
// treated as an alias for Sunday (0).
func TestParseSchedule_DowSevenAlias(t *testing.T) {
	sched, err := parseSchedule("0 0 * * 7")
	if err != nil {
		t.Fatalf("parseSchedule returned error: %v", err)
	}
	cs, ok := sched.(cronSchedule)
	if !ok {
		t.Fatalf("expected cronSchedule, got %T", sched)
	}
	if !cs.dow[0] {
		t.Error("expected dow[0] (Sunday) to be set via the 7 alias")
	}
	if cs.dow[7] {
		t.Error("expected dow[7] to be deleted after aliasing to 0")
	}
}

// TestCronSchedule_Next verifies the standard 5-field cron matcher finds the
// correct next activation time, including a case that must roll to a
// subsequent day and month.
func TestCronSchedule_Next(t *testing.T) {
	// Every day at 02:00.
	sched, err := parseSchedule("0 2 * * *")
	if err != nil {
		t.Fatalf("parseSchedule returned error: %v", err)
	}

	ref := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)
	got := sched.Next(ref)
	want := time.Date(2024, 1, 2, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, got, want)
	}

	// Specific day-of-month that requires rolling into the next month.
	sched2, err := parseSchedule("0 0 1 * *")
	if err != nil {
		t.Fatalf("parseSchedule returned error: %v", err)
	}
	ref2 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	got2 := sched2.Next(ref2)
	want2 := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	if !got2.Equal(want2) {
		t.Errorf("Next(%v) = %v, want %v", ref2, got2, want2)
	}
}

// TestCronSchedule_NextUnreachable verifies an impossible field combination
// (Feb 30th) is bounded by maxScanMinutes and returns the zero time rather
// than looping forever.
func TestCronSchedule_NextUnreachable(t *testing.T) {
	sched, err := parseSchedule("0 0 30 2 *")
	if err != nil {
		t.Fatalf("parseSchedule returned error: %v", err)
	}
	ref := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	got := sched.Next(ref)
	if !got.IsZero() {
		t.Errorf("expected zero time for unreachable schedule, got %v", got)
	}
}

// TestParseField covers wildcard, list, range, and step syntax, plus the
// out-of-range and malformed-value error paths.
func TestParseField(t *testing.T) {
	set, err := parseField("*", 0, 4)
	if err != nil {
		t.Fatalf("parseField(*) returned error: %v", err)
	}
	for i := 0; i <= 4; i++ {
		if !set[i] {
			t.Errorf("expected %d in wildcard set", i)
		}
	}

	set, err = parseField("1,3,5", 0, 10)
	if err != nil {
		t.Fatalf("parseField(1,3,5) returned error: %v", err)
	}
	for _, v := range []int{1, 3, 5} {
		if !set[v] {
			t.Errorf("expected %d in list set", v)
		}
	}
	if set[2] || set[4] {
		t.Error("expected only listed values to be set")
	}

	set, err = parseField("2-4", 0, 10)
	if err != nil {
		t.Fatalf("parseField(2-4) returned error: %v", err)
	}
	for _, v := range []int{2, 3, 4} {
		if !set[v] {
			t.Errorf("expected %d in range set", v)
		}
	}
	if set[1] || set[5] {
		t.Error("expected only in-range values to be set")
	}

	set, err = parseField("0-10/5", 0, 10)
	if err != nil {
		t.Fatalf("parseField(0-10/5) returned error: %v", err)
	}
	for _, v := range []int{0, 5, 10} {
		if !set[v] {
			t.Errorf("expected %d in stepped range set", v)
		}
	}
	if set[1] || set[3] || set[7] {
		t.Error("expected only stepped values to be set")
	}

	if _, err := parseField("99", 0, 10); err == nil {
		t.Error("expected error for out-of-range value, got nil")
	}
	if _, err := parseField("5-2", 0, 10); err == nil {
		t.Error("expected error for inverted range (lo > hi), got nil")
	}
	if _, err := parseField("abc", 0, 10); err == nil {
		t.Error("expected error for non-numeric value, got nil")
	}
	if _, err := parseField("1-abc", 0, 10); err == nil {
		t.Error("expected error for non-numeric range bound, got nil")
	}
}

// TestSplitStep covers the range/step splitting helper, including the
// default step of 1 and invalid/non-positive step errors.
func TestSplitStep(t *testing.T) {
	rangePart, step, err := splitStep("1-5")
	if err != nil {
		t.Fatalf("splitStep(1-5) returned error: %v", err)
	}
	if rangePart != "1-5" || step != 1 {
		t.Errorf("splitStep(1-5) = (%q, %d), want (%q, %d)", rangePart, step, "1-5", 1)
	}

	rangePart, step, err = splitStep("1-30/5")
	if err != nil {
		t.Fatalf("splitStep(1-30/5) returned error: %v", err)
	}
	if rangePart != "1-30" || step != 5 {
		t.Errorf("splitStep(1-30/5) = (%q, %d), want (%q, %d)", rangePart, step, "1-30", 5)
	}

	if _, _, err := splitStep("1-30/abc"); err == nil {
		t.Error("expected error for non-numeric step, got nil")
	}
	if _, _, err := splitStep("1-30/0"); err == nil {
		t.Error("expected error for zero step, got nil")
	}
	if _, _, err := splitStep("1-30/-1"); err == nil {
		t.Error("expected error for negative step, got nil")
	}
}
