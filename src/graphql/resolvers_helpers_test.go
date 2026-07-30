package graphql

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server/handler"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// TestHurricaneCategory covers every Saffir-Simpson threshold boundary pair
// exactly, plus zero/negative wind speeds.
func TestHurricaneCategory(t *testing.T) {
	tests := []struct {
		name      string
		windSpeed int
		want      int
	}{
		{"negative", -10, 0},
		{"zero", 0, 0},
		{"just below cat1", 73, 0},
		{"cat1 boundary", 74, 1},
		{"just below cat2", 95, 1},
		{"cat2 boundary", 96, 2},
		{"just below cat3", 110, 2},
		{"cat3 boundary", 111, 3},
		{"just below cat4", 129, 3},
		{"cat4 boundary", 130, 4},
		{"just below cat5", 156, 4},
		{"cat5 boundary", 157, 5},
		{"well above cat5", 200, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hurricaneCategory(tt.windSpeed)
			if got != tt.want {
				t.Errorf("hurricaneCategory(%d) = %d, want %d", tt.windSpeed, got, tt.want)
			}
		})
	}
}

// TestParseGraphQLTime covers every supported format, empty string,
// unparseable garbage, and whitespace trimming.
func TestParseGraphQLTime(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantZero bool
	}{
		{
			name:  "RFC3339Nano",
			value: "2024-03-15T10:30:00.123456789Z",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 123456789, time.UTC),
		},
		{
			name:  "RFC3339",
			value: "2024-03-15T10:30:00Z",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC1123Z",
			value: "Fri, 15 Mar 2024 10:30:00 +0000",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC1123",
			value: "Fri, 15 Mar 2024 10:30:00 UTC",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC822Z",
			value: "15 Mar 24 10:30 +0000",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC822",
			value: "15 Mar 24 10:30 UTC",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "custom Mon, 02 Jan 2006 15:04:05 MST",
			value: "Fri, 15 Mar 2024 10:30:00 UTC",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "custom 2006-01-02 15:04:05",
			value: "2024-03-15 10:30:00",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "custom 2006-01-02",
			value: "2024-03-15",
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "empty string returns zero time",
			value:    "",
			wantZero: true,
		},
		{
			name:     "garbage returns zero time",
			value:    "not-a-time-at-all",
			wantZero: true,
		},
		{
			name:  "leading and trailing whitespace trimmed before parsing",
			value: "   2024-03-15   ",
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGraphQLTime(tt.value)
			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("parseGraphQLTime(%q) = %v, want zero time", tt.value, got)
				}
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseGraphQLTime(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestFirstNonEmpty verifies the exact fallback semantics, including the
// subtlety that the returned value is NOT trimmed even though the emptiness
// check itself trims for comparison purposes.
func TestFirstNonEmpty(t *testing.T) {
	t.Run("no args", func(t *testing.T) {
		got := firstNonEmpty()
		if got != "" {
			t.Errorf("firstNonEmpty() = %q, want empty string", got)
		}
	})

	t.Run("all empty args", func(t *testing.T) {
		got := firstNonEmpty("", "", "")
		if got != "" {
			t.Errorf("firstNonEmpty(\"\",\"\",\"\") = %q, want empty string", got)
		}
	})

	t.Run("all whitespace args are treated as empty", func(t *testing.T) {
		got := firstNonEmpty("   ", "\t", "x")
		if got != "x" {
			t.Errorf("firstNonEmpty(whitespace, whitespace, x) = %q, want %q", got, "x")
		}
	})

	t.Run("first empty second set returns UNTRIMMED original value", func(t *testing.T) {
		got := firstNonEmpty("", "  x  ")
		if got != "  x  " {
			t.Errorf("firstNonEmpty(\"\", \"  x  \") = %q, want %q (untrimmed)", got, "  x  ")
		}
	})

	t.Run("all set returns first", func(t *testing.T) {
		got := firstNonEmpty("first", "second", "third")
		if got != "first" {
			t.Errorf("firstNonEmpty(first,second,third) = %q, want %q", got, "first")
		}
	})
}

// TestMapGraphQLEarthquake covers the Tsunami tri-state mapping, Depth
// zero-vs-nonzero, and URL empty-vs-nonempty branches.
func TestMapGraphQLEarthquake(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		item        service.Earthquake
		wantTsunami *bool
		wantDepth   *float64
		wantURL     *string
	}{
		{
			name:        "tsunami 0 maps to false pointer",
			item:        service.Earthquake{ID: "eq1", Tsunami: 0},
			wantTsunami: boolPtr(false),
		},
		{
			name:        "tsunami 1 maps to true pointer",
			item:        service.Earthquake{ID: "eq2", Tsunami: 1},
			wantTsunami: boolPtr(true),
		},
		{
			name:        "tsunami 2 maps to nil (only 0/1 are valid)",
			item:        service.Earthquake{ID: "eq3", Tsunami: 2},
			wantTsunami: nil,
		},
		{
			name:        "tsunami -1 maps to nil",
			item:        service.Earthquake{ID: "eq4", Tsunami: -1},
			wantTsunami: nil,
		},
		{
			name:      "zero depth maps to nil pointer",
			item:      service.Earthquake{ID: "eq5", Depth: 0, Tsunami: 2},
			wantDepth: nil,
		},
		{
			name:      "nonzero depth maps to pointer",
			item:      service.Earthquake{ID: "eq6", Depth: 12.5, Tsunami: 2},
			wantDepth: float64Ptr(12.5),
		},
		{
			name:    "empty URL maps to nil",
			item:    service.Earthquake{ID: "eq7", URL: "", Tsunami: 2},
			wantURL: nil,
		},
		{
			name:    "nonempty URL maps to pointer",
			item:    service.Earthquake{ID: "eq8", URL: "https://example.com/eq8", Tsunami: 2},
			wantURL: stringPtr("https://example.com/eq8"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.item.Time = baseTime
			got := mapGraphQLEarthquake(tt.item)
			if got == nil {
				t.Fatal("mapGraphQLEarthquake returned nil")
			}
			if got.ID != tt.item.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.item.ID)
			}
			if !ptrBoolEqual(got.Tsunami, tt.wantTsunami) {
				t.Errorf("Tsunami = %v, want %v", derefBool(got.Tsunami), derefBool(tt.wantTsunami))
			}
			if !ptrFloatEqual(got.Depth, tt.wantDepth) {
				t.Errorf("Depth = %v, want %v", derefFloat(got.Depth), derefFloat(tt.wantDepth))
			}
			if !ptrStringEqual(got.URL, tt.wantURL) {
				t.Errorf("URL = %v, want %v", derefString(got.URL), derefString(tt.wantURL))
			}
		})
	}
}

