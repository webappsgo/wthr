package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/server/service"
	"github.com/casapps/wthr/src/util"
)

// MoonHandler handles moon phase API requests
type MoonHandler struct {
	weatherService   *service.WeatherService
	locationEnhancer *service.LocationEnhancer
	moonService      *service.MoonService
}

// NewMoonHandler creates a new moon handler
func NewMoonHandler(ws *service.WeatherService, le *service.LocationEnhancer) *MoonHandler {
	return &MoonHandler{
		weatherService:   ws,
		locationEnhancer: le,
		moonService:      service.NewMoonService(),
	}
}

// GetMoonData returns moon data for a given date at a specific location
// Used by GraphQL resolvers to access MoonService
func (h *MoonHandler) GetMoonData(lat, lon float64, date time.Time) *service.MoonData {
	return h.moonService.Calculate(lat, lon, date)
}

// HandleMoonAPI handles GET /api/v1/moon
// @Summary Get moon phase data
// @Description Get current moon phase, illumination, rise/set times, and sun data for a location
// @Tags moon
// @Accept json
// @Produce json
// @Param location query string false "Location (city name, zip code, or coordinates)"
// @Param lat query number false "Latitude (use with lon instead of location)"
// @Param lon query number false "Longitude (use with lat instead of location)"
// @Success 200 {object} map[string]interface{} "Moon and sun data for location"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid coordinates"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/moon [get]
func (h *MoonHandler) HandleMoonAPI(c *gin.Context) {
	coords, enhanced, err := h.getLocationFromRequest(c)
	if err != nil {
		return // Error already sent
	}

	// Calculate moon data using astronomical algorithms
	moon := h.moonService.Calculate(enhanced.Latitude, enhanced.Longitude, time.Now())

	// Calculate sun times
	sunData := h.calculateSunTimes(coords.Latitude, coords.Longitude, time.Now())

	moonData := gin.H{
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
		},
		"moon": gin.H{
			"phase":        moon.Phase,
			"illumination": moon.Illumination,
			"icon":         moon.Icon,
			"age":          moon.Age,
			"rise":         moon.Rise,
			"set":          moon.Set,
			"nextNewMoon":  moon.NextNewMoon,
			"nextFullMoon": moon.NextFullMoon,
			"distance_km":  moon.Distance,
			"angular_size": moon.AngularSize,
		},
		"sun": sunData,
	}

	RespondNegotiatedData(c, http.StatusOK, moonData)
}

// HandleMoonCalendarAPI handles GET /api/v1/moon/calendar
// @Summary Get moon phase calendar
// @Description Get moon phases for each day of a month
// @Tags moon
// @Accept json
// @Produce json
// @Param location query string false "Location (city name, zip code, or coordinates)"
// @Param lat query number false "Latitude (use with lon instead of location)"
// @Param lon query number false "Longitude (use with lat instead of location)"
// @Param year query integer false "Year (1900-2100)" default(current year)
// @Param month query integer false "Month (1-12)" default(current month)
// @Success 200 {object} map[string]interface{} "Moon calendar with daily phases"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/moon/calendar [get]
func (h *MoonHandler) HandleMoonCalendarAPI(c *gin.Context) {
	coords, enhanced, err := h.getLocationFromRequest(c)
	if err != nil {
		return // Error already sent
	}

	// Get year and month parameters
	yearStr := c.DefaultQuery("year", "")
	monthStr := c.DefaultQuery("month", "")

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil && y >= 1900 && y <= 2100 {
			year = y
		}
	}

	if monthStr != "" {
		if m, err := strconv.Atoi(monthStr); err == nil && m >= 1 && m <= 12 {
			month = m
		}
	}

	// Calculate moon phases for each day of the month
	startDate := time.Date(year, time.Month(month), 1, 12, 0, 0, 0, time.UTC)
	daysInMonth := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()

	days := make([]gin.H, daysInMonth)
	for i := 0; i < daysInMonth; i++ {
		date := startDate.AddDate(0, 0, i)
		moon := h.moonService.Calculate(coords.Latitude, coords.Longitude, date)
		days[i] = gin.H{
			"date":         date.Format("2006-01-02"),
			"phase":        moon.Phase,
			"illumination": moon.Illumination,
			"icon":         moon.Icon,
			"age":          moon.Age,
		}
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"ok": true,
		"location": gin.H{
			"name":      enhanced.Name,
			"shortName": enhanced.ShortName,
			"latitude":  enhanced.Latitude,
			"longitude": enhanced.Longitude,
		},
		"calendar": gin.H{
			"year":  year,
			"month": month,
			"days":  days,
		},
	})
}

