package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/renderer"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// WeatherHandler handles main weather routes (/, /:location)
type WeatherHandler struct {
	weatherService   *service.WeatherService
	locationEnhancer *service.LocationEnhancer
	asciiRenderer    *renderer.ASCIIRenderer
	oneLineRenderer  *renderer.OneLineRenderer
}

// NewWeatherHandler creates a new weather handler
func NewWeatherHandler(ws *service.WeatherService, le *service.LocationEnhancer) *WeatherHandler {
	return &WeatherHandler{
		weatherService:   ws,
		locationEnhancer: le,
		asciiRenderer:    renderer.NewASCIIRenderer(),
		oneLineRenderer:  renderer.NewOneLineRenderer(),
	}
}

// HandleRoot serves the root endpoint (uses saved location, then IP detection)
func (h *WeatherHandler) HandleRoot(c *gin.Context) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(c)
		return
	}

	clientIP := util.GetClientIP(c)
	isBrowser := util.IsBrowser(c)
	params := util.ParseQueryParams(c)

	// Check for location query parameter
	locationQuery := c.Query("location")
	if locationQuery != "" {
		// If location param provided, redirect to path-based route
		c.Redirect(http.StatusMovedPermanently, "/"+strings.ReplaceAll(locationQuery, " ", "+"))
		return
	}

	var coords *service.Coordinates
	var err error

	// Priority 1: Check for saved location in cookies
	if latStr, latErr := c.Cookie("user_lat"); latErr == nil {
		if lonStr, lonErr := c.Cookie("user_lon"); lonErr == nil {
			if lat, err1 := strconv.ParseFloat(latStr, 64); err1 == nil {
				if lon, err2 := strconv.ParseFloat(lonStr, 64); err2 == nil {
					// Get location name from cookie or reverse geocode
					locationName, _ := c.Cookie("user_location_name")
					if locationName == "" {
						locationName = fmt.Sprintf("%.4f,%.4f", lat, lon)
					}

					// Create coordinates from cookies
					coords = &service.Coordinates{
						Latitude:  lat,
						Longitude: lon,
						Name:      locationName,
						ShortName: locationName,
					}
				}
			}
		}
	}

	// Priority 2: Use IP-based location detection
	if coords == nil {
		coords, err = h.weatherService.GetCoordinatesFromIP(clientIP)
		if err != nil {
			h.handleError(c, err, "", isBrowser)
			return
		}
	}

	// First enhancement to get ShortName
	tempEnhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Parse and resolve the location to get the best match (like moon page does)
	coords, err = h.weatherService.ParseAndResolveLocation(tempEnhanced.ShortName, clientIP)
	if err != nil {
		// Fall back to original coords if parsing fails
		coords = &service.Coordinates{
			Latitude:    tempEnhanced.Latitude,
			Longitude:   tempEnhanced.Longitude,
			Name:        tempEnhanced.Name,
			Country:     tempEnhanced.Country,
			CountryCode: tempEnhanced.CountryCode,
			Timezone:    tempEnhanced.Timezone,
			Admin1:      tempEnhanced.Admin1,
			Admin2:      tempEnhanced.Admin2,
			Population:  tempEnhanced.Population,
			FullName:    tempEnhanced.FullName,
			ShortName:   tempEnhanced.ShortName,
		}
	}

	// Final enhancement with the resolved location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units (auto-detect based on country if not specified)
	units := util.GetUnits(params, enhanced.CountryCode)

	// If browser and no explicit format requested, serve HTML
	if isBrowser && params.Format == 0 && !params.ForceANSI {
		h.serveHTMLWeather(c, enhanced, units, enhanced.ShortName)
		return
	}

	// Console clients get ASCII output
	h.serveASCIIWeather(c, enhanced, units, params, "")
}

