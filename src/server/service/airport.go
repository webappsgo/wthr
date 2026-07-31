package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// AirportData represents an airport entry
type AirportData struct {
	ICAO      string  `json:"icao"`
	IATA      string  `json:"iata"`
	Name      string  `json:"name"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	Country   string  `json:"country"`
	Elevation int     `json:"elevation"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Tz        string  `json:"tz"`
}

// AirportService handles airport lookups
type AirportService struct {
	// Key is ICAO code
	airports  map[string]*AirportData
	// Index by IATA code
	iataIndex map[string]*AirportData
	mu        sync.RWMutex
	loaded    bool
	dataURL   string
	cachePath string
}

// NewAirportService creates a new airport service
// Recommended usage: Pass utils.GetAirportDataPath(dirPaths) for cachePath
func NewAirportService(dataURL, cachePath string) *AirportService {
	if dataURL == "" {
		dataURL = "https://raw.githubusercontent.com/casapps/airports/refs/heads/main/src/data/airports.json"
	}
	if cachePath == "" {
		// Fallback to temp for backward compatibility
		// Uses embedded airports data
		cachePath = "/tmp/airports.json"
	}

	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   dataURL,
		cachePath: cachePath,
	}

	// Load airport data in background
	go as.loadAirports()

	return as
}

// loadAirports loads airport data from cache or downloads
func (as *AirportService) loadAirports() {
	startTime := time.Now()
	fmt.Println("✈️  Loading airport codes database...")

	var body []byte
	var err error

	// Try loading from cache first
	if _, statErr := os.Stat(as.cachePath); statErr == nil {
		body, err = os.ReadFile(as.cachePath)
		if err == nil {
			fmt.Printf("📂 Loaded airports from cache\n")
		}
	}

	// If cache load failed or doesn't exist, download
	if body == nil {
		resp, err := downloadClient.Get(as.dataURL)
		if err != nil {
			fmt.Printf("❌ Failed to load airports: %v\n", err)
			return
		}
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("❌ Failed to read airports: %v\n", err)
			return
		}

		// Save to cache
		if err := os.WriteFile(as.cachePath, body, 0644); err != nil {
			fmt.Printf("⚠️  Failed to cache airports: %v\n", err)
		}
	}

	var airports []AirportData
	if err := json.Unmarshal(body, &airports); err != nil {
		fmt.Printf("❌ Failed to parse airports: %v\n", err)
		return
	}

	// Build indexes
	as.mu.Lock()
	validCount := 0
	for _, airport := range airports {
		if airport.Lat != 0 && airport.Lon != 0 {
			// Index by ICAO (primary)
			if airport.ICAO != "" {
				as.airports[strings.ToUpper(airport.ICAO)] = &airport
			}
			// Index by IATA (secondary)
			if airport.IATA != "" {
				as.iataIndex[strings.ToUpper(airport.IATA)] = &airport
			}
			validCount++
		}
	}
	as.loaded = true
	as.mu.Unlock()

	elapsed := time.Since(startTime)
	fmt.Printf("✅ Airport database loaded in %.2f seconds (%d airports)\n", elapsed.Seconds(), validCount)
}

// LookupAirport returns coordinates for an airport code (ICAO or IATA)
func (as *AirportService) LookupAirport(code string) (*Coordinates, error) {
	as.mu.RLock()
	defer as.mu.RUnlock()

	if !as.loaded {
		return nil, fmt.Errorf("airport database not loaded yet")
	}

	codeUpper := strings.ToUpper(code)

	// Try ICAO first
	airport, exists := as.airports[codeUpper]
	if !exists {
		// Try IATA
		airport, exists = as.iataIndex[codeUpper]
		if !exists {
			return nil, fmt.Errorf("airport code %s not found", code)
		}
	}

	// Build location name
	name := airport.Name
	if airport.City != "" {
		name = fmt.Sprintf("%s (%s)", airport.City, airport.Name)
	}

	return &Coordinates{
		Latitude:    airport.Lat,
		Longitude:   airport.Lon,
		Name:        name,
		Country:     airport.Country,
		CountryCode: "",
		Admin1:      airport.State,
		Admin2:      airport.City,
		FullName:    fmt.Sprintf("%s, %s, %s", airport.Name, airport.City, airport.Country),
		ShortName:   fmt.Sprintf("%s (%s)", airport.City, code),
	}, nil
}

// IsAirportCode checks if a string looks like an airport code (3-4 chars, alphanumeric)
func IsAirportCode(s string) bool {
	if len(s) < 3 || len(s) > 4 {
		return false
	}

	// Check if all characters are letters/digits
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}

	return true
}

// IsLoaded returns whether the airport database is loaded
func (as *AirportService) IsLoaded() bool {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.loaded
}

// Reload reloads the airport database
func (as *AirportService) Reload() error {
	fmt.Println("🔄 Reloading airport data...")

	as.mu.Lock()
	as.loaded = false
	as.mu.Unlock()

	as.loadAirports()

	as.mu.RLock()
	loaded := as.loaded
	as.mu.RUnlock()

	if !loaded {
		return fmt.Errorf("airport reload failed - database not loaded")
	}

	return nil
}
