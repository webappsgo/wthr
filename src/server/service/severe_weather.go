package service

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SevereWeatherService handles all types of severe weather tracking
type SevereWeatherService struct {
	cache      map[string]*SevereWeatherData
	cacheMutex sync.RWMutex
	cacheTime  time.Time
	cacheTTL   time.Duration
}

// SevereWeatherData represents all severe weather information
type SevereWeatherData struct {
	Hurricanes      []Storm `json:"hurricanes"`
	TornadoWarnings []Alert `json:"tornadoWarnings"`
	SevereStorms    []Alert `json:"severeStorms"`
	WinterStorms    []Alert `json:"winterStorms"`
	FloodWarnings   []Alert `json:"floodWarnings"`
	OtherAlerts     []Alert `json:"otherAlerts"`
	LastUpdate      string  `json:"lastUpdate"`
}

// Alert represents a severe weather alert
type Alert struct {
	ID            string                 `json:"id"`
	Event         string                 `json:"event"`
	Headline      string                 `json:"headline"`
	Description   string                 `json:"description"`
	// Extreme, Severe, Moderate, Minor
	Severity      string                 `json:"severity"`
	// Immediate, Expected, Future
	Urgency       string                 `json:"urgency"`
	// Actual, Exercise, Test
	Status        string                 `json:"status"`
	// Alert, Update, Cancel
	MessageType   string                 `json:"messageType"`
	// Met (Meteorological)
	Category      string                 `json:"category"`
	AreaDesc      string                 `json:"areaDesc"`
	Sent          string                 `json:"sent"`
	Effective     string                 `json:"effective"`
	Expires       string                 `json:"expires"`
	SenderName    string                 `json:"senderName"`
	Instruction   string                 `json:"instruction"`
	Response      string                 `json:"response"`
	Parameters    map[string]interface{} `json:"parameters"`
	Geometry      interface{}            `json:"geometry,omitempty"`
	DistanceMiles float64                `json:"distanceMiles,omitempty"`
}

// NewSevereWeatherService creates a new severe weather service
func NewSevereWeatherService() *SevereWeatherService {
	// Cache for 5 minutes
	return &SevereWeatherService{
		cache:    make(map[string]*SevereWeatherData),
		cacheTTL: 5 * time.Minute,
	}
}

// GetSevereWeather fetches all severe weather data with default 50-mile filter
func (s *SevereWeatherService) GetSevereWeather(latitude, longitude float64) (*SevereWeatherData, error) {
	return s.GetSevereWeatherWithDistance(latitude, longitude, 50)
}

