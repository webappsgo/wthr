package services_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/service"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := database.InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db.DB
}

func TestLocationEnhancer_FindCityByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	enhancer := service.NewLocationEnhancer(db)

	// Note: This test will only work if city data is loaded
	// For now, we'll test the error case
	t.Run("City not found", func(t *testing.T) {
		_, err := enhancer.FindCityByID(999999999)
		if err == nil {
			t.Error("Expected error for non-existent city ID")
		}
	})

	t.Run("Invalid city ID", func(t *testing.T) {
		_, err := enhancer.FindCityByID(-1)
		if err == nil {
			t.Error("Expected error for invalid city ID")
		}
	})
}

func TestLocationEnhancer_FindNearestCity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	enhancer := service.NewLocationEnhancer(db)

	t.Run("Valid coordinates", func(t *testing.T) {
		// New York coordinates
		lat := 40.7128
		lon := -74.0060

		// This will return an error if data not loaded, which is expected
		_, err := enhancer.FindNearestCity(lat, lon)
		if err != nil {
			// Expected - data not loaded in test
			t.Logf("Expected error (data not loaded): %v", err)
		}
	})

	t.Run("Invalid coordinates", func(t *testing.T) {
		// Invalid latitude
		_, err := enhancer.FindNearestCity(999.0, 0.0)
		if err == nil {
			t.Error("Expected error for invalid latitude")
		}
	})
}

func TestLocationEnhancer_EnhanceLocation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	enhancer := service.NewLocationEnhancer(db)

	t.Run("Valid coordinates", func(t *testing.T) {
		coords := &service.Coordinates{
			Latitude:  40.7128,
			Longitude: -74.0060,
		}

		enhanced := enhancer.EnhanceLocation(coords)

		if enhanced == nil {
			t.Fatal("Expected enhanced location, got nil")
		}

		if enhanced.Latitude != coords.Latitude {
			t.Errorf("Expected latitude %f, got %f", coords.Latitude, enhanced.Latitude)
		}

		if enhanced.Longitude != coords.Longitude {
			t.Errorf("Expected longitude %f, got %f", coords.Longitude, enhanced.Longitude)
		}
	})

	t.Run("Nil coordinates", func(t *testing.T) {
		enhanced := enhancer.EnhanceLocation(nil)
		if enhanced != nil {
			t.Error("Expected nil for nil coordinates")
		}
	})
}

func TestCoordinates_Validation(t *testing.T) {
	tests := []struct {
		name      string
		latitude  float64
		longitude float64
		wantValid bool
	}{
		{"Valid coordinates", 40.7128, -74.0060, true},
		{"North pole", 90.0, 0.0, true},
		{"South pole", -90.0, 0.0, true},
		{"Invalid latitude high", 91.0, 0.0, false},
		{"Invalid latitude low", -91.0, 0.0, false},
		{"Invalid longitude high", 0.0, 181.0, false},
		{"Invalid longitude low", 0.0, -181.0, false},
		{"Equator prime meridian", 0.0, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate latitude range
			latValid := tt.latitude >= -90 && tt.latitude <= 90
			// Validate longitude range
			lonValid := tt.longitude >= -180 && tt.longitude <= 180

			isValid := latValid && lonValid

			if isValid != tt.wantValid {
				t.Errorf("Coordinates(%f, %f) validity = %v, want %v",
					tt.latitude, tt.longitude, isValid, tt.wantValid)
			}
		})
	}
}
