package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/server/service"
	"github.com/casapps/wthr/src/util"
)

// APIHandler handles JSON API routes (/api/v1/*)
type APIHandler struct {
	weatherService   *service.WeatherService
	locationEnhancer *service.LocationEnhancer
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(ws *service.WeatherService, le *service.LocationEnhancer) *APIHandler {
	return &APIHandler{
		weatherService:   ws,
		locationEnhancer: le,
	}
}

// GetWeather returns current weather with today's forecast (GET /api/v1/weather)
// @Summary Get current weather
// @Description Get current weather conditions and today's forecast for a location
// @Tags Weather
// @Accept json
// @Produce json
// @Param location query string false "Location name (city, address, or place)"
// @Param lat query number false "Latitude coordinate"
// @Param lon query number false "Longitude coordinate"
// @Param city_id query integer false "City ID from database"
// @Param nearest query boolean false "Find nearest city to coordinates (default: false)"
// @Param units query string false "Units system: metric, imperial, or standard (default: metric)" Enums(metric, imperial, standard)
// @Success 200 {object} map[string]interface{} "Weather data with current conditions and forecast"
// @Failure 400 {object} map[string]interface{} "Invalid request parameters"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/weather [get]
func (h *APIHandler) GetWeather(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	location := strings.TrimSpace(c.Query("location"))
	lat := strings.TrimSpace(c.Query("lat"))
	lon := strings.TrimSpace(c.Query("lon"))
	cityID := strings.TrimSpace(c.Query("city_id"))
	nearest := strings.TrimSpace(c.Query("nearest"))
	unitsParam := strings.TrimSpace(c.Query("units"))

	var coords *service.Coordinates
	var enhanced *service.EnhancedLocation
	var err error

	// Priority: city_id > lat/lon > nearest > location > IP
	if cityID != "" {
		// Find city by ID
		id, parseErr := strconv.Atoi(cityID)
		if parseErr != nil {
			InvalidInput(c, "Invalid city ID format")
			return
		}
		enhanced, err = h.locationEnhancer.FindCityByID(id)
		if err != nil {
			NotFound(c, fmt.Sprintf("City ID %d not found", id))
			return
		}
	} else if lat != "" || lon != "" {
		// Validate that both lat and lon are provided
		if lat == "" || lon == "" {
			InvalidInput(c, "Both 'lat' and 'lon' parameters are required")
			return
		}

		// Use coordinates
		latitude, latErr := strconv.ParseFloat(lat, 64)
		longitude, lonErr := strconv.ParseFloat(lon, 64)

		// Validate coordinate parsing
		if latErr != nil {
			InvalidInput(c, "Invalid latitude format")
			return
		}
		if lonErr != nil {
			InvalidInput(c, "Invalid longitude format")
			return
		}

		// Validate coordinate ranges
		if latitude < -90 || latitude > 90 {
			InvalidInput(c, "Latitude must be between -90 and 90")
			return
		}
		if longitude < -180 || longitude > 180 {
			InvalidInput(c, "Longitude must be between -180 and 180")
			return
		}

		if nearest == "true" || nearest == "1" {
			// Find nearest city to coordinates
			enhanced, err = h.locationEnhancer.FindNearestCity(latitude, longitude)
			if err != nil {
				// Fallback to coordinate-based location
				coords, err = h.weatherService.GetCoordinates(fmt.Sprintf("%f,%f", latitude, longitude), "")
				if err != nil {
					BadRequest(c, err.Error())
					return
				}
				// Convert to GeocodeResult and enhance
				geocodeResult := &service.GeocodeResult{
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
				enhanced, _ = h.locationEnhancer.EnhanceLocationData(geocodeResult)
			}
		} else {
			coords, err = h.weatherService.GetCoordinates(fmt.Sprintf("%f,%f", latitude, longitude), "")
			if err != nil {
				RespondError(c, http.StatusBadRequest, "LOCATION_ERROR", err.Error())
				return
			}
			// Convert to GeocodeResult and enhance
			geocodeResult := &service.GeocodeResult{
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
			enhanced, _ = h.locationEnhancer.EnhanceLocationData(geocodeResult)
		}
	} else if location != "" {
		clientIP := utils.GetClientIP(c)
		coords, err = h.weatherService.ParseAndResolveLocation(location, clientIP)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "LOCATION_ERROR", err.Error())
			return
		}
		// Convert to GeocodeResult and enhance
		geocodeResult := &service.GeocodeResult{
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
		enhanced, _ = h.locationEnhancer.EnhanceLocationData(geocodeResult)
	} else {
		// IP-based location detection
		clientIP := utils.GetClientIP(c)
		coords, err = h.weatherService.GetCoordinatesFromIP(clientIP)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "LOCATION_ERROR", err.Error())
			return
		}
		// Convert to GeocodeResult and enhance
		geocodeResult := &service.GeocodeResult{
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
		enhanced, _ = h.locationEnhancer.EnhanceLocationData(geocodeResult)
	}

	// Determine units
	units := "imperial"
	if unitsParam != "" {
		units = unitsParam
	} else if enhanced.CountryCode != "US" {
		units = "metric"
	}

	// Get current weather
	current, err := h.weatherService.GetCurrentWeather(enhanced.Latitude, enhanced.Longitude, units)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "WEATHER_ERROR", err.Error())
		return
	}

	// Get today's forecast
	forecast, err := h.weatherService.GetForecast(enhanced.Latitude, enhanced.Longitude, 1, units)
	if err != nil {
		forecast = &service.Forecast{Days: []service.ForecastDay{}}
	}

	// Build response
	response := gin.H{
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
		},
		"current": gin.H{
			"temperature":   current.Temperature,
			"feelsLike":     current.FeelsLike,
			"humidity":      current.Humidity,
			"pressure":      current.Pressure,
			"windSpeed":     current.WindSpeed,
			"windDirection": current.WindDirection,
			"windGusts":     current.WindGusts,
			"precipitation": current.Precipitation,
			"cloudCover":    current.CloudCover,
			"weatherCode":   current.WeatherCode,
			"description":   h.weatherService.GetWeatherDescription(current.WeatherCode),
			"icon":          h.weatherService.GetWeatherIcon(current.WeatherCode, current.IsDay == 1),
			"isDay":         current.IsDay,
		},
		"meta": gin.H{
			"source":      "Open-Meteo",
			"timestamp":   utils.Now(),
			"api_version": "1.0",
			"units":       units,
		},
	}

	// Add today forecast if available
	if len(forecast.Days) > 0 {
		day := forecast.Days[0]
		response["today"] = gin.H{
			"date":        day.Date,
			"weatherCode": day.WeatherCode,
			"description": h.weatherService.GetWeatherDescription(day.WeatherCode),
			"icon":        h.weatherService.GetWeatherIcon(day.WeatherCode, true),
			"temperature": gin.H{
				"min": day.TempMin,
				"max": day.TempMax,
			},
			"precipitation": gin.H{
				"sum":         day.Precipitation,
				"probability": day.PrecipitationProbability,
			},
		}
	}

	RespondNegotiatedData(c, http.StatusOK, response)
}

