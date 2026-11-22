package service

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/display"
	"golang.org/x/text/language"
	cldrdisplay "golang.org/x/text/language/display"
)

const (
	defaultAirportDataURL = "https://ourairports.com/data/airports.csv"
	maxAirportDataBytes   = 32 << 20
	minAirportRecords     = 1000
	airportCacheMaxAge    = 24 * time.Hour
)

// AirportData represents a validated airport record from OurAirports.
type AirportData struct {
	ICAO        string  `json:"icao"`
	IATA        string  `json:"iata"`
	Name        string  `json:"name"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Type        string  `json:"type"`
	Scheduled   bool    `json:"scheduled_service"`
	Elevation   int     `json:"elevation"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Tz          string  `json:"tz,omitempty"`
}

// AirportService provides airport lookups by ICAO or IATA code.
type AirportService struct {
	airports  map[string]*AirportData
	iataIndex map[string]*AirportData
	mu        sync.RWMutex
	loaded    bool
	dataURL   string
	cachePath string
}

// NewAirportService creates a service and loads the daily OurAirports CSV in
// the background. Pass a project-scoped cache path to retain a local snapshot.
func NewAirportService(dataURL, cachePath string) *AirportService {
	if dataURL == "" {
		dataURL = defaultAirportDataURL
	}
	as := &AirportService{
		airports:  make(map[string]*AirportData),
		iataIndex: make(map[string]*AirportData),
		dataURL:   dataURL,
		cachePath: cachePath,
	}
	go as.loadAirports()
	return as
}

func (as *AirportService) loadAirports() {
	if err := as.loadAirportsWithRefresh(false); err != nil {
		fmt.Printf("%s Failed to load airport data: %v\n", display.Emoji("❌", "[FAIL]"), err)
	}
}