// HandleLocation serves weather for a specific location
func (h *WeatherHandler) HandleLocation(c *gin.Context) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(c)
		return
	}

	locationInput := strings.TrimPrefix(c.Request.URL.Path, "/")
	// URL decode: convert + to space (standard URL encoding)
	locationInput = strings.ReplaceAll(locationInput, "+", " ")

	// Remove common country suffixes from URLs (e.g., ", US", ", United States")
	// This handles cases where users include country in the URL
	countrySuffixes := []string{", US", ", United States", ", UK", ", United Kingdom", ", CA", ", Canada"}
	for _, suffix := range countrySuffixes {
		if strings.HasSuffix(locationInput, suffix) {
			locationInput = strings.TrimSuffix(locationInput, suffix)
			break
		}
	}

	// Handle special endpoints
	if h.handleSpecialEndpoints(c, locationInput) {
		return
	}

	// Handle moon requests
	if strings.HasPrefix(strings.ToLower(locationInput), "moon") {
		h.handleMoonRequest(c, locationInput)
		return
	}

	// Allow GPS coordinates (skip invalid path check for coordinates)
	isGPS := h.isGPSCoordinates(locationInput)

	// Filter invalid paths (but not GPS coordinates)
	if !isGPS && h.isInvalidPath(locationInput) {
		c.String(http.StatusNotFound, "404 Not Found\n")
		return
	}

	clientIP := util.GetClientIP(c)
	isBrowser := util.IsBrowser(c)
	params := util.ParseQueryParams(c)

	// Parse location
	coords, err := h.weatherService.ParseAndResolveLocation(locationInput, clientIP)
	if err != nil {
		h.handleError(c, err, locationInput, isBrowser)
		return
	}

	// Check if this is a zipcode - if so, don't do double-pass enhancement
	// Zipcode lookups are already precise and don't need re-resolution
	isZipcode := len(locationInput) == 5 && locationInput[0] >= '0' && locationInput[0] <= '9'

	if !isZipcode {
		// First enhancement to get ShortName (like HandleRoot does)
		tempEnhanced := h.locationEnhancer.EnhanceLocation(coords)

		// Re-parse with the enhanced ShortName to get better results
		coords, err = h.weatherService.ParseAndResolveLocation(tempEnhanced.ShortName, clientIP)
		if err != nil {
			// Fall back to original coords if re-parsing fails
			coords = &service.Coordinates{
				Latitude:    tempEnhanced.Latitude,
				Longitude:   tempEnhanced.Longitude,
				Name:        tempEnhanced.Name,
				Country:     tempEnhanced.Country,
				CountryCode: tempEnhanced.CountryCode,
				Timezone:    tempEnhanced.Timezone,
				Admin1:      tempEnhanced.Admin1,
				Admin2:      tempEnhanced.Admin2,
				Population:  tempEnhanced.Population,
				FullName:    tempEnhanced.FullName,
				ShortName:   tempEnhanced.ShortName,
			}
		}
	}

	// Final enhancement with the resolved location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Determine units
	units := util.GetUnits(params, enhanced.CountryCode)

	// If browser and no explicit format requested, serve HTML
	if isBrowser && params.Format == 0 && !params.ForceANSI {
		h.serveHTMLWeather(c, enhanced, units, locationInput)
		return
	}

	// Console clients get ASCII output
	h.serveASCIIWeather(c, enhanced, units, params, locationInput)
}

// isGPSCoordinates checks if a location string is GPS coordinates
func (h *WeatherHandler) isGPSCoordinates(location string) bool {
	// Check for pattern: number,number or number, number
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		return false
	}

	// Try to parse as floats
	_, err1 := fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", new(float64))
	_, err2 := fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", new(float64))

	return err1 == nil && err2 == nil
}