// GetWeatherByLocation returns weather for specific location (GET /api/v1/weather/:location)
func (h *APIHandler) GetWeatherByLocation(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	location := strings.TrimSpace(c.Param("location"))
	unitsParam := strings.TrimSpace(c.Query("units"))

	clientIP := utils.GetClientIP(c)
	coords, err := h.weatherService.ParseAndResolveLocation(location, clientIP)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "WEATHER_ERROR", err.Error())
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units
	units := "imperial"
	if unitsParam != "" {
		units = unitsParam
	} else if enhanced.CountryCode != "US" {
		units = "metric"
	}

	// Get current weather
	current, err := h.weatherService.GetCurrentWeather(enhanced.Latitude, enhanced.Longitude, units)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "WEATHER_ERROR", err.Error())
		return
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
		},
		"current": gin.H{
			"temperature":   current.Temperature,
			"feelsLike":     current.FeelsLike,
			"humidity":      current.Humidity,
			"pressure":      current.Pressure,
			"windSpeed":     current.WindSpeed,
			"windDirection": current.WindDirection,
			"windGusts":     current.WindGusts,
			"precipitation": current.Precipitation,
			"cloudCover":    current.CloudCover,
			"weatherCode":   current.WeatherCode,
			"description":   h.weatherService.GetWeatherDescription(current.WeatherCode),
			"icon":          h.weatherService.GetWeatherIcon(current.WeatherCode, current.IsDay == 1),
			"isDay":         current.IsDay,
		},
		"meta": gin.H{
			"source":      "Open-Meteo",
			"timestamp":   utils.Now(),
			"api_version": "1.0",
			"units":       units,
		},
	})
}

