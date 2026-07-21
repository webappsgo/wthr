package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

// WeatherService handles all weather-related operations
type WeatherService struct {
	client           *http.Client
	cache            *cache.Cache
	openMeteoBaseURL string
	geocodingURL     string
	locationEnhancer *LocationEnhancer
	zipcodeService   *ZipcodeService
	geoipService     *GeoIPService
}

// GeocodeResult represents raw geocoding API response
type GeocodeResult struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Name        string  `json:"name"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Admin1      string  `json:"admin1,omitempty"`
	Admin2      string  `json:"admin2,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
	Population  int     `json:"population,omitempty"`
}

// GeocodeResponse represents the geocoding API response
type GeocodeResponse struct {
	Results []GeocodeResult `json:"results"`
}

// Coordinates represents location coordinates with metadata
type Coordinates struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Name        string  `json:"name"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Timezone    string  `json:"timezone"`
	Admin1      string  `json:"admin1,omitempty"`
	Admin2      string  `json:"admin2,omitempty"`
	Population  int     `json:"population,omitempty"`
	FullName    string  `json:"fullName"`
	ShortName   string  `json:"shortName"`
}

// CurrentWeather represents current weather data
type CurrentWeather struct {
	Temperature   float64 `json:"temperature"`
	FeelsLike     float64 `json:"feelsLike"`
	Humidity      int     `json:"humidity"`
	Pressure      float64 `json:"pressure"`
	WindSpeed     float64 `json:"windSpeed"`
	WindDirection int     `json:"windDirection"`
	WindGusts     float64 `json:"windGusts"`
	Precipitation float64 `json:"precipitation"`
	CloudCover    int     `json:"cloudCover"`
	WeatherCode   int     `json:"weatherCode"`
	IsDay         int     `json:"isDay"`
	Timezone      string  `json:"timezone"`
}

// ForecastDay represents a single day forecast
type ForecastDay struct {
	Date                     string         `json:"date"`
	WeatherCode              int            `json:"weatherCode"`
	TempMax                  float64        `json:"tempMax"`
	TempMin                  float64        `json:"tempMin"`
	FeelsLikeMax             float64        `json:"feelsLikeMax"`
	FeelsLikeMin             float64        `json:"feelsLikeMin"`
	Precipitation            float64        `json:"precipitation"`
	PrecipitationHours       float64        `json:"precipitationHours"`
	PrecipitationProbability int            `json:"precipitationProbability"`
	WindSpeedMax             float64        `json:"windSpeedMax"`
	WindGustsMax             float64        `json:"windGustsMax"`
	WindDirection            int            `json:"windDirection"`
	SolarRadiation           float64        `json:"solarRadiation"`
	Hourly                   []ForecastHour `json:"hourly,omitempty"`
}

// ForecastHour represents hourly forecast data
type ForecastHour struct {
	Time                     string  `json:"time"`
	Temperature              float64 `json:"temperature"`
	FeelsLike                float64 `json:"feelsLike"`
	Humidity                 int     `json:"humidity"`
	Precipitation            float64 `json:"precipitation"`
	PrecipitationProbability int     `json:"precipitationProbability"`
	WeatherCode              int     `json:"weatherCode"`
	CloudCover               int     `json:"cloudCover"`
	WindSpeed                float64 `json:"windSpeed"`
	WindDirection            int     `json:"windDirection"`
	WindGusts                float64 `json:"windGusts"`
	Visibility               float64 `json:"visibility"`
	UVIndex                  float64 `json:"uvIndex"`
}

// Forecast represents multi-day weather forecast
type Forecast struct {
	Days     []ForecastDay `json:"days"`
	Timezone string        `json:"timezone"`
}

// LocationParts represents parsed location string
type LocationParts struct {
	City              string
	State             string
	Country           string
	HasStateOrCountry bool
	OriginalParts     []string
}

// OpenMeteoCurrentResponse represents the Open-Meteo current weather API response
type OpenMeteoCurrentResponse struct {
	Current struct {
		Temperature2m       float64 `json:"temperature_2m"`
		RelativeHumidity2m  int     `json:"relative_humidity_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		IsDay               int     `json:"is_day"`
		Precipitation       float64 `json:"precipitation"`
		WeatherCode         int     `json:"weather_code"`
		CloudCover          int     `json:"cloud_cover"`
		PressureMsl         float64 `json:"pressure_msl"`
		WindSpeed10m        float64 `json:"wind_speed_10m"`
		WindDirection10m    int     `json:"wind_direction_10m"`
		WindGusts10m        float64 `json:"wind_gusts_10m"`
	} `json:"current"`
	Timezone string `json:"timezone"`
}

// OpenMeteoForecastResponse represents the Open-Meteo forecast API response
type OpenMeteoForecastResponse struct {
	Daily struct {
		Time                        []string  `json:"time"`
		WeatherCode                 []int     `json:"weather_code"`
		Temperature2mMax            []float64 `json:"temperature_2m_max"`
		Temperature2mMin            []float64 `json:"temperature_2m_min"`
		ApparentTemperatureMax      []float64 `json:"apparent_temperature_max"`
		ApparentTemperatureMin      []float64 `json:"apparent_temperature_min"`
		PrecipitationSum            []float64 `json:"precipitation_sum"`
		PrecipitationHours          []float64 `json:"precipitation_hours"`
		PrecipitationProbabilityMax []int     `json:"precipitation_probability_max"`
		WindSpeed10mMax             []float64 `json:"wind_speed_10m_max"`
		WindGusts10mMax             []float64 `json:"wind_gusts_10m_max"`
		WindDirection10mDominant    []int     `json:"wind_direction_10m_dominant"`
		ShortwaveRadiationSum       []float64 `json:"shortwave_radiation_sum"`
	} `json:"daily"`
	Hourly struct {
		Time                     []string  `json:"time"`
		Temperature2m            []float64 `json:"temperature_2m"`
		ApparentTemperature      []float64 `json:"apparent_temperature"`
		RelativeHumidity2m       []int     `json:"relative_humidity_2m"`
		Precipitation            []float64 `json:"precipitation"`
		PrecipitationProbability []int     `json:"precipitation_probability"`
		WeatherCode              []int     `json:"weather_code"`
		CloudCover               []int     `json:"cloud_cover"`
		WindSpeed10m             []float64 `json:"wind_speed_10m"`
		WindDirection10m         []int     `json:"wind_direction_10m"`
		WindGusts10m             []float64 `json:"wind_gusts_10m"`
		Visibility               []float64 `json:"visibility"`
		UVIndex                  []float64 `json:"uv_index"`
	} `json:"hourly"`
	Timezone string `json:"timezone"`
}