// GetSevereWeatherWithDistance fetches all severe weather data with custom distance filter
func (s *SevereWeatherService) GetSevereWeatherWithDistance(latitude, longitude, maxMiles float64) (*SevereWeatherData, error) {
	cacheKey := "all"
	if latitude != 0 && longitude != 0 {
		cacheKey = fmt.Sprintf("%.4f,%.4f,%.0f", latitude, longitude, maxMiles)
	}

	s.cacheMutex.RLock()
	if time.Since(s.cacheTime) < s.cacheTTL && s.cache[cacheKey] != nil {
		data := s.cache[cacheKey]
		s.cacheMutex.RUnlock()
		return data, nil
	}
	s.cacheMutex.RUnlock()

	// Fetch hurricanes from NOAA NHC (global)
	hurricanes, err := s.fetchNOAAStorms("https://www.nhc.noaa.gov/CurrentStorms.json")
	if err != nil {
		hurricanes = []Storm{}
	}

	// Fetch alerts based on location
	alerts := []Alert{}

	// Determine country based on coordinates
	countryCode := s.getCountryFromCoordinates(latitude, longitude)

	switch countryCode {
	case "US":
		// US: National Weather Service
		nwsAlerts, err := s.fetchNWSAlerts()
		if err == nil {
			alerts = append(alerts, nwsAlerts...)
		}
	case "CA":
		// Canada: Environment Canada alerts
		caAlerts, err := s.fetchEnvironmentCanadaAlerts()
		if err == nil {
			alerts = append(alerts, caAlerts...)
		}
	case "GB", "UK":
		// UK: Met Office alerts
		ukAlerts, err := s.fetchMetOfficeAlerts()
		if err == nil {
			alerts = append(alerts, ukAlerts...)
		}
	case "AU":
		// Australia: Bureau of Meteorology
		auAlerts, err := s.fetchAustraliaAlerts()
		if err == nil {
			alerts = append(alerts, auAlerts...)
		}
	case "JP":
		// Japan: JMA (Japan Meteorological Agency)
		jpAlerts, err := s.fetchJapanAlerts()
		if err == nil {
			alerts = append(alerts, jpAlerts...)
		}
	case "MX":
		// Mexico: CONAGUA (National Water Commission)
		mxAlerts, err := s.fetchMexicoAlerts()
		if err == nil {
			alerts = append(alerts, mxAlerts...)
		}
	default:
		// For other countries, fetch from US NWS (might have some global coverage)
		nwsAlerts, err := s.fetchNWSAlerts()
		if err == nil {
			alerts = append(alerts, nwsAlerts...)
		}
	}

	// Filter by location if coordinates provided
	if latitude != 0 && longitude != 0 && maxMiles > 0 {
		fmt.Printf("📍 Filtering severe weather for location: %.4f, %.4f within %.0f miles\n", latitude, longitude, maxMiles)
		// Use maxMiles for both storms and alerts (user-configurable)
		// 10x distance for storms (they're bigger/more relevant)
		hurricanes = s.filterStormsByDistance(hurricanes, latitude, longitude, maxMiles*10)
		alerts = s.filterAlertsByDistance(alerts, latitude, longitude, maxMiles)
	} else if maxMiles == 0 {
		fmt.Printf("📡 Showing ALL alerts nationwide (distance filter: 0)\n")
	} else {
		fmt.Printf("⚠️ No location provided (lat: %.4f, lon: %.4f) - showing ALL alerts nationwide\n", latitude, longitude)
	}

	// Categorize alerts
	tornadoWarnings := []Alert{}
	severeStorms := []Alert{}
	winterStorms := []Alert{}
	floodWarnings := []Alert{}
	otherAlerts := []Alert{}

	for _, alert := range alerts {
		event := strings.ToLower(alert.Event)
		if strings.Contains(event, "tornado") {
			tornadoWarnings = append(tornadoWarnings, alert)
		} else if strings.Contains(event, "severe thunderstorm") || strings.Contains(event, "severe weather") {
			severeStorms = append(severeStorms, alert)
		} else if strings.Contains(event, "winter") || strings.Contains(event, "snow") || strings.Contains(event, "ice") || strings.Contains(event, "blizzard") {
			winterStorms = append(winterStorms, alert)
		} else if strings.Contains(event, "flood") || strings.Contains(event, "flash flood") {
			floodWarnings = append(floodWarnings, alert)
		} else {
			otherAlerts = append(otherAlerts, alert)
		}
	}

	// Combine all data
	data := &SevereWeatherData{
		Hurricanes:      hurricanes,
		TornadoWarnings: tornadoWarnings,
		SevereStorms:    severeStorms,
		WinterStorms:    winterStorms,
		FloodWarnings:   floodWarnings,
		OtherAlerts:     otherAlerts,
		LastUpdate:      time.Now().Format(time.RFC3339),
	}

	// Cache the result
	s.cacheMutex.Lock()
	s.cache[cacheKey] = data
	s.cacheTime = time.Now()
	s.cacheMutex.Unlock()

	return data, nil
}

