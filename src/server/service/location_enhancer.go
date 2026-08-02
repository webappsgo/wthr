package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LocationEnhancer enhances location data with external databases
type LocationEnhancer struct {
	db             *sql.DB
	client         *http.Client
	countriesData  []Country
	citiesData     []City
	mu             sync.RWMutex
	initialized    bool
	onInitComplete func(countries, cities bool)
}

// Country represents country data from external database
type Country struct {
	Name        string   `json:"name"`
	CountryCode string   `json:"country_code"`
	Capital     string   `json:"capital"`
	Timezones   []string `json:"timezones"`
	Population  int      `json:"population"`
}

// City represents city data from external database
type City struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	State   string `json:"state,omitempty"`
	Coord   struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"coord"`
	Population int `json:"population,omitempty"`
}

// EnhancedLocation represents enriched location data
type EnhancedLocation struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Name        string  `json:"name"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Timezone    string  `json:"timezone"`
	Admin1      string  `json:"admin1,omitempty"`
	Admin2      string  `json:"admin2,omitempty"`
	Population  int     `json:"population,omitempty"`
	Capital     string  `json:"capital,omitempty"`
	FullName    string  `json:"fullName"`
	ShortName   string  `json:"shortName"`
}

// NewLocationEnhancer creates a new location enhancer
func NewLocationEnhancer(db *sql.DB) *LocationEnhancer {
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	le := &LocationEnhancer{
		db: db,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		countriesData: []Country{},
		citiesData:    []City{},
		initialized:   false,
	}

	// Load data in background
	go le.loadData()

	return le
}

// loadData loads external data in parallel
func (le *LocationEnhancer) loadData() {
	startTime := time.Now()
	fmt.Printf("⏱️  Loading location databases at %s\n", startTime.Format("15:04:05.000"))

	var wg sync.WaitGroup
	// Only waiting for countries now
	wg.Add(1)

	countriesLoaded := false
	citiesLoaded := false

	go func() {
		defer wg.Done()
		countryStart := time.Now()
		fmt.Println("📥 Loading countries database...")
		err := le.loadCountriesData()
		elapsed := time.Since(countryStart)
		if err != nil {
			fmt.Printf("❌ Countries database failed after %s: %v\n", elapsed, err)
		} else {
			fmt.Printf("✅ Countries database loaded in %s (%d countries)\n", elapsed, len(le.countriesData))
			countriesLoaded = true
		}
	}()

	// Wait for countries only (zipcodes load separately in main.go)
	wg.Wait()

	// Mark as initialized so server can start serving requests
	le.mu.Lock()
	le.initialized = true
	le.mu.Unlock()

	totalElapsed := time.Since(startTime)
	fmt.Printf("🎉 Core databases loaded in %s (cities loading in background)\n", totalElapsed)

	// Load cities in background (non-blocking)
	go func() {
		cityStart := time.Now()
		fmt.Println("📥 Loading cities database (background)...")
		err := le.loadCitiesData()
		elapsed := time.Since(cityStart)
		if err != nil {
			fmt.Printf("❌ Cities database failed after %s: %v\n", elapsed, err)
			citiesLoaded = false
		} else {
			fmt.Printf("✅ Cities database loaded in %s (%d cities)\n", elapsed, len(le.citiesData))
			citiesLoaded = true
		}

		// Notify completion via callback
		if le.onInitComplete != nil {
			le.onInitComplete(countriesLoaded, citiesLoaded)
		}
	}()
}

// loadCountriesData loads country data from external source
func (le *LocationEnhancer) loadCountriesData() error {
	url := "https://raw.githubusercontent.com/casapps/countries/refs/heads/main/countries.json"

	resp, err := le.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch countries data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read countries data: %w", err)
	}

	var countries []Country
	if err := json.Unmarshal(body, &countries); err != nil {
		return fmt.Errorf("failed to parse countries data: %w", err)
	}

	le.mu.Lock()
	le.countriesData = countries
	le.mu.Unlock()

	return nil
}

// loadCitiesData loads city data from external source
func (le *LocationEnhancer) loadCitiesData() error {
	url := "https://raw.githubusercontent.com/casapps/citylist/refs/heads/main/src/data/citylist.json"

	resp, err := le.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch cities data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read cities data: %w", err)
	}

	var cities []City
	if err := json.Unmarshal(body, &cities); err != nil {
		return fmt.Errorf("failed to parse cities data: %w", err)
	}

	le.mu.Lock()
	le.citiesData = cities
	le.mu.Unlock()

	return nil
}