func (as *AirportService) loadAirportsWithRefresh(forceRefresh bool) error {
	startTime := time.Now()
	fmt.Printf("%s Loading airport codes database...\n", display.Emoji("✈️", "->"))

	cached, cacheTime, cacheErr := as.readAirportCache()
	if cacheErr == nil && len(cached) > 0 && !forceRefresh && time.Since(cacheTime) < airportCacheMaxAge {
		as.indexAirports(cached)
		fmt.Printf("%s Loaded %d airports from recent cache\n", display.Emoji("📂", "->"), len(cached))
		return nil
	}

	resp, err := downloadClient.Get(as.dataURL)
	if err != nil {
		return as.useAirportCacheAfterFailure(cached, cacheErr, fmt.Errorf("download airport data: %w", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("WARNING: failed to close OurAirports response: %v", err)
		}
	}()
	if resp.StatusCode != 200 {
		return as.useAirportCacheAfterFailure(cached, cacheErr, fmt.Errorf("OurAirports returned HTTP %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAirportDataBytes+1))
	if err != nil {
		return as.useAirportCacheAfterFailure(cached, cacheErr, fmt.Errorf("read OurAirports data: %w", err))
	}
	if len(body) > maxAirportDataBytes {
		return as.useAirportCacheAfterFailure(cached, cacheErr, fmt.Errorf("OurAirports data exceeds %d bytes", maxAirportDataBytes))
	}

	airports, err := parseOurAirportsCSV(body)
	if err != nil {
		return as.useAirportCacheAfterFailure(cached, cacheErr, err)
	}
	if len(airports) < minAirportRecords {
		return as.useAirportCacheAfterFailure(cached, cacheErr,
			fmt.Errorf("OurAirports data contains only %d valid airport records; expected at least %d", len(airports), minAirportRecords))
	}

	if as.cachePath != "" {
		if err := as.writeAirportCache(airports); err != nil {
			log.Printf("WARNING: failed to cache OurAirports data: %v", err)
		}
	}
	as.indexAirports(airports)
	fmt.Printf("%s Airport database loaded in %s (%d airports)\n", display.Emoji("✅", "[OK]"), time.Since(startTime), len(airports))
	return nil
}

func (as *AirportService) readAirportCache() ([]AirportData, time.Time, error) {
	if as.cachePath == "" {
		return nil, time.Time{}, os.ErrNotExist
	}
	info, err := os.Stat(as.cachePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	if info.Size() <= 0 || info.Size() > maxAirportDataBytes {
		return nil, info.ModTime(), fmt.Errorf("airport cache has invalid size %d", info.Size())
	}
	body, err := os.ReadFile(as.cachePath)
	if err != nil {
		return nil, info.ModTime(), err
	}
	var airports []AirportData
	if err := json.Unmarshal(body, &airports); err != nil {
		return nil, info.ModTime(), fmt.Errorf("parse airport cache: %w", err)
	}
	airports, err = validateCachedAirports(airports)
	if err != nil {
		return nil, info.ModTime(), fmt.Errorf("validate airport cache: %w", err)
	}
	return airports, info.ModTime(), nil
}

func validateCachedAirports(airports []AirportData) ([]AirportData, error) {
	valid := make([]AirportData, 0, len(airports))
	for _, airport := range airports {
		airport.ICAO = strings.ToUpper(strings.TrimSpace(airport.ICAO))
		airport.IATA = strings.ToUpper(strings.TrimSpace(airport.IATA))
		if !validAirportCoordinates(airport.Lat, airport.Lon) || airport.Name == "" || (!IsAirportCode(airport.ICAO) && !IsAirportCode(airport.IATA)) {
			continue
		}
		valid = append(valid, airport)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("airport cache contains no valid airport records")
	}
	if len(valid) < minAirportRecords {
		return nil, fmt.Errorf("airport cache contains only %d valid records; expected at least %d", len(valid), minAirportRecords)
	}
	return valid, nil
}

func parseOurAirportsCSV(body []byte) ([]AirportData, error) {
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read OurAirports CSV header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	required := []string{"type", "name", "latitude_deg", "longitude_deg", "elevation_ft", "iso_country", "iso_region", "municipality", "icao_code", "iata_code", "scheduled_service"}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("OurAirports CSV is missing required column %q", name)
		}
	}
	get := func(row []string, name string) string {
		index := columns[name]
		if index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}

	airports := make([]AirportData, 0, 20000)
	line := 1
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("read OurAirports CSV row %d: %w", line, err)
		}
		airportType := get(row, "type")
		if airportType != "large_airport" && airportType != "medium_airport" && airportType != "small_airport" && airportType != "seaplane_base" {
			continue
		}
		name := get(row, "name")
		latitude, latErr := strconv.ParseFloat(get(row, "latitude_deg"), 64)
		longitude, lonErr := strconv.ParseFloat(get(row, "longitude_deg"), 64)
		if name == "" || latErr != nil || lonErr != nil || !validAirportCoordinates(latitude, longitude) {
			continue
		}
		icao := strings.ToUpper(get(row, "icao_code"))
		iata := strings.ToUpper(get(row, "iata_code"))
		if !IsAirportCode(icao) {
			icao = ""
		}
		if !IsAirportCode(iata) {
			iata = ""
		}
		if icao == "" && iata == "" {
			continue
		}
		countryCode := strings.ToUpper(get(row, "iso_country"))
		country := countryName(countryCode)
		elevation := 0
		if elevationFeet := get(row, "elevation_ft"); elevationFeet != "" {
			value, err := strconv.ParseFloat(elevationFeet, 64)
			if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= float64(-1200) && value <= float64(30000) {
				elevation = int(math.Round(value))
			}
		}
		region := get(row, "iso_region")
		if _, suffix, found := strings.Cut(region, "-"); found {
			region = suffix
		}
		scheduled := get(row, "scheduled_service") == "yes"
		airports = append(airports, AirportData{
			ICAO:        icao,
			IATA:        iata,
			Name:        name,
			City:        get(row, "municipality"),
			State:       region,
			Country:     country,
			CountryCode: countryCode,
			Type:        airportType,
			Scheduled:   scheduled,
			Elevation:   elevation,
			Lat:         latitude,
			Lon:         longitude,
		})
	}
	if len(airports) == 0 {
		return nil, fmt.Errorf("OurAirports CSV contains no valid airport records")
	}
	return airports, nil
}

func countryName(countryCode string) string {
	region, err := language.ParseRegion(countryCode)
	if err != nil {
		return countryCode
	}
	if name := cldrdisplay.Regions(language.English).Name(region); name != "" {
		return name
	}
	return countryCode
}

func validAirportCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) && !math.IsNaN(longitude) && !math.IsInf(longitude, 0) && latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 && (latitude != 0 || longitude != 0)
}

func (as *AirportService) useAirportCacheAfterFailure(cached []AirportData, cacheErr, downloadErr error) error {
	if cacheErr == nil && len(cached) > 0 {
		as.indexAirports(cached)
		log.Printf("WARNING: airport data refresh failed; using the last validated cached snapshot: %v", downloadErr)
		return fmt.Errorf("refresh airport data: %w", downloadErr)
	}
	if cacheErr != nil && !os.IsNotExist(cacheErr) {
		return fmt.Errorf("refresh airport data: %w (cached snapshot is unusable: %v)", downloadErr, cacheErr)
	}
	return fmt.Errorf("refresh airport data: %w", downloadErr)
}

