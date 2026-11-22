package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/display"
)

const (
	geoNamesUSZipURL       = "https://download.geonames.org/export/zip/US.zip"
	maxZipcodeArchiveBytes = 8 << 20
	maxZipcodeDataBytes    = 32 << 20
	minZipcodeRecords      = 30000
)

// ZipcodeData is one GeoNames US postal-code location. Coordinates identify
// a representative place for the ZIP code; they are not an address boundary.
type ZipcodeData struct {
	State     string  `json:"state"`
	City      string  `json:"city"`
	County    string  `json:"county"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// Accuracy is the GeoNames accuracy code; 0 means the source omitted it.
	Accuracy int `json:"accuracy"`
}

// ZipcodeService handles US ZIP-code lookups.
type ZipcodeService struct {
	zipcodes map[int]*ZipcodeData
	mu       sync.RWMutex
	loaded   bool
}

// NewZipcodeService creates a service and starts loading the daily GeoNames
// US postal-code snapshot in the background.
func NewZipcodeService() *ZipcodeService {
	zs := &ZipcodeService{zipcodes: make(map[int]*ZipcodeData)}
	go func() {
		if err := zs.loadZipcodes(); err != nil {
			log.Printf("WARNING: US ZIP-code data is unavailable: %v", err)
		}
	}()
	return zs
}

// loadZipcodes downloads, validates, and atomically replaces the current
// index. A failed refresh leaves a previously loaded index intact.
func (zs *ZipcodeService) loadZipcodes() error {
	startTime := time.Now()
	fmt.Printf("%s Loading US ZIP-code database...\n", display.Emoji("📮", "->"))

	resp, err := downloadClient.Get(geoNamesUSZipURL)
	if err != nil {
		return fmt.Errorf("download GeoNames US postal codes: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("WARNING: failed to close GeoNames ZIP response: %v", err)
		}
	}()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GeoNames US postal-code download returned HTTP %d", resp.StatusCode)
	}

	archiveBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxZipcodeArchiveBytes+1))
	if err != nil {
		return fmt.Errorf("read GeoNames US postal-code archive: %w", err)
	}
	if len(archiveBytes) > maxZipcodeArchiveBytes {
		return fmt.Errorf("GeoNames US postal-code archive exceeds %d bytes", maxZipcodeArchiveBytes)
	}
	archive, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return fmt.Errorf("open GeoNames US postal-code archive: %w", err)
	}

	var source *zip.File
	for _, file := range archive.File {
		if file.Name == "US.txt" {
			source = file
			break
		}
	}
	if source == nil {
		return fmt.Errorf("GeoNames US postal-code archive does not contain US.txt")
	}
	dataFile, err := source.Open()
	if err != nil {
		return fmt.Errorf("open GeoNames US.txt: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(dataFile, maxZipcodeDataBytes+1))
	closeErr := dataFile.Close()
	if readErr != nil {
		return fmt.Errorf("read GeoNames US.txt: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close GeoNames US.txt: %w", closeErr)
	}
	if len(data) > maxZipcodeDataBytes {
		return fmt.Errorf("GeoNames US.txt exceeds %d bytes", maxZipcodeDataBytes)
	}

	zipcodes, err := parseGeoNamesUSZipcodes(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if len(zipcodes) < minZipcodeRecords {
		return fmt.Errorf("GeoNames US postal-code data contains only %d valid ZIP codes; expected at least %d", len(zipcodes), minZipcodeRecords)
	}

	zs.mu.Lock()
	zs.zipcodes = zipcodes
	zs.loaded = true
	zs.mu.Unlock()
	fmt.Printf("%s ZIP-code database loaded in %s (%d ZIP codes)\n", display.Emoji("✅", "[OK]"), time.Since(startTime), len(zipcodes))
	return nil
}

// parseGeoNamesUSZipcodes validates the tab-separated US postal-code file and
// resolves duplicate ZIPs by retaining the record with the best GeoNames
// accuracy, then the most complete place metadata, then a stable lexical tie.
func parseGeoNamesUSZipcodes(reader io.Reader) (map[int]*ZipcodeData, error) {
	zipcodes := make(map[int]*ZipcodeData)
	scanner := bufio.NewScanner(io.LimitReader(reader, maxZipcodeDataBytes+1))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 12 || fields[0] != "US" {
			continue
		}
		postalCode := fields[1]
		if !IsZipcode(postalCode) {
			continue
		}
		city := strings.TrimSpace(fields[2])
		state := strings.TrimSpace(fields[4])
		county := strings.TrimSpace(fields[5])
		latitude, latErr := strconv.ParseFloat(fields[9], 64)
		longitude, lonErr := strconv.ParseFloat(fields[10], 64)
		if latErr != nil || lonErr != nil || math.IsNaN(latitude) || math.IsInf(latitude, 0) || math.IsNaN(longitude) || math.IsInf(longitude, 0) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 || latitude == 0 && longitude == 0 || city == "" {
			continue
		}
		accuracy := 0
		if rawAccuracy := strings.TrimSpace(fields[11]); rawAccuracy != "" {
			parsedAccuracy, err := strconv.Atoi(rawAccuracy)
			if err != nil || (parsedAccuracy != 1 && parsedAccuracy != 4 && parsedAccuracy != 6) {
				continue
			}
			accuracy = parsedAccuracy
		}
		zipNumber, _ := strconv.Atoi(postalCode)
		candidate := &ZipcodeData{
			State:     state,
			City:      city,
			County:    county,
			ZipCode:   postalCode,
			Latitude:  latitude,
			Longitude: longitude,
			Accuracy:  accuracy,
		}
		if current := zipcodes[zipNumber]; current == nil || betterZipcodeRecord(candidate, current) {
			zipcodes[zipNumber] = candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse GeoNames US postal-code rows: %w", err)
	}
	if len(zipcodes) == 0 {
		return nil, fmt.Errorf("GeoNames US postal-code file contains no valid entries")
	}
	return zipcodes, nil
}

func betterZipcodeRecord(candidate, current *ZipcodeData) bool {
	if candidate.Accuracy != current.Accuracy {
		return candidate.Accuracy > current.Accuracy
	}
	candidateFields := nonEmptyZipcodeFields(candidate)
	currentFields := nonEmptyZipcodeFields(current)
	if candidateFields != currentFields {
		return candidateFields > currentFields
	}
	if candidate.City != current.City {
		return candidate.City < current.City
	}
	if candidate.State != current.State {
		return candidate.State < current.State
	}
	return candidate.County < current.County
}

func nonEmptyZipcodeFields(zipcode *ZipcodeData) int {
	count := 0
	if zipcode.City != "" {
		count++
	}
	if zipcode.State != "" {
		count++
	}
	if zipcode.County != "" {
		count++
	}
	return count
}

// LookupZipcode returns a representative location for a US ZIP code.
func (zs *ZipcodeService) LookupZipcode(zipcode int) (*Coordinates, error) {
	zs.mu.RLock()
	defer zs.mu.RUnlock()

	if !zs.loaded {
		return nil, fmt.Errorf("ZIP-code database not loaded yet")
	}
	if zipcode < 0 || zipcode > 99999 {
		return nil, fmt.Errorf("ZIP code %d is invalid", zipcode)
	}
	zc, exists := zs.zipcodes[zipcode]
	if !exists {
		return nil, fmt.Errorf("ZIP code %05d not found", zipcode)
	}

	return &Coordinates{
		Latitude:    zc.Latitude,
		Longitude:   zc.Longitude,
		Name:        zc.City,
		Country:     "United States",
		CountryCode: "US",
		Admin1:      zc.State,
		Admin2:      zc.County,
		FullName:    fmt.Sprintf("%s, %s, United States", zc.City, zc.State),
		ShortName:   fmt.Sprintf("%s, %s", zc.City, zc.State),
	}, nil
}

// IsZipcode reports whether s is exactly five ASCII digits.
func IsZipcode(s string) bool {
	if len(s) != 5 {
		return false
	}
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// ParseZipcode parses a five-digit US ZIP code to its numeric lookup key.
func ParseZipcode(s string) (int, error) {
	if !IsZipcode(s) {
		return 0, fmt.Errorf("invalid five-digit US ZIP code %q", s)
	}
	return strconv.Atoi(s)
}

// IsLoaded reports whether the service has a validated dataset.
func (zs *ZipcodeService) IsLoaded() bool {
	zs.mu.RLock()
	defer zs.mu.RUnlock()
	return zs.loaded
}

// Reload downloads and validates the current GeoNames US postal-code snapshot.
func (zs *ZipcodeService) Reload() error {
	fmt.Printf("%s Reloading US ZIP-code database...\n", display.Emoji("🔄", "->"))
	startTime := time.Now()
	if err := zs.loadZipcodes(); err != nil {
		return fmt.Errorf("reload ZIP-code database: %w", err)
	}
	zs.mu.RLock()
	count := len(zs.zipcodes)
	zs.mu.RUnlock()
	fmt.Printf("%s ZIP-code database reloaded in %s (%d entries)\n", display.Emoji("✅", "[OK]"), time.Since(startTime), count)
	return nil
}