// TestMapGraphQLEarthquakes covers nil, empty, and multi-item slices.
func TestMapGraphQLEarthquakes(t *testing.T) {
	t.Run("nil slice returns non-nil empty slice", func(t *testing.T) {
		got := mapGraphQLEarthquakes(nil)
		if got == nil {
			t.Fatal("mapGraphQLEarthquakes(nil) returned nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("empty slice returns non-nil empty slice", func(t *testing.T) {
		got := mapGraphQLEarthquakes([]service.Earthquake{})
		if got == nil {
			t.Fatal("mapGraphQLEarthquakes([]) returned nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("multi item slice preserves order and count", func(t *testing.T) {
		items := []service.Earthquake{
			{ID: "a"}, {ID: "b"}, {ID: "c"},
		}
		got := mapGraphQLEarthquakes(items)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		for i, want := range []string{"a", "b", "c"} {
			if got[i].ID != want {
				t.Errorf("item[%d].ID = %q, want %q", i, got[i].ID, want)
			}
		}
	})
}

// TestMapGraphQLHurricane covers MinPressure boundary conditions, all four
// Movement combinations, and the Category delegation to hurricaneCategory.
func TestMapGraphQLHurricane(t *testing.T) {
	t.Run("pressure zero yields nil MinPressure", func(t *testing.T) {
		storm := service.Storm{ID: "s1", Pressure: 0}
		got := mapGraphQLHurricane(storm)
		if got.MinPressure != nil {
			t.Errorf("MinPressure = %v, want nil", *got.MinPressure)
		}
	})

	t.Run("negative pressure yields nil MinPressure", func(t *testing.T) {
		storm := service.Storm{ID: "s2", Pressure: -5}
		got := mapGraphQLHurricane(storm)
		if got.MinPressure != nil {
			t.Errorf("MinPressure = %v, want nil", *got.MinPressure)
		}
	})

	t.Run("positive pressure yields set MinPressure", func(t *testing.T) {
		storm := service.Storm{ID: "s3", Pressure: 950}
		got := mapGraphQLHurricane(storm)
		if got.MinPressure == nil || *got.MinPressure != 950 {
			t.Errorf("MinPressure = %v, want 950", got.MinPressure)
		}
	})

	t.Run("neither movement dir nor speed set yields nil Movement", func(t *testing.T) {
		storm := service.Storm{ID: "s4", MovementDir: "", MovementSpeed: 0}
		got := mapGraphQLHurricane(storm)
		if got.Movement != nil {
			t.Errorf("Movement = %+v, want nil", got.Movement)
		}
	})

	t.Run("movement dir only sets Direction, leaves Speed nil", func(t *testing.T) {
		storm := service.Storm{ID: "s5", MovementDir: "NW", MovementSpeed: 0}
		got := mapGraphQLHurricane(storm)
		if got.Movement == nil {
			t.Fatal("Movement = nil, want non-nil")
		}
		if got.Movement.Direction == nil || *got.Movement.Direction != "NW" {
			t.Errorf("Movement.Direction = %v, want NW", got.Movement.Direction)
		}
		if got.Movement.Speed != nil {
			t.Errorf("Movement.Speed = %v, want nil", *got.Movement.Speed)
		}
	})

	t.Run("movement speed only sets Speed, leaves Direction nil", func(t *testing.T) {
		storm := service.Storm{ID: "s6", MovementDir: "", MovementSpeed: 15}
		got := mapGraphQLHurricane(storm)
		if got.Movement == nil {
			t.Fatal("Movement = nil, want non-nil")
		}
		if got.Movement.Direction != nil {
			t.Errorf("Movement.Direction = %v, want nil", *got.Movement.Direction)
		}
		if got.Movement.Speed == nil || *got.Movement.Speed != 15 {
			t.Errorf("Movement.Speed = %v, want 15", got.Movement.Speed)
		}
	})

	t.Run("both movement dir and speed set", func(t *testing.T) {
		storm := service.Storm{ID: "s7", MovementDir: "NE", MovementSpeed: 20}
		got := mapGraphQLHurricane(storm)
		if got.Movement == nil {
			t.Fatal("Movement = nil, want non-nil")
		}
		if got.Movement.Direction == nil || *got.Movement.Direction != "NE" {
			t.Errorf("Movement.Direction = %v, want NE", got.Movement.Direction)
		}
		if got.Movement.Speed == nil || *got.Movement.Speed != 20 {
			t.Errorf("Movement.Speed = %v, want 20", got.Movement.Speed)
		}
	})

	t.Run("Category equals hurricaneCategory(WindSpeed)", func(t *testing.T) {
		storm := service.Storm{ID: "s8", WindSpeed: 120}
		got := mapGraphQLHurricane(storm)
		want := hurricaneCategory(storm.WindSpeed)
		if got.Category != want {
			t.Errorf("Category = %d, want %d", got.Category, want)
		}
		if got.MaxWindSpeed != float64(storm.WindSpeed) {
			t.Errorf("MaxWindSpeed = %v, want %v", got.MaxWindSpeed, float64(storm.WindSpeed))
		}
	})
}

func TestMapGraphQLHurricanes(t *testing.T) {
	t.Run("nil slice returns non-nil empty slice", func(t *testing.T) {
		got := mapGraphQLHurricanes(nil)
		if got == nil || len(got) != 0 {
			t.Errorf("mapGraphQLHurricanes(nil) = %v, want non-nil empty slice", got)
		}
	})

	t.Run("multi item slice", func(t *testing.T) {
		got := mapGraphQLHurricanes([]service.Storm{{ID: "a"}, {ID: "b"}})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})
}

// TestMapGraphQLSevereWeather covers nil-input non-nil-empty-slice semantics
// and concatenation order across the 5 alert categories.
func TestMapGraphQLSevereWeather(t *testing.T) {
	t.Run("nil input returns non-nil empty slice", func(t *testing.T) {
		got := mapGraphQLSevereWeather(nil)
		if got == nil {
			t.Fatal("mapGraphQLSevereWeather(nil) returned nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("multi-category data concatenates in field declaration order", func(t *testing.T) {
		data := &service.SevereWeatherData{
			TornadoWarnings: []service.Alert{{ID: "tornado-1"}},
			SevereStorms:    []service.Alert{{ID: "storm-1"}, {ID: "storm-2"}},
			WinterStorms:    []service.Alert{{ID: "winter-1"}},
			FloodWarnings:   []service.Alert{{ID: "flood-1"}},
			OtherAlerts:     []service.Alert{{ID: "other-1"}},
		}
		got := mapGraphQLSevereWeather(data)
		wantOrder := []string{"tornado-1", "storm-1", "storm-2", "winter-1", "flood-1", "other-1"}
		if len(got) != len(wantOrder) {
			t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
		}
		for i, wantID := range wantOrder {
			if got[i].ID != wantID {
				t.Errorf("item[%d].ID = %q, want %q", i, got[i].ID, wantID)
			}
		}
	})

	t.Run("empty struct returns non-nil empty slice", func(t *testing.T) {
		got := mapGraphQLSevereWeather(&service.SevereWeatherData{})
		if got == nil || len(got) != 0 {
			t.Errorf("mapGraphQLSevereWeather(empty) = %v, want non-nil empty slice", got)
		}
	})
}

// TestMapGraphQLSevereWeatherAlert covers Description/AreaDesc fallbacks,
// Instruction nil-vs-set, and the Effective/Expires firstNonEmpty chains.
func TestMapGraphQLSevereWeatherAlert(t *testing.T) {
	t.Run("description falls back to headline when empty", func(t *testing.T) {
		alert := service.Alert{Description: "", Headline: "Tornado headline"}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Description != "Tornado headline" {
			t.Errorf("Description = %q, want %q", got.Description, "Tornado headline")
		}
	})

	t.Run("description used as-is when set", func(t *testing.T) {
		alert := service.Alert{Description: "Real description", Headline: "ignored headline"}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Description != "Real description" {
			t.Errorf("Description = %q, want %q", got.Description, "Real description")
		}
	})

	t.Run("area desc falls back to Unknown area literal when empty", func(t *testing.T) {
		alert := service.Alert{AreaDesc: ""}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Location == nil || got.Location.Name != "Unknown area" {
			t.Errorf("Location.Name = %v, want %q", got.Location, "Unknown area")
		}
	})

	t.Run("area desc used as-is when set", func(t *testing.T) {
		alert := service.Alert{AreaDesc: "Dallas County"}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Location == nil || got.Location.Name != "Dallas County" {
			t.Errorf("Location.Name = %v, want %q", got.Location, "Dallas County")
		}
	})

	t.Run("instruction nil when blank", func(t *testing.T) {
		alert := service.Alert{Instruction: ""}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Instruction != nil {
			t.Errorf("Instruction = %v, want nil", *got.Instruction)
		}
	})

	t.Run("instruction nil when whitespace only", func(t *testing.T) {
		alert := service.Alert{Instruction: "   \t  "}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Instruction != nil {
			t.Errorf("Instruction = %v, want nil", *got.Instruction)
		}
	})

	t.Run("instruction set when non-blank", func(t *testing.T) {
		alert := service.Alert{Instruction: "Seek shelter immediately"}
		got := mapGraphQLSevereWeatherAlert(alert)
		if got.Instruction == nil || *got.Instruction != "Seek shelter immediately" {
			t.Errorf("Instruction = %v, want %q", got.Instruction, "Seek shelter immediately")
		}
	})

	t.Run("effective falls back through Sent when Effective empty", func(t *testing.T) {
		alert := service.Alert{Effective: "", Sent: "2024-03-15"}
		got := mapGraphQLSevereWeatherAlert(alert)
		want := parseGraphQLTime("2024-03-15")
		if !got.Effective.Equal(want) {
			t.Errorf("Effective = %v, want %v", got.Effective, want)
		}
	})

	t.Run("effective uses Effective when set, ignoring Sent", func(t *testing.T) {
		alert := service.Alert{Effective: "2024-03-16", Sent: "2024-03-15"}
		got := mapGraphQLSevereWeatherAlert(alert)
		want := parseGraphQLTime("2024-03-16")
		if !got.Effective.Equal(want) {
			t.Errorf("Effective = %v, want %v", got.Effective, want)
		}
	})

	t.Run("expires falls back through Expires, Effective, Sent in order", func(t *testing.T) {
		// Expires empty -> falls to Effective
		alert := service.Alert{Expires: "", Effective: "2024-03-17", Sent: "2024-03-15"}
		got := mapGraphQLSevereWeatherAlert(alert)
		want := parseGraphQLTime("2024-03-17")
		if !got.Expires.Equal(want) {
			t.Errorf("Expires = %v, want %v", got.Expires, want)
		}
	})

	t.Run("expires falls all the way to Sent when Expires and Effective both empty", func(t *testing.T) {
		alert := service.Alert{Expires: "", Effective: "", Sent: "2024-03-18"}
		got := mapGraphQLSevereWeatherAlert(alert)
		want := parseGraphQLTime("2024-03-18")
		if !got.Expires.Equal(want) {
			t.Errorf("Expires = %v, want %v", got.Expires, want)
		}
	})
}

// TestGraphQLUserInviteStatus exercises every branch: nil invite, used
// (UsedAt set), used (MaxUses reached), expired, and pending.
func TestGraphQLUserInviteStatus(t *testing.T) {
	now := time.Now()

	t.Run("nil invite is pending", func(t *testing.T) {
		got := graphQLUserInviteStatus(nil)
		if got != "pending" {
			t.Errorf("graphQLUserInviteStatus(nil) = %q, want %q", got, "pending")
		}
	})

	t.Run("UsedAt set is used", func(t *testing.T) {
		usedAt := now.Add(-time.Minute)
		invite := &models.UserInvite{
			UsedAt:    &usedAt,
			ExpiresAt: now.Add(time.Hour),
		}
		got := graphQLUserInviteStatus(invite)
		if got != "used" {
			t.Errorf("graphQLUserInviteStatus = %q, want %q", got, "used")
		}
	})

	t.Run("MaxUses reached is used even without UsedAt", func(t *testing.T) {
		invite := &models.UserInvite{
			MaxUses:   3,
			UseCount:  3,
			ExpiresAt: now.Add(time.Hour),
		}
		got := graphQLUserInviteStatus(invite)
		if got != "used" {
			t.Errorf("graphQLUserInviteStatus = %q, want %q", got, "used")
		}
	})

	t.Run("MaxUses set but UseCount below limit is not used solely by that rule", func(t *testing.T) {
		invite := &models.UserInvite{
			MaxUses:   3,
			UseCount:  1,
			ExpiresAt: now.Add(time.Hour),
		}
		got := graphQLUserInviteStatus(invite)
		if got != "pending" {
			t.Errorf("graphQLUserInviteStatus = %q, want %q", got, "pending")
		}
	})

	t.Run("ExpiresAt in the past is expired", func(t *testing.T) {
		invite := &models.UserInvite{
			ExpiresAt: now.Add(-time.Hour),
		}
		got := graphQLUserInviteStatus(invite)
		if got != "expired" {
			t.Errorf("graphQLUserInviteStatus = %q, want %q", got, "expired")
		}
	})

	t.Run("ExpiresAt in the future and not used is pending", func(t *testing.T) {
		invite := &models.UserInvite{
			ExpiresAt: now.Add(time.Hour),
		}
		got := graphQLUserInviteStatus(invite)
		if got != "pending" {
			t.Errorf("graphQLUserInviteStatus = %q, want %q", got, "pending")
		}
	})
}

// TestMapSchedulerTaskHistory verifies Ok is exact-match on "success"
// (case-sensitive), CompletedAt nil-vs-set from EndTime.IsZero, and the
// Error trimming/nil-vs-set logic.
func TestMapSchedulerTaskHistory(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	tests := []struct {
		name            string
		run             scheduler.TaskRun
		wantOk          bool
		wantCompletedAt *time.Time
		wantErrorText   *string
	}{
		{
			name:            "status success (lowercase) is Ok true",
			run:             scheduler.TaskRun{TaskName: "t1", StartTime: start, EndTime: end, Status: "success"},
			wantOk:          true,
			wantCompletedAt: &end,
		},
		{
			name:            "status Success (capital S) is NOT Ok - case sensitive",
			run:             scheduler.TaskRun{TaskName: "t2", StartTime: start, EndTime: end, Status: "Success"},
			wantOk:          false,
			wantCompletedAt: &end,
		},
		{
			name:            "status error is not Ok",
			run:             scheduler.TaskRun{TaskName: "t3", StartTime: start, EndTime: end, Status: "error"},
			wantOk:          false,
			wantCompletedAt: &end,
		},
		{
			name:            "zero EndTime yields nil CompletedAt",
			run:             scheduler.TaskRun{TaskName: "t4", StartTime: start, EndTime: time.Time{}, Status: "success"},
			wantOk:          true,
			wantCompletedAt: nil,
		},
		{
			name:            "empty error yields nil Error",
			run:             scheduler.TaskRun{TaskName: "t5", StartTime: start, EndTime: end, Status: "success", Error: ""},
			wantOk:          true,
			wantCompletedAt: &end,
			wantErrorText:   nil,
		},
		{
			name:            "whitespace only error yields nil Error",
			run:             scheduler.TaskRun{TaskName: "t6", StartTime: start, EndTime: end, Status: "success", Error: "   "},
			wantOk:          true,
			wantCompletedAt: &end,
			wantErrorText:   nil,
		},
		{
			name:            "nonblank error yields set Error",
			run:             scheduler.TaskRun{TaskName: "t7", StartTime: start, EndTime: end, Status: "error", Error: "boom"},
			wantOk:          false,
			wantCompletedAt: &end,
			wantErrorText:   stringPtr("boom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapSchedulerTaskHistory(tt.run)
			if got.Ok != tt.wantOk {
				t.Errorf("Ok = %v, want %v", got.Ok, tt.wantOk)
			}
			if got.TaskName != tt.run.TaskName {
				t.Errorf("TaskName = %q, want %q", got.TaskName, tt.run.TaskName)
			}
			if !got.StartedAt.Equal(tt.run.StartTime) {
				t.Errorf("StartedAt = %v, want %v", got.StartedAt, tt.run.StartTime)
			}
			if tt.wantCompletedAt == nil {
				if got.CompletedAt != nil {
					t.Errorf("CompletedAt = %v, want nil", *got.CompletedAt)
				}
			} else {
				if got.CompletedAt == nil || !got.CompletedAt.Equal(*tt.wantCompletedAt) {
					t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, *tt.wantCompletedAt)
				}
			}
			if !ptrStringEqual(got.Error, tt.wantErrorText) {
				t.Errorf("Error = %v, want %v", derefString(got.Error), derefString(tt.wantErrorText))
			}
			if got.Duration == nil || *got.Duration != float64(tt.run.Duration) {
				t.Errorf("Duration = %v, want %v", got.Duration, float64(tt.run.Duration))
			}
		})
	}
}

// TestBuildGraphQLUserToken exercises the Name/Scopes trim-to-nil-or-pointer
// logic and the ExpiresAt/LastUsedAt sql.NullTime validity gating.
func TestBuildGraphQLUserToken(t *testing.T) {
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty name and scopes yield nil pointers", func(t *testing.T) {
		got := buildGraphQLUserToken(1, "", "prefix", "", created, sql.NullTime{}, sql.NullTime{}, nil)
		if got.Name != nil {
			t.Errorf("Name = %v, want nil", *got.Name)
		}
		if got.Scopes != nil {
			t.Errorf("Scopes = %v, want nil", *got.Scopes)
		}
	})

	t.Run("whitespace-only name and scopes yield nil pointers", func(t *testing.T) {
		got := buildGraphQLUserToken(1, "   ", "prefix", "\t", created, sql.NullTime{}, sql.NullTime{}, nil)
		if got.Name != nil {
			t.Errorf("Name = %v, want nil", *got.Name)
		}
		if got.Scopes != nil {
			t.Errorf("Scopes = %v, want nil", *got.Scopes)
		}
	})

	t.Run("non-empty name and scopes are trimmed and set", func(t *testing.T) {
		got := buildGraphQLUserToken(1, "  My Token  ", "prefix", "  read write  ", created, sql.NullTime{}, sql.NullTime{}, nil)
		if got.Name == nil || *got.Name != "My Token" {
			t.Errorf("Name = %v, want %q", got.Name, "My Token")
		}
		if got.Scopes == nil || *got.Scopes != "read write" {
			t.Errorf("Scopes = %v, want %q", got.Scopes, "read write")
		}
	})

	t.Run("invalid NullTime yields nil ExpiresAt and LastUsedAt", func(t *testing.T) {
		got := buildGraphQLUserToken(1, "n", "p", "s", created, sql.NullTime{Valid: false}, sql.NullTime{Valid: false}, nil)
		if got.ExpiresAt != nil {
			t.Errorf("ExpiresAt = %v, want nil", *got.ExpiresAt)
		}
		if got.LastUsedAt != nil {
			t.Errorf("LastUsedAt = %v, want nil", *got.LastUsedAt)
		}
	})

	t.Run("valid NullTime yields set ExpiresAt and LastUsedAt", func(t *testing.T) {
		expires := created.Add(24 * time.Hour)
		lastUsed := created.Add(time.Hour)
		got := buildGraphQLUserToken(1, "n", "p", "s", created,
			sql.NullTime{Time: expires, Valid: true},
			sql.NullTime{Time: lastUsed, Valid: true}, nil)
		if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
			t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, expires)
		}
		if got.LastUsedAt == nil || !got.LastUsedAt.Equal(lastUsed) {
			t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, lastUsed)
		}
	})

	t.Run("id, prefix, createdAt, and token pass through unchanged", func(t *testing.T) {
		tok := "raw-token-value"
		got := buildGraphQLUserToken(42, "n", "abcd1234", "s", created, sql.NullTime{}, sql.NullTime{}, &tok)
		if got.ID != "42" {
			t.Errorf("ID = %q, want %q", got.ID, "42")
		}
		if got.TokenPrefix != "abcd1234" {
			t.Errorf("TokenPrefix = %q, want %q", got.TokenPrefix, "abcd1234")
		}
		if !got.CreatedAt.Equal(created) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created)
		}
		if got.Token == nil || *got.Token != tok {
			t.Errorf("Token = %v, want %q", got.Token, tok)
		}
	})
}

