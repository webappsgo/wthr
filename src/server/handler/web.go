package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/service"
	"github.com/casapps/wthr/src/util"
)

// WebHandler handles web interface routes (HTML pages)
type WebHandler struct {
	weatherService   *service.WeatherService
	locationEnhancer *service.LocationEnhancer
}

// NewWebHandler creates a new web handler
func NewWebHandler(ws *service.WeatherService, le *service.LocationEnhancer) *WebHandler {
	return &WebHandler{
		weatherService:   ws,
		locationEnhancer: le,
	}
}

// ServeWebInterface serves the main web interface
func (h *WebHandler) ServeWebInterface(c *gin.Context) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(c)
		return
	}

	location := c.Query("location")
	units := c.Query("units")
	if units == "" {
		units = "imperial"
	}

	var weatherData interface{}
	var errorMsg string
	clientIP := utils.GetClientIP(c)

	// Auto-detect location using priority order:
	// 1. URL parameter (already checked above)
	// 2. Saved cookies
	// 3. IP geolocation (fallback)
	if location == "" {
		// Check cookies first
		if latStr, err := c.Cookie("user_lat"); err == nil {
			if lonStr, err := c.Cookie("user_lon"); err == nil {
				if _, err1 := strconv.ParseFloat(latStr, 64); err1 == nil {
					if _, err2 := strconv.ParseFloat(lonStr, 64); err2 == nil {
						if locationName, err := c.Cookie("user_location_name"); err == nil {
							location = locationName
						}
					}
				}
			}
		}

		// If still no location, use IP geolocation as fallback
		if location == "" {
			coords, err := h.weatherService.GetCoordinatesFromIP(clientIP)
			if err == nil {
				enhanced := h.locationEnhancer.EnhanceLocation(coords)
				location = enhanced.ShortName
			}
		}
	}

	// If location is available (provided or auto-detected), fetch weather
	if location != "" {
		coords, err := h.weatherService.ParseAndResolveLocation(location, clientIP)
		if err != nil {
			errorMsg = err.Error()
		} else {
			// Enhance location
			enhanced := h.locationEnhancer.EnhanceLocation(coords)

			// Get current weather and forecast
			current, err := h.weatherService.GetCurrentWeather(enhanced.Latitude, enhanced.Longitude, units)
			if err != nil {
				errorMsg = err.Error()
			} else {
				forecast, _ := h.weatherService.GetForecast(enhanced.Latitude, enhanced.Longitude, 16, units)

				// Enrich current weather with icon and description
				currentData := gin.H{
					"Temperature":   current.Temperature,
					"FeelsLike":     current.FeelsLike,
					"Humidity":      current.Humidity,
					"Pressure":      current.Pressure,
					"WindSpeed":     current.WindSpeed,
					"WindDirection": current.WindDirection,
					"Precipitation": current.Precipitation,
					"WeatherCode":   current.WeatherCode,
					"Icon":          h.weatherService.GetWeatherIcon(current.WeatherCode, current.IsDay == 1),
					"Description":   h.weatherService.GetWeatherDescription(current.WeatherCode),
				}

				// Format location data
				locationData := gin.H{
					"Name":                enhanced.Name,
					"ShortName":           enhanced.ShortName,
					"FullName":            enhanced.FullName,
					"Latitude":            enhanced.Latitude,
					"Longitude":           enhanced.Longitude,
					"Country":             enhanced.Country,
					"CountryCode":         enhanced.CountryCode,
					"Population":          enhanced.Population,
					"PopulationFormatted": fmt.Sprintf("%d", enhanced.Population),
					"Timezone":            enhanced.Timezone,
				}

				weatherData = gin.H{
					"Location": locationData,
					"Current":  currentData,
					"Forecast": forecast,
					"Units":    units,
				}
			}
		}
	}

	hostInfo := utils.GetHostInfo(c)

	// Format location for URLs (replace spaces with +, keep commas)
	locationFormatted := strings.ReplaceAll(location, " ", "+")

	NegotiateResponse(c, "page/weather.tmpl", gin.H{
		"Title":             "Weather",
		"WeatherData":       weatherData,
		"HostInfo":          hostInfo,
		"Location":          location,
		"LocationFormatted": locationFormatted,
		"Units":             units,
		"Error":             errorMsg,
		"HideFooter":        false,
	})
}

