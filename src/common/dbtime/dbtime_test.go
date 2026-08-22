package dbtime

import (
	"testing"
	"time"
)

// zoneWest and zoneEast are deliberately extreme fixed offsets. A timestamp
// rendered in either zone has wall-clock text that disagrees with its true
// instant by 11 or 13 hours, so a test built on them cannot accidentally pass
// because the host happens to run in UTC. Both names are short, all-uppercase
// and digit-free because the "MST" element of localLayout reads back only such
// names: a digit-carrying or over-long name would make every fixture below
// unparseable and silently reduce these cases to the fail-closed path.
var (
	zoneWest = time.FixedZone("WST", -11*60*60)
	zoneEast = time.FixedZone("EAT", 13*60*60)
)

// localLayout mirrors the layout time.Time.String() emits, which is what
// modernc.org/sqlite writes when a Go time.Time is bound directly.
const localLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// reference is the instant every fixture in this file is positioned around.
var reference = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func TestFormatSQLTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  string
	}{
		{
			name:  "already utc",
			input: reference,
			want:  "2025-01-01 12:00:00",
		},
		{
			name:  "west of utc is converted, not truncated",
			input: reference.In(zoneWest),
			want:  "2025-01-01 12:00:00",
		},
		{
			name:  "east of utc is converted, not truncated",
			input: reference.In(zoneEast),
			want:  "2025-01-01 12:00:00",
		},
		{
			name:  "sub-second precision is dropped",
			input: reference.Add(500 * time.Millisecond),
			want:  "2025-01-01 12:00:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatSQLTimestamp(tt.input); got != tt.want {
				t.Fatalf("FormatSQLTimestamp(%s) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseStoredTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		stored interface{}
		want   time.Time
		wantOK bool
	}{
		{
			name:   "canonical utc text",
			stored: "2025-01-01 12:00:00",
			want:   reference,
			wantOK: true,
		},
		{
			name:   "canonical utc bytes",
			stored: []byte("2025-01-01 12:00:00"),
			want:   reference,
			wantOK: true,
		},
		{
			name:   "west local zone string layout",
			stored: reference.In(zoneWest).Format(localLayout),
			want:   reference,
			wantOK: true,
		},
		{
			name:   "east local zone string layout",
			stored: reference.In(zoneEast).Format(localLayout),
			want:   reference,
			wantOK: true,
		},
		{
			name:   "local zone string layout with monotonic suffix",
			stored: reference.In(zoneWest).Format(localLayout) + " m=+0.000000001",
			want:   reference,
			wantOK: true,
		},
		{
			name:   "rfc3339",
			stored: "2025-01-01T12:00:00Z",
			want:   reference,
			wantOK: true,
		},
		{
			name:   "driver returned time in a non-utc zone",
			stored: reference.In(zoneEast),
			want:   reference,
			wantOK: true,
		},
		{
			name:   "null",
			stored: nil,
			wantOK: false,
		},
		{
			name:   "blank",
			stored: "   ",
			wantOK: false,
		},
		{
			name:   "unparseable",
			stored: "not-a-timestamp",
			wantOK: false,
		},
		{
			name:   "unsupported type",
			stored: 12345,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseStoredTimestamp(tt.stored)
			if ok != tt.wantOK {
				t.Fatalf("ParseStoredTimestamp(%v) ok = %v, want %v", tt.stored, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseStoredTimestamp(%v) = %s, want %s", tt.stored, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("ParseStoredTimestamp(%v) location = %s, want UTC", tt.stored, got.Location())
			}
		})
	}
}

func TestNormalizeScannedRowID(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{
			name:  "text primary key arrives as bytes",
			input: []byte("01HQ8Z0000000000000000000"),
			want:  "01HQ8Z0000000000000000000",
		},
		{
			name:  "integer primary key is untouched",
			input: int64(42),
			want:  int64(42),
		},
		{
			name:  "string primary key is untouched",
			input: "session-token",
			want:  "session-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeScannedRowID(tt.input); got != tt.want {
				t.Fatalf("NormalizeScannedRowID(%v) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsAfter checks the comparison used by every "is this row still valid"
// filter. The zone fixtures matter: a row one hour in the future written in the
// -11h zone has wall-clock text ten hours in the past, and a row one hour in
// the past written in the +13h zone has wall-clock text twelve hours in the
// future, so a comparison that looks at the text instead of the instant gets
// both cases exactly backwards.
func TestIsAfter(t *testing.T) {
	tests := []struct {
		name      string
		stored    interface{}
		threshold time.Time
		want      bool
	}{
		{
			name:      "future utc",
			stored:    FormatSQLTimestamp(reference.Add(time.Hour)),
			threshold: reference,
			want:      true,
		},
		{
			name:      "past utc",
			stored:    FormatSQLTimestamp(reference.Add(-time.Hour)),
			threshold: reference,
			want:      false,
		},
		{
			name:      "future instant whose west-zone text reads as past",
			stored:    reference.Add(time.Hour).In(zoneWest).Format(localLayout),
			threshold: reference,
			want:      true,
		},
		{
			name:      "past instant whose east-zone text reads as future",
			stored:    reference.Add(-time.Hour).In(zoneEast).Format(localLayout),
			threshold: reference,
			want:      false,
		},
		{
			name:      "exact threshold is not after",
			stored:    FormatSQLTimestamp(reference),
			threshold: reference,
			want:      false,
		},
		{
			name:      "unparseable is never after",
			stored:    "not-a-timestamp",
			threshold: reference,
			want:      false,
		},
		{
			name:      "null is never after",
			stored:    nil,
			threshold: reference,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAfter(tt.stored, tt.threshold); got != tt.want {
				t.Fatalf("IsAfter(%v, %s) = %v, want %v", tt.stored, tt.threshold, got, tt.want)
			}
		})
	}
}