// fetchNOAAStorms fetches storm data from NOAA API
func (s *SevereWeatherService) fetchNOAAStorms(url string) ([]Storm, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NOAA API returned status %d", resp.StatusCode)
	}

	var noaaData struct {
		ActiveStorms []struct {
			ID               string  `json:"id"`
			BinNumber        int     `json:"binNumber"`
			Name             string  `json:"name"`
			Classification   string  `json:"classification"`
			IntensityMPH     int     `json:"intensityMPH"`
			PressureMB       int     `json:"pressureMB"`
			MovementSpeed    int     `json:"movementSpeed"`
			MovementDir      string  `json:"movementDir"`
			Latitude         float64 `json:"latitude"`
			Longitude        float64 `json:"longitude"`
			LastUpdate       string  `json:"lastUpdate"`
			PublicAdvisory   string  `json:"publicAdvisory"`
			ForecastAdvisory string  `json:"forecastAdvisory"`
			DiscussionLink   string  `json:"discussionLink"`
		} `json:"activeStorms"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&noaaData); err != nil {
		// If JSON parsing fails, might be no active storms
		return []Storm{}, nil
	}

	storms := make([]Storm, 0, len(noaaData.ActiveStorms))
	for _, st := range noaaData.ActiveStorms {
		intensity := st.Classification
		if st.IntensityMPH > 0 {
			intensity = fmt.Sprintf("%s (%d mph)", st.Classification, st.IntensityMPH)
		}

		storm := Storm{
			ID:               st.ID,
			BinNumber:        st.BinNumber,
			Name:             st.Name,
			Classification:   st.Classification,
			Intensity:        intensity,
			Pressure:         st.PressureMB,
			WindSpeed:        st.IntensityMPH,
			MovementSpeed:    st.MovementSpeed,
			MovementDir:      st.MovementDir,
			Latitude:         st.Latitude,
			Longitude:        st.Longitude,
			LastUpdate:       st.LastUpdate,
			Basin:            "Atlantic",
			PublicAdvisory:   st.PublicAdvisory,
			ForecastAdvisory: st.ForecastAdvisory,
			DiscussionLink:   st.DiscussionLink,
		}
		storms = append(storms, storm)
	}

	return storms, nil
}

// fetchNWSAlerts fetches alerts from National Weather Service API
func (s *SevereWeatherService) fetchNWSAlerts() ([]Alert, error) {
	// NWS API endpoint for active alerts
	url := "https://api.weather.gov/alerts/active?status=actual&message_type=alert"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// NWS requires a User-Agent header
	req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")
	req.Header.Set("Accept", "application/geo+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NWS API returned status %d", resp.StatusCode)
	}

	var nwsData struct {
		Features []struct {
			ID         string `json:"id"`
			Properties struct {
				Event       string                 `json:"event"`
				Headline    string                 `json:"headline"`
				Description string                 `json:"description"`
				Severity    string                 `json:"severity"`
				Urgency     string                 `json:"urgency"`
				Status      string                 `json:"status"`
				MessageType string                 `json:"messageType"`
				Category    string                 `json:"category"`
				AreaDesc    string                 `json:"areaDesc"`
				Sent        string                 `json:"sent"`
				Effective   string                 `json:"effective"`
				Expires     string                 `json:"expires"`
				SenderName  string                 `json:"senderName"`
				Instruction string                 `json:"instruction"`
				Response    string                 `json:"response"`
				Parameters  map[string]interface{} `json:"parameters"`
			} `json:"properties"`
			Geometry interface{} `json:"geometry"`
		} `json:"features"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&nwsData); err != nil {
		return []Alert{}, nil
	}

	alerts := make([]Alert, 0, len(nwsData.Features))
	for _, feature := range nwsData.Features {
		alert := Alert{
			ID:          feature.ID,
			Event:       feature.Properties.Event,
			Headline:    feature.Properties.Headline,
			Description: feature.Properties.Description,
			Severity:    feature.Properties.Severity,
			Urgency:     feature.Properties.Urgency,
			Status:      feature.Properties.Status,
			MessageType: feature.Properties.MessageType,
			Category:    feature.Properties.Category,
			AreaDesc:    feature.Properties.AreaDesc,
			Sent:        feature.Properties.Sent,
			Effective:   feature.Properties.Effective,
			Expires:     feature.Properties.Expires,
			SenderName:  feature.Properties.SenderName,
			Instruction: feature.Properties.Instruction,
			Response:    feature.Properties.Response,
			Parameters:  feature.Properties.Parameters,
			Geometry:    feature.Geometry,
		}
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// filterStormsByDistance filters storms by distance from a location
func (s *SevereWeatherService) filterStormsByDistance(storms []Storm, lat, lon, maxMiles float64) []Storm {
	filtered := []Storm{}
	for _, storm := range storms {
		distance := calculateDistance(lat, lon, storm.Latitude, storm.Longitude)
		if distance <= maxMiles {
			storm.DistanceMiles = distance
			filtered = append(filtered, storm)
		}
	}
	return filtered
}

// filterAlertsByDistance filters alerts by distance from a location
func (s *SevereWeatherService) filterAlertsByDistance(alerts []Alert, lat, lon, maxMiles float64) []Alert {
	filtered := []Alert{}

	for _, alert := range alerts {
		// Try to extract coordinates from geometry
		if alert.Geometry != nil {
			// Check if alert is within distance
			nearLocation := isAlertNearLocation(alert.Geometry, lat, lon, maxMiles)
			distance := getAlertDistance(alert.Geometry, lat, lon)

			if nearLocation {
				alert.DistanceMiles = distance
				filtered = append(filtered, alert)
			}
		}
		// If no geometry and we're filtering by location, exclude it
		// Only alerts without geometry are included when showing all alerts (no location filter)
	}

	return filtered
}

// isAlertNearLocation checks if alert geometry is within maxMiles of location
func isAlertNearLocation(geometry interface{}, lat, lon, maxMiles float64) bool {
	// Parse geometry (can be Point, Polygon, or MultiPolygon)
	geomMap, ok := geometry.(map[string]interface{})
	if !ok {
		// Include if we can't parse
		return true
	}

	geomType, ok := geomMap["type"].(string)
	if !ok {
		return true
	}

	coords, ok := geomMap["coordinates"]
	if !ok {
		return true
	}

	switch geomType {
	case "Point":
		// Single point
		if coordSlice, ok := coords.([]interface{}); ok && len(coordSlice) >= 2 {
			lon2, _ := coordSlice[0].(float64)
			lat2, _ := coordSlice[1].(float64)
			distance := calculateDistance(lat, lon, lat2, lon2)
			return distance <= maxMiles
		}

	case "Polygon", "MultiPolygon":
		// For polygons, check if any point is within range
		// This is simplified - a proper implementation would check if the location
		// is inside the polygon or near its boundary
		return checkPolygonDistance(coords, lat, lon, maxMiles)
	}

	// Include if unknown type
	return true
}

// checkPolygonDistance checks if any point in polygon is within maxMiles
func checkPolygonDistance(coords interface{}, lat, lon, maxMiles float64) bool {
	// Recursive function to handle nested coordinate arrays
	switch v := coords.(type) {
	case []interface{}:
		for _, item := range v {
			if checkPolygonDistance(item, lat, lon, maxMiles) {
				return true
			}
		}
	case float64:
		// Leaf node, not useful alone
		return false
	}

	// Try to parse as coordinate pair
	if coordSlice, ok := coords.([]interface{}); ok && len(coordSlice) >= 2 {
		if lon2, ok := coordSlice[0].(float64); ok {
			if lat2, ok := coordSlice[1].(float64); ok {
				distance := calculateDistance(lat, lon, lat2, lon2)
				if distance <= maxMiles {
					return true
				}
			}
		}
	}

	return false
}

// getAlertDistance gets approximate distance from alert to location
func getAlertDistance(geometry interface{}, lat, lon float64) float64 {
	geomMap, ok := geometry.(map[string]interface{})
	if !ok {
		return 0
	}

	coords, ok := geomMap["coordinates"]
	if !ok {
		return 0
	}

	geomType, _ := geomMap["type"].(string)

	if geomType == "Point" {
		if coordSlice, ok := coords.([]interface{}); ok && len(coordSlice) >= 2 {
			lon2, _ := coordSlice[0].(float64)
			lat2, _ := coordSlice[1].(float64)
			return calculateDistance(lat, lon, lat2, lon2)
		}
	}

	// For polygons, find closest point (simplified)
	minDist := 9999.0
	findMinDistance(coords, lat, lon, &minDist)
	return minDist
}

// findMinDistance recursively finds minimum distance in coordinate structure
func findMinDistance(coords interface{}, lat, lon float64, minDist *float64) {
	switch v := coords.(type) {
	case []interface{}:
		// Check if this is a coordinate pair
		if len(v) >= 2 {
			if lon2, ok := v[0].(float64); ok {
				if lat2, ok := v[1].(float64); ok {
					dist := calculateDistance(lat, lon, lat2, lon2)
					if dist < *minDist {
						*minDist = dist
					}
					return
				}
			}
		}
		// Otherwise recurse
		for _, item := range v {
			findMinDistance(item, lat, lon, minDist)
		}
	}
}

// calculateDistance calculates distance in miles using Haversine formula
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMiles = 3959.0

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMiles * c
}

