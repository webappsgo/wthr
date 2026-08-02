package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/patrickmn/go-cache"
)

// EarthquakeService handles earthquake data from USGS
type EarthquakeService struct {
	client     *http.Client
	cache      *cache.Cache
	usgsAPIURL string
}

// Earthquake represents a single earthquake event
type Earthquake struct {
	ID            string    `json:"id"`
	Magnitude     float64   `json:"magnitude"`
	Place         string    `json:"place"`
	Time          time.Time `json:"time"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	Depth         float64   `json:"depth"`
	Type          string    `json:"type"`
	URL           string    `json:"url"`
	Tsunami       int       `json:"tsunami"`
	Status        string    `json:"status"`
	Felt          *int      `json:"felt,omitempty"`
	CDI           *float64  `json:"cdi,omitempty"`
	MMI           *float64  `json:"mmi,omitempty"`
	MagnitudeType string    `json:"magnitudeType"`
	Network       string    `json:"network"`
	UpdatedTime   time.Time `json:"updated"`
	// Distance from user location in km (calculated at request time)
	Distance float64 `json:"distance,omitempty"`
	// Formatted distance string for display
	DistanceFmt string `json:"distanceFmt,omitempty"`
}

// EarthquakeCollection represents a collection of earthquakes
type EarthquakeCollection struct {
	Earthquakes []Earthquake `json:"earthquakes"`
	Metadata    Metadata     `json:"metadata"`
}

// Metadata represents feed metadata
type Metadata struct {
	Generated int64  `json:"generated"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Count     int    `json:"count"`
	Status    int    `json:"status"`
	API       string `json:"api"`
}