// IsInitialized returns whether the enhancer has loaded external data
func (le *LocationEnhancer) IsInitialized() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.initialized
}

// SetOnInitComplete sets the callback to be called when initialization completes
func (le *LocationEnhancer) SetOnInitComplete(callback func(countries, cities bool)) {
	le.mu.Lock()
	defer le.mu.Unlock()
	le.onInitComplete = callback
}

// Reload reloads all location data (countries and cities) from external sources
// This is used by the scheduler for periodic data refresh
func (le *LocationEnhancer) Reload() error {
	log.Println("INFO: Reloading location databases...")
	startTime := time.Now()

	// Reload countries (blocking)
	if err := le.loadCountriesData(); err != nil {
		return fmt.Errorf("failed to reload countries: %w", err)
	}
	log.Printf("OK: Countries reloaded (%d entries)", len(le.countriesData))

	// Reload cities (blocking for reload)
	if err := le.loadCitiesData(); err != nil {
		return fmt.Errorf("failed to reload cities: %w", err)
	}
	log.Printf("OK: Cities reloaded (%d entries)", len(le.citiesData))

	elapsed := time.Since(startTime)
	log.Printf("OK: Location databases reloaded in %s", elapsed)
	return nil
}

// EnhanceLocationData enhances geocode result with external data
func (le *LocationEnhancer) EnhanceLocationData(result *GeocodeResult) (*EnhancedLocation, error) {
	le.mu.RLock()
	defer le.mu.RUnlock()

	enhanced := &EnhancedLocation{
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		Name:        result.Name,
		Country:     result.Country,
		CountryCode: strings.ToUpper(result.CountryCode),
		Timezone:    result.Timezone,
		Admin1:      result.Admin1,
		Admin2:      result.Admin2,
		Population:  result.Population,
	}

	// Find country info
	for _, country := range le.countriesData {
		if strings.EqualFold(country.CountryCode, result.CountryCode) {
			enhanced.Country = country.Name
			enhanced.Capital = country.Capital
			if enhanced.Timezone == "" && len(country.Timezones) > 0 {
				enhanced.Timezone = country.Timezones[0]
			}
			break
		}
	}

	// Find city match for state/population data
	// Look for the city with matching name/country AND closest coordinates
	var bestMatch *City
	// Only consider cities within 100km
	minDistance := 100.0

	for i := range le.citiesData {
		city := &le.citiesData[i]
		if strings.EqualFold(city.Name, result.Name) &&
			strings.EqualFold(city.Country, result.CountryCode) {
			// Calculate distance to find closest match
			distance := haversineDistance(
				result.Latitude, result.Longitude,
				city.Coord.Lat, city.Coord.Lon,
			)

			if distance < minDistance {
				minDistance = distance
				bestMatch = city
			}
		}
	}

	// If we found a good match, use its state data
	if bestMatch != nil {
		if bestMatch.State != "" {
			enhanced.Admin1 = bestMatch.State
		}
		if bestMatch.Population > enhanced.Population {
			enhanced.Population = bestMatch.Population
		}
	}

	// Build formatted names
	enhanced.FullName = le.buildFullName(enhanced)
	enhanced.ShortName = le.buildShortName(enhanced)

	return enhanced, nil
}

// FindNearestCity finds the nearest city to given coordinates
func (le *LocationEnhancer) FindNearestCity(latitude, longitude float64) (*EnhancedLocation, error) {
	le.mu.RLock()
	defer le.mu.RUnlock()

	if len(le.citiesData) == 0 {
		return nil, fmt.Errorf("cities data not loaded")
	}

	// Find nearest city using haversine distance
	var nearestCity *City
	minDistance := math.MaxFloat64

	for i := range le.citiesData {
		city := &le.citiesData[i]
		distance := haversineDistance(latitude, longitude, city.Coord.Lat, city.Coord.Lon)

		if distance < minDistance {
			minDistance = distance
			nearestCity = city
		}
	}

	if nearestCity == nil {
		return nil, fmt.Errorf("no cities found")
	}

	// Build enhanced location from nearest city
	enhanced := &EnhancedLocation{
		Latitude:    latitude,
		Longitude:   longitude,
		Name:        nearestCity.Name,
		CountryCode: strings.ToUpper(nearestCity.Country),
		Admin1:      nearestCity.State,
		Population:  nearestCity.Population,
	}

	// Find country info
	for _, country := range le.countriesData {
		if strings.EqualFold(country.CountryCode, nearestCity.Country) {
			enhanced.Country = country.Name
			enhanced.Capital = country.Capital
			if len(country.Timezones) > 0 {
				enhanced.Timezone = country.Timezones[0]
			}
			break
		}
	}

	// Build formatted names
	enhanced.FullName = le.buildFullName(enhanced)
	enhanced.ShortName = le.buildShortName(enhanced)

	return enhanced, nil
}