// GetForecast returns multi-day forecast (GET /api/v1/forecast)
// @Summary Get weather forecast
// @Description Get multi-day weather forecast for a location (up to 16 days)
// @Tags Weather
// @Accept json
// @Produce json
// @Param location query string false "Location name (city, address, or place)"
// @Param lat query number false "Latitude coordinate"
// @Param lon query number false "Longitude coordinate"
// @Param days query integer false "Number of forecast days (1-16, default: 7)" minimum(1) maximum(16)
// @Param units query string false "Units system: metric, imperial, or standard (default: metric)" Enums(metric, imperial, standard)
// @Success 200 {object} map[string]interface{} "Forecast data with daily predictions"
// @Failure 400 {object} map[string]interface{} "Invalid request parameters"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/forecast [get]
func (h *APIHandler) GetForecast(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	location := strings.TrimSpace(c.Query("location"))
	lat := strings.TrimSpace(c.Query("lat"))
	lon := strings.TrimSpace(c.Query("lon"))
	daysParam := strings.TrimSpace(c.Query("days"))
	unitsParam := strings.TrimSpace(c.Query("units"))

	// Parse days
	days := 7
	if daysParam != "" {
		if d, err := strconv.Atoi(daysParam); err == nil && d > 0 && d <= 16 {
			days = d
		}
	}

	var coords *service.Coordinates
	var err error

	// Parse coordinates or location
	if lat != "" || lon != "" {
		// Validate that both lat and lon are provided
		if lat == "" || lon == "" {
			InvalidInput(c, "Both 'lat' and 'lon' parameters are required")
			return
		}

		latitude, latErr := strconv.ParseFloat(lat, 64)
		longitude, lonErr := strconv.ParseFloat(lon, 64)

		// Validate coordinate parsing
		if latErr != nil {
			InvalidInput(c, "Invalid latitude format")
			return
		}
		if lonErr != nil {
			InvalidInput(c, "Invalid longitude format")
			return
		}

		// Validate coordinate ranges
		if latitude < -90 || latitude > 90 {
			InvalidInput(c, "Latitude must be between -90 and 90")
			return
		}
		if longitude < -180 || longitude > 180 {
			InvalidInput(c, "Longitude must be between -180 and 180")
			return
		}

		coords, err = h.weatherService.GetCoordinates(fmt.Sprintf("%f,%f", latitude, longitude), "")
	} else if location != "" {
		clientIP := utils.GetClientIP(c)
		coords, err = h.weatherService.ParseAndResolveLocation(location, clientIP)
	} else {
		clientIP := utils.GetClientIP(c)
		coords, err = h.weatherService.GetCoordinatesFromIP(clientIP)
	}

	if err != nil {
		RespondError(c, http.StatusBadRequest, "FORECAST_ERROR", err.Error())
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units
	units := "imperial"
	if unitsParam != "" {
		units = unitsParam
	} else if enhanced.CountryCode != "US" {
		units = "metric"
	}

	// Get forecast
	forecast, err := h.weatherService.GetForecast(enhanced.Latitude, enhanced.Longitude, days, units)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "FORECAST_ERROR", err.Error())
		return
	}

	// Build forecast response
	forecastDays := make([]gin.H, len(forecast.Days))
	for i, day := range forecast.Days {
		forecastDays[i] = gin.H{
			"date":        day.Date,
			"weatherCode": day.WeatherCode,
			"description": h.weatherService.GetWeatherDescription(day.WeatherCode),
			"icon":        h.weatherService.GetWeatherIcon(day.WeatherCode, true),
			"temperature": gin.H{
				"min": day.TempMin,
				"max": day.TempMax,
			},
			"feelsLike": gin.H{
				"min": day.FeelsLikeMin,
				"max": day.FeelsLikeMax,
			},
			"precipitation": gin.H{
				"sum":         day.Precipitation,
				"hours":       day.PrecipitationHours,
				"probability": day.PrecipitationProbability,
			},
			"wind": gin.H{
				"speedMax":  day.WindSpeedMax,
				"gustsMax":  day.WindGustsMax,
				"direction": day.WindDirection,
			},
			"solarRadiation": day.SolarRadiation,
		}
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
		},
		"forecast": gin.H{
			"days": forecastDays,
		},
		"meta": gin.H{
			"source":      "Open-Meteo",
			"timestamp":   utils.Now(),
			"api_version": "1.0",
			"units":       units,
		},
	})
}