func (as *AirportService) writeAirportCache(airports []AirportData) error {
	body, err := json.Marshal(airports)
	if err != nil {
		return fmt.Errorf("encode airport cache: %w", err)
	}
	dir := filepath.Dir(as.cachePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create airport cache directory: %w", err)
	}
	tempFile, err := os.CreateTemp(dir, ".airports-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary airport cache: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			log.Printf("WARNING: failed to remove temporary airport cache %q: %v", tempPath, err)
		}
	}()
	if err := tempFile.Chmod(0644); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("set airport cache permissions: %w", err)
	}
	if _, err := tempFile.Write(body); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write airport cache: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync airport cache: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close airport cache: %w", err)
	}
	if err := os.Rename(tempPath, as.cachePath); err != nil {
		return fmt.Errorf("replace airport cache: %w", err)
	}
	return nil
}

func (as *AirportService) indexAirports(airports []AirportData) {
	icaoIndex := make(map[string]*AirportData, len(airports))
	iataIndex := make(map[string]*AirportData, len(airports))
	for index := range airports {
		airport := airports[index]
		if !validAirportCoordinates(airport.Lat, airport.Lon) {
			continue
		}
		if IsAirportCode(airport.ICAO) {
			code := strings.ToUpper(airport.ICAO)
			if current := icaoIndex[code]; current == nil || betterAirportRecord(&airport, current) {
				icaoIndex[code] = &airport
			}
		}
		if IsAirportCode(airport.IATA) {
			code := strings.ToUpper(airport.IATA)
			if current := iataIndex[code]; current == nil || betterAirportRecord(&airport, current) {
				iataIndex[code] = &airport
			}
		}
	}
	as.mu.Lock()
	as.airports = icaoIndex
	as.iataIndex = iataIndex
	as.loaded = len(icaoIndex) > 0 || len(iataIndex) > 0
	as.mu.Unlock()
}

func betterAirportRecord(candidate, current *AirportData) bool {
	if candidate.Scheduled != current.Scheduled {
		return candidate.Scheduled
	}
	typeRank := func(kind string) int {
		switch kind {
		case "large_airport":
			return 0
		case "medium_airport":
			return 1
		case "small_airport":
			return 2
		case "seaplane_base":
			return 3
		default:
			return 4
		}
	}
	if typeRank(candidate.Type) != typeRank(current.Type) {
		return typeRank(candidate.Type) < typeRank(current.Type)
	}
	if candidate.ICAO != current.ICAO {
		return candidate.ICAO < current.ICAO
	}
	return candidate.Name < current.Name
}

// LookupAirport returns coordinates for an airport code (ICAO or IATA).
func (as *AirportService) LookupAirport(code string) (*Coordinates, error) {
	as.mu.RLock()
	defer as.mu.RUnlock()
	if !as.loaded {
		return nil, fmt.Errorf("airport database not loaded yet")
	}
	codeUpper := strings.ToUpper(strings.TrimSpace(code))
	airport, exists := as.airports[codeUpper]
	if !exists {
		airport, exists = as.iataIndex[codeUpper]
		if !exists {
			return nil, fmt.Errorf("airport code %s not found", code)
		}
	}
	name := airport.Name
	if airport.City != "" {
		name = fmt.Sprintf("%s (%s)", airport.City, airport.Name)
	}
	return &Coordinates{
		Latitude:    airport.Lat,
		Longitude:   airport.Lon,
		Name:        name,
		Country:     airport.Country,
		CountryCode: airport.CountryCode,
		Admin1:      airport.State,
		Admin2:      airport.City,
		FullName:    fmt.Sprintf("%s, %s, %s", airport.Name, airport.City, airport.Country),
		ShortName:   fmt.Sprintf("%s (%s)", airport.City, code),
	}, nil
}

// IsAirportCode checks whether s has the shape of a three- or four-character
// ASCII airport code.
func IsAirportCode(s string) bool {
	if len(s) < 3 || len(s) > 4 {
		return false
	}
	for _, char := range s {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

// IsLoaded reports whether a validated airport index is available.
func (as *AirportService) IsLoaded() bool {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.loaded
}

// Reload forces a download of the current OurAirports snapshot. If the
// download fails, the last validated cache remains available for lookups.
func (as *AirportService) Reload() error {
	fmt.Printf("%s Reloading airport data...\n", display.Emoji("🔄", "->"))
	return as.loadAirportsWithRefresh(true)
}