// GetStormCategory returns the Saffir-Simpson category for a storm
func (s *SevereWeatherService) GetStormCategory(windSpeed int) string {
	if windSpeed < 39 {
		return "Tropical Depression"
	} else if windSpeed < 74 {
		return "Tropical Storm"
	} else if windSpeed < 96 {
		return "Category 1 Hurricane"
	} else if windSpeed < 111 {
		return "Category 2 Hurricane"
	} else if windSpeed < 130 {
		return "Category 3 Hurricane (Major)"
	} else if windSpeed < 157 {
		return "Category 4 Hurricane (Major)"
	} else {
		return "Category 5 Hurricane (Major)"
	}
}

// GetStormIcon returns an emoji icon for the storm
func (s *SevereWeatherService) GetStormIcon(classification string, windSpeed int) string {
	if windSpeed >= 74 {
		// Hurricane
		return "🌀"
	} else if windSpeed >= 39 {
		// Tropical storm
		return "🌊"
	} else {
		// Tropical depression
		return "🌧️"
	}
}

// GetAlertIcon returns an emoji icon for an alert type
func (s *SevereWeatherService) GetAlertIcon(event string) string {
	eventLower := strings.ToLower(event)
	if strings.Contains(eventLower, "tornado") {
		return "🌪️"
	} else if strings.Contains(eventLower, "severe thunderstorm") {
		return "⛈️"
	} else if strings.Contains(eventLower, "winter") || strings.Contains(eventLower, "snow") || strings.Contains(eventLower, "blizzard") {
		return "❄️"
	} else if strings.Contains(eventLower, "flood") {
		return "🌊"
	} else if strings.Contains(eventLower, "hurricane") {
		return "🌀"
	} else {
		return "⚠️"
	}
}

// getCountryFromCoordinates determines country code from coordinates (simplified)
func (s *SevereWeatherService) getCountryFromCoordinates(lat, lon float64) string {
	// If no coordinates, default to US
	if lat == 0 && lon == 0 {
		return "US"
	}

	// Simple geographic bounds (approximate)
	// US (including territories): lat 25-50, lon -125 to -65
	if lat >= 25 && lat <= 50 && lon >= -125 && lon <= -65 {
		return "US"
	}

	// Alaska: lat 50-72, lon -180 to -130
	if lat >= 50 && lat <= 72 && lon >= -180 && lon <= -130 {
		return "US"
	}

	// Hawaii: lat 18-23, lon -161 to -154
	if lat >= 18 && lat <= 23 && lon >= -161 && lon <= -154 {
		return "US"
	}

	// Canada: lat 42-84, lon -141 to -52
	if lat >= 42 && lat <= 84 && lon >= -141 && lon <= -52 {
		return "CA"
	}

	// UK: lat 49-61, lon -8 to 2
	if lat >= 49 && lat <= 61 && lon >= -8 && lon <= 2 {
		return "GB"
	}

	// Mexico: lat 14-33, lon -118 to -86
	if lat >= 14 && lat <= 33 && lon >= -118 && lon <= -86 {
		return "MX"
	}

	// Australia: lat -44 to -10, lon 113 to 154
	if lat >= -44 && lat <= -10 && lon >= 113 && lon <= 154 {
		return "AU"
	}

	// Japan: lat 24-46, lon 122-146
	if lat >= 24 && lat <= 46 && lon >= 122 && lon <= 146 {
		return "JP"
	}

	// Default to US NWS (has some international coverage)
	return "US"
}