// USGSGeoJSONResponse represents USGS API response
type USGSGeoJSONResponse struct {
	Type     string `json:"type"`
	Metadata struct {
		Generated int64  `json:"generated"`
		URL       string `json:"url"`
		Title     string `json:"title"`
		Status    int    `json:"status"`
		API       string `json:"api"`
		Count     int    `json:"count"`
	} `json:"metadata"`
	Features []struct {
		Type       string `json:"type"`
		Properties struct {
			Mag     float64  `json:"mag"`
			Place   string   `json:"place"`
			Time    int64    `json:"time"`
			Updated int64    `json:"updated"`
			Tz      *int     `json:"tz"`
			URL     string   `json:"url"`
			Detail  string   `json:"detail"`
			Felt    *int     `json:"felt"`
			CDI     *float64 `json:"cdi"`
			MMI     *float64 `json:"mmi"`
			Alert   *string  `json:"alert"`
			Status  string   `json:"status"`
			Tsunami int      `json:"tsunami"`
			Sig     int      `json:"sig"`
			Net     string   `json:"net"`
			Code    string   `json:"code"`
			IDs     string   `json:"ids"`
			Sources string   `json:"sources"`
			Types   string   `json:"types"`
			NST     *int     `json:"nst"`
			Dmin    *float64 `json:"dmin"`
			RMS     float64  `json:"rms"`
			Gap     *float64 `json:"gap"`
			MagType string   `json:"magType"`
			Type    string   `json:"type"`
			Title   string   `json:"title"`
		} `json:"properties"`
		Geometry struct {
			Type string `json:"type"`
			// [longitude, latitude, depth]
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
		ID string `json:"id"`
	} `json:"features"`
	BBox []float64 `json:"bbox"`
}

// NewEarthquakeService creates a new earthquake service
func NewEarthquakeService() *EarthquakeService {
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &EarthquakeService{
		client:     client,
		cache:      cache.New(1*time.Minute, 2*time.Minute), // IDEA.md: 1 minute update frequency
		usgsAPIURL: "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary",
	}
}

// GetEarthquakes fetches earthquakes based on feed type
// feedType options: "all_hour", "all_day", "all_week", "all_month"
//
//	"1.0_hour", "1.0_day", "1.0_week", "1.0_month"
//	"2.5_hour", "2.5_day", "2.5_week", "2.5_month"
//	"4.5_hour", "4.5_day", "4.5_week", "4.5_month"
//	"significant_hour", "significant_day", "significant_week", "significant_month"
func (es *EarthquakeService) GetEarthquakes(feedType string) (*EarthquakeCollection, error) {
	cacheKey := fmt.Sprintf("earthquakes_%s", feedType)
	if cached, found := es.cache.Get(cacheKey); found {
		return cached.(*EarthquakeCollection), nil
	}

	url := fmt.Sprintf("%s/%s.geojson", es.usgsAPIURL, feedType)

	resp, err := es.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch earthquake data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("USGS API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var usgsData USGSGeoJSONResponse
	if err := json.Unmarshal(body, &usgsData); err != nil {
		return nil, fmt.Errorf("failed to parse earthquake data: %w", err)
	}

	// Convert USGS format to our format
	earthquakes := make([]Earthquake, 0, len(usgsData.Features))
	for _, feature := range usgsData.Features {
		// USGS occasionally emits features with null/empty geometry or fewer
		// than 3 coordinates; skip them rather than panic on out-of-range index.
		if len(feature.Geometry.Coordinates) < 3 {
			continue
		}
		eq := Earthquake{
			ID:            feature.ID,
			Magnitude:     feature.Properties.Mag,
			Place:         feature.Properties.Place,
			Time:          time.UnixMilli(feature.Properties.Time),
			Latitude:      feature.Geometry.Coordinates[1],
			Longitude:     feature.Geometry.Coordinates[0],
			Depth:         feature.Geometry.Coordinates[2],
			Type:          feature.Properties.Type,
			URL:           feature.Properties.URL,
			Tsunami:       feature.Properties.Tsunami,
			Status:        feature.Properties.Status,
			Felt:          feature.Properties.Felt,
			CDI:           feature.Properties.CDI,
			MMI:           feature.Properties.MMI,
			MagnitudeType: feature.Properties.MagType,
			Network:       feature.Properties.Net,
			UpdatedTime:   time.UnixMilli(feature.Properties.Updated),
		}
		earthquakes = append(earthquakes, eq)
	}

	collection := &EarthquakeCollection{
		Earthquakes: earthquakes,
		Metadata: Metadata{
			Generated: usgsData.Metadata.Generated,
			URL:       usgsData.Metadata.URL,
			Title:     usgsData.Metadata.Title,
			Count:     usgsData.Metadata.Count,
			Status:    usgsData.Metadata.Status,
			API:       usgsData.Metadata.API,
		},
	}

	es.cache.Set(cacheKey, collection, cache.DefaultExpiration)
	return collection, nil
}

// GetEarthquakesByLocation filters earthquakes within radius of location
// and calculates distance from user location for each earthquake
func (es *EarthquakeService) GetEarthquakesByLocation(latitude, longitude, radiusKm float64, feedType string) (*EarthquakeCollection, error) {
	allEarthquakes, err := es.GetEarthquakes(feedType)
	if err != nil {
		return nil, err
	}

	// Filter earthquakes within radius and calculate distance
	filtered := make([]Earthquake, 0)
	for _, eq := range allEarthquakes.Earthquakes {
		distance := haversineDistance(latitude, longitude, eq.Latitude, eq.Longitude)
		if distance <= radiusKm {
			// Store distance for sorting and display
			eq.Distance = distance
			eq.DistanceFmt = formatDistanceKm(distance)
			filtered = append(filtered, eq)
		}
	}

	return &EarthquakeCollection{
		Earthquakes: filtered,
		Metadata:    allEarthquakes.Metadata,
	}, nil
}

// formatDistanceKm formats distance in a human-readable way
func formatDistanceKm(km float64) string {
	if km < 1 {
		return fmt.Sprintf("%.0f m", km*1000)
	} else if km < 10 {
		return fmt.Sprintf("%.1f km", km)
	}
	return fmt.Sprintf("%.0f km", km)
}

// SortAndLimit sorts and limits earthquake collection
func (ec *EarthquakeCollection) SortAndLimit(sortBy string, limit int) {
	// Sort earthquakes
	switch sortBy {
	case "magnitude":
		// Sort by magnitude descending
		for i := 0; i < len(ec.Earthquakes)-1; i++ {
			for j := i + 1; j < len(ec.Earthquakes); j++ {
				if ec.Earthquakes[i].Magnitude < ec.Earthquakes[j].Magnitude {
					ec.Earthquakes[i], ec.Earthquakes[j] = ec.Earthquakes[j], ec.Earthquakes[i]
				}
			}
		}
	case "oldest":
		// Sort by time ascending (oldest first)
		for i := 0; i < len(ec.Earthquakes)-1; i++ {
			for j := i + 1; j < len(ec.Earthquakes); j++ {
				if ec.Earthquakes[i].Time.After(ec.Earthquakes[j].Time) {
					ec.Earthquakes[i], ec.Earthquakes[j] = ec.Earthquakes[j], ec.Earthquakes[i]
				}
			}
		}
	case "depth":
		// Sort by depth descending (deepest first)
		for i := 0; i < len(ec.Earthquakes)-1; i++ {
			for j := i + 1; j < len(ec.Earthquakes); j++ {
				if ec.Earthquakes[i].Depth < ec.Earthquakes[j].Depth {
					ec.Earthquakes[i], ec.Earthquakes[j] = ec.Earthquakes[j], ec.Earthquakes[i]
				}
			}
		}
	case "closest":
		// Sort by distance ascending (closest first) - requires Distance field to be set
		for i := 0; i < len(ec.Earthquakes)-1; i++ {
			for j := i + 1; j < len(ec.Earthquakes); j++ {
				if ec.Earthquakes[i].Distance > ec.Earthquakes[j].Distance {
					ec.Earthquakes[i], ec.Earthquakes[j] = ec.Earthquakes[j], ec.Earthquakes[i]
				}
			}
		}
	default:
		// "newest" or any other value - sort by time descending (newest first) - already default from USGS
	}

	// Limit results
	if limit > 0 && limit < len(ec.Earthquakes) {
		ec.Earthquakes = ec.Earthquakes[:limit]
		ec.Metadata.Count = limit
	}
}