// HandleSunAPI handles GET /api/v1/sun
// @Summary Get sun times
// @Description Get sunrise, sunset, solar noon, and day length for a location
// @Tags sun
// @Accept json
// @Produce json
// @Param location query string false "Location (city name, zip code, or coordinates)"
// @Param lat query number false "Latitude (use with lon instead of location)"
// @Param lon query number false "Longitude (use with lat instead of location)"
// @Param date query string false "Date in YYYY-MM-DD format" default(today)
// @Success 200 {object} map[string]interface{} "Sun times for location"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/sun [get]
func (h *MoonHandler) HandleSunAPI(c *gin.Context) {
	coords, enhanced, err := h.getLocationFromRequest(c)
	if err != nil {
		return // Error already sent
	}

	// Get optional date parameter
	dateStr := c.DefaultQuery("date", "")
	date := time.Now()

	if dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			date = d
		}
	}

	sunData := h.calculateSunTimes(coords.Latitude, coords.Longitude, date)

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"ok": true,
		"location": gin.H{
			"name":        enhanced.Name,
			"shortName":   enhanced.ShortName,
			"fullName":    enhanced.FullName,
			"latitude":    enhanced.Latitude,
			"longitude":   enhanced.Longitude,
			"timezone":    enhanced.Timezone,
			"country":     enhanced.Country,
			"countryCode": enhanced.CountryCode,
		},
		"date": date.Format("2006-01-02"),
		"sun":  sunData,
	})
}

// getLocationFromRequest extracts location from request parameters
func (h *MoonHandler) getLocationFromRequest(c *gin.Context) (*service.Coordinates, *service.Coordinates, error) {
	location := c.Query("location")
	lat := c.Query("lat")
	lon := c.Query("lon")
	clientIP := utils.GetClientIP(c)

	var coords *service.Coordinates
	var err error

	// Priority: lat/lon > location > cookie > IP geolocation
	if lat != "" && lon != "" {
		latFloat, err1 := strconv.ParseFloat(lat, 64)
		lonFloat, err2 := strconv.ParseFloat(lon, 64)
		if err1 != nil || err2 != nil {
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Invalid coordinates format")
			return nil, nil, err1
		}
		coords = &service.Coordinates{
			Latitude:  latFloat,
			Longitude: lonFloat,
		}
	} else if location != "" {
		coords, err = h.weatherService.ParseAndResolveLocation(location, clientIP)
		if err != nil {
			RespondError(c, http.StatusBadRequest, ErrInvalidInput, err.Error())
			return nil, nil, err
		}
	} else {
		// Try cookies first
		if latStr, err := c.Cookie("user_lat"); err == nil {
			if lonStr, err := c.Cookie("user_lon"); err == nil {
				if latFloat, err1 := strconv.ParseFloat(latStr, 64); err1 == nil {
					if lonFloat, err2 := strconv.ParseFloat(lonStr, 64); err2 == nil {
						coords = &service.Coordinates{
							Latitude:  latFloat,
							Longitude: lonFloat,
						}
					}
				}
			}
		}

		// Fallback to IP geolocation
		if coords == nil {
			coords, err = h.weatherService.GetCoordinatesFromIP(clientIP)
			if err != nil {
				RespondError(c, http.StatusInternalServerError, ErrInternal, "Could not determine location")
				return nil, nil, err
			}
		}
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	return coords, enhanced, nil
}

// calculateSunTimes calculates sunrise, sunset, and related times
// Uses basic astronomical algorithms
func (h *MoonHandler) calculateSunTimes(lat, lon float64, date time.Time) gin.H {
	// Simplified sun calculation based on location and date
	// For accurate production use, would integrate with Open-Meteo or similar

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
	eqtime := 229.18 * (0.000075 + 0.001868*cos(gamma) - 0.032077*sin(gamma) -
		0.014615*cos(2*gamma) - 0.040849*sin(2*gamma))

	// Solar declination (radians)
	decl := 0.006918 - 0.399912*cos(gamma) + 0.070257*sin(gamma) -
		0.006758*cos(2*gamma) + 0.000907*sin(2*gamma) -
		0.002697*cos(3*gamma) + 0.00148*sin(3*gamma)

	// Convert latitude to radians
	latRad := lat * 3.14159265359 / 180.0

	// Hour angle at sunrise/sunset (cos)
	cosHa := (cos(90.833*3.14159265359/180.0) / (cos(latRad) * cos(decl))) - tan(latRad)*tan(decl)

	// Clamp for polar regions
	if cosHa > 1 {
		// No sunrise (polar night)
		return gin.H{
			"sunrise":   nil,
			"sunset":    nil,
			"solarNoon": formatTime(12, 0),
			"dayLength": "0h 0m",
			"note":      "Polar night - no sunrise",
		}
	} else if cosHa < -1 {
		// No sunset (midnight sun)
		return gin.H{
			"sunrise":   nil,
			"sunset":    nil,
			"solarNoon": formatTime(12, 0),
			"dayLength": "24h 0m",
			"note":      "Midnight sun - no sunset",
		}
	}

	// Hour angle in degrees
	ha := acos(cosHa) * 180 / 3.14159265359

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

	return gin.H{
		"sunrise":   formatTime(sunriseHour, sunriseMin),
		"sunset":    formatTime(sunsetHour, sunsetMin),
		"solarNoon": formatTime(noonHour, noonMin),
		"dayLength": formatDuration(dayLengthHours, dayLengthMins),
	}
}

// Helper math functions
func sin(x float64) float64 {
	return math.Sin(x)
}

func cos(x float64) float64 {
	return math.Cos(x)
}

func tan(x float64) float64 {
	return math.Tan(x)
}

func acos(x float64) float64 {
	return math.Acos(x)
}

func formatTime(hour, min int) string {
	if min < 0 {
		min = 0
	}
	if min > 59 {
		min = 59
	}
	return fmt.Sprintf("%02d:%02d", hour, min)
}

func formatDuration(hours, mins int) string {
	if mins < 0 {
		mins = 0
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}
