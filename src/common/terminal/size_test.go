package terminal

import (
	"testing"
)

func TestCalculateMode(t *testing.T) {
	tests := []struct {
		name string
		cols int
		rows int
		want SizeMode
	}{
		// Micro / Minimal boundary at cols=40, rows=10
		{"cols just below micro/minimal boundary (39)", 39, 24, SizeModeMicro},
		{"cols exactly at minimal boundary (40)", 40, 24, SizeModeMinimal},
		{"rows just below micro/minimal boundary (9)", 80, 9, SizeModeMicro},
		{"rows exactly at minimal boundary (10)", 80, 10, SizeModeMinimal},

		// Minimal / Compact boundary at cols=60, rows=16
		{"cols just below compact boundary (59)", 59, 24, SizeModeMinimal},
		{"cols exactly at compact boundary (60)", 60, 24, SizeModeCompact},
		{"rows just below compact boundary (15)", 80, 15, SizeModeMinimal},
		{"rows exactly at compact boundary (16)", 80, 16, SizeModeCompact},

		// Compact / Standard boundary at cols=80, rows=24
		{"cols just below standard boundary (79)", 79, 24, SizeModeCompact},
		{"cols exactly at standard boundary (80)", 80, 24, SizeModeStandard},
		{"rows just below standard boundary (23)", 80, 23, SizeModeCompact},
		{"rows exactly at standard boundary (24)", 80, 24, SizeModeStandard},

		// Standard / Wide boundary at cols=120, rows=40
		{"cols just below wide boundary (119)", 119, 40, SizeModeStandard},
		{"cols exactly at wide boundary (120)", 120, 40, SizeModeWide},
		{"rows just below wide boundary (39)", 120, 39, SizeModeStandard},
		{"rows exactly at wide boundary (40)", 120, 40, SizeModeWide},

		// Wide / Ultrawide boundary at cols=200, rows=60
		{"cols just below ultrawide boundary (199)", 199, 60, SizeModeWide},
		{"cols exactly at ultrawide boundary (200)", 200, 60, SizeModeUltrawide},
		{"rows just below ultrawide boundary (59)", 200, 59, SizeModeWide},
		{"rows exactly at ultrawide boundary (60)", 200, 60, SizeModeUltrawide},

		// Ultrawide / Massive boundary at cols=400, rows=80
		{"cols just below massive boundary (399)", 399, 80, SizeModeUltrawide},
		{"cols exactly at massive boundary (400)", 400, 80, SizeModeMassive},
		{"rows just below massive boundary (79)", 400, 79, SizeModeUltrawide},
		{"rows exactly at massive boundary (80)", 400, 80, SizeModeMassive},

		// OR-based interaction: the smaller dimension's category dominates.
		{"wide but short forces micro (rows dominate)", 300, 5, SizeModeMicro},
		{"narrow but tall forces micro (cols dominate)", 30, 100, SizeModeMicro},
		{"wide but minimal rows forces minimal", 300, 12, SizeModeMinimal},
		{"minimal cols but tall rows forces minimal", 45, 200, SizeModeMinimal},

		// Absolute extremes
		{"zero cols zero rows", 0, 0, SizeModeMicro},
		{"huge cols huge rows", 1000, 1000, SizeModeMassive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMode(tt.cols, tt.rows)
			if got != tt.want {
				t.Errorf("calculateMode(%d, %d) = %v, want %v", tt.cols, tt.rows, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowASCIIArt(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, false},
		{SizeModeStandard, true},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
		{SizeModeMassive, true},
	}
	for _, tt := range tests {
		if got := tt.mode.ShowASCIIArt(); got != tt.want {
			t.Errorf("SizeMode(%d).ShowASCIIArt() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestSizeModeShowBorders(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, true},
		{SizeModeStandard, true},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
		{SizeModeMassive, true},
	}
	for _, tt := range tests {
		if got := tt.mode.ShowBorders(); got != tt.want {
			t.Errorf("SizeMode(%d).ShowBorders() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestSizeModeShowSidebar(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, false},
		{SizeModeStandard, false},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
		{SizeModeMassive, true},
	}
	for _, tt := range tests {
		if got := tt.mode.ShowSidebar(); got != tt.want {
			t.Errorf("SizeMode(%d).ShowSidebar() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestSizeModeShowIcons(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, true},
		{SizeModeCompact, true},
		{SizeModeStandard, true},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
		{SizeModeMassive, true},
	}
	for _, tt := range tests {
		if got := tt.mode.ShowIcons(); got != tt.want {
			t.Errorf("SizeMode(%d).ShowIcons() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestSizeModeMaxTableColumns(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want int
	}{
		{SizeModeMicro, 2},
		{SizeModeMinimal, 3},
		{SizeModeCompact, 4},
		{SizeModeStandard, 6},
		{SizeModeWide, 10},
		{SizeModeUltrawide, 10},
		{SizeModeMassive, 10},
	}
	for _, tt := range tests {
		if got := tt.mode.MaxTableColumns(); got != tt.want {
			t.Errorf("SizeMode(%d).MaxTableColumns() = %d, want %d", tt.mode, got, tt.want)
		}
	}
}

// TestGetTerminalSize verifies the fallback path: in Docker/CI, os.Stdout is
// not a TTY, so term.GetSize errors and GetTerminalSize falls back to the
// documented 80x24 default, resolving to SizeModeStandard.
func TestGetTerminalSize(t *testing.T) {
	size := GetTerminalSize()

	if size.Cols == 0 || size.Rows == 0 {
		t.Fatalf("GetTerminalSize() returned zero dimensions: %+v", size)
	}

	// When not a TTY (the expected case in this test environment) the
	// fallback is deterministic: 80x24 -> SizeModeStandard.
	if size.Cols == 80 && size.Rows == 24 {
		if size.Mode != SizeModeStandard {
			t.Errorf("GetTerminalSize() fallback Mode = %v, want SizeModeStandard", size.Mode)
		}
	}

	wantMode := calculateMode(size.Cols, size.Rows)
	if size.Mode != wantMode {
		t.Errorf("GetTerminalSize() Mode = %v, want calculateMode(%d,%d) = %v", size.Mode, size.Cols, size.Rows, wantMode)
	}
}