// HistoricalDay represents weather data for a single historical day
type HistoricalDay struct {
	Date                 string  `json:"date"`
	Year                 int     `json:"year"`
	Month                int     `json:"month"`
	Day                  int     `json:"day"`
	WeatherCode          int     `json:"weatherCode"`
	TempMax              float64 `json:"tempMax"`
	TempMin              float64 `json:"tempMin"`
	TempAvg              float64 `json:"tempAvg"`
	ApparentTempMax      float64 `json:"apparentTempMax"`
	ApparentTempMin      float64 `json:"apparentTempMin"`
	Precipitation        float64 `json:"precipitation"`
	Rain                 float64 `json:"rain"`
	Snowfall             float64 `json:"snowfall"`
	SnowDepth            float64 `json:"snowDepth"`
	PrecipitationHours   float64 `json:"precipitationHours"`
	WindSpeedMax         float64 `json:"windSpeedMax"`
	WindGustsMax         float64 `json:"windGustsMax"`
	WindDirection        int     `json:"windDirection"`
	SunshineDuration     float64 `json:"sunshineDuration"`
	DaylightDuration     float64 `json:"daylightDuration"`
	PressureMean         float64 `json:"pressureMean"`
	HumidityMean         int     `json:"humidityMean"`
	CloudCoverMean       int     `json:"cloudCoverMean"`
	SolarRadiation       float64 `json:"solarRadiation"`
	ET0Evapotranspiration float64 `json:"et0Evapotranspiration"`
}

// HistoricalWeather represents historical weather data for a specific day across multiple years
type HistoricalWeather struct {
	// MM/DD format
	Date      string          `json:"date"`
	// 1-12
	Month     int             `json:"month"`
	// 1-31
	Day       int             `json:"day"`
	// Year to start from
	StartYear int             `json:"startYear"`
	// Data for each year
	Years     []HistoricalDay `json:"years"`
	// Location information
	Location  Coordinates     `json:"location"`
	// Statistical summary
	Stats     HistoricalStats `json:"stats"`
}

// HistoricalStats provides statistical analysis of historical data
type HistoricalStats struct {
	WarmestYear      int     `json:"warmestYear"`
	ColdestYear      int     `json:"coldestYear"`
	WettestYear      int     `json:"wettestYear"`
	DriestYear       int     `json:"driestYear"`
	AvgTempMax       float64 `json:"avgTempMax"`
	AvgTempMin       float64 `json:"avgTempMin"`
	AvgPrecipitation float64 `json:"avgPrecipitation"`
	MaxTempEver      float64 `json:"maxTempEver"`
	MinTempEver      float64 `json:"minTempEver"`
	MaxPrecipitation float64 `json:"maxPrecipitation"`
	TotalYears       int     `json:"totalYears"`
}