// ServeMoonInterface serves the moon phase interface
func (h *WebHandler) ServeMoonInterface(c *gin.Context) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(c)
		return
	}

	// Get location from path parameter or query
	location := c.Param("location")
	if location == "" {
		location = c.Query("location")
	}

	// Get units from query parameter (default to imperial)
	units := c.Query("units")
	if units == "" {
		units = "imperial"
	}

	clientIP := utils.GetClientIP(c)

	// Auto-detect location using priority order:
	// 1. URL parameter (already checked above)
	// 2. Saved cookies
	// 3. IP geolocation (fallback)
	if location == "" {
		// Check cookies first
		if latStr, err := c.Cookie("user_lat"); err == nil {
			if lonStr, err := c.Cookie("user_lon"); err == nil {
				if _, err1 := strconv.ParseFloat(latStr, 64); err1 == nil {
					if _, err2 := strconv.ParseFloat(lonStr, 64); err2 == nil {
						if locationName, err := c.Cookie("user_location_name"); err == nil {
							location = locationName
						}
					}
				}
			}
		}

		// If still no location, use IP geolocation as fallback
		if location == "" {
			coords, err := h.weatherService.GetCoordinatesFromIP(clientIP)
			if err == nil {
				enhanced := h.locationEnhancer.EnhanceLocation(coords)
				location = enhanced.ShortName
			}
		}
	}

	// If still no location, show empty moon page
	if location == "" {
		NegotiateResponse(c, "page/moon.tmpl", gin.H{
			"Title":      "Moon Phase - Weather",
			"HostInfo":   utils.GetHostInfo(c),
			"Location":   "",
			"Units":      units,
			"HideFooter": false,
		})
		return
	}

	var coords *service.Coordinates
	var err error

	coords, err = h.weatherService.ParseAndResolveLocation(location, clientIP)

	if err != nil {
		NegotiateErrorResponse(c, http.StatusInternalServerError, "page/moon.tmpl", ErrInternal, err.Error(), gin.H{
			"Title":      "Moon Phase - Weather",
			"Error":      err.Error(),
			"HostInfo":   utils.GetHostInfo(c),
			"Location":   location,
			"Units":      units,
			"HideFooter": false,
		})
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Save location to cookies for persistence across navigation
	middleware.SaveLocationCookies(c, enhanced.Latitude, enhanced.Longitude, enhanced.ShortName)

	// Get moon data from moon service
	moonService := service.NewMoonService()
	moonDataCalc := moonService.Calculate(enhanced.Latitude, enhanced.Longitude, time.Now())

	// Calculate sun times
	sunTimes := calculateSunTimesForWeb(enhanced.Latitude, enhanced.Longitude, time.Now())

	// Parse next new moon and full moon dates for formatted display
	nextNewMoon, _ := time.Parse(time.RFC3339, moonDataCalc.NextNewMoon)
	nextFullMoon, _ := time.Parse(time.RFC3339, moonDataCalc.NextFullMoon)

	moonData := gin.H{
		"Location": gin.H{
			"Name":                enhanced.Name,
			"ShortName":           enhanced.ShortName,
			"FullName":            enhanced.FullName,
			"NameEncoded":         strings.ReplaceAll(enhanced.ShortName, " ", "+"),
			"Latitude":            enhanced.Latitude,
			"Longitude":           enhanced.Longitude,
			"Timezone":            enhanced.Timezone,
			"Country":             enhanced.Country,
			"CountryCode":         enhanced.CountryCode,
			"Population":          enhanced.Population,
			"PopulationFormatted": fmt.Sprintf("%d", enhanced.Population),
		},
		"Moon": gin.H{
			"Phase":             moonDataCalc.Phase,
			"Illumination":      fmt.Sprintf("%.1f", moonDataCalc.Illumination),
			"Icon":              moonDataCalc.Icon,
			"Age":               moonDataCalc.Age,
			"AgeFmt":            fmt.Sprintf("%.1f", moonDataCalc.Age),
			"Rise":              moonDataCalc.Rise,
			"Set":               moonDataCalc.Set,
			"RiseFormatted":     formatTimeToAMPM(moonDataCalc.Rise),
			"SetFormatted":      formatTimeToAMPM(moonDataCalc.Set),
			"NextNewMoon":       moonDataCalc.NextNewMoon,
			"NextNewMoonFmt":    nextNewMoon.Format("Jan 2, 2006"),
			"NextFullMoon":      moonDataCalc.NextFullMoon,
			"NextFullMoonFmt":   nextFullMoon.Format("Jan 2, 2006"),
			"Distance":          moonDataCalc.Distance,
			"DistanceFormatted": formatDistance(moonDataCalc.Distance),
			"AngularSize":       moonDataCalc.AngularSize,
			"AngularSizeFmt":    fmt.Sprintf("%.4f°", moonDataCalc.AngularSize),
		},
		"Sun": sunTimes,
	}

	// Always use full location (ShortName) for clarity
	// This shows "Albany, NY" instead of just "Albany"
	displayLocation := enhanced.ShortName

	c.HTML(http.StatusOK, "page/moon.tmpl", gin.H{
		"Title":             "Moon Phase - " + enhanced.ShortName,
		"MoonData":          moonData,
		"HostInfo":          utils.GetHostInfo(c),
		"Location":          displayLocation,
		"LocationFormatted": strings.ReplaceAll(enhanced.ShortName, " ", "+"),
		"Units":             units,
		"HideFooter":        false,
	})
}

