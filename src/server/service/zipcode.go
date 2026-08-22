package service

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/display"
)

// ZipcodeData represents a US zipcode entry
type ZipcodeData struct {
	State     string      `json:"state"`
	City      string      `json:"city"`
	County    string      `json:"county"`
	ZipCode   int         `json:"zip_code"`
	Latitude  interface{} `json:"latitude"`
	Longitude interface{} `json:"longitude"`
}

// ZipcodeService handles zipcode lookups
type ZipcodeService struct {
	zipcodes map[int]*ZipcodeData
	mu       sync.RWMutex
	loaded   bool
}

// NewZipcodeService creates a new zipcode service
func NewZipcodeService() *ZipcodeService {
	zs := &ZipcodeService{
		zipcodes: make(map[int]*ZipcodeData),
	}

	// Load zipcode data in background
	go zs.loadZipcodes()

	return zs
}

// toFloat64 converts interface{} to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case int:
		return float64(val)
	default:
		return 0
	}
}

// loadZipcodes loads zipcode data from GitHub
func (zs *ZipcodeService) loadZipcodes() {
	startTime := time.Now()
	fmt.Printf("%s Loading US zipcode database...\n", display.Emoji("📮", "->"))

	zipcodeURL := "https://raw.githubusercontent.com/webappsgo/zipcodes/refs/heads/main/src/data/zipcodes.json"

	resp, err := downloadClient.Get(zipcodeURL)
	if err != nil {
		fmt.Printf("%s Zipcode database unavailable: %v (continuing without zipcodes)\n", display.Emoji("⚠️", "WARNING:"), err)
		zs.mu.Lock()
		// Mark as loaded even if failed, so we don't keep retrying
		zs.loaded = true
		zs.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("%s Zipcode database returned status %d (continuing without zipcodes)\n", display.Emoji("⚠️", "WARNING:"), resp.StatusCode)
		zs.mu.Lock()
		zs.loaded = true
		zs.mu.Unlock()
		return
	}

	// Try to decode as array first
	var zipcodesArray []ZipcodeData
	bodyBytes, _ := io.ReadAll(resp.Body)

	if err := json.Unmarshal(bodyBytes, &zipcodesArray); err != nil {
		// Try as object/map
		var zipcodesMap map[string]ZipcodeData
		if err2 := json.Unmarshal(bodyBytes, &zipcodesMap); err2 != nil {
			fmt.Printf("%s Failed to parse zipcodes (array: %v, map: %v) - continuing without zipcodes\n", display.Emoji("⚠️", "WARNING:"), err, err2)
			zs.mu.Lock()
			zs.loaded = true
			zs.mu.Unlock()
			return
		}
		// Convert map to array
		for _, zc := range zipcodesMap {
			zipcodesArray = append(zipcodesArray, zc)
		}
	}

	zipcodes := zipcodesArray

	// Build map, only include entries with valid coordinates
	zs.mu.Lock()
	validCount := 0
	for _, zc := range zipcodes {
		lat := toFloat64(zc.Latitude)
		lon := toFloat64(zc.Longitude)
		if lat != 0 && lon != 0 {
			zs.zipcodes[zc.ZipCode] = &zc
			validCount++
		}
	}
	zs.loaded = true
	zs.mu.Unlock()

	elapsed := time.Since(startTime)
	fmt.Printf("%s Zipcode database loaded in %s (%d valid zipcodes with coordinates)\n", display.Emoji("✅", "[OK]"), elapsed, validCount)
}

// LookupZipcode returns coordinates for a US zipcode
func (zs *ZipcodeService) LookupZipcode(zipcode int) (*Coordinates, error) {
	zs.mu.RLock()
	defer zs.mu.RUnlock()

	if !zs.loaded {
		return nil, fmt.Errorf("zipcode database not loaded yet")
	}

	zc, exists := zs.zipcodes[zipcode]
	if !exists {
		return nil, fmt.Errorf("zipcode %d not found", zipcode)
	}

	lat := toFloat64(zc.Latitude)
	lon := toFloat64(zc.Longitude)

	if lat == 0 || lon == 0 {
		return nil, fmt.Errorf("zipcode %d has no coordinates", zipcode)
	}

	return &Coordinates{
		Latitude:    lat,
		Longitude:   lon,
		Name:        zc.City,
		Country:     "United States",
		CountryCode: "US",
		Admin1:      zc.State,
		Admin2:      zc.County,
		FullName:    fmt.Sprintf("%s, %s, United States", zc.City, zc.State),
		ShortName:   fmt.Sprintf("%s, %s", zc.City, zc.State),
	}, nil
}

// IsZipcode checks if a string is a valid 5-digit US zipcode
func IsZipcode(s string) bool {
	if len(s) != 5 {
		return false
	}

	_, err := strconv.Atoi(s)
	return err == nil
}

// ParseZipcode parses a string into a zipcode integer
func ParseZipcode(s string) (int, error) {
	return strconv.Atoi(s)
}

// IsLoaded returns whether the zipcode database is loaded
func (zs *ZipcodeService) IsLoaded() bool {
	zs.mu.RLock()
	defer zs.mu.RUnlock()
	return zs.loaded
}

// Reload reloads the zipcode database from external source
// This is used by the scheduler for periodic data refresh
func (zs *ZipcodeService) Reload() error {
	fmt.Printf("%s Reloading US zipcode database...\n", display.Emoji("🔄", "->"))
	startTime := time.Now()

	// Clear current data
	zs.mu.Lock()
	zs.zipcodes = make(map[int]*ZipcodeData)
	zs.loaded = false
	zs.mu.Unlock()

	// Reload data (call the same loading logic)
	zs.loadZipcodes()

	// Check if loading succeeded
	zs.mu.RLock()
	loaded := zs.loaded
	count := len(zs.zipcodes)
	zs.mu.RUnlock()

	if !loaded || count == 0 {
		return fmt.Errorf("failed to reload zipcode database")
	}

	elapsed := time.Since(startTime)
	fmt.Printf("%s Zipcode database reloaded in %s (%d entries)\n", display.Emoji("✅", "[OK]"), elapsed, count)
	return nil
}