// GetForecastByLocation returns forecast for specific location (GET /api/v1/forecast/:location)
func (h *APIHandler) GetForecastByLocation(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	location := strings.TrimSpace(c.Param("location"))
	daysParam := strings.TrimSpace(c.Query("days"))
	unitsParam := strings.TrimSpace(c.Query("units"))

	// Parse days
	days := 7
	if daysParam != "" {
		if d, err := strconv.Atoi(daysParam); err == nil && d > 0 && d <= 16 {
			days = d
		}
	}

	clientIP := utils.GetClientIP(c)
	coords, err := h.weatherService.ParseAndResolveLocation(location, clientIP)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "FORECAST_ERROR", err.Error())
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units
	units := "imperial"
	if unitsParam != "" {
		units = unitsParam
	} else if enhanced.CountryCode != "US" {
		units = "metric"
	}

	// Get forecast
	forecast, err := h.weatherService.GetForecast(enhanced.Latitude, enhanced.Longitude, days, units)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "FORECAST_ERROR", err.Error())
		return
	}

	// Build forecast response
	forecastDays := make([]gin.H, len(forecast.Days))
	for i, day := range forecast.Days {
		forecastDays[i] = gin.H{
			"date":        day.Date,
			"weatherCode": day.WeatherCode,
			"description": h.weatherService.GetWeatherDescription(day.WeatherCode),
			"icon":        h.weatherService.GetWeatherIcon(day.WeatherCode, true),
			"temperature": gin.H{
				"min": day.TempMin,
				"max": day.TempMax,
			},
			"feelsLike": gin.H{
				"min": day.FeelsLikeMin,
				"max": day.FeelsLikeMax,
			},
			"precipitation": gin.H{
				"sum":         day.Precipitation,
				"hours":       day.PrecipitationHours,
				"probability": day.PrecipitationProbability,
			},
			"wind": gin.H{
				"speedMax":  day.WindSpeedMax,
				"gustsMax":  day.WindGustsMax,
				"direction": day.WindDirection,
			},
			"solarRadiation": day.SolarRadiation,
		}
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
		},
		"forecast": gin.H{
			"days": forecastDays,
		},
		"meta": gin.H{
			"source":      "Open-Meteo",
			"timestamp":   utils.Now(),
			"api_version": "1.0",
			"units":       units,
		},
	})
}

// SearchLocations searches for locations (GET /api/v1/search)
// @Summary Search locations
// @Description Search for cities and places by name
// @Tags Locations
// @Accept json
// @Produce json
// @Param q query string true "Search query (city name or place)"
// @Success 200 {object} map[string]interface{} "List of matching locations"
// @Failure 400 {object} map[string]interface{} "Missing or invalid search query"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/search [get]
func (h *APIHandler) SearchLocations(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	query := strings.TrimSpace(c.Query("q"))

	if query == "" {
		InvalidInput(c, "Query parameter 'q' is required")
		return
	}

	// Get limit from query parameter (default 50, max 100)
	limit := 50
	if limitStr := strings.TrimSpace(c.Query("limit")); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				// Cap at 100
				l = 100
			}
			limit = l
		}
	}

	// Search for locations
	results, err := h.weatherService.SearchLocations(query, limit)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "SEARCH_ERROR", err.Error())
		return
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"query":   query,
		"results": results,
		"meta": gin.H{
			"source":      "Open-Meteo Geocoding",
			"timestamp":   utils.Now(),
			"api_version": "1.0",
		},
	})
}

