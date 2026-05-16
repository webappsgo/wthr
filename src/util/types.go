package utils

import "time"

// WeatherData represents complete weather information
type WeatherData struct {
	Location LocationData   `json:"location"`
	Current  CurrentData    `json:"current"`
	Forecast []ForecastData `json:"forecast"`
	Moon     MoonData       `json:"moon"`
}

// LocationData represents enhanced location information
type LocationData struct {
	Name        string  `json:"name"`
	ShortName   string  `json:"shortName"`
	FullName    string  `json:"fullName"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	State       string  `json:"state,omitempty"`
	Population  int     `json:"population,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
}

// CurrentData represents current weather conditions
type CurrentData struct {
	Temperature   float64 `json:"temperature"`
	FeelsLike     float64 `json:"feelsLike"`
	Humidity      int     `json:"humidity"`
	Pressure      float64 `json:"pressure"`
	WindSpeed     float64 `json:"windSpeed"`
	WindDirection int     `json:"windDirection"`
	WeatherCode   int     `json:"weatherCode"`
	Condition     string  `json:"condition"`
	Icon          string  `json:"icon"`
	Time          string  `json:"time"`
	Precipitation float64 `json:"precipitation"`
}

// ForecastData represents forecast for a single day
type ForecastData struct {
	Date          string  `json:"date"`
	TempMax       float64 `json:"tempMax"`
	TempMin       float64 `json:"tempMin"`
	Condition     string  `json:"condition"`
	Icon          string  `json:"icon"`
	WeatherCode   int     `json:"weatherCode"`
	Precipitation float64 `json:"precipitation"`
	WindSpeed     float64 `json:"windSpeed"`
	WindDirection int     `json:"windDirection"`
}

// MoonData represents moon phase information
type MoonData struct {
	Phase        string  `json:"phase"`
	Illumination float64 `json:"illumination"`
	Icon         string  `json:"icon"`
	Age          float64 `json:"age"`
}

// RenderParams represents rendering parameters for weather output
type RenderParams struct {
	// 0-4: different output formats
	Format     int    `json:"format"`
	// metric, imperial, M (m/s)
	Units      string `json:"units"`
	// en, es, fr, etc.
	Language   string `json:"language"`
	// 0, 1, 2 (number of forecast days)
	Days       int    `json:"days"`
	// F: hide footer
	NoFooter   bool   `json:"noFooter"`
	// q: quiet mode
	Quiet      bool   `json:"quiet"`
	// Q: super quiet
	SuperQuiet bool   `json:"superQuiet"`
	// T: no terminal colors
	NoColors   bool   `json:"noColors"`
	// n: narrow output
	Narrow     bool   `json:"narrow"`
	// A: force ANSI/terminal
	ForceANSI  bool   `json:"forceANSI"`
	// Terminal width in columns (for adaptive layout)
	Width      int    `json:"width"`
}

// Country represents country data
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

// CacheEntry represents a cached item with timestamp
type CacheEntry struct {
	Data      interface{}
	Timestamp time.Time
}

// HostInfo represents detected host information
type HostInfo struct {
	Protocol   string
	Hostname   string
	Port       string
	FullHost   string
	ExampleURL string
}

// InitializationStatus tracks service initialization
type InitializationStatus struct {
	Countries bool      `json:"countries"`
	Cities    bool      `json:"cities"`
	Weather   bool      `json:"weather"`
	Started   time.Time `json:"started"`
}

// Now returns the current time in RFC3339 format
func Now() string {
	return time.Now().Format(time.RFC3339)
}