// buildFullName builds full location name
func (le *LocationEnhancer) buildFullName(location *EnhancedLocation) string {
	parts := []string{location.Name}

	// Add Admin2 (e.g., "Greater London")
	if location.Admin2 != "" && location.Admin2 != location.Name && location.Admin2 != location.Admin1 {
		parts = append(parts, location.Admin2)
	}

	// Add Admin1 (e.g., "England", "California")
	if location.Admin1 != "" && location.Admin1 != location.Name {
		parts = append(parts, location.Admin1)
	}

	// Add country name
	if location.Country != "" {
		parts = append(parts, location.Country)
	}

	return strings.Join(parts, ", ")
}

// buildShortName builds short location name
func (le *LocationEnhancer) buildShortName(location *EnhancedLocation) string {
	city := location.Name
	countryCode := location.CountryCode

	// For US locations, use state abbreviation if available
	if countryCode == "US" && location.Admin1 != "" {
		// Check if Admin1 is already a 2-letter state code
		if len(location.Admin1) == 2 && strings.ToUpper(location.Admin1) == location.Admin1 {
			return fmt.Sprintf("%s, %s", city, location.Admin1)
		}
		// Otherwise, try to get abbreviation from full state name
		stateAbbrev := le.getStateAbbreviation(location.Admin1)
		if stateAbbrev != "" {
			return fmt.Sprintf("%s, %s", city, stateAbbrev)
		}
		// If we have state but no abbreviation, use full state name
		return fmt.Sprintf("%s, %s", city, location.Admin1)
	}

	// For Canadian locations, use province abbreviation if available
	if countryCode == "CA" && location.Admin1 != "" {
		// Check if Admin1 is already a 2-letter province code
		if len(location.Admin1) == 2 && strings.ToUpper(location.Admin1) == location.Admin1 {
			return fmt.Sprintf("%s, %s", city, location.Admin1)
		}
		provinceAbbrev := le.getProvinceAbbreviation(location.Admin1)
		if provinceAbbrev != "" {
			return fmt.Sprintf("%s, %s", city, provinceAbbrev)
		}
		// If we have province but no abbreviation, use full province name
		return fmt.Sprintf("%s, %s", city, location.Admin1)
	}

	// If countryCode is empty, try to infer from Admin1 or default
	if countryCode == "" {
		// Check if Admin1 looks like a US state
		if location.Admin1 != "" {
			stateAbbrev := le.getStateAbbreviation(location.Admin1)
			if stateAbbrev != "" {
				// It's a US state
				return fmt.Sprintf("%s, %s, US", city, stateAbbrev)
			}
			// Check if it's a Canadian province
			provAbbrev := le.getProvinceAbbreviation(location.Admin1)
			if provAbbrev != "" {
				return fmt.Sprintf("%s, %s, CA", city, provAbbrev)
			}
		}
		// Fallback: use just city name if we truly have no country info
		return city
	}

	// For all other locations, use country code
	return fmt.Sprintf("%s, %s", city, countryCode)
}

// getStateAbbreviation returns US state abbreviation
func (le *LocationEnhancer) getStateAbbreviation(stateName string) string {
	// Normalize the state name to lowercase for matching
	normalizedState := strings.ToLower(strings.TrimSpace(stateName))

	states := map[string]string{
		"alabama": "AL", "alaska": "AK", "arizona": "AZ", "arkansas": "AR", "california": "CA",
		"colorado": "CO", "connecticut": "CT", "delaware": "DE", "florida": "FL", "georgia": "GA",
		"hawaii": "HI", "idaho": "ID", "illinois": "IL", "indiana": "IN", "iowa": "IA",
		"kansas": "KS", "kentucky": "KY", "louisiana": "LA", "maine": "ME", "maryland": "MD",
		"massachusetts": "MA", "michigan": "MI", "minnesota": "MN", "mississippi": "MS", "missouri": "MO",
		"montana": "MT", "nebraska": "NE", "nevada": "NV", "new hampshire": "NH", "new jersey": "NJ",
		"new mexico": "NM", "new york": "NY", "north carolina": "NC", "north dakota": "ND", "ohio": "OH",
		"oklahoma": "OK", "oregon": "OR", "pennsylvania": "PA", "rhode island": "RI", "south carolina": "SC",
		"south dakota": "SD", "tennessee": "TN", "texas": "TX", "utah": "UT", "vermont": "VT",
		"virginia": "VA", "washington": "WA", "west virginia": "WV", "wisconsin": "WI", "wyoming": "WY",
	}

	// Try exact match first
	if abbrev, ok := states[normalizedState]; ok {
		return abbrev
	}

	// If no exact match, return empty string (caller will use country code)
	return ""
}