// GetIP returns client IP information (GET /api/v1/ip)
// @Summary Get client IP address
// @Description Get the client's IP address and request headers
// @Tags Utility
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Client IP information and headers"
// @Router /api/v1/ip [get]
func (h *APIHandler) GetIP(c *gin.Context) {
	clientIP := utils.GetClientIP(c)

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"ip":        clientIP,
		"timestamp": utils.Now(),
		"headers": gin.H{
			"x-forwarded-for":  c.GetHeader("X-Forwarded-For"),
			"x-real-ip":        c.GetHeader("X-Real-IP"),
			"cf-connecting-ip": c.GetHeader("CF-Connecting-IP"),
		},
	})
}

// GetLocation returns detected location from IP (GET /api/v1/location)
// @Summary Detect location from IP
// @Description Automatically detect user's location based on their IP address using GeoIP
// @Tags Locations
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Detected location with coordinates and place information"
// @Failure 500 {object} map[string]interface{} "Unable to detect location"
// @Router /api/v1/location [get]
func (h *APIHandler) GetLocation(c *gin.Context) {
	clientIP := utils.GetClientIP(c)

	coords, err := h.weatherService.GetCoordinatesFromIP(clientIP)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "LOCATION_ERROR", err.Error())
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units based on country
	units := "metric"
	if enhanced.CountryCode == "US" {
		units = "imperial"
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"ip": clientIP,
		"location": gin.H{
			"city":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
			"state":       enhanced.Admin1,
			"units":       units,
			"coordinates": gin.H{
				"latitude":  enhanced.Latitude,
				"longitude": enhanced.Longitude,
			},
			"population": enhanced.Population,
			"timezone":   enhanced.Timezone,
		},
		"timestamp": utils.Now(),
		"source":    "IP Geolocation with Coordinates",
	})
}

// GetDocsJSON returns API documentation in JSON format (GET /api/v1/docs)
func (h *APIHandler) GetDocsJSON(c *gin.Context) {
	hostInfo := utils.GetHostInfo(c)

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"service":     "Weather API",
		"version":     "2.0.0",
		"description": "Free weather API with no API key required",
		"base_url":    hostInfo.FullHost,
		"endpoints": gin.H{
			"weather": gin.H{
				"GET /api/v1/weather":           "Get weather for current location (IP-based)",
				"GET /api/v1/weather/:location": "Get weather for specific location",
			},
			"forecast": gin.H{
				"GET /api/v1/forecast":           "Get forecast for current location (IP-based)",
				"GET /api/v1/forecast/:location": "Get forecast for specific location",
			},
			"location": gin.H{
				"GET /api/v1/location": "Get current location from IP",
				"GET /api/v1/search":   "Search for locations by name",
			},
			"utility": gin.H{
				"GET /api/v1/ip":   "Get client IP address",
				"GET /api/v1/docs": "API documentation (JSON)",
			},
		},
		"parameters": gin.H{
			"location": "City name, coordinates (lat,lon), or zipcode",
			"units":    "imperial (°F, mph) or metric (°C, km/h). Default: imperial",
			"days":     "Number of forecast days (1-7). Default: 7",
			"q":        "Search query for location search",
		},
		"examples": []string{
			hostInfo.FullHost + "/api/v1/weather?location=London",
			hostInfo.FullHost + "/api/v1/forecast?location=Paris&days=5",
			hostInfo.FullHost + "/api/v1/search?q=New+York",
			hostInfo.FullHost + "/api/v1/ip",
		},
	})
}