// fetchEnvironmentCanadaAlerts fetches alerts from Environment Canada
func (s *SevereWeatherService) fetchEnvironmentCanadaAlerts() ([]Alert, error) {
	// Environment Canada national alert feed
	// Using multiple provincial feeds for comprehensive coverage
	// Alberta
	// British Columbia
	// Manitoba
	// New Brunswick
	// Newfoundland and Labrador
	// Nova Scotia
	// Ontario
	// Prince Edward Island
	// Quebec
	// Saskatchewan
	provinces := []string{
		"ab-30_e.xml",
		"bc-31_e.xml",
		"mb-38_e.xml",
		"nb-34_e.xml",
		"nl-24_e.xml",
		"ns-19_e.xml",
		"on-82_e.xml",
		"pe-11_e.xml",
		"qc-67_e.xml",
		"sk-32_e.xml",
	}

	allAlerts := []Alert{}

	for _, province := range provinces {
		url := fmt.Sprintf("https://weather.gc.ca/rss/warning/%s", province)

		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			// Skip this province on error
			continue
		}

		req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")

		resp, err := client.Do(req)
		if err != nil {
			// Skip this province on error
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		// Parse RSS/Atom feed
		var feed struct {
			XMLName xml.Name `xml:"feed"`
			Entries []struct {
				ID      string `xml:"id"`
				Title   string `xml:"title"`
				Summary string `xml:"summary"`
				Updated string `xml:"updated"`
				Link    struct {
					Href string `xml:"href,attr"`
				} `xml:"link"`
				Category struct {
					Term string `xml:"term,attr"`
				} `xml:"category"`
			} `xml:"entry"`
		}

		if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Convert entries to alerts
		for _, entry := range feed.Entries {
			// Parse severity from title
			severity := "Moderate"
			urgency := "Expected"
			if strings.Contains(strings.ToLower(entry.Title), "warning") {
				severity = "Severe"
				urgency = "Immediate"
			} else if strings.Contains(strings.ToLower(entry.Title), "watch") {
				severity = "Moderate"
				urgency = "Future"
			}

			alert := Alert{
				ID:          entry.ID,
				Event:       entry.Title,
				Headline:    entry.Title,
				Description: entry.Summary,
				Severity:    severity,
				Urgency:     urgency,
				Status:      "Actual",
				MessageType: "Alert",
				Category:    "Met",
				AreaDesc:    extractAreaFromTitle(entry.Title),
				Sent:        entry.Updated,
				Effective:   entry.Updated,
				// Not always provided in feed
				Expires:     "",
				SenderName:  "Environment Canada",
				Instruction: "",
			}
			allAlerts = append(allAlerts, alert)
		}
	}

	fmt.Printf("✅ Environment Canada: Fetched %d alerts\n", len(allAlerts))
	return allAlerts, nil
}

// extractAreaFromTitle extracts area description from alert title
func extractAreaFromTitle(title string) string {
	// Typically format: "Alert Type issued for Area"
	parts := strings.Split(title, " for ")
	if len(parts) > 1 {
		return parts[1]
	}
	parts = strings.Split(title, " in effect for ")
	if len(parts) > 1 {
		return parts[1]
	}
	return "Multiple areas"
}

// fetchMetOfficeAlerts fetches alerts from UK Met Office
func (s *SevereWeatherService) fetchMetOfficeAlerts() ([]Alert, error) {
	// UK Met Office warnings RSS feed - national warnings
	url := "https://www.metoffice.gov.uk/public/data/PWSCache/WarningsRSS/Region/UK"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("⚠️  UK Met Office: Request error: %v\n", err)
		return []Alert{}, nil
	}

	req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  UK Met Office: Connection error: %v\n", err)
		return []Alert{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("⚠️  UK Met Office: HTTP %d\n", resp.StatusCode)
		return []Alert{}, nil
	}

	// Parse RSS feed
	var rss struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Description string `xml:"description"`
				Link        string `xml:"link"`
				PubDate     string `xml:"pubDate"`
				GUID        string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		fmt.Printf("⚠️  UK Met Office: Parse error: %v\n", err)
		return []Alert{}, nil
	}

	alerts := []Alert{}
	for _, item := range rss.Channel.Items {
		// Parse severity from title (Yellow, Amber, Red)
		severity := "Moderate"
		urgency := "Expected"
		titleLower := strings.ToLower(item.Title)

		if strings.Contains(titleLower, "red warning") {
			severity = "Extreme"
			urgency = "Immediate"
		} else if strings.Contains(titleLower, "amber warning") {
			severity = "Severe"
			urgency = "Expected"
		} else if strings.Contains(titleLower, "yellow warning") {
			severity = "Moderate"
			urgency = "Future"
		}

		// Extract event type (rain, wind, snow, etc.)
		event := "Weather Warning"
		if strings.Contains(titleLower, "rain") {
			event = "Heavy Rain Warning"
		} else if strings.Contains(titleLower, "wind") {
			event = "High Wind Warning"
		} else if strings.Contains(titleLower, "snow") {
			event = "Snow Warning"
		} else if strings.Contains(titleLower, "ice") {
			event = "Ice Warning"
		} else if strings.Contains(titleLower, "fog") {
			event = "Fog Warning"
		} else if strings.Contains(titleLower, "thunderstorm") {
			event = "Thunderstorm Warning"
		}

		alert := Alert{
			ID:          item.GUID,
			Event:       event,
			Headline:    item.Title,
			Description: item.Description,
			Severity:    severity,
			Urgency:     urgency,
			Status:      "Actual",
			MessageType: "Alert",
			Category:    "Met",
			AreaDesc:    extractUKArea(item.Title),
			Sent:        item.PubDate,
			Effective:   item.PubDate,
			Expires:     "",
			SenderName:  "UK Met Office",
			Instruction: "",
		}
		alerts = append(alerts, alert)
	}

	fmt.Printf("✅ UK Met Office: Fetched %d alerts\n", len(alerts))
	return alerts, nil
}

