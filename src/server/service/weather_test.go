package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

// newTestWeatherService builds a WeatherService with cache initialized and no
// network-touching dependencies, suitable for pure-logic tests.
func newTestWeatherService() *WeatherService {
	return &WeatherService{
		client:           http.DefaultClient,
		cache:            cache.New(5*time.Minute, 10*time.Minute),
		openMeteoBaseURL: "https://api.open-meteo.com/v1",
		geocodingURL:     "https://geocoding-api.open-meteo.com/v1",
	}
}

func TestWeatherService_isCoordinates(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name     string
		location string
		want     bool
	}{
		{"simple pair", "40.7128,-74.0060", true},
		{"spaced pair", "40.7128, -74.0060", true},
		{"negative both", "-33.8688,-151.2093", true},
		{"integer pair", "40,-74", true},
		{"city name", "New York", false},
		{"empty", "", false},
		{"partial number", "40.7128,", false},
		{"trailing text", "40.7128,-74.0060 NYC", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.isCoordinates(tt.location); got != tt.want {
				t.Errorf("isCoordinates(%q) = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

func TestWeatherService_isValidLocationInput(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name     string
		location string
		want     bool
	}{
		{"empty", "", false},
		{"normal city", "New York", true},
		{"starts with digit", "5th Avenue", true},
		{"only special chars", "!!!", false},
		{"starts with special char", "!New York", false},
		{"single letter", "a", true},
		{"whitespace only", "   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.isValidLocationInput(tt.location); got != tt.want {
				t.Errorf("isValidLocationInput(%q) = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

func TestWeatherService_parseLocationString(t *testing.T) {
	ws := newTestWeatherService()

	t.Run("empty string", func(t *testing.T) {
		got := ws.parseLocationString("")
		if got.City != "" || got.State != "" || got.Country != "" || got.HasStateOrCountry {
			t.Errorf("parseLocationString(\"\") = %+v, want zero-value struct", got)
		}
		if len(got.OriginalParts) != 0 {
			t.Errorf("OriginalParts = %v, want empty", got.OriginalParts)
		}
	})

	t.Run("city only", func(t *testing.T) {
		got := ws.parseLocationString("Chicago")
		if got.City != "Chicago" || got.HasStateOrCountry {
			t.Errorf("parseLocationString(\"Chicago\") = %+v", got)
		}
	})

	t.Run("city, state", func(t *testing.T) {
		got := ws.parseLocationString("Chicago, IL")
		if got.City != "Chicago" || got.State != "IL" || got.Country != "IL" || !got.HasStateOrCountry {
			t.Errorf("parseLocationString(\"Chicago, IL\") = %+v", got)
		}
	})

	t.Run("city, state, country", func(t *testing.T) {
		got := ws.parseLocationString("Chicago, IL, US")
		if got.City != "Chicago" || got.State != "IL" || got.Country != "US" || !got.HasStateOrCountry {
			t.Errorf("parseLocationString(\"Chicago, IL, US\") = %+v", got)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got := ws.parseLocationString(" Chicago ,  IL ")
		if got.City != "Chicago" || got.State != "IL" {
			t.Errorf("parseLocationString did not trim: %+v", got)
		}
	})
}

func TestWeatherService_matchesStateAbbreviation(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name        string
		input       string
		admin1      string
		countryCode string
		want        bool
	}{
		{"exact match", "il", "Illinois", "US", true},
		{"case insensitive country", "il", "Illinois", "us", false}, // countryCode must be "US" exactly
		{"non-US country", "il", "Illinois", "CA", false},
		{"unknown abbreviation", "zz", "Illinois", "US", false},
		{"mismatched state", "ca", "Illinois", "US", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.matchesStateAbbreviation(tt.input, tt.admin1, tt.countryCode); got != tt.want {
				t.Errorf("matchesStateAbbreviation(%q,%q,%q) = %v, want %v", tt.input, tt.admin1, tt.countryCode, got, tt.want)
			}
		})
	}
}

func TestWeatherService_matchesCanadianProvince(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name        string
		input       string
		admin1      string
		countryCode string
		want        bool
	}{
		{"exact match", "on", "Ontario", "CA", true},
		{"non-CA country", "on", "Ontario", "US", false},
		{"unknown abbreviation", "zz", "Ontario", "CA", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.matchesCanadianProvince(tt.input, tt.admin1, tt.countryCode); got != tt.want {
				t.Errorf("matchesCanadianProvince(%q,%q,%q) = %v, want %v", tt.input, tt.admin1, tt.countryCode, got, tt.want)
			}
		})
	}
}

func TestWeatherService_selectBestLocationMatch(t *testing.T) {
	ws := newTestWeatherService()

	t.Run("single result short-circuit", func(t *testing.T) {
		results := []GeocodeResult{{Name: "Springfield", CountryCode: "US"}}
		got := ws.selectBestLocationMatch(results, &LocationParts{}, "")
		if got != &results[0] {
			t.Error("expected single result to be returned directly")
		}
	})

	t.Run("matches state abbreviation", func(t *testing.T) {
		results := []GeocodeResult{
			{Name: "Springfield", Admin1: "Illinois", CountryCode: "US"},
			{Name: "Springfield", Admin1: "Missouri", CountryCode: "US"},
		}
		lp := &LocationParts{HasStateOrCountry: true, State: "mo"}
		got := ws.selectBestLocationMatch(results, lp, "")
		if got.Admin1 != "Missouri" {
			t.Errorf("expected Missouri match, got %+v", got)
		}
	})

	t.Run("falls back to userCountry", func(t *testing.T) {
		results := []GeocodeResult{
			{Name: "Springfield", Country: "United States", CountryCode: "US"},
			{Name: "Springfield", Country: "Canada", CountryCode: "CA"},
		}
		lp := &LocationParts{}
		got := ws.selectBestLocationMatch(results, lp, "CA")
		if got.CountryCode != "CA" {
			t.Errorf("expected CA match via userCountry, got %+v", got)
		}
	})

	t.Run("defaults to first result", func(t *testing.T) {
		results := []GeocodeResult{
			{Name: "First", CountryCode: "FR"},
			{Name: "Second", CountryCode: "DE"},
		}
		got := ws.selectBestLocationMatch(results, &LocationParts{}, "")
		if got.Name != "First" {
			t.Errorf("expected default first result, got %+v", got)
		}
	})
}

func TestWeatherService_buildFullLocationName(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name   string
		result *GeocodeResult
		want   string
	}{
		{"all parts", &GeocodeResult{Name: "Chicago", Admin1: "Illinois", Country: "United States"}, "Chicago, Illinois, United States"},
		{"admin1 equals name", &GeocodeResult{Name: "Chicago", Admin1: "Chicago", Country: "United States"}, "Chicago, United States"},
		{"no admin1", &GeocodeResult{Name: "Paris", Country: "France"}, "Paris, France"},
		{"name only", &GeocodeResult{Name: "Atlantis"}, "Atlantis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.buildFullLocationName(tt.result); got != tt.want {
				t.Errorf("buildFullLocationName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeatherService_buildShortLocationName(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name   string
		result *GeocodeResult
		want   string
	}{
		{"US with state", &GeocodeResult{Name: "Chicago", Admin1: "Illinois", CountryCode: "us"}, "Chicago, IL"},
		{"US unknown state falls to code", &GeocodeResult{Name: "Chicago", Admin1: "Nowhere", CountryCode: "us"}, "Chicago, US"},
		{"non-US uses country code", &GeocodeResult{Name: "Paris", CountryCode: "fr"}, "Paris, FR"},
		{"empty country code", &GeocodeResult{Name: "Atlantis"}, "Atlantis, XX"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.buildShortLocationName(tt.result); got != tt.want {
				t.Errorf("buildShortLocationName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeatherService_getStateAbbreviation(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name  string
		state string
		want  string
	}{
		{"exact", "California", "CA"},
		{"case insensitive", "california", "CA"},
		{"whitespace", " Texas ", "TX"},
		{"unknown", "Ontario", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.getStateAbbreviation(tt.state); got != tt.want {
				t.Errorf("getStateAbbreviation(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestWeatherService_UnitConversions(t *testing.T) {
	ws := newTestWeatherService()

	if got, want := ws.celsiusToFahrenheit(0), 32.0; got != want {
		t.Errorf("celsiusToFahrenheit(0) = %v, want %v", got, want)
	}
	if got, want := ws.celsiusToFahrenheit(100), 212.0; got != want {
		t.Errorf("celsiusToFahrenheit(100) = %v, want %v", got, want)
	}
	if got, want := ws.kmhToMph(100), 62.1371; got < want-0.001 || got > want+0.001 {
		t.Errorf("kmhToMph(100) = %v, want ~%v", got, want)
	}
	if got, want := ws.hpaToInhg(1013.25), 29.9214225; got < want-0.001 || got > want+0.001 {
		t.Errorf("hpaToInhg(1013.25) = %v, want ~%v", got, want)
	}
	if got, want := ws.mmToInches(25.4), 1.0000; got < want-0.01 || got > want+0.01 {
		t.Errorf("mmToInches(25.4) = %v, want ~%v", got, want)
	}
	if got, want := ws.metersToMiles(1609.34), 1.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("metersToMiles(1609.34) = %v, want ~%v", got, want)
	}
}

func TestWeatherService_convertWeatherUnits(t *testing.T) {
	ws := newTestWeatherService()
	base := &CurrentWeather{Temperature: 20, FeelsLike: 18, Pressure: 1000, WindSpeed: 10, WindGusts: 15, Precipitation: 5}

	t.Run("metric unchanged", func(t *testing.T) {
		got := ws.convertWeatherUnits(base, "metric")
		if got != base {
			t.Error("metric conversion should return the same pointer unchanged")
		}
	})

	t.Run("imperial converts", func(t *testing.T) {
		got := ws.convertWeatherUnits(base, "imperial")
		if got.Temperature != ws.celsiusToFahrenheit(20) {
			t.Errorf("Temperature = %v, want %v", got.Temperature, ws.celsiusToFahrenheit(20))
		}
		if got.WindSpeed != ws.kmhToMph(10) {
			t.Errorf("WindSpeed = %v, want %v", got.WindSpeed, ws.kmhToMph(10))
		}
		// original must be untouched
		if base.Temperature != 20 {
			t.Errorf("original weather mutated: %v", base.Temperature)
		}
	})
}

func TestWeatherService_convertForecastUnits(t *testing.T) {
	ws := newTestWeatherService()
	forecast := &Forecast{
		Days: []ForecastDay{
			{TempMax: 30, TempMin: 10, Hourly: []ForecastHour{{Temperature: 20, Visibility: 10000}}},
		},
	}

	t.Run("metric unchanged", func(t *testing.T) {
		got := ws.convertForecastUnits(forecast, "metric")
		if got != forecast {
			t.Error("metric conversion should return the same pointer")
		}
	})

	t.Run("imperial converts days and hourly", func(t *testing.T) {
		got := ws.convertForecastUnits(forecast, "imperial")
		if got.Days[0].TempMax != ws.celsiusToFahrenheit(30) {
			t.Errorf("TempMax = %v", got.Days[0].TempMax)
		}
		if got.Days[0].Hourly[0].Temperature != ws.celsiusToFahrenheit(20) {
			t.Errorf("Hourly Temperature = %v", got.Days[0].Hourly[0].Temperature)
		}
		if got.Days[0].Hourly[0].Visibility != ws.metersToMiles(10000) {
			t.Errorf("Hourly Visibility = %v", got.Days[0].Hourly[0].Visibility)
		}
		// original untouched
		if forecast.Days[0].TempMax != 30 {
			t.Error("original forecast mutated")
		}
	})
}

func TestWeatherService_GetWeatherDescription(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		code int
		want string
	}{
		{0, "Clear sky"},
		{95, "Thunderstorm"},
		{99, "Thunderstorm with heavy hail"},
		{-1, "Unknown"},
		{1000, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			if got := ws.GetWeatherDescription(tt.code); got != tt.want {
				t.Errorf("GetWeatherDescription(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestWeatherService_GetWeatherIcon(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name  string
		code  int
		isDay bool
		want  string
	}{
		{"clear day", 0, true, "☀️"},
		{"clear night", 0, false, "🌙"},
		{"mainly clear day", 1, true, "🌤️"},
		{"mainly clear night", 1, false, "🌙"},
		{"partly cloudy day", 2, true, "⛅"},
		{"partly cloudy night", 2, false, "☁️"},
		{"thunderstorm", 95, true, "⛈️"},
		{"unknown code", 12345, true, "❓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.GetWeatherIcon(tt.code, tt.isDay); got != tt.want {
				t.Errorf("GetWeatherIcon(%d,%v) = %q, want %q", tt.code, tt.isDay, got, tt.want)
			}
		})
	}
}

func TestWeatherService_isLocalIP(t *testing.T) {
	ws := newTestWeatherService()
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"invalid IP", "not-an-ip", true},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"private 10.x", "10.0.0.5", true},
		{"private 172.16.x", "172.16.5.5", true},
		{"private 192.168.x", "192.168.1.1", true},
		{"link-local v6", "fe80::1", true},
		{"unique-local v6", "fd12:3456:789a::1", true},
		{"public v4", "8.8.8.8", false},
		{"public v6", "2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ws.isLocalIP(tt.ip); got != tt.want {
				t.Errorf("isLocalIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestSafeFloatAt(t *testing.T) {
	slice := []float64{1.1, 2.2, 3.3}
	tests := []struct {
		name  string
		index int
		want  float64
	}{
		{"valid middle", 1, 2.2},
		{"first", 0, 1.1},
		{"last", 2, 3.3},
		{"negative", -1, 0},
		{"out of range", 3, 0},
		{"way out of range", 100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeFloatAt(slice, tt.index); got != tt.want {
				t.Errorf("safeFloatAt(slice,%d) = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
	if got := safeFloatAt([]float64{}, 0); got != 0 {
		t.Errorf("safeFloatAt(empty,0) = %v, want 0", got)
	}
}

func TestSafeIntAt(t *testing.T) {
	slice := []int{10, 20, 30}
	if got := safeIntAt(slice, 1); got != 20 {
		t.Errorf("safeIntAt(slice,1) = %v, want 20", got)
	}
	if got := safeIntAt(slice, -1); got != 0 {
		t.Errorf("safeIntAt(slice,-1) = %v, want 0", got)
	}
	if got := safeIntAt(slice, 3); got != 0 {
		t.Errorf("safeIntAt(slice,3) = %v, want 0", got)
	}
	if got := safeIntAt([]int{}, 0); got != 0 {
		t.Errorf("safeIntAt(empty,0) = %v, want 0", got)
	}
}

func TestCalculateHistoricalStats(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		got := calculateHistoricalStats(nil)
		want := HistoricalStats{}
		if got != want {
			t.Errorf("calculateHistoricalStats(nil) = %+v, want zero value", got)
		}
	})

	t.Run("single year", func(t *testing.T) {
		years := []HistoricalDay{
			{Year: 2020, TempMax: 30, TempMin: 10, Precipitation: 5},
		}
		got := calculateHistoricalStats(years)
		if got.WarmestYear != 2020 || got.ColdestYear != 2020 || got.WettestYear != 2020 || got.DriestYear != 2020 {
			t.Errorf("single-year stats = %+v", got)
		}
		if got.MaxTempEver != 30 || got.MinTempEver != 10 {
			t.Errorf("record temps = %+v", got)
		}
		if got.AvgTempMax != 30 || got.AvgTempMin != 10 || got.AvgPrecipitation != 5 {
			t.Errorf("averages = %+v", got)
		}
		if got.TotalYears != 1 {
			t.Errorf("TotalYears = %d, want 1", got.TotalYears)
		}
	})

	t.Run("multiple years tracks warmest coldest wettest", func(t *testing.T) {
		years := []HistoricalDay{
			{Year: 2018, TempMax: 25, TempMin: 5, Precipitation: 2},
			{Year: 2019, TempMax: 35, TempMin: -5, Precipitation: 10},
			{Year: 2020, TempMax: 20, TempMin: 15, Precipitation: 1},
		}
		got := calculateHistoricalStats(years)
		if got.WarmestYear != 2019 {
			t.Errorf("WarmestYear = %d, want 2019", got.WarmestYear)
		}
		if got.ColdestYear != 2019 {
			t.Errorf("ColdestYear = %d, want 2019", got.ColdestYear)
		}
		if got.WettestYear != 2019 {
			t.Errorf("WettestYear = %d, want 2019", got.WettestYear)
		}
		if got.MaxTempEver != 35 || got.MinTempEver != -5 || got.MaxPrecipitation != 10 {
			t.Errorf("records = %+v", got)
		}
		if got.TotalYears != 3 {
			t.Errorf("TotalYears = %d, want 3", got.TotalYears)
		}
	})
}

func TestWeatherService_GetCoordinates_Validation(t *testing.T) {
	ws := newTestWeatherService()

	t.Run("empty location errors", func(t *testing.T) {
		_, err := ws.GetCoordinates("", "")
		if err == nil {
			t.Fatal("expected error for empty location")
		}
	})

	t.Run("whitespace-only location errors", func(t *testing.T) {
		_, err := ws.GetCoordinates("   ", "")
		if err == nil {
			t.Fatal("expected error for whitespace-only location")
		}
	})

	t.Run("invalid format errors", func(t *testing.T) {
		_, err := ws.GetCoordinates("!!!", "")
		if err == nil {
			t.Fatal("expected error for invalid location format")
		}
	})
}

func TestWeatherService_GetHistoricalWeather_Validation(t *testing.T) {
	ws := newTestWeatherService()

	tests := []struct {
		name                                 string
		month, day, startYear, numberOfYears int
		wantErrSubstr                        string
	}{
		{"month too low", 0, 15, 2020, 5, "invalid month"},
		{"month too high", 13, 15, 2020, 5, "invalid month"},
		{"day too low", 6, 0, 2020, 5, "invalid day"},
		{"day too high", 6, 32, 2020, 5, "invalid day"},
		{"start year in future", 6, 15, time.Now().Year() + 1, 5, "cannot be in the future"},
		{"years too low", 6, 15, 2020, 0, "invalid number of years"},
		{"years too high", 6, 15, 2020, 101, "invalid number of years"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ws.GetHistoricalWeather(40.0, -74.0, tt.month, tt.day, tt.startYear, tt.numberOfYears)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}

func TestWeatherService_LookupIP_NilService(t *testing.T) {
	ws := newTestWeatherService()
	_, err := ws.LookupIP("8.8.8.8")
	if err == nil {
		t.Fatal("expected error when geoipService is nil")
	}
}

func TestWeatherService_LookupZipcode_NilService(t *testing.T) {
	ws := newTestWeatherService()
	_, err := ws.LookupZipcode(90210)
	if err == nil {
		t.Fatal("expected error when zipcodeService is nil")
	}
}

func TestWeatherService_GetCurrentWeather_HTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"current": {
				"temperature_2m": 22.5,
				"relative_humidity_2m": 55,
				"apparent_temperature": 21.0,
				"is_day": 1,
				"precipitation": 0,
				"weather_code": 1,
				"cloud_cover": 20,
				"pressure_msl": 1015.0,
				"wind_speed_10m": 12.0,
				"wind_direction_10m": 180,
				"wind_gusts_10m": 18.0
			},
			"timezone": "America/New_York"
		}`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.openMeteoBaseURL = server.URL

	weather, err := ws.GetCurrentWeather(40.7128, -74.0060, "metric")
	if err != nil {
		t.Fatalf("GetCurrentWeather returned error: %v", err)
	}
	if weather.Temperature != 22.5 {
		t.Errorf("Temperature = %v, want 22.5", weather.Temperature)
	}
	if weather.Timezone != "America/New_York" {
		t.Errorf("Timezone = %v, want America/New_York", weather.Timezone)
	}

	// Second call should hit the cache and not error.
	weather2, err := ws.GetCurrentWeather(40.7128, -74.0060, "metric")
	if err != nil {
		t.Fatalf("cached GetCurrentWeather returned error: %v", err)
	}
	if weather2.Temperature != weather.Temperature {
		t.Errorf("cached result differs: %v vs %v", weather2.Temperature, weather.Temperature)
	}
}

func TestWeatherService_GetCurrentWeather_Imperial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":0,"wind_speed_10m":10},"timezone":"UTC"}`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.openMeteoBaseURL = server.URL

	weather, err := ws.GetCurrentWeather(0, 0, "imperial")
	if err != nil {
		t.Fatalf("GetCurrentWeather returned error: %v", err)
	}
	if weather.Temperature != 32 {
		t.Errorf("imperial Temperature = %v, want 32", weather.Temperature)
	}
}

func TestWeatherService_GetCurrentWeather_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.openMeteoBaseURL = server.URL

	_, err := ws.GetCurrentWeather(1, 1, "metric")
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}

func TestWeatherService_GetForecast_HTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"daily": {
				"time": ["2024-01-01"],
				"weather_code": [1],
				"temperature_2m_max": [10],
				"temperature_2m_min": [0],
				"apparent_temperature_max": [9],
				"apparent_temperature_min": [-1],
				"precipitation_sum": [0],
				"precipitation_hours": [0],
				"precipitation_probability_max": [10],
				"wind_speed_10m_max": [5],
				"wind_gusts_10m_max": [8],
				"wind_direction_10m_dominant": [90],
				"shortwave_radiation_sum": [100]
			},
			"hourly": {
				"time": ["2024-01-01T00:00"],
				"temperature_2m": [5],
				"apparent_temperature": [4],
				"relative_humidity_2m": [60],
				"precipitation": [0],
				"precipitation_probability": [5],
				"weather_code": [1],
				"cloud_cover": [10],
				"wind_speed_10m": [3],
				"wind_direction_10m": [90],
				"wind_gusts_10m": [6],
				"visibility": [10000],
				"uv_index": [0]
			},
			"timezone": "UTC"
		}`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.openMeteoBaseURL = server.URL

	forecast, err := ws.GetForecast(40.0, -74.0, 1, "metric")
	if err != nil {
		t.Fatalf("GetForecast returned error: %v", err)
	}
	if len(forecast.Days) != 1 {
		t.Fatalf("len(Days) = %d, want 1", len(forecast.Days))
	}
	if forecast.Days[0].TempMax != 10 {
		t.Errorf("TempMax = %v, want 10", forecast.Days[0].TempMax)
	}
	if len(forecast.Days[0].Hourly) != 1 {
		t.Fatalf("hourly not grouped into matching day: %+v", forecast.Days[0])
	}
}

func TestWeatherService_SearchLocations_HTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"latitude":41.8,"longitude":-87.6,"name":"Chicago","country":"United States","country_code":"us","admin1":"Illinois"}]}`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.geocodingURL = server.URL

	results, err := ws.SearchLocations("Chicago", 5)
	if err != nil {
		t.Fatalf("SearchLocations returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].CountryCode != "US" {
		t.Errorf("CountryCode = %v, want US (uppercased)", results[0].CountryCode)
	}
	if results[0].ShortName != "Chicago, IL" {
		t.Errorf("ShortName = %v, want 'Chicago, IL'", results[0].ShortName)
	}
}

func TestWeatherService_SearchLocations_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	ws := newTestWeatherService()
	ws.geocodingURL = server.URL

	results, err := ws.SearchLocations("Nowhere", 5)
	if err != nil {
		t.Fatalf("SearchLocations returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}
