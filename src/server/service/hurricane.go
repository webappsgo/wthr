package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HurricaneService handles hurricane/cyclone tracking
type HurricaneService struct {
	cache      map[string]*HurricaneData
	cacheMutex sync.RWMutex
	cacheTime  time.Time
	cacheTTL   time.Duration
}

// HurricaneData represents hurricane information
type HurricaneData struct {
	ActiveStorms []Storm `json:"activeStorms"`
}

// Storm represents a tropical storm/hurricane
type Storm struct {
	ID               string  `json:"id"`
	BinNumber        int     `json:"binNumber"`
	Name             string  `json:"name"`
	Classification   string  `json:"classification"`
	Intensity        string  `json:"intensity"`
	Pressure         int     `json:"pressure"`
	WindSpeed        int     `json:"windSpeed"`
	MovementSpeed    int     `json:"movementSpeed"`
	MovementDir      string  `json:"movementDir"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	LastUpdate       string  `json:"lastUpdate"`
	Basin            string  `json:"basin"`
	DistanceMiles    float64 `json:"distanceMiles,omitempty"`
	PublicAdvisory   string  `json:"publicAdvisory,omitempty"`
	ForecastAdvisory string  `json:"forecastAdvisory,omitempty"`
	DiscussionLink   string  `json:"discussionLink,omitempty"`
}

// NewHurricaneService creates a new hurricane service
func NewHurricaneService() *HurricaneService {
	return &HurricaneService{
		cache: make(map[string]*HurricaneData),
		// Cache for 15 minutes per IDEA.md
		cacheTTL: 15 * time.Minute,
	}
}

// GetActiveStorms fetches all active tropical storms and hurricanes
func (s *HurricaneService) GetActiveStorms() (*HurricaneData, error) {
	s.cacheMutex.RLock()
	if time.Since(s.cacheTime) < s.cacheTTL && s.cache["active"] != nil {
		data := s.cache["active"]
		s.cacheMutex.RUnlock()
		return data, nil
	}
	s.cacheMutex.RUnlock()

	// Fetch from NOAA NHC
	atlanticStorms, err := s.fetchNOAAStorms("https://www.nhc.noaa.gov/CurrentStorms.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Atlantic storms: %w", err)
	}

	// Combine all storms
	allStorms := &HurricaneData{
		ActiveStorms: atlanticStorms,
	}

	// Cache the result
	s.cacheMutex.Lock()
	s.cache["active"] = allStorms
	s.cacheTime = time.Now()
	s.cacheMutex.Unlock()

	return allStorms, nil
}

// fetchNOAAStorms fetches storm data from NOAA API
func (s *HurricaneService) fetchNOAAStorms(url string) ([]Storm, error) {
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
	for _, s := range noaaData.ActiveStorms {
		intensity := s.Classification
		if s.IntensityMPH > 0 {
			intensity = fmt.Sprintf("%s (%d mph)", s.Classification, s.IntensityMPH)
		}

		storm := Storm{
			ID:               s.ID,
			BinNumber:        s.BinNumber,
			Name:             s.Name,
			Classification:   s.Classification,
			Intensity:        intensity,
			Pressure:         s.PressureMB,
			WindSpeed:        s.IntensityMPH,
			MovementSpeed:    s.MovementSpeed,
			MovementDir:      s.MovementDir,
			Latitude:         s.Latitude,
			Longitude:        s.Longitude,
			LastUpdate:       s.LastUpdate,
			Basin:            "Atlantic",
			PublicAdvisory:   s.PublicAdvisory,
			ForecastAdvisory: s.ForecastAdvisory,
			DiscussionLink:   s.DiscussionLink,
		}
		storms = append(storms, storm)
	}

	return storms, nil
}

// GetStormCategory returns the Saffir-Simpson category for a storm
func (s *HurricaneService) GetStormCategory(windSpeed int) string {
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
func (s *HurricaneService) GetStormIcon(classification string, windSpeed int) string {
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