// ServeExamplesPage serves the examples/documentation page
func (h *WebHandler) ServeExamplesPage(c *gin.Context) {
	hostInfo := utils.GetHostInfo(c)

	c.HTML(http.StatusOK, "examples.tmpl", gin.H{
		"title":    "Examples - Weather",
		"hostInfo": hostInfo,
	})
}

// calculateSunTimesForWeb calculates sun times for the web template
func calculateSunTimesForWeb(lat, lon float64, date time.Time) gin.H {
	// Calculate day of year
	dayOfYear := date.YearDay()

	// Calculate fractional year
	year := float64(date.Year())
	daysInYear := 365.0
	if year == float64(int(year/4)*4) { // Leap year check
		daysInYear = 366.0
	}
	gamma := 2 * 3.14159265359 / daysInYear * (float64(dayOfYear) - 1 + (12-12)/24)

	// Equation of time (minutes)
	eqtime := 229.18 * (0.000075 + 0.001868*cosVal(gamma) - 0.032077*sinVal(gamma) -
		0.014615*cosVal(2*gamma) - 0.040849*sinVal(2*gamma))

	// Solar declination (radians)
	decl := 0.006918 - 0.399912*cosVal(gamma) + 0.070257*sinVal(gamma) -
		0.006758*cosVal(2*gamma) + 0.000907*sinVal(2*gamma) -
		0.002697*cosVal(3*gamma) + 0.00148*sinVal(3*gamma)

	// Convert latitude to radians
	latRad := lat * 3.14159265359 / 180.0

	// Hour angle at sunrise/sunset (cos)
	cosHa := (cosVal(90.833*3.14159265359/180.0) / (cosVal(latRad) * cosVal(decl))) - tanVal(latRad)*tanVal(decl)

	// Clamp for polar regions
	if cosHa > 1 {
		return gin.H{
			"SunriseFormatted":   "No sunrise",
			"SunsetFormatted":    "No sunset",
			"SolarNoonFormatted": "12:00 PM",
			"DayLengthFormatted": "0h 0m",
		}
	} else if cosHa < -1 {
		return gin.H{
			"SunriseFormatted":   "Midnight sun",
			"SunsetFormatted":    "Midnight sun",
			"SolarNoonFormatted": "12:00 PM",
			"DayLengthFormatted": "24h 0m",
		}
	}

	// Hour angle in degrees
	ha := acosVal(cosHa) * 180 / 3.14159265359

	// Calculate sunrise and sunset in minutes from midnight UTC
	sunrise := 720 - 4*(lon+ha) - eqtime
	sunset := 720 - 4*(lon-ha) - eqtime
	solarNoon := 720 - 4*lon - eqtime

	// Day length in minutes
	dayLength := sunset - sunrise

	// Format times
	sunriseHour := int(sunrise / 60)
	sunriseMin := int(sunrise) % 60
	sunsetHour := int(sunset / 60)
	sunsetMin := int(sunset) % 60
	noonHour := int(solarNoon / 60)
	noonMin := int(solarNoon) % 60

	// Adjust for timezone (approximate based on longitude)
	tzOffset := int(lon / 15)
	sunriseHour = (sunriseHour + tzOffset + 24) % 24
	sunsetHour = (sunsetHour + tzOffset + 24) % 24
	noonHour = (noonHour + tzOffset + 24) % 24

	dayLengthHours := int(dayLength / 60)
	dayLengthMins := int(dayLength) % 60
	if dayLengthMins < 0 {
		dayLengthMins = 0
	}

	return gin.H{
		"SunriseFormatted":   formatHourMinToAMPM(sunriseHour, sunriseMin),
		"SunsetFormatted":    formatHourMinToAMPM(sunsetHour, sunsetMin),
		"SolarNoonFormatted": formatHourMinToAMPM(noonHour, noonMin),
		"DayLengthFormatted": fmt.Sprintf("%dh %dm", dayLengthHours, dayLengthMins),
	}
}

// formatTimeToAMPM converts 24h "HH:MM" to 12h "H:MM AM/PM"
func formatTimeToAMPM(timeStr string) string {
	var hour, min int
	fmt.Sscanf(timeStr, "%d:%d", &hour, &min)
	return formatHourMinToAMPM(hour, min)
}

// formatHourMinToAMPM formats hour and minute to 12h "H:MM AM/PM"
func formatHourMinToAMPM(hour, min int) string {
	if min < 0 {
		min = 0
	}
	if min > 59 {
		min = 59
	}
	ampm := "AM"
	if hour >= 12 {
		ampm = "PM"
	}
	h := hour % 12
	if h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d %s", h, min, ampm)
}

// formatDistance formats distance in km with thousands separator
func formatDistance(km float64) string {
	return fmt.Sprintf("%.0f km", km)
}

// Math helper functions for sun calculations (wrappers around math package)
func sinVal(x float64) float64 {
	return math.Sin(x)
}

func cosVal(x float64) float64 {
	return math.Cos(x)
}

func tanVal(x float64) float64 {
	return math.Tan(x)
}

func acosVal(x float64) float64 {
	return math.Acos(x)
}