// GetHistoricalWeather returns historical weather data for a specific day across multiple years (GET /api/v1/history)
func (h *APIHandler) GetHistoricalWeather(c *gin.Context) {
	// AI.md: Strip leading/trailing whitespace from all user inputs
	location := strings.TrimSpace(c.Query("location"))
	lat := strings.TrimSpace(c.Query("lat"))
	lon := strings.TrimSpace(c.Query("lon"))
	dateStr := strings.TrimSpace(c.Query("date"))
	yearsStr := strings.TrimSpace(c.Query("years"))

	// Parse coordinates
	var coords *service.Coordinates
	var err error

	if lat != "" || lon != "" {
		// Validate that both lat and lon are provided
		if lat == "" || lon == "" {
			InvalidInput(c, "Both 'lat' and 'lon' parameters are required")
			return
		}

		// Use provided coordinates
		latitude, latErr := strconv.ParseFloat(lat, 64)
		longitude, lonErr := strconv.ParseFloat(lon, 64)

		// Validate coordinate parsing
		if latErr != nil {
			InvalidInput(c, "Invalid latitude format")
			return
		}
		if lonErr != nil {
			InvalidInput(c, "Invalid longitude format")
			return
		}

		// Validate coordinate ranges per AI.md
		if latitude < -90 || latitude > 90 {
			InvalidInput(c, "Latitude must be between -90 and 90")
			return
		}
		if longitude < -180 || longitude > 180 {
			InvalidInput(c, "Longitude must be between -180 and 180")
			return
		}

		coords = &service.Coordinates{
			Latitude:  latitude,
			Longitude: longitude,
		}
	} else if location != "" {
		// Geocode location
		coords, err = h.weatherService.GetCoordinates(location, "")
		if err != nil {
			NotFound(c, fmt.Sprintf("Could not find location: %s", location))
			return
		}
	} else {
		InvalidInput(c, "Please provide either 'location' or 'lat' and 'lon' parameters")
		return
	}

	// Parse date
	month, day, startYear, err := parseHistoricalDateAPI(dateStr)
	if err != nil {
		InvalidInput(c, fmt.Sprintf("Invalid date format: %v", err))
		return
	}

	// Parse number of years (default: 10)
	numberOfYears := 10
	if yearsStr != "" {
		years, err := strconv.Atoi(yearsStr)
		if err != nil || years < 1 || years > 100 {
			InvalidInput(c, "Number of years must be between 1 and 100")
			return
		}
		numberOfYears = years
	}

	// Fetch historical weather data
	historical, err := h.weatherService.GetHistoricalWeather(
		coords.Latitude,
		coords.Longitude,
		month,
		day,
		startYear,
		numberOfYears,
	)
	if err != nil {
		InternalError(c, fmt.Sprintf("Error fetching historical weather: %v", err))
		return
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"historical": historical,
		"location": gin.H{
			"latitude":  coords.Latitude,
			"longitude": coords.Longitude,
		},
	})
}

// parseHistoricalDateAPI parses a date string for the API
// Supported formats: MM/DD, MM/DD/YYYY, YYYY-MM-DD
func parseHistoricalDateAPI(dateStr string) (month, day, year int, err error) {
	if dateStr == "" {
		// Default to today
		now := time.Now()
		return int(now.Month()), now.Day(), now.Year(), nil
	}

	// Try ISO format: YYYY-MM-DD
	parsedDate, err := time.Parse("2006-01-02", dateStr)
	if err == nil {
		return int(parsedDate.Month()), parsedDate.Day(), parsedDate.Year(), nil
	}

	// Try MM/DD/YYYY
	parsedDate, err = time.Parse("01/02/2006", dateStr)
	if err == nil {
		return int(parsedDate.Month()), parsedDate.Day(), parsedDate.Year(), nil
	}

	// Try MM/DD (no year - use current year)
	parsedDate, err = time.Parse("01/02", dateStr)
	if err == nil {
		currentYear := time.Now().Year()
		return int(parsedDate.Month()), parsedDate.Day(), currentYear, nil
	}

	return 0, 0, 0, fmt.Errorf("unsupported date format: %s (use MM/DD, MM/DD/YYYY, or YYYY-MM-DD)", dateStr)
}

// GetDocsHTML returns API documentation as HTML page (GET /docs)
func (h *APIHandler) GetDocsHTML(c *gin.Context) {
	hostInfo := utils.GetHostInfo(c)

	NegotiateResponse(c, "page/api_docs.tmpl", gin.H{
		"Title":      "API Documentation - Weather",
		"HostInfo":   hostInfo,
		"HideFooter": false,
	})
}