// OpenMeteoHistoricalResponse represents the Open-Meteo historical archive API response
type OpenMeteoHistoricalResponse struct {
	Daily struct {
		Time                    []string  `json:"time"`
		WeatherCode             []int     `json:"weather_code"`
		Temperature2mMax        []float64 `json:"temperature_2m_max"`
		Temperature2mMin        []float64 `json:"temperature_2m_min"`
		Temperature2mMean       []float64 `json:"temperature_2m_mean"`
		ApparentTemperatureMax  []float64 `json:"apparent_temperature_max"`
		ApparentTemperatureMin  []float64 `json:"apparent_temperature_min"`
		PrecipitationSum        []float64 `json:"precipitation_sum"`
		Rain                    []float64 `json:"rain_sum"`
		Snowfall                []float64 `json:"snowfall_sum"`
		SnowDepth               []float64 `json:"snow_depth_mean"`
		PrecipitationHours      []float64 `json:"precipitation_hours"`
		WindSpeed10mMax         []float64 `json:"wind_speed_10m_max"`
		WindGusts10mMax         []float64 `json:"wind_gusts_10m_max"`
		WindDirection10mDominant []int     `json:"wind_direction_10m_dominant"`
		SunshineDuration        []float64 `json:"sunshine_duration"`
		DaylightDuration        []float64 `json:"daylight_duration"`
		PressureMslMean         []float64 `json:"pressure_msl_mean"`
		RelativeHumidity2mMean  []int     `json:"relative_humidity_2m_mean"`
		CloudCoverMean          []int     `json:"cloud_cover_mean"`
		ShortwaveRadiationSum   []float64 `json:"shortwave_radiation_sum"`
		ET0Evapotranspiration   []float64 `json:"et0_evapotranspiration"`
	} `json:"daily"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

// NewWeatherService creates a new weather service instance
func NewWeatherService(locationEnhancer *LocationEnhancer, geoipService *GeoIPService) *WeatherService {
	// Create HTTP client with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return &WeatherService{
		client:           client,
		cache:            cache.New(15*time.Minute, 30*time.Minute),
		openMeteoBaseURL: "https://api.open-meteo.com/v1",
		geocodingURL:     "https://geocoding-api.open-meteo.com/v1",
		locationEnhancer: locationEnhancer,
		zipcodeService:   NewZipcodeService(),
		geoipService:     geoipService,
	}
}

// GetCoordinates retrieves coordinates for a location with geocoding
func (ws *WeatherService) GetCoordinates(location string, country string) (*Coordinates, error) {
	// Validate location input
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, fmt.Errorf("location cannot be empty")
	}

	// Validate location doesn't contain only special characters
	if !ws.isValidLocationInput(location) {
		return nil, fmt.Errorf("invalid location format: %s", location)
	}

	cacheKey := fmt.Sprintf("coords_%s_%s", location, country)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.(*Coordinates), nil
	}

	// Check if location is a US zipcode
	if IsZipcode(location) {
		zipcode, _ := ParseZipcode(location)
		coords, err := ws.zipcodeService.LookupZipcode(zipcode)
		if err == nil {
			// Zipcode data already has correct city, state info - don't enhance
			ws.cache.Set(cacheKey, coords, cache.DefaultExpiration)
			return coords, nil
		}
		// If zipcode lookup fails, fall through to regular geocoding
	}

	// Check if location is coordinates for reverse geocoding
	if ws.isCoordinates(location) {
		parts := strings.Split(location, ",")
		lat, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lon, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		return ws.ReverseGeocode(lat, lon)
	}

	// Parse the location to extract city and state/country info
	locationParts := ws.parseLocationString(location)
	searchName := location

	// If we have country info from IP, use it unless location already has country/state info
	if country != "" && !locationParts.HasStateOrCountry {
		searchName = fmt.Sprintf("%s, %s", location, country)
	}

	// Try multiple search approaches
	searchResults, err := ws.tryMultipleGeocodingApproaches(searchName, locationParts)
	if err == nil && len(searchResults) > 0 {
		// Find the best match based on location context
		bestMatch := ws.selectBestLocationMatch(searchResults, locationParts, country)

		// Enhance location data with external sources
		enhancedData, err := ws.locationEnhancer.EnhanceLocationData(bestMatch)
		if err == nil {
			coords := &Coordinates{
				Latitude:    enhancedData.Latitude,
				Longitude:   enhancedData.Longitude,
				Name:        enhancedData.Name,
				Country:     enhancedData.Country,
				CountryCode: strings.ToUpper(enhancedData.CountryCode),
				Timezone:    enhancedData.Timezone,
				Admin1:      enhancedData.Admin1,
				Admin2:      enhancedData.Admin2,
				Population:  enhancedData.Population,
				FullName:    enhancedData.FullName,
				ShortName:   enhancedData.ShortName,
			}
			ws.cache.Set(cacheKey, coords, cache.DefaultExpiration)
			return coords, nil
		}
	}

	// Try fallback coordinates for common cities
	fallbackLocation := CreateFallbackLocation(location)
	if fallbackLocation != nil {
		ws.cache.Set(cacheKey, fallbackLocation, cache.DefaultExpiration)
		return fallbackLocation, nil
	}

	// If no fallback available, provide a default location instead of failing
	defaultLocation := &Coordinates{
		Latitude:    40.7128,
		Longitude:   -74.0060,
		// Keep the requested name
		Name:        location,
		Country:     "United States",
		CountryCode: "US",
		Timezone:    "America/New_York",
		FullName:    location,
		ShortName:   location,
	}
	ws.cache.Set(cacheKey, defaultLocation, cache.DefaultExpiration)
	return defaultLocation, nil
}

// GetCurrentWeather retrieves current weather data
func (ws *WeatherService) GetCurrentWeather(latitude, longitude float64, units string) (*CurrentWeather, error) {
	cacheKey := fmt.Sprintf("current_%.4f_%.4f_%s", latitude, longitude, units)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.(*CurrentWeather), nil
	}

	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", longitude))
	params.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,is_day,precipitation,weather_code,cloud_cover,pressure_msl,wind_speed_10m,wind_direction_10m,wind_gusts_10m")
	params.Set("timezone", "auto")

	apiURL := fmt.Sprintf("%s/forecast?%s", ws.openMeteoBaseURL, params.Encode())

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var data OpenMeteoCurrentResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse weather data: %w", err)
	}

	weather := &CurrentWeather{
		Temperature:   data.Current.Temperature2m,
		FeelsLike:     data.Current.ApparentTemperature,
		Humidity:      data.Current.RelativeHumidity2m,
		Pressure:      data.Current.PressureMsl,
		WindSpeed:     data.Current.WindSpeed10m,
		WindDirection: data.Current.WindDirection10m,
		WindGusts:     data.Current.WindGusts10m,
		Precipitation: data.Current.Precipitation,
		CloudCover:    data.Current.CloudCover,
		WeatherCode:   data.Current.WeatherCode,
		IsDay:         data.Current.IsDay,
		Timezone:      data.Timezone,
	}

	// Convert units if needed
	weather = ws.convertWeatherUnits(weather, units)

	ws.cache.Set(cacheKey, weather, cache.DefaultExpiration)
	return weather, nil
}

// GetForecast retrieves weather forecast
func (ws *WeatherService) GetForecast(latitude, longitude float64, days int, units string) (*Forecast, error) {
	cacheKey := fmt.Sprintf("forecast_%.4f_%.4f_%d_%s", latitude, longitude, days, units)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.(*Forecast), nil
	}

	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", longitude))
	params.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,apparent_temperature_max,apparent_temperature_min,precipitation_sum,precipitation_hours,precipitation_probability_max,wind_speed_10m_max,wind_gusts_10m_max,wind_direction_10m_dominant,shortwave_radiation_sum")
	params.Set("hourly", "temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,precipitation_probability,weather_code,cloud_cover,wind_speed_10m,wind_direction_10m,wind_gusts_10m,visibility,uv_index")
	params.Set("forecast_days", strconv.Itoa(days))
	params.Set("timezone", "auto")

	apiURL := fmt.Sprintf("%s/forecast?%s", ws.openMeteoBaseURL, params.Encode())

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var data OpenMeteoForecastResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse forecast data: %w", err)
	}

	forecast := &Forecast{
		Days:     make([]ForecastDay, len(data.Daily.Time)),
		Timezone: data.Timezone,
	}

	// Parse daily data
	for i := range data.Daily.Time {
		day := ForecastDay{
			Date:                     data.Daily.Time[i],
			WeatherCode:              data.Daily.WeatherCode[i],
			TempMax:                  data.Daily.Temperature2mMax[i],
			TempMin:                  data.Daily.Temperature2mMin[i],
			FeelsLikeMax:             data.Daily.ApparentTemperatureMax[i],
			FeelsLikeMin:             data.Daily.ApparentTemperatureMin[i],
			Precipitation:            data.Daily.PrecipitationSum[i],
			PrecipitationHours:       data.Daily.PrecipitationHours[i],
			PrecipitationProbability: data.Daily.PrecipitationProbabilityMax[i],
			WindSpeedMax:             data.Daily.WindSpeed10mMax[i],
			WindGustsMax:             data.Daily.WindGusts10mMax[i],
			WindDirection:            data.Daily.WindDirection10mDominant[i],
			SolarRadiation:           data.Daily.ShortwaveRadiationSum[i],
			Hourly:                   []ForecastHour{},
		}
		forecast.Days[i] = day
	}

	// Group hourly data by day
	if len(data.Hourly.Time) > 0 {
		for i := range data.Hourly.Time {
			// Extract date from hourly timestamp (format: "2024-10-02T14:00")
			hourTime := data.Hourly.Time[i]
			// Get YYYY-MM-DD
			hourDate := hourTime[:10]

			// Find matching day and append hourly data
			for j := range forecast.Days {
				if forecast.Days[j].Date == hourDate {
					hour := ForecastHour{
						Time:                     hourTime,
						Temperature:              data.Hourly.Temperature2m[i],
						FeelsLike:                data.Hourly.ApparentTemperature[i],
						Humidity:                 data.Hourly.RelativeHumidity2m[i],
						Precipitation:            data.Hourly.Precipitation[i],
						PrecipitationProbability: data.Hourly.PrecipitationProbability[i],
						WeatherCode:              data.Hourly.WeatherCode[i],
						CloudCover:               data.Hourly.CloudCover[i],
						WindSpeed:                data.Hourly.WindSpeed10m[i],
						WindDirection:            data.Hourly.WindDirection10m[i],
						WindGusts:                data.Hourly.WindGusts10m[i],
						Visibility:               data.Hourly.Visibility[i],
						UVIndex:                  data.Hourly.UVIndex[i],
					}
					forecast.Days[j].Hourly = append(forecast.Days[j].Hourly, hour)
					break
				}
			}
		}
	}

	// Convert units if needed
	forecast = ws.convertForecastUnits(forecast, units)

	ws.cache.Set(cacheKey, forecast, cache.DefaultExpiration)
	return forecast, nil
}

// SearchLocations searches for multiple locations for autocomplete
func (ws *WeatherService) SearchLocations(query string, limit int) ([]Coordinates, error) {
	cacheKey := fmt.Sprintf("search_%s_%d", query, limit)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.([]Coordinates), nil
	}

	params := url.Values{}
	params.Set("name", query)
	params.Set("count", strconv.Itoa(limit))
	params.Set("language", "en")
	params.Set("format", "json")

	apiURL := fmt.Sprintf("%s/search?%s", ws.geocodingURL, params.Encode())

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var data GeocodeResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	if len(data.Results) == 0 {
		return []Coordinates{}, nil
	}

	// Convert results to simpler format for autocomplete
	results := make([]Coordinates, len(data.Results))
	for i, result := range data.Results {
		results[i] = Coordinates{
			Latitude:    result.Latitude,
			Longitude:   result.Longitude,
			Name:        result.Name,
			Country:     result.Country,
			CountryCode: strings.ToUpper(result.CountryCode),
			Timezone:    result.Timezone,
			Admin1:      result.Admin1,
			Admin2:      result.Admin2,
			Population:  result.Population,
			FullName:    ws.buildFullLocationName(&result),
			ShortName:   ws.buildShortLocationName(&result),
		}
	}

	ws.cache.Set(cacheKey, results, 1*time.Hour)
	return results, nil
}

// ReverseGeocode performs reverse geocoding from coordinates
func (ws *WeatherService) ReverseGeocode(latitude, longitude float64) (*Coordinates, error) {
	cacheKey := fmt.Sprintf("reverse_%.4f_%.4f", latitude, longitude)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.(*Coordinates), nil
	}

	// Try Open-Meteo geocoding first
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", longitude))
	params.Set("count", "1")
	params.Set("language", "en")
	params.Set("format", "json")

	apiURL := fmt.Sprintf("%s/search?%s", ws.geocodingURL, params.Encode())

	resp, err := ws.client.Get(apiURL)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var data GeocodeResponse
		if json.Unmarshal(body, &data) == nil && len(data.Results) > 0 {
			result := data.Results[0]

			// Enhance with external database
			enhancedData, err := ws.locationEnhancer.EnhanceLocationData(&result)
			if err == nil {
				coords := &Coordinates{
					Latitude:    latitude,
					Longitude:   longitude,
					Name:        enhancedData.Name,
					Country:     enhancedData.Country,
					CountryCode: strings.ToUpper(enhancedData.CountryCode),
					Timezone:    enhancedData.Timezone,
					Admin1:      enhancedData.Admin1,
					Admin2:      enhancedData.Admin2,
					Population:  enhancedData.Population,
					FullName:    enhancedData.FullName,
					ShortName:   enhancedData.ShortName,
				}
				ws.cache.Set(cacheKey, coords, 1*time.Hour)
				return coords, nil
			}
		}
	}

	// Fallback: Try OpenStreetMap Nominatim reverse geocoding
	nominatimURL := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%.4f&lon=%.4f&format=json", latitude, longitude)
	req, _ := http.NewRequest("GET", nominatimURL, nil)
	req.Header.Set("User-Agent", "WeatherService/2.0")

	nominatimResp, err := ws.client.Do(req)
	if err == nil {
		defer nominatimResp.Body.Close()
		body, _ := io.ReadAll(nominatimResp.Body)

		var nominatimData struct {
			Address struct {
				City        string `json:"city"`
				Town        string `json:"town"`
				Village     string `json:"village"`
				State       string `json:"state"`
				Country     string `json:"country"`
				CountryCode string `json:"country_code"`
			} `json:"address"`
		}

		if json.Unmarshal(body, &nominatimData) == nil {
			cityName := nominatimData.Address.City
			if cityName == "" {
				cityName = nominatimData.Address.Town
			}
			if cityName == "" {
				cityName = nominatimData.Address.Village
			}

			if cityName != "" {
				geocodeResult := &GeocodeResult{
					Name:        cityName,
					Latitude:    latitude,
					Longitude:   longitude,
					Country:     nominatimData.Address.Country,
					CountryCode: strings.ToUpper(nominatimData.Address.CountryCode),
					Admin1:      nominatimData.Address.State,
				}

				// Enhance with our databases
				enhancedData, err := ws.locationEnhancer.EnhanceLocationData(geocodeResult)
				if err == nil {
					coords := &Coordinates{
						Latitude:    latitude,
						Longitude:   longitude,
						Name:        enhancedData.Name,
						Country:     enhancedData.Country,
						CountryCode: strings.ToUpper(enhancedData.CountryCode),
						Timezone:    enhancedData.Timezone,
						Admin1:      enhancedData.Admin1,
						Admin2:      enhancedData.Admin2,
						Population:  enhancedData.Population,
						FullName:    enhancedData.FullName,
						ShortName:   enhancedData.ShortName,
					}
					ws.cache.Set(cacheKey, coords, 1*time.Hour)
					return coords, nil
				}
			}
		}
	}

	// Last resort: Find nearest city using external city database
	nearestCity, err := ws.locationEnhancer.FindNearestCity(latitude, longitude)
	if err == nil && nearestCity != nil {
		coords := &Coordinates{
			Latitude:    latitude,
			Longitude:   longitude,
			Name:        nearestCity.Name,
			Country:     nearestCity.Country,
			CountryCode: nearestCity.CountryCode,
			Timezone:    nearestCity.Timezone,
			Admin1:      nearestCity.Admin1,
			Admin2:      nearestCity.Admin2,
			Population:  nearestCity.Population,
			FullName:    nearestCity.FullName,
			ShortName:   nearestCity.ShortName,
		}
		ws.cache.Set(cacheKey, coords, 1*time.Hour)
		return coords, nil
	}

	// Final fallback: return coordinates with minimal info
	coords := &Coordinates{
		Latitude:    latitude,
		Longitude:   longitude,
		Name:        fmt.Sprintf("%.4f, %.4f", latitude, longitude),
		Country:     "Unknown",
		CountryCode: "XX",
		Timezone:    "UTC",
		FullName:    fmt.Sprintf("Coordinates %.4f, %.4f", latitude, longitude),
		ShortName:   fmt.Sprintf("%.4f, %.4f", latitude, longitude),
	}
	ws.cache.Set(cacheKey, coords, 1*time.Hour)
	return coords, nil
}

// GetWeatherDescription returns human-readable weather description
func (ws *WeatherService) GetWeatherDescription(code int) string {
	weatherCodes := map[int]string{
		0:  "Clear sky",
		1:  "Mainly clear",
		2:  "Partly cloudy",
		3:  "Overcast",
		45: "Fog",
		48: "Depositing rime fog",
		51: "Light drizzle",
		53: "Moderate drizzle",
		55: "Dense drizzle",
		56: "Light freezing drizzle",
		57: "Dense freezing drizzle",
		61: "Slight rain",
		63: "Moderate rain",
		65: "Heavy rain",
		66: "Light freezing rain",
		67: "Heavy freezing rain",
		71: "Slight snow",
		73: "Moderate snow",
		75: "Heavy snow",
		77: "Snow grains",
		80: "Slight rain showers",
		81: "Moderate rain showers",
		82: "Violent rain showers",
		85: "Slight snow showers",
		86: "Heavy snow showers",
		95: "Thunderstorm",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}

	if desc, ok := weatherCodes[code]; ok {
		return desc
	}
	return "Unknown"
}

// GetWeatherIcon returns emoji icon for weather code
func (ws *WeatherService) GetWeatherIcon(code int, isDay bool) string {
	icons := map[int]string{
		0:  "☀️",
		1:  "🌤️",
		2:  "⛅",
		3:  "☁️",
		45: "🌫️",
		48: "🌫️",
		51: "🌦️",
		53: "🌦️",
		55: "🌧️",
		56: "🌨️",
		57: "🌨️",
		61: "🌧️",
		63: "🌧️",
		65: "⛈️",
		66: "🌨️",
		67: "🌨️",
		71: "🌨️",
		73: "❄️",
		75: "❄️",
		77: "🌨️",
		80: "🌦️",
		81: "🌧️",
		82: "⛈️",
		85: "🌨️",
		86: "❄️",
		95: "⛈️",
		96: "⛈️",
		99: "⛈️",
	}

	// Special handling for clear/mainly clear based on day/night
	if code == 0 {
		if isDay {
			return "☀️"
		}
		return "🌙"
	}
	if code == 1 {
		if isDay {
			return "🌤️"
		}
		return "🌙"
	}
	if code == 2 && !isDay {
		return "☁️"
	}

	if icon, ok := icons[code]; ok {
		return icon
	}
	return "❓"
}

// isCoordinates checks if location string is coordinates
func (ws *WeatherService) isCoordinates(location string) bool {
	coordRegex := regexp.MustCompile(`^-?\d+\.?\d*\s*,\s*-?\d+\.?\d*$`)
	return coordRegex.MatchString(strings.TrimSpace(location))
}

// isValidLocationInput validates that location is a valid input format
func (ws *WeatherService) isValidLocationInput(location string) bool {
	if len(location) == 0 {
		return false
	}
	// Location must start with a letter or digit (not special characters)
	first := rune(location[0])
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9')) {
		return false
	}
	// Must contain at least one letter or digit
	hasAlphanumeric := false
	for _, c := range location {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			hasAlphanumeric = true
			break
		}
	}
	return hasAlphanumeric
}

// parseLocationString parses location string into parts
func (ws *WeatherService) parseLocationString(location string) *LocationParts {
	if location == "" {
		return &LocationParts{
			City:              "",
			State:             "",
			Country:           "",
			HasStateOrCountry: false,
			OriginalParts:     []string{},
		}
	}

	parts := strings.Split(location, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	hasStateOrCountry := len(parts) > 1

	city := ""
	state := ""
	country := ""

	if len(parts) > 0 {
		city = parts[0]
	}
	if len(parts) > 1 {
		state = parts[1]
	}
	if len(parts) > 2 {
		country = parts[2]
	} else if len(parts) > 1 {
		country = parts[1]
	}

	return &LocationParts{
		City:              city,
		State:             state,
		Country:           country,
		HasStateOrCountry: hasStateOrCountry,
		OriginalParts:     parts,
	}
}

// tryMultipleGeocodingApproaches tries different geocoding strategies
func (ws *WeatherService) tryMultipleGeocodingApproaches(searchName string, locationParts *LocationParts) ([]GeocodeResult, error) {
	type approach struct {
		name  string
		count int
	}

	approaches := []approach{
		{name: searchName, count: 10},
	}

	// Also search by city name alone to get more results for matching
	// The selectBestLocationMatch will filter by state if provided
	if locationParts.HasStateOrCountry && locationParts.City != "" {
		approaches = append(approaches, approach{name: locationParts.City, count: 20})
	}

	for _, app := range approaches {
		params := url.Values{}
		params.Set("name", app.name)
		params.Set("count", strconv.Itoa(app.count))
		params.Set("language", "en")
		params.Set("format", "json")

		apiURL := fmt.Sprintf("%s/search?%s", ws.geocodingURL, params.Encode())

		resp, err := ws.client.Get(apiURL)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var data GeocodeResponse
		if err := json.Unmarshal(body, &data); err != nil {
			continue
		}

		if len(data.Results) > 0 {
			return data.Results, nil
		}
	}

	return []GeocodeResult{}, fmt.Errorf("no results found")
}

// selectBestLocationMatch selects best match from multiple results
func (ws *WeatherService) selectBestLocationMatch(results []GeocodeResult, locationParts *LocationParts, userCountry string) *GeocodeResult {
	if len(results) == 1 {
		return &results[0]
	}

	// If location has state/country info, try to match it
	if locationParts.HasStateOrCountry {
		stateCountryPart := strings.ToLower(locationParts.State)

		// Priority 1: Check ALL results for US state abbreviation matches first
		for i := range results {
			admin1 := strings.ToLower(results[i].Admin1)
			countryCode := strings.ToUpper(results[i].CountryCode)

			if ws.matchesStateAbbreviation(stateCountryPart, admin1, countryCode) {
				return &results[i]
			}
		}

		// Priority 2: Check ALL results for Canadian province abbreviation matches
		for i := range results {
			admin1 := strings.ToLower(results[i].Admin1)
			countryCode := strings.ToUpper(results[i].CountryCode)

			if ws.matchesCanadianProvince(stateCountryPart, admin1, countryCode) {
				return &results[i]
			}
		}

		// Priority 3: Check for general matches (admin1, country, country code)
		for i := range results {
			admin1 := strings.ToLower(results[i].Admin1)
			country := strings.ToLower(results[i].Country)
			countryCode := strings.ToLower(results[i].CountryCode)

			if strings.Contains(admin1, stateCountryPart) ||
				strings.Contains(country, stateCountryPart) ||
				countryCode == stateCountryPart {
				return &results[i]
			}
		}
	}

	// If user's country context available, prefer locations in that country
	if userCountry != "" {
		for i := range results {
			if results[i].CountryCode == userCountry ||
				strings.Contains(strings.ToLower(results[i].Country), strings.ToLower(userCountry)) {
				return &results[i]
			}
		}
	}

	// Default to the first result (usually most populated)
	return &results[0]
}

// matchesStateAbbreviation checks if input matches US state abbreviation
func (ws *WeatherService) matchesStateAbbreviation(input, admin1, countryCode string) bool {
	if countryCode != "US" {
		return false
	}

	stateAbbreviations := map[string]string{
		"al": "alabama", "ak": "alaska", "az": "arizona", "ar": "arkansas", "ca": "california",
		"co": "colorado", "ct": "connecticut", "de": "delaware", "fl": "florida", "ga": "georgia",
		"hi": "hawaii", "id": "idaho", "il": "illinois", "in": "indiana", "ia": "iowa",
		"ks": "kansas", "ky": "kentucky", "la": "louisiana", "me": "maine", "md": "maryland",
		"ma": "massachusetts", "mi": "michigan", "mn": "minnesota", "ms": "mississippi", "mo": "missouri",
		"mt": "montana", "ne": "nebraska", "nv": "nevada", "nh": "new hampshire", "nj": "new jersey",
		"nm": "new mexico", "ny": "new york", "nc": "north carolina", "nd": "north dakota", "oh": "ohio",
		"ok": "oklahoma", "or": "oregon", "pa": "pennsylvania", "ri": "rhode island", "sc": "south carolina",
		"sd": "south dakota", "tn": "tennessee", "tx": "texas", "ut": "utah", "vt": "vermont",
		"va": "virginia", "wa": "washington", "wv": "west virginia", "wi": "wisconsin", "wy": "wyoming",
	}

	fullStateName := stateAbbreviations[strings.ToLower(input)]
	return fullStateName != "" && strings.Contains(strings.ToLower(admin1), fullStateName)
}

// matchesCanadianProvince checks if input matches Canadian province abbreviation
func (ws *WeatherService) matchesCanadianProvince(input, admin1, countryCode string) bool {
	if countryCode != "CA" {
		return false
	}

	provinceAbbreviations := map[string]string{
		"ab": "alberta", "bc": "british columbia", "mb": "manitoba", "nb": "new brunswick",
		"nl": "newfoundland and labrador", "nt": "northwest territories", "ns": "nova scotia",
		"nu": "nunavut", "on": "ontario", "pe": "prince edward island", "qc": "quebec",
		"sk": "saskatchewan", "yt": "yukon",
	}

	fullProvinceName := provinceAbbreviations[strings.ToLower(input)]
	return fullProvinceName != "" && strings.Contains(strings.ToLower(admin1), fullProvinceName)
}

// buildFullLocationName builds full location name for display
func (ws *WeatherService) buildFullLocationName(result *GeocodeResult) string {
	parts := []string{result.Name}

	if result.Admin1 != "" && result.Admin1 != result.Name {
		parts = append(parts, result.Admin1)
	}

	if result.Country != "" {
		parts = append(parts, result.Country)
	}

	return strings.Join(parts, ", ")
}

// buildShortLocationName builds short location name for display
func (ws *WeatherService) buildShortLocationName(result *GeocodeResult) string {
	city := result.Name
	countryCode := strings.ToUpper(result.CountryCode)
	if countryCode == "" {
		countryCode = "XX"
	}

	// For US locations, use state abbreviation if available
	if countryCode == "US" && result.Admin1 != "" {
		stateAbbrev := ws.getStateAbbreviation(result.Admin1)
		if stateAbbrev != "" {
			return fmt.Sprintf("%s, %s", city, stateAbbrev)
		}
	}

	// For all other locations, use country code
	return fmt.Sprintf("%s, %s", city, countryCode)
}

// getStateAbbreviation returns US state abbreviation
func (ws *WeatherService) getStateAbbreviation(stateName string) string {
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

// convertWeatherUnits converts weather data to specified units
func (ws *WeatherService) convertWeatherUnits(weather *CurrentWeather, units string) *CurrentWeather {
	if units == "metric" {
		return weather
	}

	converted := *weather

	if units == "imperial" {
		converted.Temperature = ws.celsiusToFahrenheit(weather.Temperature)
		converted.FeelsLike = ws.celsiusToFahrenheit(weather.FeelsLike)
		converted.Pressure = ws.hpaToInhg(weather.Pressure)
		converted.WindSpeed = ws.kmhToMph(weather.WindSpeed)
		converted.WindGusts = ws.kmhToMph(weather.WindGusts)
		converted.Precipitation = ws.mmToInches(weather.Precipitation)
	}

	return &converted
}

// convertForecastUnits converts forecast data to specified units
func (ws *WeatherService) convertForecastUnits(forecast *Forecast, units string) *Forecast {
	if units == "metric" {
		return forecast
	}

	converted := *forecast
	converted.Days = make([]ForecastDay, len(forecast.Days))

	for i, day := range forecast.Days {
		convertedDay := day
		if units == "imperial" {
			convertedDay.TempMax = ws.celsiusToFahrenheit(day.TempMax)
			convertedDay.TempMin = ws.celsiusToFahrenheit(day.TempMin)
			convertedDay.FeelsLikeMax = ws.celsiusToFahrenheit(day.FeelsLikeMax)
			convertedDay.FeelsLikeMin = ws.celsiusToFahrenheit(day.FeelsLikeMin)
			convertedDay.Precipitation = ws.mmToInches(day.Precipitation)
			convertedDay.WindSpeedMax = ws.kmhToMph(day.WindSpeedMax)
			convertedDay.WindGustsMax = ws.kmhToMph(day.WindGustsMax)

			// Convert hourly data
			convertedDay.Hourly = make([]ForecastHour, len(day.Hourly))
			for j, hour := range day.Hourly {
				convertedHour := hour
				convertedHour.Temperature = ws.celsiusToFahrenheit(hour.Temperature)
				convertedHour.FeelsLike = ws.celsiusToFahrenheit(hour.FeelsLike)
				convertedHour.Precipitation = ws.mmToInches(hour.Precipitation)
				convertedHour.WindSpeed = ws.kmhToMph(hour.WindSpeed)
				convertedHour.WindGusts = ws.kmhToMph(hour.WindGusts)
				convertedHour.Visibility = ws.metersToMiles(hour.Visibility)
				convertedDay.Hourly[j] = convertedHour
			}
		}
		converted.Days[i] = convertedDay
	}

	return &converted
}

// Unit conversion functions
func (ws *WeatherService) celsiusToFahrenheit(celsius float64) float64 {
	return (celsius * 9 / 5) + 32
}

func (ws *WeatherService) kmhToMph(kmh float64) float64 {
	return kmh * 0.621371
}

func (ws *WeatherService) hpaToInhg(hpa float64) float64 {
	return hpa * 0.02953
}

func (ws *WeatherService) mmToInches(mm float64) float64 {
	return mm * 0.0393701
}

func (ws *WeatherService) metersToMiles(meters float64) float64 {
	return meters * 0.000621371
}

// ParseAndResolveLocation parses location string and resolves to coordinates
func (ws *WeatherService) ParseAndResolveLocation(location string, clientIP string) (*Coordinates, error) {
	if location == "" {
		return ws.GetCoordinatesFromIP(clientIP)
	}

	// Try to get coordinates
	coords, err := ws.GetCoordinates(location, "")
	if err != nil {
		return nil, fmt.Errorf("location not found: %s", location)
	}

	return coords, nil
}

// GetCoordinatesFromIP gets location coordinates from IP address
func (ws *WeatherService) GetCoordinatesFromIP(ip string) (*Coordinates, error) {
	// For localhost/private IPs, use default location
	if ws.isLocalIP(ip) {
		return ws.GetCoordinates("New York", "US")
	}

	// Try IP geolocation services
	coords, err := ws.tryIPGeolocation(ip)
	if err != nil {
		// Fallback to default location
		return ws.GetCoordinates("New York", "US")
	}

	return coords, nil
}

// isLocalIP checks if IP is localhost or private (supports IPv4 and IPv6)
// AI.md: Host-specific values detected at runtime
func (ws *WeatherService) isLocalIP(ip string) bool {
	// Parse IP address properly
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		// Invalid IP, treat as local for safety
		return true
	}

	// Check if loopback (127.0.0.1 or ::1)
	if parsedIP.IsLoopback() {
		return true
	}

	// Check if private (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7, fe80::/10)
	// This covers Docker bridge networks (172.17.0.0/16, etc.)
	if parsedIP.IsPrivate() {
		return true
	}

	// Check for link-local IPv6 (fe80::/10)
	if parsedIP.IsLinkLocalUnicast() {
		return true
	}

	// Check for unique local IPv6 (fc00::/7)
	if len(parsedIP) == 16 && (parsedIP[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

// tryIPGeolocation tries GeoIP database first, then falls back to API services
func (ws *WeatherService) tryIPGeolocation(ip string) (*Coordinates, error) {
	// Try GeoIP service first (local database, fast and reliable)
	geoipCoords := (*Coordinates)(nil)
	if ws.geoipService != nil && ws.geoipService.IsEnabled() {
		geoData, err := ws.geoipService.LookupIP(ip)
		if err == nil {
			// Convert GeoIPData to Coordinates
			locationStr := geoData.City
			if geoData.Region != "" {
				locationStr = fmt.Sprintf("%s, %s", geoData.City, geoData.Region)
			}
			coords, err := ws.GetCoordinates(locationStr, geoData.CountryCode)
			if err == nil {
				geoipCoords = coords
				// Verify GeoIP result by checking with API for IPv6 addresses or suspicious results
				// If IPv6 or result seems wrong (e.g., Tokyo for US IPv6), verify with API
				if strings.Contains(ip, ":") {
					// IPv6 - verify with API service as GeoLite2 IPv6 data can be inaccurate
					fmt.Printf("🔍 Verifying IPv6 GeoIP result (%s) with API...\n", coords.ShortName)
					// Don't return yet, try API verification below
				} else {
					// IPv4 - trust GeoIP
					return coords, nil
				}
			}
		} else {
			// If GeoIP fails, fall through to API services
			fmt.Printf("⚠️ GeoIP lookup failed: %v, falling back to API services\n", err)
		}
	}

	// Fallback to API services (or verify IPv6)
	// Service 1: ipapi.co
	coords, err := ws.tryIPAPIco(ip)
	if err == nil {
		// If we have both GeoIP and API results, compare them
		if geoipCoords != nil {
			// Check if results differ significantly (>100 miles apart)
			distanceKM := haversineDistance(geoipCoords.Latitude, geoipCoords.Longitude, coords.Latitude, coords.Longitude)
			distanceMiles := distanceKM * 0.621371
			if distanceMiles > 100 {
				fmt.Printf("⚠️ GeoIP mismatch: %s (GeoIP) vs %s (API), distance: %.0f miles - using API result\n",
					geoipCoords.ShortName, coords.ShortName, distanceMiles)
				// Use API result
				return coords, nil
			}
			fmt.Printf("✅ GeoIP verified: %s matches API result\n", geoipCoords.ShortName)
			// Use GeoIP result (faster)
			return geoipCoords, nil
		}
		return coords, nil
	}

	// Service 2: ip-api.com
	coords, err = ws.tryIPAPIcom(ip)
	if err == nil {
		if geoipCoords != nil {
			distanceKM := haversineDistance(geoipCoords.Latitude, geoipCoords.Longitude, coords.Latitude, coords.Longitude)
			distanceMiles := distanceKM * 0.621371
			if distanceMiles > 100 {
				fmt.Printf("⚠️ GeoIP mismatch: using API result (%s)\n", coords.ShortName)
				return coords, nil
			}
			return geoipCoords, nil
		}
		return coords, nil
	}

	// Service 3: ipwho.is
	coords, err = ws.tryIPWhois(ip)
	if err == nil {
		if geoipCoords != nil {
			distanceKM := haversineDistance(geoipCoords.Latitude, geoipCoords.Longitude, coords.Latitude, coords.Longitude)
			distanceMiles := distanceKM * 0.621371
			if distanceMiles > 100 {
				fmt.Printf("⚠️ GeoIP mismatch: using API result (%s)\n", coords.ShortName)
				return coords, nil
			}
			return geoipCoords, nil
		}
		return coords, nil
	}

	// If we have GeoIP coords but all APIs failed, use GeoIP
	if geoipCoords != nil {
		fmt.Printf("⚠️ All API services failed, using GeoIP result: %s\n", geoipCoords.ShortName)
		return geoipCoords, nil
	}

	return nil, fmt.Errorf("all IP geolocation services failed")
}

// tryIPAPIco tries ipapi.co service
func (ws *WeatherService) tryIPAPIco(ip string) (*Coordinates, error) {
	apiURL := fmt.Sprintf("https://ipapi.co/%s/json/", ip)

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipapi.co returned status %d", resp.StatusCode)
	}

	var data struct {
		City        string  `json:"city"`
		Region      string  `json:"region"`
		Country     string  `json:"country_name"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.City == "" {
		return nil, fmt.Errorf("no city data from ipapi.co")
	}

	// Get full location data
	return ws.GetCoordinates(fmt.Sprintf("%s, %s", data.City, data.Region), data.CountryCode)
}

