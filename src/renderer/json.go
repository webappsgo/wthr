package renderer

import (
	"encoding/json"
	"fmt"

	"github.com/apimgr/weather/src/util"
)

// JSONRenderer handles JSON API response formatting
type JSONRenderer struct{}

// NewJSONRenderer creates a new JSON renderer
func NewJSONRenderer() *JSONRenderer {
	return &JSONRenderer{}
}

// WeatherResponse represents the full weather API response
type WeatherResponse struct {
	Location utils.LocationData   `json:"location"`
	Current  utils.CurrentData    `json:"current"`
	Forecast []utils.ForecastData `json:"forecast"`
	Moon     utils.MoonData       `json:"moon,omitempty"`
}

// Render converts weather data to JSON string
func (r *JSONRenderer) Render(weather *utils.WeatherData) (string, error) {
	response := WeatherResponse{
		Location: weather.Location,
		Current:  weather.Current,
		Forecast: weather.Forecast,
		Moon:     weather.Moon,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// RenderCompact converts weather data to compact JSON string (no indentation)
func (r *JSONRenderer) RenderCompact(weather *utils.WeatherData) (string, error) {
	response := WeatherResponse{
		Location: weather.Location,
		Current:  weather.Current,
		Forecast: weather.Forecast,
		Moon:     weather.Moon,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// RenderCurrentOnly renders only current weather as JSON
func (r *JSONRenderer) RenderCurrentOnly(location utils.LocationData, current utils.CurrentData) (string, error) {
	response := struct {
		Location utils.LocationData `json:"location"`
		Current  utils.CurrentData  `json:"current"`
	}{
		Location: location,
		Current:  current,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// RenderForecastOnly renders only forecast data as JSON
func (r *JSONRenderer) RenderForecastOnly(location utils.LocationData, forecast []utils.ForecastData) (string, error) {
	response := struct {
		Location utils.LocationData   `json:"location"`
		Forecast []utils.ForecastData `json:"forecast"`
	}{
		Location: location,
		Forecast: forecast,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// RenderError renders an error response as JSON
func (r *JSONRenderer) RenderError(message string, statusCode int) (string, error) {
	response := struct {
		Error  string `json:"error"`
		Status int    `json:"status"`
	}{
		Error:  message,
		Status: statusCode,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// SearchResult represents a location search result
type SearchResult struct {
	Name        string  `json:"name"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	State       string  `json:"state,omitempty"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Population  int     `json:"population,omitempty"`
	FullName    string  `json:"fullName"`
	ShortName   string  `json:"shortName"`
}

// RenderSearchResults renders location search results as JSON
func (r *JSONRenderer) RenderSearchResults(results []utils.LocationData) (string, error) {
	searchResults := make([]SearchResult, len(results))
	for i, loc := range results {
		searchResults[i] = SearchResult{
			Name:        loc.Name,
			Country:     loc.Country,
			CountryCode: loc.CountryCode,
			State:       loc.State,
			Latitude:    loc.Latitude,
			Longitude:   loc.Longitude,
			Population:  loc.Population,
			FullName:    loc.FullName,
			ShortName:   loc.ShortName,
		}
	}

	response := struct {
		Results []SearchResult `json:"results"`
		Count   int            `json:"count"`
	}{
		Results: searchResults,
		Count:   len(searchResults),
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

// HealthStatus represents the service health status
type HealthStatus struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Version   string `json:"version"`
}

// RenderHealthCheck renders a health check response as JSON
func (r *JSONRenderer) RenderHealthCheck(status, timestamp, service, version string) (string, error) {
	response := HealthStatus{
		Status:    status,
		Timestamp: timestamp,
		Service:   service,
		Version:   version,
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonData), nil
}
