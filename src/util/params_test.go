package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newParamsTestContext(rawQuery string, headers map[string]string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	target := "/"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

// TestParseQueryParams_Defaults verifies the zero-query default values.
func TestParseQueryParams_Defaults(t *testing.T) {
	c := newParamsTestContext("", nil)
	p := ParseQueryParams(c)

	if p.Format != 0 {
		t.Errorf("Format = %d, want 0", p.Format)
	}
	if p.Units != "auto" {
		t.Errorf("Units = %q, want auto", p.Units)
	}
	if p.Days != 3 {
		t.Errorf("Days = %d, want 3", p.Days)
	}
	if p.Width != 0 {
		t.Errorf("Width = %d, want 0", p.Width)
	}
	if p.NoColors || p.NoFooter || p.Quiet || p.SuperQuiet || p.Narrow || p.ForceANSI {
		t.Error("all boolean flags should default to false")
	}
}

// TestParseQueryParams_UnitFlags covers unit-selecting query flags, in
// priority order (u > m > M > units=).
func TestParseQueryParams_UnitFlags(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"imperial_flag", "u", "imperial"},
		{"metric_flag_m", "m", "metric"},
		{"metric_flag_M", "M", "metric"},
		{"units_param", "units=metric", "metric"},
		{"u_beats_m", "u&m", "imperial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newParamsTestContext(tt.query, nil)
			p := ParseQueryParams(c)
			if p.Units != tt.want {
				t.Errorf("Units = %q, want %q", p.Units, tt.want)
			}
		})
	}
}

// TestParseQueryParams_StyleFlags covers the boolean presence-only flags.
func TestParseQueryParams_StyleFlags(t *testing.T) {
	tests := []struct {
		name  string
		query string
		check func(*RenderParams) bool
	}{
		{"no_footer", "F", func(p *RenderParams) bool { return p.NoFooter }},
		{"narrow", "n", func(p *RenderParams) bool { return p.Narrow }},
		{"quiet", "q", func(p *RenderParams) bool { return p.Quiet }},
		{"super_quiet", "Q", func(p *RenderParams) bool { return p.SuperQuiet }},
		{"no_colors", "T", func(p *RenderParams) bool { return p.NoColors }},
		{"force_ansi", "A", func(p *RenderParams) bool { return p.ForceANSI }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newParamsTestContext(tt.query, nil)
			p := ParseQueryParams(c)
			if !tt.check(p) {
				t.Errorf("flag from query %q not set", tt.query)
			}
		})
	}
}

// TestParseQueryParams_CombinedFlags covers a single combined-flag query
// key like "?Tqn" applying multiple flags at once.
func TestParseQueryParams_CombinedFlags(t *testing.T) {
	c := newParamsTestContext("Tqn", nil)
	p := ParseQueryParams(c)
	if !p.NoColors {
		t.Error("combined flag Tqn: NoColors not set")
	}
	if !p.Quiet {
		t.Error("combined flag Tqn: Quiet not set")
	}
	if !p.Narrow {
		t.Error("combined flag Tqn: Narrow not set")
	}
}

// TestParseQueryParams_Format covers format values 0-4 and their
// NoColors side effect (formats 1-4 force plain text).
func TestParseQueryParams_Format(t *testing.T) {
	tests := []struct {
		format       string
		wantFormat   int
		wantNoColors bool
	}{
		{"0", 0, false},
		{"1", 1, true},
		{"2", 2, true},
		{"3", 3, true},
		{"4", 4, true},
		{"99", 0, false},
	}
	for _, tt := range tests {
		t.Run("format_"+tt.format, func(t *testing.T) {
			c := newParamsTestContext("format="+tt.format, nil)
			p := ParseQueryParams(c)
			if p.Format != tt.wantFormat {
				t.Errorf("Format = %d, want %d", p.Format, tt.wantFormat)
			}
			if p.NoColors != tt.wantNoColors {
				t.Errorf("NoColors = %v, want %v", p.NoColors, tt.wantNoColors)
			}
		})
	}
}

// TestParseQueryParams_Days covers the [0, MaxForecastDays] clamping,
// including negative and over-max values.
func TestParseQueryParams_Days(t *testing.T) {
	tests := []struct {
		name string
		days string
		want int
	}{
		{"default_unspecified", "", 3},
		{"zero", "0", 0},
		{"negative_clamped_to_zero", "-5", 0},
		{"within_range", "7", 7},
		{"max", "16", 16},
		{"over_max_clamped", "100", 16},
		{"non_numeric_treated_as_zero", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := ""
			if tt.days != "" {
				query = "days=" + tt.days
			}
			c := newParamsTestContext(query, nil)
			p := ParseQueryParams(c)
			if p.Days != tt.want {
				t.Errorf("Days = %d, want %d", p.Days, tt.want)
			}
		})
	}
}

// TestParseQueryParams_AcceptHeaderTextPlain verifies the Accept header
// also disables colors, independent of query params.
func TestParseQueryParams_AcceptHeaderTextPlain(t *testing.T) {
	c := newParamsTestContext("", map[string]string{"Accept": "text/plain"})
	p := ParseQueryParams(c)
	if !p.NoColors {
		t.Error("Accept: text/plain should force NoColors = true")
	}
}

// TestParseQueryParams_Language verifies the lang query param overrides
// the "en" default.
func TestParseQueryParams_Language(t *testing.T) {
	c := newParamsTestContext("lang=es", nil)
	p := ParseQueryParams(c)
	if p.Language != "es" {
		t.Errorf("Language = %q, want es", p.Language)
	}
}

// TestParseQueryParams_Width covers width, cols, and User-Agent
// COLUMNS= parsing, in priority order.
func TestParseQueryParams_Width(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		headers map[string]string
		want    int
	}{
		{"width_param", "width=120", nil, 120},
		{"cols_param", "cols=100", nil, 100},
		{"width_beats_cols", "width=120&cols=100", nil, 120},
		{"user_agent_columns", "", map[string]string{"User-Agent": "curl/8.0 (COLUMNS=90)"}, 90},
		{"no_width_source", "", nil, 0},
		{"zero_width_ignored", "width=0", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newParamsTestContext(tt.query, tt.headers)
			p := ParseQueryParams(c)
			if p.Width != tt.want {
				t.Errorf("Width = %d, want %d", p.Width, tt.want)
			}
		})
	}
}

// TestParseIntSafe covers valid, invalid, and empty inputs.
func TestParseIntSafe(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"positive", "42", 42},
		{"negative", "-7", -7},
		{"zero", "0", 0},
		{"empty", "", 0},
		{"non_numeric", "abc", 0},
		{"leading_numeric", "12abc", 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIntSafe(tt.in); got != tt.want {
				t.Errorf("parseIntSafe(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestGetUnits covers explicit unit override and country-based auto-detect.
func TestGetUnits(t *testing.T) {
	tests := []struct {
		name        string
		units       string
		countryCode string
		want        string
	}{
		{"explicit_imperial", "imperial", "GB", "imperial"},
		{"explicit_metric", "metric", "US", "metric"},
		{"auto_us_is_imperial", "auto", "US", "imperial"},
		{"auto_non_us_is_metric", "auto", "GB", "metric"},
		{"auto_empty_country_is_metric", "auto", "", "metric"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RenderParams{Units: tt.units}
			if got := GetUnits(p, tt.countryCode); got != tt.want {
				t.Errorf("GetUnits() = %q, want %q", got, tt.want)
			}
		})
	}
}