// serveHTMLWeather renders HTML weather page for browsers
func (h *WeatherHandler) serveHTMLWeather(c *gin.Context, location *service.Coordinates, units string, locationInput string) {
	// Get current weather and forecast
	current, err := h.weatherService.GetCurrentWeather(location.Latitude, location.Longitude, units)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "page/weather.tmpl", util.TemplateData(c, gin.H{
			"Error":    err.Error(),
			"HostInfo": util.GetHostInfo(c),
			"page":     "weather",
		}))
		return
	}

	forecast, err := h.weatherService.GetForecast(location.Latitude, location.Longitude, 16, units)
	if err != nil {
		// Non-fatal, continue without forecast
		forecast = &service.Forecast{Days: []service.ForecastDay{}}
	}

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

	// Format location with population
	locationData := gin.H{
		"Name":                location.Name,
		"ShortName":           location.ShortName,
		"FullName":            location.FullName,
		"Latitude":            location.Latitude,
		"Longitude":           location.Longitude,
		"Country":             location.Country,
		"CountryCode":         location.CountryCode,
		"Population":          location.Population,
		"PopulationFormatted": formatPopulation(location.Population),
		"Timezone":            location.Timezone,
	}

	// Enrich forecast days with icon and formatted date
	enrichedDays := make([]gin.H, len(forecast.Days))
	for i, day := range forecast.Days {
		// Parse date for formatting (format as "Mon 2 Jan")
		dateFormatted := day.Date
		if t, err := time.Parse("2006-01-02", day.Date); err == nil {
			dateFormatted = t.Format("Mon 2 Jan")
		}

		enrichedDays[i] = gin.H{
			"Date":          day.Date,
			"DateFormatted": dateFormatted,
			"WeatherCode":   day.WeatherCode,
			"Icon":          h.weatherService.GetWeatherIcon(day.WeatherCode, true),
			"Description":   h.weatherService.GetWeatherDescription(day.WeatherCode),
			"TempMax":       day.TempMax,
			"TempMin":       day.TempMin,
			// Use TempMin as morning temp
			"TempMorn":                 day.TempMin,
			"FeelsLikeMax":             day.FeelsLikeMax,
			"FeelsLikeMin":             day.FeelsLikeMin,
			"Precipitation":            day.Precipitation,
			"PrecipitationHours":       day.PrecipitationHours,
			"PrecipitationProbability": day.PrecipitationProbability,
			"WindSpeedMax":             day.WindSpeedMax,
			"WindGustsMax":             day.WindGustsMax,
			"WindDirection":            day.WindDirection,
			"SolarRadiation":           day.SolarRadiation,
		}
	}

	// Create enriched forecast
	enrichedForecast := gin.H{
		"Days":     enrichedDays,
		"Timezone": forecast.Timezone,
	}

	// Save location to cookies for persistence across navigation
	middleware.SaveLocationCookies(c, location.Latitude, location.Longitude, location.ShortName)

	// Format location for URLs (replace spaces with +, use ShortName for clean format)
	locationFormatted := strings.ReplaceAll(location.ShortName, " ", "+")

	// Always use full location (ShortName) for clarity
	// This shows "Albany, NY" instead of just "Albany"
	displayLocation := location.ShortName

	c.HTML(http.StatusOK, "page/weather.tmpl", util.TemplateData(c, gin.H{
		"Title": location.ShortName + " Weather",
		"WeatherData": gin.H{
			"Location": locationData,
			"Current":  currentData,
			"Forecast": enrichedForecast,
			"Units":    units,
		},
		"HostInfo":          util.GetHostInfo(c),
		"Location":          displayLocation,
		"LocationFormatted": locationFormatted,
		"Units":             units,
		"HideFooter":        false,
		"page":              "weather",
	}))
}

