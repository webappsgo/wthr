// Package backup - retention config tests per AI.md PART 22 (Backup Retention)
package backup

import "testing"

func TestDefaultRetention(t *testing.T) {
	r := DefaultRetention()
	if r.MaxBackups != 1 {
		t.Errorf("MaxBackups = %d, want 1", r.MaxBackups)
	}
	if r.MaxTotalSize != "10%" {
		t.Errorf("MaxTotalSize = %q, want \"10%%\"", r.MaxTotalSize)
	}
	if r.KeepWeekly != 0 || r.KeepMonthly != 0 || r.KeepYearly != 0 {
		t.Errorf("tiers should default to disabled, got %+v", r)
	}
}

func TestNormalizeSubstitutesDefaults(t *testing.T) {
	tests := []struct {
		name        string
		in          RetentionConfig
		want        RetentionConfig
		wantWarning bool
	}{
		{
			name:        "zero max_backups falls back to 1",
			in:          RetentionConfig{MaxBackups: 0, MaxTotalSize: "10%"},
			want:        RetentionConfig{MaxBackups: 1, MaxTotalSize: "10%"},
			wantWarning: true,
		},
		{
			name:        "negative tiers fall back to 0",
			in:          RetentionConfig{MaxBackups: 1, KeepWeekly: -2, KeepMonthly: -1, KeepYearly: -5, MaxTotalSize: "10%"},
			want:        RetentionConfig{MaxBackups: 1, MaxTotalSize: "10%"},
			wantWarning: true,
		},
		{
			name:        "empty max_total_size falls back to 10%",
			in:          RetentionConfig{MaxBackups: 2},
			want:        RetentionConfig{MaxBackups: 2, MaxTotalSize: "10%"},
			wantWarning: false,
		},
		{
			name:        "valid config is unchanged and warning-free",
			in:          RetentionConfig{MaxBackups: 7, KeepWeekly: 8, KeepMonthly: 12, KeepYearly: 2, MaxTotalSize: "50G"},
			want:        RetentionConfig{MaxBackups: 7, KeepWeekly: 8, KeepMonthly: 12, KeepYearly: 2, MaxTotalSize: "50G"},
			wantWarning: false,
		},
		{
			name:        "above-recommended values warn but are preserved",
			in:          RetentionConfig{MaxBackups: 30, KeepWeekly: 99, KeepMonthly: 99, KeepYearly: 99, MaxTotalSize: "10%"},
			want:        RetentionConfig{MaxBackups: 30, KeepWeekly: 99, KeepMonthly: 99, KeepYearly: 99, MaxTotalSize: "10%"},
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings := tt.in.Normalize()
			if got != tt.want {
				t.Errorf("Normalize() = %+v, want %+v", got, tt.want)
			}
			if tt.wantWarning && len(warnings) == 0 {
				t.Error("expected at least one warning, got none")
			}
			if !tt.wantWarning && len(warnings) != 0 {
				t.Errorf("expected no warnings, got %v", warnings)
			}
		})
	}
}

func TestParseMaxTotalSizeBytes(t *testing.T) {
	const volume = int64(1000) << 30

	tests := []struct {
		name    string
		spec    string
		volume  int64
		want    int64
		wantErr bool
	}{
		{name: "empty disables cap", spec: "", volume: volume, want: 0},
		{name: "zero disables cap", spec: "0", volume: volume, want: 0},
		{name: "false disables cap", spec: "false", volume: volume, want: 0},
		{name: "disabled word disables cap", spec: "Disabled", volume: volume, want: 0},
		{name: "off disables cap", spec: "off", volume: volume, want: 0},
		{name: "ten percent of volume", spec: "10%", volume: volume, want: volume / 10},
		{name: "percent with spaces", spec: " 25 % ", volume: volume, want: volume / 4},
		{name: "percent with unknown volume disables cap", spec: "10%", volume: 0, want: 0},
		{name: "bare bytes", spec: "1024", volume: volume, want: 1024},
		{name: "kilobytes short suffix", spec: "8K", volume: volume, want: 8 << 10},
		{name: "megabytes long suffix", spec: "512MB", volume: volume, want: 512 << 20},
		{name: "gigabytes lowercase", spec: "50g", volume: volume, want: 50 << 30},
		{name: "terabytes", spec: "2TB", volume: volume, want: 2 << 40},
		{name: "fractional gigabytes", spec: "1.5G", volume: volume, want: int64(1.5 * float64(int64(1)<<30))},
		{name: "negative percent is an error", spec: "-5%", volume: volume, wantErr: true},
		{name: "zero percent is an error", spec: "0%", volume: volume, wantErr: true},
		{name: "non-numeric percent is an error", spec: "abc%", volume: volume, wantErr: true},
		{name: "unknown unit is an error", spec: "50XB", volume: volume, wantErr: true},
		{name: "garbage is an error", spec: "not-a-size", volume: volume, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMaxTotalSizeBytes(tt.spec, tt.volume)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMaxTotalSizeBytes(%q) = %d, want error", tt.spec, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMaxTotalSizeBytes(%q) unexpected error: %v", tt.spec, err)
			}
			if got != tt.want {
				t.Errorf("ParseMaxTotalSizeBytes(%q) = %d, want %d", tt.spec, got, tt.want)
			}
		})
	}
}