// extractUKArea extracts region from UK Met Office warning title
func extractUKArea(title string) string {
	// Titles often include regions like "England", "Scotland", "Wales"
	if strings.Contains(title, "England") {
		return "England"
	} else if strings.Contains(title, "Scotland") {
		return "Scotland"
	} else if strings.Contains(title, "Wales") {
		return "Wales"
	} else if strings.Contains(title, "Northern Ireland") {
		return "Northern Ireland"
	}
	return "United Kingdom"
}

// fetchAustraliaAlerts fetches alerts from Australian Bureau of Meteorology
func (s *SevereWeatherService) fetchAustraliaAlerts() ([]Alert, error) {
	// Australian BOM national warnings feed
	url := "http://www.bom.gov.au/fwo/IDZ00059.warnings_national.xml"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("⚠️  Australia BOM: Request error: %v\n", err)
		return []Alert{}, nil
	}

	req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  Australia BOM: Connection error: %v\n", err)
		return []Alert{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("⚠️  Australia BOM: HTTP %d\n", resp.StatusCode)
		return []Alert{}, nil
	}

	// Parse CAP-style XML
	var capFeed struct {
		XMLName xml.Name `xml:"feed"`
		Entries []struct {
			ID      string `xml:"id"`
			Title   string `xml:"title"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
			Content string `xml:"content"`
			Link    struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&capFeed); err != nil {
		fmt.Printf("⚠️  Australia BOM: Parse error: %v\n", err)
		return []Alert{}, nil
	}

	alerts := []Alert{}
	for _, entry := range capFeed.Entries {
		// Parse severity from title
		severity := "Moderate"
		urgency := "Expected"
		titleLower := strings.ToLower(entry.Title)

		if strings.Contains(titleLower, "severe") || strings.Contains(titleLower, "extreme") {
			severity = "Severe"
			urgency = "Immediate"
		} else if strings.Contains(titleLower, "warning") {
			severity = "Moderate"
			urgency = "Expected"
		} else if strings.Contains(titleLower, "watch") {
			severity = "Moderate"
			urgency = "Future"
		}

		// Determine event type
		event := "Weather Warning"
		if strings.Contains(titleLower, "fire") {
			event = "Fire Weather Warning"
			severity = "Severe"
		} else if strings.Contains(titleLower, "cyclone") || strings.Contains(titleLower, "tropical") {
			event = "Tropical Cyclone Warning"
			severity = "Extreme"
			urgency = "Immediate"
		} else if strings.Contains(titleLower, "storm") || strings.Contains(titleLower, "thunderstorm") {
			event = "Severe Thunderstorm Warning"
		} else if strings.Contains(titleLower, "flood") {
			event = "Flood Warning"
		} else if strings.Contains(titleLower, "wind") {
			event = "Wind Warning"
		} else if strings.Contains(titleLower, "rain") {
			event = "Heavy Rain Warning"
		}

		// Use content if available, otherwise summary
		description := entry.Content
		if description == "" {
			description = entry.Summary
		}

		alert := Alert{
			ID:          entry.ID,
			Event:       event,
			Headline:    entry.Title,
			Description: description,
			Severity:    severity,
			Urgency:     urgency,
			Status:      "Actual",
			MessageType: "Alert",
			Category:    "Met",
			AreaDesc:    extractAustraliaArea(entry.Title),
			Sent:        entry.Updated,
			Effective:   entry.Updated,
			Expires:     "",
			SenderName:  "Australian Bureau of Meteorology",
			Instruction: "",
		}
		alerts = append(alerts, alert)
	}

	fmt.Printf("✅ Australia BOM: Fetched %d alerts\n", len(alerts))
	return alerts, nil
}

// extractAustraliaArea extracts state/region from BOM warning title
func extractAustraliaArea(title string) string {
	states := []string{"NSW", "QLD", "VIC", "TAS", "SA", "WA", "NT", "ACT"}
	for _, state := range states {
		if strings.Contains(strings.ToUpper(title), state) {
			return state
		}
	}
	// Try full state names
	if strings.Contains(title, "New South Wales") {
		return "NSW"
	} else if strings.Contains(title, "Queensland") {
		return "QLD"
	} else if strings.Contains(title, "Victoria") {
		return "VIC"
	} else if strings.Contains(title, "Tasmania") {
		return "TAS"
	} else if strings.Contains(title, "South Australia") {
		return "SA"
	} else if strings.Contains(title, "Western Australia") {
		return "WA"
	} else if strings.Contains(title, "Northern Territory") {
		return "NT"
	}
	return "Australia"
}

// fetchJapanAlerts fetches alerts from Japan Meteorological Agency
func (s *SevereWeatherService) fetchJapanAlerts() ([]Alert, error) {
	// JMA provides warnings via their API
	// Using the overview forecast which includes warnings
	url := "https://www.jma.go.jp/bosai/warning/data/warning.json"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("⚠️  Japan JMA: Request error: %v\n", err)
		return []Alert{}, nil
	}

	req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  Japan JMA: Connection error: %v\n", err)
		return []Alert{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("⚠️  Japan JMA: HTTP %d\n", resp.StatusCode)
		return []Alert{}, nil
	}

	// JMA warning data structure (simplified)
	var jmaData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jmaData); err != nil {
		fmt.Printf("⚠️  Japan JMA: Parse error: %v\n", err)
		return []Alert{}, nil
	}

	alerts := []Alert{}

	// Parse the warning data (structure varies by region)
	for areaCode, areaData := range jmaData {
		areaMap, ok := areaData.(map[string]interface{})
		if !ok {
			continue
		}

		// Look for warning information
		warnings, ok := areaMap["warning"].(map[string]interface{})
		if !ok {
			continue
		}

		for warningType, warningInfo := range warnings {
			warningMap, ok := warningInfo.(map[string]interface{})
			if !ok {
				continue
			}

			// Extract warning details
			status, _ := warningMap["status"].(string)
			// Skip if cancelled
			if status == "" || status == "解除" {
				continue
			}

			// Map Japanese warning types to English
			event := mapJapaneseWarningType(warningType)
			severity := mapJapaneseSeverity(warningType, status)
			urgency := "Expected"
			if strings.Contains(strings.ToLower(event), "emergency") || strings.Contains(strings.ToLower(event), "special") {
				urgency = "Immediate"
			}

			// Get area name
			areaName, _ := areaMap["name"].(string)
			if areaName == "" {
				areaName = areaCode
			}

			alert := Alert{
				ID:          fmt.Sprintf("jma-%s-%s", areaCode, warningType),
				Event:       event,
				Headline:    fmt.Sprintf("%s for %s", event, areaName),
				Description: fmt.Sprintf("Warning status: %s", status),
				Severity:    severity,
				Urgency:     urgency,
				Status:      "Actual",
				MessageType: "Alert",
				Category:    "Met",
				AreaDesc:    areaName,
				Sent:        time.Now().Format(time.RFC3339),
				Effective:   time.Now().Format(time.RFC3339),
				Expires:     "",
				SenderName:  "Japan Meteorological Agency",
				Instruction: "",
			}
			alerts = append(alerts, alert)
		}
	}

	fmt.Printf("✅ Japan JMA: Fetched %d alerts\n", len(alerts))
	return alerts, nil
}

// mapJapaneseWarningType converts JMA warning types to English
func mapJapaneseWarningType(warningType string) string {
	mappings := map[string]string{
		"大雨":  "Heavy Rain Warning",
		"洪水":  "Flood Warning",
		"暴風":  "Storm Warning",
		"暴風雪": "Blizzard Warning",
		"大雪":  "Heavy Snow Warning",
		"波浪":  "High Wave Warning",
		"高潮":  "Storm Surge Warning",
		"津波":  "Tsunami Warning",
		"地震":  "Earthquake Warning",
		"火山":  "Volcanic Warning",
		"雷":   "Thunderstorm Warning",
		"竜巻":  "Tornado Warning",
		"濃霧":  "Dense Fog Warning",
		"乾燥":  "Dry Weather Warning",
		"なだれ": "Avalanche Warning",
		"着氷":  "Ice Accretion Warning",
		"着雪":  "Snow Accretion Warning",
		"融雪":  "Snowmelt Warning",
		"低温":  "Low Temperature Warning",
		"霜":   "Frost Warning",
		"台風":  "Typhoon Warning",
	}

	if english, ok := mappings[warningType]; ok {
		return english
	}
	return fmt.Sprintf("%s Warning", warningType)
}

// mapJapaneseSeverity determines severity from warning type and status
func mapJapaneseSeverity(warningType, status string) string {
	// Special warnings (特別警報) are extreme
	if strings.Contains(status, "特別") {
		return "Extreme"
	}
	// Warnings (警報) are severe
	if strings.Contains(status, "警報") {
		return "Severe"
	}
	// Advisories (注意報) are moderate
	if strings.Contains(status, "注意報") {
		return "Moderate"
	}
	// Tsunami and earthquake are always severe or extreme
	if warningType == "津波" || warningType == "地震" {
		return "Extreme"
	}
	// Typhoon warnings are severe
	if warningType == "台風" {
		return "Severe"
	}
	return "Moderate"
}

// fetchMexicoAlerts fetches alerts from Mexican weather services
func (s *SevereWeatherService) fetchMexicoAlerts() ([]Alert, error) {
	// CONAGUA/SMN alert feed
	url := "https://smn.conagua.gob.mx/tools/RESOURCES/AlertasTempranasdePronosticos.xml"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("⚠️  Mexico CONAGUA: Request error: %v\n", err)
		return []Alert{}, nil
	}

	req.Header.Set("User-Agent", "WeatherApp/2.0 (https://github.com/casapps/wthr)")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("⚠️  Mexico CONAGUA: Connection error: %v\n", err)
		return []Alert{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("⚠️  Mexico CONAGUA: HTTP %d\n", resp.StatusCode)
		return []Alert{}, nil
	}

	// Parse XML alert feed
	// Type
	// Level (Verde/Amarillo/Naranja/Rojo)
	// State
	// Municipality
	// Title
	// Description
	// Date
	// Validity
	var alertFeed struct {
		XMLName xml.Name `xml:"alertas"`
		Alerts  []struct {
			ID          string `xml:"id"`
			Tipo        string `xml:"tipo"`
			Nivel       string `xml:"nivel"`
			Estado      string `xml:"estado"`
			Municipio   string `xml:"municipio"`
			Titulo      string `xml:"titulo"`
			Descripcion string `xml:"descripcion"`
			Fecha       string `xml:"fecha"`
			Vigencia    string `xml:"vigencia"`
		} `xml:"alerta"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&alertFeed); err != nil {
		fmt.Printf("⚠️  Mexico CONAGUA: Parse error: %v\n", err)
		return []Alert{}, nil
	}

	alerts := []Alert{}
	for _, mxAlert := range alertFeed.Alerts {
		// Map Spanish alert levels to severity
		severity := "Moderate"
		urgency := "Expected"
		nivel := strings.ToLower(mxAlert.Nivel)

		switch nivel {
		case "rojo", "red":
			severity = "Extreme"
			urgency = "Immediate"
		case "naranja", "orange":
			severity = "Severe"
			urgency = "Expected"
		case "amarillo", "yellow":
			severity = "Moderate"
			urgency = "Future"
		case "verde", "green":
			severity = "Minor"
			urgency = "Future"
		}

		// Map Spanish event types to English
		event := mapSpanishEventType(mxAlert.Tipo)

		// Build area description
		areaDesc := mxAlert.Estado
		if mxAlert.Municipio != "" {
			areaDesc = fmt.Sprintf("%s, %s", mxAlert.Municipio, mxAlert.Estado)
		}

		alert := Alert{
			ID:          mxAlert.ID,
			Event:       event,
			Headline:    mxAlert.Titulo,
			Description: mxAlert.Descripcion,
			Severity:    severity,
			Urgency:     urgency,
			Status:      "Actual",
			MessageType: "Alert",
			Category:    "Met",
			AreaDesc:    areaDesc,
			Sent:        mxAlert.Fecha,
			Effective:   mxAlert.Fecha,
			Expires:     mxAlert.Vigencia,
			SenderName:  "CONAGUA - Servicio Meteorológico Nacional",
			Instruction: "",
		}
		alerts = append(alerts, alert)
	}

	fmt.Printf("✅ Mexico CONAGUA: Fetched %d alerts\n", len(alerts))
	return alerts, nil
}

// mapSpanishEventType converts Spanish event types to English
func mapSpanishEventType(tipo string) string {
	mappings := map[string]string{
		"lluvia":            "Heavy Rain Warning",
		"tormenta":          "Thunderstorm Warning",
		"viento":            "High Wind Warning",
		"ciclón":            "Cyclone Warning",
		"huracán":           "Hurricane Warning",
		"tormenta tropical": "Tropical Storm Warning",
		"inundación":        "Flood Warning",
		"sequía":            "Drought Warning",
		"calor":             "Heat Warning",
		"frío":              "Cold Weather Warning",
		"helada":            "Freeze Warning",
		"nevada":            "Snow Warning",
		"granizada":         "Hail Warning",
		"niebla":            "Fog Warning",
		"oleaje":            "High Surf Warning",
		"marejada":          "Storm Surge Warning",
		"tornado":           "Tornado Warning",
		"incendio":          "Fire Weather Warning",
	}

	tipoLower := strings.ToLower(tipo)
	for spanish, english := range mappings {
		if strings.Contains(tipoLower, spanish) {
			return english
		}
	}
	return fmt.Sprintf("%s Warning", tipo)
}