// serveASCIIWeather renders ASCII weather for console clients
func (h *WeatherHandler) serveASCIIWeather(c *gin.Context, location *service.Coordinates, units string, params *util.RenderParams, locationInput string) {
	// Check if we need forecast (formats 1-4 don't need forecast)
	needsForecast := params.Format == 0

	var current *service.CurrentWeather
	var forecast *service.Forecast
	var err error

	if needsForecast {
		// Fetch both current and forecast in parallel
		currentChan := make(chan *service.CurrentWeather)
		forecastChan := make(chan *service.Forecast)
		errChan := make(chan error, 2)

		go func() {
			curr, err := h.weatherService.GetCurrentWeather(location.Latitude, location.Longitude, units)
			if err != nil {
				errChan <- err
				return
			}
			currentChan <- curr
		}()

		go func() {
			fcst, err := h.weatherService.GetForecast(location.Latitude, location.Longitude, 16, units)
			if err != nil {
				errChan <- err
				return
			}
			forecastChan <- fcst
		}()

		// Wait for results
		select {
		case err = <-errChan:
			h.handleError(c, err, locationInput, false)
			return
		case current = <-currentChan:
			forecast = <-forecastChan
		}
	} else {
		// Only fetch current weather
		current, err = h.weatherService.GetCurrentWeather(location.Latitude, location.Longitude, units)
		if err != nil {
			h.handleError(c, err, locationInput, false)
			return
		}
	}

	// Convert to WeatherData
	weatherData := &util.WeatherData{
		Location: util.LocationData{
			Name:        location.Name,
			ShortName:   location.ShortName,
			FullName:    location.FullName,
			Latitude:    location.Latitude,
			Longitude:   location.Longitude,
			Country:     location.Country,
			CountryCode: location.CountryCode,
			State:       location.Admin1,
			Population:  location.Population,
			Timezone:    location.Timezone,
		},
		Current: util.CurrentData{
			Temperature:   current.Temperature,
			FeelsLike:     current.FeelsLike,
			Humidity:      current.Humidity,
			Pressure:      current.Pressure,
			WindSpeed:     current.WindSpeed,
			WindDirection: current.WindDirection,
			WeatherCode:   current.WeatherCode,
			Condition:     h.weatherService.GetWeatherDescription(current.WeatherCode),
			Icon:          h.weatherService.GetWeatherIcon(current.WeatherCode, current.IsDay == 1),
			Precipitation: current.Precipitation,
		},
	}

	// Convert forecast if available
	if forecast != nil {
		weatherData.Forecast = make([]util.ForecastData, len(forecast.Days))
		for i, day := range forecast.Days {
			weatherData.Forecast[i] = util.ForecastData{
				Date:          day.Date,
				TempMax:       day.TempMax,
				TempMin:       day.TempMin,
				Condition:     h.weatherService.GetWeatherDescription(day.WeatherCode),
				Icon:          h.weatherService.GetWeatherIcon(day.WeatherCode, true),
				WeatherCode:   day.WeatherCode,
				Precipitation: day.Precipitation,
				WindSpeed:     day.WindSpeedMax,
				WindDirection: day.WindDirection,
			}
		}
	}

	// Render based on format
	var output string

	switch params.Format {
	case 1:
		output = h.oneLineRenderer.RenderFormat1(weatherData.Current, units, params.NoColors)
	case 2:
		output = h.oneLineRenderer.RenderFormat2(weatherData.Current, units, params.NoColors)
	case 3:
		output = h.oneLineRenderer.RenderFormat3(weatherData.Location, weatherData.Current, units, params.NoColors)
	case 4:
		output = h.oneLineRenderer.RenderFormat4(weatherData.Location, weatherData.Current, units, params.NoColors)
	default:
		output = h.asciiRenderer.RenderFull(weatherData, *params)
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, output)
}

// handleSpecialEndpoints handles :help and :bash.function endpoints
func (h *WeatherHandler) handleSpecialEndpoints(c *gin.Context, path string) bool {
	if path == ":help" {
		h.handleHelp(c)
		return true
	}

	if path == ":bash.function" {
		h.handleBashFunction(c)
		return true
	}

	return false
}