// TestScanGraphQLScheduledTask uses a hand-rolled scan closure to exercise
// error propagation and NullTime valid/invalid mapping without a real *sql.Row.
func TestScanGraphQLScheduledTask(t *testing.T) {
	t.Run("scan error propagates", func(t *testing.T) {
		wantErr := errors.New("scan failed")
		_, err := scanGraphQLScheduledTask(func(dest ...any) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want %v", err, wantErr)
		}
	})

	t.Run("valid NullTime fields populate LastRun and NextRun", func(t *testing.T) {
		lastRun := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		nextRun := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

		task, err := scanGraphQLScheduledTask(func(dest ...any) error {
			*(dest[0].(*string)) = "my-task"
			*(dest[1].(*string)) = "* * * * *"
			*(dest[2].(*bool)) = true
			*(dest[3].(*sql.NullTime)) = sql.NullTime{Time: lastRun, Valid: true}
			*(dest[4].(*sql.NullTime)) = sql.NullTime{Time: nextRun, Valid: true}
			*(dest[5].(*int)) = 10
			*(dest[6].(*int)) = 2
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Name != "my-task" || task.Schedule != "* * * * *" || !task.Enabled {
			t.Errorf("task = %+v, unexpected base fields", task)
		}
		if task.LastRun == nil || !task.LastRun.Equal(lastRun) {
			t.Errorf("LastRun = %v, want %v", task.LastRun, lastRun)
		}
		if task.NextRun == nil || !task.NextRun.Equal(nextRun) {
			t.Errorf("NextRun = %v, want %v", task.NextRun, nextRun)
		}
		if task.RunCount != 10 || task.ErrorCount != 2 {
			t.Errorf("RunCount/ErrorCount = %d/%d, want 10/2", task.RunCount, task.ErrorCount)
		}
	})

	t.Run("invalid NullTime fields leave LastRun and NextRun nil", func(t *testing.T) {
		task, err := scanGraphQLScheduledTask(func(dest ...any) error {
			*(dest[0].(*string)) = "my-task"
			*(dest[1].(*string)) = "* * * * *"
			*(dest[2].(*bool)) = false
			*(dest[3].(*sql.NullTime)) = sql.NullTime{Valid: false}
			*(dest[4].(*sql.NullTime)) = sql.NullTime{Valid: false}
			*(dest[5].(*int)) = 0
			*(dest[6].(*int)) = 0
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.LastRun != nil {
			t.Errorf("LastRun = %v, want nil", *task.LastRun)
		}
		if task.NextRun != nil {
			t.Errorf("NextRun = %v, want nil", *task.NextRun)
		}
	})
}

// TestScanGraphQLNotificationChannel covers scan error propagation, valid
// JSON config decoding, valid-but-non-JSON config falling back to raw
// string, and NULL config leaving Config unset.
func TestScanGraphQLNotificationChannel(t *testing.T) {
	t.Run("scan error propagates", func(t *testing.T) {
		wantErr := errors.New("scan failed")
		_, err := scanGraphQLNotificationChannel(func(dest ...any) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want %v", err, wantErr)
		}
	})

	t.Run("valid JSON config is decoded", func(t *testing.T) {
		channel, err := scanGraphQLNotificationChannel(func(dest ...any) error {
			*(dest[0].(*string)) = "email"
			*(dest[1].(*bool)) = true
			*(dest[2].(*sql.NullString)) = sql.NullString{String: `{"host":"smtp.example.com"}`, Valid: true}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		decoded, ok := channel.Config.(map[string]any)
		if !ok {
			t.Fatalf("Config = %T, want map[string]any", channel.Config)
		}
		if decoded["host"] != "smtp.example.com" {
			t.Errorf("Config[host] = %v, want smtp.example.com", decoded["host"])
		}
	})

	t.Run("non-JSON string config falls back to raw string", func(t *testing.T) {
		channel, err := scanGraphQLNotificationChannel(func(dest ...any) error {
			*(dest[0].(*string)) = "sms"
			*(dest[1].(*bool)) = false
			*(dest[2].(*sql.NullString)) = sql.NullString{String: "not json at all", Valid: true}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if channel.Config != "not json at all" {
			t.Errorf("Config = %v, want raw string", channel.Config)
		}
	})

	t.Run("invalid NullString leaves Config nil", func(t *testing.T) {
		channel, err := scanGraphQLNotificationChannel(func(dest ...any) error {
			*(dest[0].(*string)) = "push"
			*(dest[1].(*bool)) = true
			*(dest[2].(*sql.NullString)) = sql.NullString{Valid: false}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if channel.Config != nil {
			t.Errorf("Config = %v, want nil", channel.Config)
		}
	})
}

// TestScanGraphQLSetting covers scan error propagation and the
// Description/UpdatedAt NullString/NullTime valid/invalid mapping.
func TestScanGraphQLSetting(t *testing.T) {
	t.Run("scan error propagates", func(t *testing.T) {
		wantErr := errors.New("scan failed")
		_, err := scanGraphQLSetting(func(dest ...any) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want %v", err, wantErr)
		}
	})

	t.Run("valid description and updatedAt populate fields", func(t *testing.T) {
		updated := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		setting, err := scanGraphQLSetting(func(dest ...any) error {
			*(dest[0].(*string)) = "site.title"
			*(dest[1].(*string)) = "WTHR"
			*(dest[2].(*string)) = "string"
			*(dest[3].(*sql.NullString)) = sql.NullString{String: "Site title", Valid: true}
			*(dest[4].(*sql.NullTime)) = sql.NullTime{Time: updated, Valid: true}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if setting.Description != "Site title" {
			t.Errorf("Description = %q, want %q", setting.Description, "Site title")
		}
		if !setting.UpdatedAt.Equal(updated) {
			t.Errorf("UpdatedAt = %v, want %v", setting.UpdatedAt, updated)
		}
	})

	t.Run("invalid description and updatedAt leave zero values", func(t *testing.T) {
		setting, err := scanGraphQLSetting(func(dest ...any) error {
			*(dest[0].(*string)) = "site.title"
			*(dest[1].(*string)) = "WTHR"
			*(dest[2].(*string)) = "string"
			*(dest[3].(*sql.NullString)) = sql.NullString{Valid: false}
			*(dest[4].(*sql.NullTime)) = sql.NullTime{Valid: false}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if setting.Description != "" {
			t.Errorf("Description = %q, want empty", setting.Description)
		}
		if !setting.UpdatedAt.IsZero() {
			t.Errorf("UpdatedAt = %v, want zero", setting.UpdatedAt)
		}
	})
}

// TestMapGraphQLUserSettings covers the nil-input case and a full happy path
// mapping across all four nested settings groups.
func TestMapGraphQLUserSettings(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLUserSettings(nil); got != nil {
			t.Errorf("mapGraphQLUserSettings(nil) = %+v, want nil", got)
		}
	})

	t.Run("happy path maps all nested groups", func(t *testing.T) {
		src := &handler.UserSettingsResponse{
			Account:       handler.AccountSettings{DisplayName: "Jane", Language: "en"},
			Privacy:       handler.PrivacySettings{Visibility: "public", ShowEmail: true},
			Notifications: handler.NotificationSettings{EmailSecurity: true, EmailDigest: "daily"},
			Appearance:    handler.AppearanceSettings{Theme: "dark", FontSize: "medium"},
		}
		got := mapGraphQLUserSettings(src)
		if got == nil {
			t.Fatal("mapGraphQLUserSettings returned nil for non-nil input")
		}
		if got.Account.DisplayName != "Jane" || got.Account.Language != "en" {
			t.Errorf("Account = %+v", got.Account)
		}
		if got.Privacy.Visibility != "public" || !got.Privacy.ShowEmail {
			t.Errorf("Privacy = %+v", got.Privacy)
		}
		if !got.Notifications.EmailSecurity || got.Notifications.EmailDigest != "daily" {
			t.Errorf("Notifications = %+v", got.Notifications)
		}
		if got.Appearance.Theme != "dark" || got.Appearance.FontSize != "medium" {
			t.Errorf("Appearance = %+v", got.Appearance)
		}
	})
}

// TestMapGraphQLPublicUserProfile covers the nil-input case and the
// trim-to-nil-or-pointer branches for DisplayName/Bio/Location/Website.
func TestMapGraphQLPublicUserProfile(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLPublicUserProfile(nil); got != nil {
			t.Errorf("mapGraphQLPublicUserProfile(nil) = %+v, want nil", got)
		}
	})

	t.Run("blank optional fields map to nil pointers", func(t *testing.T) {
		src := &handler.PublicUserProfile{
			Username: "jane",
			Avatar:   handler.AvatarInfo{Type: "gravatar", URLs: map[string]string{"32": "url32"}},
		}
		got := mapGraphQLPublicUserProfile(src)
		if got.DisplayName != nil || got.Bio != nil || got.Location != nil || got.Website != nil {
			t.Errorf("expected all optional fields nil, got %+v", got)
		}
		if got.Avatar == nil || got.Avatar.Type != "gravatar" {
			t.Errorf("Avatar = %+v", got.Avatar)
		}
	})

	t.Run("non-blank optional fields map to trimmed pointers", func(t *testing.T) {
		src := &handler.PublicUserProfile{
			Username:    "jane",
			DisplayName: "  Jane Doe  ",
			Bio:         "  hello  ",
			Location:    "  NYC  ",
			Website:     "  https://example.com  ",
			Avatar:      handler.AvatarInfo{Type: "upload"},
		}
		got := mapGraphQLPublicUserProfile(src)
		if got.DisplayName == nil || *got.DisplayName != "Jane Doe" {
			t.Errorf("DisplayName = %v", got.DisplayName)
		}
		if got.Bio == nil || *got.Bio != "hello" {
			t.Errorf("Bio = %v", got.Bio)
		}
		if got.Location == nil || *got.Location != "NYC" {
			t.Errorf("Location = %v", got.Location)
		}
		if got.Website == nil || *got.Website != "https://example.com" {
			t.Errorf("Website = %v", got.Website)
		}
	})
}

func TestMapGraphQLTOTPStatus(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLTOTPStatus(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		got := mapGraphQLTOTPStatus(&handler.TwoFactorStatusResponse{Enabled: true, RecoveryKeysCount: 5})
		if !got.Enabled || got.RecoveryKeysCount != 5 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestMapGraphQLTOTPSetup(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLTOTPSetup(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		src := &handler.TwoFactorSetupResponse{Secret: "s", QRCode: "q", ManualURL: "m", Account: "a", Issuer: "i"}
		got := mapGraphQLTOTPSetup(src)
		if got.Secret != "s" || got.QrCode != "q" || got.ManualURL != "m" || got.Account != "a" || got.Issuer != "i" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestMapGraphQLRecoveryKeysResponse(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLRecoveryKeysResponse(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("copies recovery keys slice", func(t *testing.T) {
		src := &handler.RecoveryKeysResponse{Message: "ok", RecoveryKeys: []string{"a", "b"}}
		got := mapGraphQLRecoveryKeysResponse(src)
		if got.Message != "ok" || len(got.RecoveryKeys) != 2 || got.RecoveryKeys[0] != "a" || got.RecoveryKeys[1] != "b" {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("nil recovery keys slice yields empty (non-nil append) result", func(t *testing.T) {
		src := &handler.RecoveryKeysResponse{Message: "ok", RecoveryKeys: nil}
		got := mapGraphQLRecoveryKeysResponse(src)
		if len(got.RecoveryKeys) != 0 {
			t.Errorf("RecoveryKeys = %v, want empty", got.RecoveryKeys)
		}
	})
}

func TestMapGraphQLAuthUser(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLAuthUser(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path formats ID as decimal string", func(t *testing.T) {
		src := &handler.AuthUserSummary{ID: 99, Username: "u", Email: "e@example.com", Role: "user"}
		got := mapGraphQLAuthUser(src)
		if got.ID != "99" || got.Username != "u" || got.Email != "e@example.com" || got.Role != "user" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestMapGraphQLAuthLoginResponse(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLAuthLoginResponse(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("blank tokens map to nil pointers", func(t *testing.T) {
		src := &handler.AuthLoginResponse{SessionToken: "  ", Token: ""}
		got := mapGraphQLAuthLoginResponse(src)
		if got.SessionToken != nil || got.Token != nil {
			t.Errorf("got %+v, want nil tokens", got)
		}
	})
	t.Run("non-blank tokens are trimmed and set, and nested user maps", func(t *testing.T) {
		src := &handler.AuthLoginResponse{
			RequiresTwoFactor: true,
			SessionToken:      "  sess-token  ",
			Token:             "  bearer-token  ",
			User:              &handler.AuthUserSummary{ID: 1, Username: "u"},
		}
		got := mapGraphQLAuthLoginResponse(src)
		if got.SessionToken == nil || *got.SessionToken != "sess-token" {
			t.Errorf("SessionToken = %v", got.SessionToken)
		}
		if got.Token == nil || *got.Token != "bearer-token" {
			t.Errorf("Token = %v", got.Token)
		}
		if !got.RequiresTwoFactor {
			t.Error("RequiresTwoFactor = false, want true")
		}
		if got.User == nil || got.User.ID != "1" {
			t.Errorf("User = %+v", got.User)
		}
	})
}

func TestMapGraphQLAuthRegisterResponse(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLAuthRegisterResponse(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("blank token maps to nil pointer", func(t *testing.T) {
		src := &handler.AuthRegisterResponse{Token: "   "}
		got := mapGraphQLAuthRegisterResponse(src)
		if got.Token != nil {
			t.Errorf("Token = %v, want nil", *got.Token)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		src := &handler.AuthRegisterResponse{
			VerificationRequired: true,
			Token:                "  tok  ",
			User:                 &handler.AuthUserSummary{ID: 2, Username: "u2"},
		}
		got := mapGraphQLAuthRegisterResponse(src)
		if !got.VerificationRequired {
			t.Error("VerificationRequired = false, want true")
		}
		if got.Token == nil || *got.Token != "tok" {
			t.Errorf("Token = %v", got.Token)
		}
		if got.User == nil || got.User.ID != "2" {
			t.Errorf("User = %+v", got.User)
		}
	})
}

func TestMapGraphQLUserInviteValidation(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLUserInviteValidation(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		expires := time.Now().Add(time.Hour)
		src := &handler.UserInviteValidationResponse{Username: "u", Email: "e", Role: "user", ExpiresAt: expires}
		got := mapGraphQLUserInviteValidation(src)
		if got.Username != "u" || got.Email != "e" || got.Role != "user" || !got.ExpiresAt.Equal(expires) {
			t.Errorf("got %+v", got)
		}
	})
}

func TestMapGraphQLServerInviteValidation(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLServerInviteValidation(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		expires := time.Now().Add(time.Hour)
		src := &handler.ServerInviteValidationResponse{Email: "e", ExpiresAt: expires}
		got := mapGraphQLServerInviteValidation(src)
		if got.Email != "e" || !got.ExpiresAt.Equal(expires) {
			t.Errorf("got %+v", got)
		}
	})
}

func TestMapGraphQLUserInviteCompletion(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := mapGraphQLUserInviteCompletion(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("blank message and token map to nil pointers", func(t *testing.T) {
		src := &handler.UserInviteCompletionResponse{Message: "  ", Token: ""}
		got := mapGraphQLUserInviteCompletion(src)
		if got.Message != nil || got.Token != nil {
			t.Errorf("got %+v, want nil pointers", got)
		}
	})
	t.Run("happy path with nested user", func(t *testing.T) {
		src := &handler.UserInviteCompletionResponse{
			Message: "  welcome  ",
			Token:   "  tok  ",
			User:    &handler.AuthUserSummary{ID: 3, Username: "u3"},
		}
		got := mapGraphQLUserInviteCompletion(src)
		if got.Message == nil || *got.Message != "welcome" {
			t.Errorf("Message = %v", got.Message)
		}
		if got.Token == nil || *got.Token != "tok" {
			t.Errorf("Token = %v", got.Token)
		}
		if got.User == nil || got.User.ID != "3" {
			t.Errorf("User = %+v", got.User)
		}
	})
}

// TestMapGraphQLServerInviteCompletion covers the distinct nil-response and
// non-nil-response-with-nil-Admin cases (both must return nil).
func TestMapGraphQLServerInviteCompletion(t *testing.T) {
	t.Run("nil response returns nil", func(t *testing.T) {
		if got := mapGraphQLServerInviteCompletion(nil); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("non-nil response with nil Admin also returns nil", func(t *testing.T) {
		src := &handler.ServerInviteCompletionResponse{Message: "hi", Admin: nil}
		if got := mapGraphQLServerInviteCompletion(src); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
	t.Run("happy path with non-nil Admin", func(t *testing.T) {
		src := &handler.ServerInviteCompletionResponse{
			Message: "welcome admin",
			Admin:   &handler.InvitedAdminSummary{ID: 7, Username: "adm", Email: "adm@example.com"},
		}
		got := mapGraphQLServerInviteCompletion(src)
		if got == nil {
			t.Fatal("got nil, want non-nil")
		}
		if got.Message != "welcome admin" {
			t.Errorf("Message = %q", got.Message)
		}
		if got.Admin == nil || got.Admin.ID != "7" || got.Admin.Username != "adm" || got.Admin.Email != "adm@example.com" {
			t.Errorf("Admin = %+v", got.Admin)
		}
	})
}

// --- small pointer test helpers (not exported, test-file only) ---

func boolPtr(b bool) *bool          { return &b }
func float64Ptr(f float64) *float64 { return &f }

// stringPtr is provided by schema.resolvers_impl.go in this package.

func derefBool(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}
func derefFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
func derefString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func ptrBoolEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func ptrFloatEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func ptrStringEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
