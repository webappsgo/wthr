package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func clearLocationEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MYLOCATION_NAME", "MYLOCATION_ZIP", "MYLOCATION_CITY_ID"} {
		orig, had := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if had {
				os.Setenv(key, orig)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}

func TestBuildWeatherPath(t *testing.T) {
	clearLocationEnv(t)

	tests := []struct {
		name     string
		lat, lon float64
		zip      string
		location string
		config   *CLIConfig
		env      map[string]string
		want     string
	}{
		{
			name: "lat lon takes priority",
			lat:  40.0, lon: -73.0,
			config: &CLIConfig{},
			want:   "/api/v1/weather?lat=40.000000&lon=-73.000000",
		},
		{
			name:   "zip used when no lat lon",
			zip:    "10001",
			config: &CLIConfig{},
			want:   "/api/v1/weather?zip=10001",
		},
		{
			name:     "location used when no zip",
			location: "New York City",
			config:   &CLIConfig{},
			want:     "/api/v1/weather?location=New+York+City",
		},
		{
			name:   "config location used as fallback",
			config: &CLIConfig{Location: "Boston"},
			want:   "/api/v1/weather?location=Boston",
		},
		{
			name:   "env MYLOCATION_NAME used when config empty",
			config: &CLIConfig{},
			env:    map[string]string{"MYLOCATION_NAME": "Chicago"},
			want:   "/api/v1/weather?location=Chicago",
		},
		{
			name:   "env MYLOCATION_ZIP used after name",
			config: &CLIConfig{},
			env:    map[string]string{"MYLOCATION_ZIP": "60601"},
			want:   "/api/v1/weather?zip=60601",
		},
		{
			name:   "env MYLOCATION_CITY_ID used last, must be numeric",
			config: &CLIConfig{},
			env:    map[string]string{"MYLOCATION_CITY_ID": "4887398"},
			want:   "/api/v1/weather?city_id=4887398",
		},
		{
			name:   "non-numeric city id is rejected",
			config: &CLIConfig{},
			env:    map[string]string{"MYLOCATION_CITY_ID": "not-a-number"},
			want:   "",
		},
		{
			name:   "nothing specified returns empty",
			config: &CLIConfig{},
			want:   "",
		},
		{
			name: "zero zero lat lon treated as unset",
			lat:  0, lon: 0,
			zip:    "90210",
			config: &CLIConfig{},
			want:   "/api/v1/weather?zip=90210",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}
			got := buildWeatherPath("/api/v1", "/weather", tt.lat, tt.lon, tt.zip, tt.location, tt.config)
			if got != tt.want {
				t.Errorf("buildWeatherPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Run("returns trimmed value", func(t *testing.T) {
		os.Setenv("WTHR_TEST_GETENV", "  value  ")
		defer os.Unsetenv("WTHR_TEST_GETENV")
		if got := getEnv("WTHR_TEST_GETENV"); got != "value" {
			t.Errorf("getEnv() = %q, want %q", got, "value")
		}
	})

	t.Run("returns empty for unset", func(t *testing.T) {
		os.Unsetenv("WTHR_TEST_GETENV_UNSET")
		if got := getEnv("WTHR_TEST_GETENV_UNSET"); got != "" {
			t.Errorf("getEnv() = %q, want empty", got)
		}
	})
}

func newTestConfig(serverURL string) *CLIConfig {
	return &CLIConfig{
		Server: ServerConfig{Primary: serverURL, APIVersion: "v1"},
		Output: OutputConfig{Format: "json", Color: "no"},
	}
}

func TestHandleCurrentCommand(t *testing.T) {
	clearLocationEnv(t)

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.String(), "/api/v1/weather") {
				t.Errorf("unexpected path: %s", r.URL.String())
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"temperature": 70.0})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		err := handleCurrentCommand(config, []string{"--location", "NYC"})
		if err != nil {
			t.Fatalf("handleCurrentCommand() failed: %v", err)
		}
	})

	t.Run("missing location returns usage error", func(t *testing.T) {
		config := newTestConfig("http://example.invalid")
		err := handleCurrentCommand(config, []string{})
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitUsageError {
			t.Fatalf("expected usage error, got %v", err)
		}
	})

	t.Run("bad flag returns usage error", func(t *testing.T) {
		config := newTestConfig("http://example.invalid")
		err := handleCurrentCommand(config, []string{"--not-a-flag"})
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitUsageError {
			t.Fatalf("expected usage error, got %v", err)
		}
	})

	t.Run("server error propagates", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"boom"}`))
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		err := handleCurrentCommand(config, []string{"--zip", "10001"})
		if err == nil {
			t.Fatal("expected error from server failure")
		}
	})
}

func TestHandleForecastCommand(t *testing.T) {
	clearLocationEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "days=5") {
			t.Errorf("expected days=5 in query, got %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"forecast": []interface{}{}})
	}))
	defer server.Close()

	config := newTestConfig(server.URL)
	err := handleForecastCommand(config, []string{"--zip", "10001", "--days", "5"})
	if err != nil {
		t.Fatalf("handleForecastCommand() failed: %v", err)
	}
}

func TestHandleAlertsCommand(t *testing.T) {
	clearLocationEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": []interface{}{}})
	}))
	defer server.Close()

	config := newTestConfig(server.URL)
	if err := handleAlertsCommand(config, []string{"--location", "Miami"}); err != nil {
		t.Fatalf("handleAlertsCommand() failed: %v", err)
	}

	t.Run("missing location", func(t *testing.T) {
		config := newTestConfig(server.URL)
		err := handleAlertsCommand(config, []string{})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestHandleMoonCommand(t *testing.T) {
	t.Run("without date", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				t.Errorf("expected no query params, got %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"phase": "full"})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		if err := handleMoonCommand(config, []string{}); err != nil {
			t.Fatalf("handleMoonCommand() failed: %v", err)
		}
	})

	t.Run("with date", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "date=2024-01-15" {
				t.Errorf("expected date query, got %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"phase": "new"})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		if err := handleMoonCommand(config, []string{"--date", "2024-01-15"}); err != nil {
			t.Fatalf("handleMoonCommand() failed: %v", err)
		}
	})
}

func TestHandleHistoryCommand(t *testing.T) {
	clearLocationEnv(t)

	t.Run("missing date returns usage error", func(t *testing.T) {
		config := newTestConfig("http://example.invalid")
		err := handleHistoryCommand(config, []string{"--zip", "10001"})
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitUsageError {
			t.Fatalf("expected usage error, got %v", err)
		}
	})

	t.Run("missing location returns usage error", func(t *testing.T) {
		config := newTestConfig("http://example.invalid")
		err := handleHistoryCommand(config, []string{"--date", "2024-01-01"})
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitUsageError {
			t.Fatalf("expected usage error, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.RawQuery, "date=2024-01-01") {
				t.Errorf("expected date in query, got %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"temperature": 55.0})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		if err := handleHistoryCommand(config, []string{"--zip", "10001", "--date", "2024-01-01"}); err != nil {
			t.Fatalf("handleHistoryCommand() failed: %v", err)
		}
	})
}

func TestHandleEarthquakesCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "min_magnitude=") || !strings.Contains(r.URL.RawQuery, "limit=") {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"quakes": []interface{}{}})
	}))
	defer server.Close()

	config := newTestConfig(server.URL)
	if err := handleEarthquakesCommand(config, []string{"--min-mag", "4.5", "--limit", "5"}); err != nil {
		t.Fatalf("handleEarthquakesCommand() failed: %v", err)
	}

	t.Run("bad flag", func(t *testing.T) {
		err := handleEarthquakesCommand(config, []string{"--bogus"})
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitUsageError {
			t.Fatalf("expected usage error, got %v", err)
		}
	})
}

func TestHandleHurricanesCommand(t *testing.T) {
	t.Run("active default true", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.RawQuery, "active=true") {
				t.Errorf("expected active=true, got %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"storms": []interface{}{}})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		if err := handleHurricanesCommand(config, []string{}); err != nil {
			t.Fatalf("handleHurricanesCommand() failed: %v", err)
		}
	})

	t.Run("active false omits query param", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				t.Errorf("expected no query params, got %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"storms": []interface{}{}})
		}))
		defer server.Close()

		config := newTestConfig(server.URL)
		if err := handleHurricanesCommand(config, []string{"--active=false"}); err != nil {
			t.Fatalf("handleHurricanesCommand() failed: %v", err)
		}
	})
}