// handleHelp renders the help page
func (h *WeatherHandler) handleHelp(c *gin.Context) {
	hostInfo := util.GetHostInfo(c)
	baseURL := hostInfo.FullHost

	helpText := fmt.Sprintf(`Weather

USAGE:
    curl -q -LSs %s/
    curl -q -LSs %s/London,GB

PARAMETERS:
    F                 Remove footer
    format=1          Icon + temperature: 🌦 +11°C
    format=2          Icon + temp + wind: 🌦 🌡️+11°C 🌬️↓4km/h
    format=3          Location + weather: London, GB: 🌦 +11°C
    format=4          Location + detailed: London, GB: 🌦 🌡️+11°C 🌬️↓4km/h
    u                 Imperial units (°F, mph)
    m                 Metric units (°C, km/h)

LOCATION FORMATS:
    /London,GB        City with country code
    /Albany,NY        City with state code
    /New+York,NY      Spaces as + symbols
    /33.0392,-80.1805 GPS coordinates (resolves to nearest city)
    /moon             Moon phase (current location)
    /moon/{location}  Moon phase for specific location

EXAMPLES:
    curl -q -LSs %s/London,GB?format=3
    curl -q -LSs %s/Albany,NY?u&format=4
    curl -q -LSs %s/33.0392,-80.1805
    curl -q -LSs %s/moon
    curl -q -LSs %s/moon/Tokyo,JP

SPECIAL ENDPOINTS:
    /:help            This help message
    /:bash.function   Bash integration function

JSON API:
    curl -q -LSs %s/api/v1/weather?location=paris
    curl -q -LSs %s/api/v1/search?q=alb

WEB INTERFACE:
    %s (browser interface with autocomplete)

More info: %s/api/v1/docs
`, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, helpText)
}

// handleBashFunction serves the bash integration function
func (h *WeatherHandler) handleBashFunction(c *gin.Context) {
	hostInfo := util.GetHostInfo(c)
	hostname := strings.TrimPrefix(strings.TrimPrefix(hostInfo.FullHost, "http://"), "https://")

	bashFunction := fmt.Sprintf(`wttr()
{
    # Check if location is passed as argument
    local request="%s/${1-}"

    # If we have a parameter, use it; otherwise, detect location automatically
    if [ "$#" -eq 0 ]; then
        # No location specified - let server detect from IP
        request="%s/"
    else
        # Location specified
        request="%s/${1}"
    fi

    # Use curl to fetch weather data with proper flags
    # -q: quiet mode (no .curlrc)
    # -L: follow redirects
    # -S: show errors
    # -s: silent mode (no progress)
    if [[ -t 1 ]] && [[ "${TERM-}" != "dumb" ]]; then
        # Interactive terminal with color support
        curl -q -LSs "$request" 2>/dev/null || echo "Weather service unavailable"
    else
        # Non-interactive or dumb terminal
        curl -q -LSs "$request?format=3" 2>/dev/null || echo "Weather service unavailable"
    fi
}

# Alternative short function name
w() { wttr "$@"; }

# Examples:
# wttr              # Weather for current location
# wttr London,GB    # Weather for London, UK
# wttr Albany,NY    # Weather for Albany, New York
# wttr "New York"   # Weather for New York (spaces need quotes)
# w Tokyo,JP        # Short version for Tokyo

# Install instructions:
# 1. Add this function to your ~/.bashrc or ~/.zshrc
# 2. Or download directly:
#    curl -o ~/.wttr '%s/:bash.function'
#    echo 'source ~/.wttr' >> ~/.bashrc
# 3. Reload your shell: source ~/.bashrc
`, hostname, hostname, hostname, hostInfo.FullHost)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, bashFunction)
}

// handleMoonRequest handles moon phase requests
func (h *WeatherHandler) handleMoonRequest(c *gin.Context, locationInput string) {
	hostInfo := util.GetHostInfo(c)
	isBrowser := util.IsBrowser(c)

	// Extract location from moon/location or just moon
	location := strings.TrimPrefix(strings.ToLower(locationInput), "moon")
	location = strings.TrimPrefix(location, "/")

	if isBrowser {
		// Get units from query parameter
		units := c.DefaultQuery("units", "imperial")

		// Use default coordinates (can be enhanced to parse location)
		// Default to New York
		lat, lon := 40.7128, -74.0060

		moonService := service.NewMoonService()
		moonData := moonService.Calculate(lat, lon, time.Now())

		// Serve moon HTML page
		c.HTML(http.StatusOK, "page/moon.tmpl", util.TemplateData(c, gin.H{
			"Title":    "Moon Phase",
			"Location": location,
			"Units":    units,
			"HostInfo": hostInfo,
			"MoonData": moonData,
		}))
		return
	}

	// ASCII moon report
	cyan := "\x1b[38;2;139;233;253m"
	yellow := "\x1b[38;2;241;250;140m"
	purple := "\x1b[38;2;189;147;249m"
	reset := "\x1b[0m"

	output := fmt.Sprintf(`%sMoon Phase Feature%s

🌙 Moon phase calculations are available via the web interface:
   %s/moon

For location-specific moon phases:
   %s/moon/london,gb
   %s/moon/tokyo,jp

%sComing soon to ASCII interface!%s

Visit %s%s/moon%s for the full moon phase interface.
`, yellow, reset, hostInfo.FullHost, hostInfo.FullHost, hostInfo.FullHost, purple, reset, cyan, hostInfo.FullHost, reset)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, output)
}