// getProvinceAbbreviation returns Canadian province abbreviation
func (le *LocationEnhancer) getProvinceAbbreviation(provinceName string) string {
	provinces := map[string]string{
		"alberta":                   "AB",
		"british columbia":          "BC",
		"manitoba":                  "MB",
		"new brunswick":             "NB",
		"newfoundland and labrador": "NL",
		"northwest territories":     "NT",
		"nova scotia":               "NS",
		"nunavut":                   "NU",
		"ontario":                   "ON",
		"prince edward island":      "PE",
		"quebec":                    "QC",
		"saskatchewan":              "SK",
		"yukon":                     "YT",
	}

	return provinces[strings.ToLower(provinceName)]
}

// haversineDistance calculates the distance between two points on Earth
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Earth radius in kilometers
	const earthRadius = 6371.0

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	// Haversine formula
	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// EnhanceLocation enhances a Coordinates struct (wrapper for EnhanceLocationData)
func (le *LocationEnhancer) EnhanceLocation(coords *Coordinates) *Coordinates {
	if coords == nil {
		return nil
	}
	// Convert Coordinates to GeocodeResult
	geocodeResult := &GeocodeResult{
		Latitude:    coords.Latitude,
		Longitude:   coords.Longitude,
		Name:        coords.Name,
		Country:     coords.Country,
		CountryCode: coords.CountryCode,
		Admin1:      coords.Admin1,
		Admin2:      coords.Admin2,
		Timezone:    coords.Timezone,
		Population:  coords.Population,
	}

	// Enhance the location
	enhanced, err := le.EnhanceLocationData(geocodeResult)
	if err != nil {
		// Return original coordinates if enhancement fails
		return coords
	}

	// Convert back to Coordinates
	return &Coordinates{
		Latitude:    enhanced.Latitude,
		Longitude:   enhanced.Longitude,
		Name:        enhanced.Name,
		Country:     enhanced.Country,
		CountryCode: enhanced.CountryCode,
		Timezone:    enhanced.Timezone,
		Admin1:      enhanced.Admin1,
		Admin2:      enhanced.Admin2,
		Population:  enhanced.Population,
		FullName:    enhanced.FullName,
		ShortName:   enhanced.ShortName,
	}
}

// GetCitiesData returns the loaded cities data
func (le *LocationEnhancer) GetCitiesData() []City {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.citiesData
}

// FindCityByID finds a city by its ID and returns enhanced location
func (le *LocationEnhancer) FindCityByID(cityID int) (*EnhancedLocation, error) {
	le.mu.RLock()
	defer le.mu.RUnlock()

	if !le.initialized {
		return nil, fmt.Errorf("location enhancer not initialized")
	}

	// Search for city by ID
	for _, city := range le.citiesData {
		if city.ID == cityID {
			// Find country details
			var countryData *Country
			for i := range le.countriesData {
				if strings.EqualFold(le.countriesData[i].CountryCode, city.Country) {
					countryData = &le.countriesData[i]
					break
				}
			}

			enhanced := &EnhancedLocation{
				Latitude:    city.Coord.Lat,
				Longitude:   city.Coord.Lon,
				Name:        city.Name,
				Country:     city.Country,
				CountryCode: city.Country,
				Admin1:      city.State,
				Population:  city.Population,
			}

			if countryData != nil {
				enhanced.Capital = countryData.Capital
				if len(countryData.Timezones) > 0 {
					enhanced.Timezone = countryData.Timezones[0]
				}
			}

			// Build display names
			if city.State != "" {
				enhanced.FullName = fmt.Sprintf("%s, %s, %s", city.Name, city.State, city.Country)
				enhanced.ShortName = fmt.Sprintf("%s, %s", city.Name, city.State)
			} else {
				enhanced.FullName = fmt.Sprintf("%s, %s", city.Name, city.Country)
				enhanced.ShortName = city.Name
			}

			return enhanced, nil
		}
	}

	return nil, fmt.Errorf("city ID %d not found", cityID)
}