// tryIPAPIcom tries ip-api.com service
func (ws *WeatherService) tryIPAPIcom(ip string) (*Coordinates, error) {
	apiURL := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,city,regionName,country,countryCode,lat,lon", ip)

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Status      string  `json:"status"`
		City        string  `json:"city"`
		Region      string  `json:"regionName"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Latitude    float64 `json:"lat"`
		Longitude   float64 `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.Status != "success" || data.City == "" {
		return nil, fmt.Errorf("no city data from ip-api.com")
	}

	// Get full location data
	return ws.GetCoordinates(fmt.Sprintf("%s, %s", data.City, data.Region), data.CountryCode)
}

// tryIPWhois tries ipwho.is service
func (ws *WeatherService) tryIPWhois(ip string) (*Coordinates, error) {
	apiURL := fmt.Sprintf("http://ipwho.is/%s", ip)

	resp, err := ws.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Success     bool    `json:"success"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if !data.Success || data.City == "" {
		return nil, fmt.Errorf("no city data from ipwho.is")
	}

	// Get full location data
	return ws.GetCoordinates(fmt.Sprintf("%s, %s", data.City, data.Region), data.CountryCode)
}

// GetHistoricalWeather retrieves historical weather data for a specific day across multiple years
func (ws *WeatherService) GetHistoricalWeather(latitude, longitude float64, month, day, startYear, numberOfYears int) (*HistoricalWeather, error) {
	// Validate inputs
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month: %d (must be 1-12)", month)
	}
	if day < 1 || day > 31 {
		return nil, fmt.Errorf("invalid day: %d (must be 1-31)", day)
	}
	currentYear := time.Now().Year()
	if startYear > currentYear {
		return nil, fmt.Errorf("start year %d cannot be in the future", startYear)
	}
	if numberOfYears < 1 || numberOfYears > 100 {
		return nil, fmt.Errorf("invalid number of years: %d (must be 1-100)", numberOfYears)
	}

	// Create cache key
	cacheKey := fmt.Sprintf("historical_%f_%f_%d_%d_%d_%d", latitude, longitude, month, day, startYear, numberOfYears)
	if cached, found := ws.cache.Get(cacheKey); found {
		return cached.(*HistoricalWeather), nil
	}

	historical := &HistoricalWeather{
		Date:      fmt.Sprintf("%02d/%02d", month, day),
		Month:     month,
		Day:       day,
		StartYear: startYear,
		Years:     make([]HistoricalDay, 0, numberOfYears),
		Location: Coordinates{
			Latitude:  latitude,
			Longitude: longitude,
		},
	}

	// Fetch data for each year going backwards from startYear
	for i := 0; i < numberOfYears; i++ {
		year := startYear - i
		if year < 1940 {
			// Open-Meteo historical data starts from 1940
			break
		}

		// Construct date strings for API call
		targetDate := fmt.Sprintf("%04d-%02d-%02d", year, month, day)

		// Build API URL for single day
		apiURL := fmt.Sprintf("%s/archive?latitude=%f&longitude=%f&start_date=%s&end_date=%s&daily=weather_code,temperature_2m_max,temperature_2m_min,temperature_2m_mean,apparent_temperature_max,apparent_temperature_min,precipitation_sum,rain_sum,snowfall_sum,snow_depth_mean,precipitation_hours,wind_speed_10m_max,wind_gusts_10m_max,wind_direction_10m_dominant,sunshine_duration,daylight_duration,pressure_msl_mean,relative_humidity_2m_mean,cloud_cover_mean,shortwave_radiation_sum,et0_evapotranspiration&timezone=auto",
			ws.openMeteoBaseURL, latitude, longitude, targetDate, targetDate)

		// Make API request
		resp, err := ws.client.Get(apiURL)
		if err != nil {
			log.Printf("⚠️  Failed to fetch historical data for %s: %v", targetDate, err)
			continue
		}

		var apiResp OpenMeteoHistoricalResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			resp.Body.Close()
			log.Printf("⚠️  Failed to decode historical data for %s: %v", targetDate, err)
			continue
		}
		resp.Body.Close()

		// Parse the response - should have exactly 1 day of data
		if len(apiResp.Daily.Time) != 1 {
			log.Printf("⚠️  Unexpected number of days in response for %s: %d", targetDate, len(apiResp.Daily.Time))
			continue
		}

		// Create HistoricalDay struct
		histDay := HistoricalDay{
			Date:        apiResp.Daily.Time[0],
			Year:        year,
			Month:       month,
			Day:         day,
			WeatherCode: safeIntAt(apiResp.Daily.WeatherCode, 0),
			TempMax:     safeFloatAt(apiResp.Daily.Temperature2mMax, 0),
			TempMin:     safeFloatAt(apiResp.Daily.Temperature2mMin, 0),
			TempAvg:     safeFloatAt(apiResp.Daily.Temperature2mMean, 0),
			ApparentTempMax: safeFloatAt(apiResp.Daily.ApparentTemperatureMax, 0),
			ApparentTempMin: safeFloatAt(apiResp.Daily.ApparentTemperatureMin, 0),
			Precipitation:   safeFloatAt(apiResp.Daily.PrecipitationSum, 0),
			Rain:            safeFloatAt(apiResp.Daily.Rain, 0),
			Snowfall:        safeFloatAt(apiResp.Daily.Snowfall, 0),
			SnowDepth:       safeFloatAt(apiResp.Daily.SnowDepth, 0),
			PrecipitationHours: safeFloatAt(apiResp.Daily.PrecipitationHours, 0),
			WindSpeedMax:    safeFloatAt(apiResp.Daily.WindSpeed10mMax, 0),
			WindGustsMax:    safeFloatAt(apiResp.Daily.WindGusts10mMax, 0),
			WindDirection:   safeIntAt(apiResp.Daily.WindDirection10mDominant, 0),
			SunshineDuration: safeFloatAt(apiResp.Daily.SunshineDuration, 0),
			DaylightDuration: safeFloatAt(apiResp.Daily.DaylightDuration, 0),
			PressureMean:    safeFloatAt(apiResp.Daily.PressureMslMean, 0),
			HumidityMean:    safeIntAt(apiResp.Daily.RelativeHumidity2mMean, 0),
			CloudCoverMean:  safeIntAt(apiResp.Daily.CloudCoverMean, 0),
			SolarRadiation:  safeFloatAt(apiResp.Daily.ShortwaveRadiationSum, 0),
			ET0Evapotranspiration: safeFloatAt(apiResp.Daily.ET0Evapotranspiration, 0),
		}

		historical.Years = append(historical.Years, histDay)
	}

	// Calculate statistics
	if len(historical.Years) > 0 {
		historical.Stats = calculateHistoricalStats(historical.Years)
	}

	// Cache the result for 24 hours (historical data doesn't change)
	ws.cache.Set(cacheKey, historical, 24*time.Hour)

	return historical, nil
}

// calculateHistoricalStats computes statistical analysis of historical data
func calculateHistoricalStats(years []HistoricalDay) HistoricalStats {
	if len(years) == 0 {
		return HistoricalStats{}
	}

	stats := HistoricalStats{
		TotalYears:       len(years),
		MaxTempEver:      -999,
		MinTempEver:      999,
		MaxPrecipitation: 0,
	}

	var sumTempMax, sumTempMin, sumPrecipitation float64
	var warmestTemp, coldestTemp, maxPrecip float64
	warmestTemp = -999
	coldestTemp = 999

	for _, year := range years {
		// Sum for averages
		sumTempMax += year.TempMax
		sumTempMin += year.TempMin
		sumPrecipitation += year.Precipitation

		// Track warmest year
		if year.TempMax > warmestTemp {
			warmestTemp = year.TempMax
			stats.WarmestYear = year.Year
		}

		// Track coldest year
		if year.TempMin < coldestTemp {
			coldestTemp = year.TempMin
			stats.ColdestYear = year.Year
		}

		// Track wettest year
		if year.Precipitation > maxPrecip {
			maxPrecip = year.Precipitation
			stats.WettestYear = year.Year
		}

		// Track driest year (non-zero only)
		if stats.DriestYear == 0 || (year.Precipitation >= 0 && year.Precipitation < sumPrecipitation/float64(len(years))) {
			stats.DriestYear = year.Year
		}

		// Track all-time records
		if year.TempMax > stats.MaxTempEver {
			stats.MaxTempEver = year.TempMax
		}
		if year.TempMin < stats.MinTempEver {
			stats.MinTempEver = year.TempMin
		}
		if year.Precipitation > stats.MaxPrecipitation {
			stats.MaxPrecipitation = year.Precipitation
		}
	}

	// Calculate averages
	count := float64(len(years))
	stats.AvgTempMax = sumTempMax / count
	stats.AvgTempMin = sumTempMin / count
	stats.AvgPrecipitation = sumPrecipitation / count

	return stats
}

// LookupIP returns GeoIP data for an IP address
// Wraps the internal geoipService for external access
func (ws *WeatherService) LookupIP(ip string) (*GeoIPData, error) {
	if ws.geoipService == nil {
		return nil, fmt.Errorf("GeoIP service not initialized")
	}
	return ws.geoipService.LookupIP(ip)
}

// LookupZipcode returns coordinates and location data for a US zip code
// Wraps the internal zipcodeService for external access
func (ws *WeatherService) LookupZipcode(zipcode int) (*Coordinates, error) {
	if ws.zipcodeService == nil {
		return nil, fmt.Errorf("zipcode service not initialized")
	}
	return ws.zipcodeService.LookupZipcode(zipcode)
}

// safeFloatAt safely retrieves a float64 from a slice at the given index
func safeFloatAt(slice []float64, index int) float64 {
	if index >= 0 && index < len(slice) {
		return slice[index]
	}
	return 0.0
}

// safeIntAt safely retrieves an int from a slice at the given index
func safeIntAt(slice []int, index int) int {
	if index >= 0 && index < len(slice) {
		return slice[index]
	}
	return 0
}