// isInvalidPath filters out invalid paths that should return 404
func (h *WeatherHandler) isInvalidPath(path string) bool {
	// Don't reject paths that look like GPS coordinates
	if h.isGPSCoordinates(path) {
		return false
	}

	invalidPatterns := []string{
		"wp-", "admin/", ".well-known/", "favicon.ico", "robots.txt",
		"sitemap", ".php", ".asp", ".jsp", ".cgi",
	}

	// File extensions (but not decimal points in coordinates)
	fileExtensions := []string{
		".js", ".css", ".png", ".jpg", ".gif", ".ico",
		".svg", ".html", ".xml", ".json",
	}

	lowerPath := strings.ToLower(path)

	// Check invalid patterns
	for _, pattern := range invalidPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	// Check file extensions at the end of the path
	for _, ext := range fileExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}

	return false
}

// handleError renders appropriate error messages
func (h *WeatherHandler) handleError(c *gin.Context, err error, location string, isBrowser bool) {
	hostInfo := util.GetHostInfo(c)
	errMsg := err.Error()

	if isBrowser {
		c.HTML(http.StatusInternalServerError, "page/weather.tmpl", util.TemplateData(c, gin.H{
			"error":    errMsg,
			"hostInfo": hostInfo,
			"page":     "weather",
		}))
		return
	}

	// Console error messages with helpful suggestions
	var errorMessage string
	statusCode := http.StatusInternalServerError

	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "Location") {
		statusCode = http.StatusNotFound
		errorMessage = fmt.Sprintf(`📍 Location not found: "%s"

Try these alternatives:
• Use city with country: London,GB or Albany,NY
• Try coordinates: 40.7128,-74.0060
• Search for locations: %s/api/v1/search?q=your-city
• Current location: %s/

📍 Need help? %s/:help
`, location, hostInfo.FullHost, hostInfo.FullHost, hostInfo.FullHost)
	} else if strings.Contains(errMsg, "timeout") {
		errorMessage = fmt.Sprintf(`⏰ Request timeout - services are busy. Please try again.

Quick alternatives:
• Try again: curl -q -LSs %s/%s
• Different location: curl -q -LSs %s/London,GB
• Service status: curl -q -LSs %s/healthz

⏰ Usually resolves within a few moments.
`, hostInfo.FullHost, location, hostInfo.FullHost, hostInfo.FullHost)
	} else {
		errorMessage = fmt.Sprintf(`☁️ Weather service error: %s

Try these options:
• A different location: curl -q -LSs %s/London,GB
• Our JSON API: curl -q -LSs %s/api/v1/weather?location=london
• Check service status: curl -q -LSs %s/healthz

☁️ Thank you for your patience!
`, errMsg, hostInfo.FullHost, hostInfo.FullHost, hostInfo.FullHost)
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(statusCode, errorMessage+"\n")
}

// formatPopulation formats population with thousands separators
func formatPopulation(pop int) string {
	if pop == 0 {
		return ""
	}

	// Convert to string
	str := strconv.Itoa(pop)

	// Add commas
	n := len(str)
	if n <= 3 {
		return str
	}

	// Build result with commas
	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(digit)
	}

	return result.String()
}
