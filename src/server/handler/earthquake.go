package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// EarthquakeHandler handles earthquake-related routes
type EarthquakeHandler struct {
	earthquakeService *service.EarthquakeService
	weatherService    *service.WeatherService
	locationEnhancer  *service.LocationEnhancer
}

// NewEarthquakeHandler creates a new earthquake handler
func NewEarthquakeHandler(es *service.EarthquakeService, ws *service.WeatherService, le *service.LocationEnhancer) *EarthquakeHandler {
	return &EarthquakeHandler{
		earthquakeService: es,
		weatherService:    ws,
		locationEnhancer:  le,
	}
}

// ListEarthquakes returns earthquake data for non-HTTP callers such as GraphQL.
func (h *EarthquakeHandler) ListEarthquakes(feedType string, minMagnitude *float64, limit *int) ([]service.Earthquake, error) {
	if h == nil || h.earthquakeService == nil {
		return nil, fmt.Errorf("earthquake service not initialized")
	}

	if feedType == "" {
		feedType = "all_day"
	}

	collection, err := h.earthquakeService.GetEarthquakes(feedType)
	if err != nil {
		return nil, err
	}

	earthquakes := make([]service.Earthquake, 0, len(collection.Earthquakes))
	for _, eq := range collection.Earthquakes {
		if minMagnitude != nil && eq.Magnitude < *minMagnitude {
			continue
		}
		earthquakes = append(earthquakes, eq)
	}

	if limit != nil && *limit > 0 && len(earthquakes) > *limit {
		earthquakes = earthquakes[:*limit]
	}

	return earthquakes, nil
}

// HandleEarthquakes serves the earthquake interface
func (h *EarthquakeHandler) HandleEarthquakes(w http.ResponseWriter, r *http.Request) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(w, r)
		return
	}

	// Get query parameters
	feedType := r.URL.Query().Get("feed")
	if feedType == "" {
		feedType = "all_day"
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "newest"
	}
	numberStr := r.URL.Query().Get("number")
	if numberStr == "" {
		numberStr = "0"
	}
	number, _ := strconv.Atoi(numberStr)

	// Validate feed type
	validFeeds := map[string]bool{
		"all_hour": true, "all_day": true, "all_week": true, "all_month": true,
		"1.0_hour": true, "1.0_day": true, "1.0_week": true, "1.0_month": true,
		"2.5_hour": true, "2.5_day": true, "2.5_week": true, "2.5_month": true,
		"4.5_hour": true, "4.5_day": true, "4.5_week": true, "4.5_month": true,
		"significant_hour": true, "significant_day": true, "significant_week": true, "significant_month": true,
	}

	if !validFeeds[feedType] {
		feedType = "all_day"
	}

	// Get client location for map centering and distance calculation
	// Priority: 1. Saved cookies, 2. IP geolocation
	clientIP := util.TrustedGetClientIP(r)
	var centerLat, centerLon float64 = 0.0, 0.0
	hasUserLocation := false

	// Check cookies first
	if latCookie, err := r.Cookie("user_lat"); err == nil {
		if lonCookie, err := r.Cookie("user_lon"); err == nil {
			if lat, err1 := strconv.ParseFloat(latCookie.Value, 64); err1 == nil {
				if lon, err2 := strconv.ParseFloat(lonCookie.Value, 64); err2 == nil {
					centerLat = lat
					centerLon = lon
					hasUserLocation = true
				}
			}
		}
	}

	// If no cookies, use IP geolocation as fallback
	if !hasUserLocation {
		coords, err := h.weatherService.GetCoordinatesFromIP(clientIP)
		if err == nil {
			centerLat = coords.Latitude
			centerLon = coords.Longitude
			hasUserLocation = true
		}
	}

	// Fetch earthquake data
	earthquakes, err := h.earthquakeService.GetEarthquakes(feedType)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"error": "Failed to load earthquake data: " + err.Error(),
		}))
		return
	}

	// Calculate distance from user for each earthquake
	if hasUserLocation {
		for i := range earthquakes.Earthquakes {
			eq := &earthquakes.Earthquakes[i]
			eq.Distance = haversineDistanceCalc(centerLat, centerLon, eq.Latitude, eq.Longitude)
			eq.DistanceFmt = formatEarthquakeDistance(eq.Distance)
		}
	}

	// Sort and limit results
	earthquakes.SortAndLimit(sortBy, number)

	// Get host info for console commands
	hostInfo := util.GetHostInfo(r)

	// Render earthquake page
	middleware.RenderHTML(w, r, http.StatusOK, "page/earthquake.tmpl", util.TemplateData(r, map[string]interface{}{
		"Earthquakes":     earthquakes.Earthquakes,
		"Metadata":        earthquakes.Metadata,
		"FeedType":        feedType,
		"SortBy":          sortBy,
		"CenterLat":       centerLat,
		"CenterLon":       centerLon,
		"HasUserLocation": hasUserLocation,
		"HostInfo":        hostInfo,
	}))
}

// HandleEarthquakesByLocation serves earthquakes near a specific location
func (h *EarthquakeHandler) HandleEarthquakesByLocation(w http.ResponseWriter, r *http.Request) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(w, r)
		return
	}

	locationInput := chi.URLParam(r, "location")
	if locationInput == "" {
		http.Redirect(w, r, "/earthquake", http.StatusFound)
		return
	}

	// Get query parameters
	feedType := r.URL.Query().Get("feed")
	if feedType == "" {
		feedType = "all_week"
	}
	radiusStr := r.URL.Query().Get("radius")
	if radiusStr == "" {
		radiusStr = "500"
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "newest"
	}
	numberStr := r.URL.Query().Get("number")
	if numberStr == "" {
		numberStr = "0"
	}
	number, _ := strconv.Atoi(numberStr)

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil || radius <= 0 {
		// Default 500km
		radius = 500
	}

	// Parse location
	clientIP := util.TrustedGetClientIP(r)
	coords, err := h.weatherService.ParseAndResolveLocation(locationInput, clientIP)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"error": "Location not found: " + locationInput,
		}))
		return
	}

	// Enhance location
	enhanced := h.locationEnhancer.EnhanceLocation(coords)

	// Save location to cookies for persistence across navigation
	middleware.SaveLocationCookies(w, r, enhanced.Latitude, enhanced.Longitude, enhanced.ShortName)

	// Get earthquakes near location
	earthquakes, err := h.earthquakeService.GetEarthquakesByLocation(
		enhanced.Latitude,
		enhanced.Longitude,
		radius,
		feedType,
	)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"error": "Failed to load earthquake data: " + err.Error(),
		}))
		return
	}

	// Sort and limit results
	earthquakes.SortAndLimit(sortBy, number)

	// Get host info for console commands
	hostInfo := util.GetHostInfo(r)

	// Create LocationData for uniform display
	// Format population with commas
	popFormatted := ""
	if enhanced.Population > 0 {
		popFormatted = formatPopulation(enhanced.Population)
	}

	locationData := map[string]interface{}{
		"Location": map[string]interface{}{
			"Name":                enhanced.FullName,
			"ShortName":           enhanced.ShortName,
			"Country":             enhanced.Country,
			"Latitude":            enhanced.Latitude,
			"Longitude":           enhanced.Longitude,
			"Timezone":            enhanced.Timezone,
			"Population":          enhanced.Population,
			"PopulationFormatted": popFormatted,
		},
	}

	// Render earthquake page
	middleware.RenderHTML(w, r, http.StatusOK, "page/earthquake.tmpl", util.TemplateData(r, map[string]interface{}{
		"Earthquakes":     earthquakes.Earthquakes,
		"Metadata":        earthquakes.Metadata,
		"FeedType":        feedType,
		"SortBy":          sortBy,
		"Location":        enhanced.ShortName,
		"LocationData":    locationData,
		"Radius":          radius,
		"CenterLat":       enhanced.Latitude,
		"CenterLon":       enhanced.Longitude,
		"HasUserLocation": true,
		"HostInfo":        hostInfo,
	}))
}

// HandleEarthquakeAPI serves JSON earthquake data
// @Summary Get earthquake data
// @Description Get recent earthquake data from USGS
// @Tags earthquakes
// @Accept json
// @Produce json
// @Param feed query string false "Feed type (all_hour, all_day, all_week, all_month, 1.0_day, 2.5_day, 4.5_day, significant_day)" default(all_day)
// @Success 200 {object} map[string]interface{} "Earthquake collection with metadata"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/earthquakes [get]
func (h *EarthquakeHandler) HandleEarthquakeAPI(w http.ResponseWriter, r *http.Request) {
	feedType := r.URL.Query().Get("feed")
	if feedType == "" {
		feedType = "all_day"
	}

	earthquakes, err := h.earthquakeService.GetEarthquakes(feedType)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, err.Error())
		return
	}

	RespondNegotiatedData(w, r, http.StatusOK, earthquakes)
}

// HandleEarthquakeByIDAPI serves JSON data for a specific earthquake by ID
// @Summary Get earthquake by ID
// @Description Get detailed information for a specific earthquake by its USGS ID
// @Tags earthquakes
// @Accept json
// @Produce json
// @Param id path string true "Earthquake ID from USGS"
// @Success 200 {object} map[string]interface{} "Earthquake details"
// @Failure 400 {object} map[string]interface{} "Bad request - ID required"
// @Failure 404 {object} map[string]interface{} "Earthquake not found"
// @Router /api/v1/earthquakes/{id} [get]
func (h *EarthquakeHandler) HandleEarthquakeByIDAPI(w http.ResponseWriter, r *http.Request) {
	earthquakeID := chi.URLParam(r, "id")
	if earthquakeID == "" {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, "Earthquake ID required")
		return
	}

	// Search through available feeds to find the earthquake
	feeds := []string{"all_day", "all_week", "all_month"}
	var earthquake *service.Earthquake

	for _, feedType := range feeds {
		earthquakes, err := h.earthquakeService.GetEarthquakes(feedType)
		if err != nil {
			continue
		}

		for i := range earthquakes.Earthquakes {
			if earthquakes.Earthquakes[i].ID == earthquakeID {
				earthquake = &earthquakes.Earthquakes[i]
				break
			}
		}

		if earthquake != nil {
			break
		}
	}

	if earthquake == nil {
		NotFound(w, r, "Earthquake not found")
		return
	}

	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"earthquake": earthquake,
	})
}

// HandleEarthquakeDetail serves detailed information for a specific earthquake
func (h *EarthquakeHandler) HandleEarthquakeDetail(w http.ResponseWriter, r *http.Request) {
	earthquakeID := chi.URLParam(r, "id")
	if earthquakeID == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Earthquake ID required\n")
		return
	}

	// Fetch all earthquakes and find the one with matching ID
	feedType := r.URL.Query().Get("feed")
	if feedType == "" {
		feedType = "all_week"
	}
	earthquakes, err := h.earthquakeService.GetEarthquakes(feedType)
	if err != nil {
		if util.IsBrowser(r) {
			middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
				"error": "Failed to load earthquake data: " + err.Error(),
			}))
		} else {
			writeText(w, http.StatusInternalServerError, "Error fetching earthquake data: %v\n", err)
		}
		return
	}

	// Find earthquake by ID
	var earthquake *service.Earthquake
	for i := range earthquakes.Earthquakes {
		if earthquakes.Earthquakes[i].ID == earthquakeID {
			earthquake = &earthquakes.Earthquakes[i]
			break
		}
	}

	if earthquake == nil {
		if util.IsBrowser(r) {
			middleware.RenderHTML(w, r, http.StatusNotFound, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
				"error": "Earthquake not found",
			}))
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Earthquake not found\n")
		}
		return
	}

	// Check format parameter
	format := r.URL.Query().Get("format")
	isBrowser := util.IsBrowser(r)

	if !isBrowser || format != "" {
		// Console output
		output := h.renderASCIIEarthquakeDetail(earthquake)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, output)
	} else {
		// Browser output
		hostInfo := util.GetHostInfo(r)
		title := fmt.Sprintf("Earthquake Detail · %s", earthquake.Place)
		middleware.RenderHTML(w, r, http.StatusOK, "page/earthquake_detail.tmpl", util.TemplateData(r, map[string]interface{}{
			"Earthquake": earthquake,
			"HostInfo":   hostInfo,
			"title":      title,
			"page":       "earthquake",
		}))
	}
}

// HandleEarthquakeRequest routes earthquake requests (console vs browser)
func (h *EarthquakeHandler) HandleEarthquakeRequest(w http.ResponseWriter, r *http.Request) {
	// Check if services are initialized
	if !IsInitialized() {
		ServeLoadingPage(w, r)
		return
	}

	// Extract location from path
	location := chi.URLParam(r, "location")
	if location != "" {
		location = strings.TrimPrefix(location, "/")
	}

	// Check if this is a detail request
	if strings.HasPrefix(location, "detail/") {
		earthquakeID := strings.TrimPrefix(location, "detail/")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", earthquakeID)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		h.HandleEarthquakeDetail(w, r)
		return
	}

	isBrowser := util.IsBrowser(r)

	if isBrowser {
		// Browser users get HTML interface
		if location == "" {
			h.HandleEarthquakes(w, r)
		} else {
			// Set the param for HandleEarthquakesByLocation
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("location", location)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
			h.HandleEarthquakesByLocation(w, r)
		}
	} else {
		// Console users get ASCII text
		h.serveASCIIEarthquakes(w, r, location)
	}
}

// serveASCIIEarthquakes serves earthquake data as ASCII text for console
func (h *EarthquakeHandler) serveASCIIEarthquakes(w http.ResponseWriter, r *http.Request, locationPath string) {
	feedType := r.URL.Query().Get("feed")
	if feedType == "" {
		feedType = "all_day"
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "newest"
	}
	numberStr := r.URL.Query().Get("number")
	if numberStr == "" {
		numberStr = "0"
	}
	number, _ := strconv.Atoi(numberStr)

	var earthquakes *service.EarthquakeCollection
	var err error
	var locationName string

	if locationPath != "" {
		// Get earthquakes near location
		radius := 500.0
		if rad := r.URL.Query().Get("radius"); rad != "" {
			if parsed, err := strconv.ParseFloat(rad, 64); err == nil {
				radius = parsed
			}
		}

		clientIP := util.TrustedGetClientIP(r)
		coords, err := h.weatherService.ParseAndResolveLocation(locationPath, clientIP)
		if err != nil {
			writeText(w, http.StatusBadRequest, "Location not found: %s\n", locationPath)
			return
		}

		enhanced := h.locationEnhancer.EnhanceLocation(coords)
		locationName = enhanced.ShortName

		earthquakes, err = h.earthquakeService.GetEarthquakesByLocation(
			enhanced.Latitude,
			enhanced.Longitude,
			radius,
			feedType,
		)
		if err != nil {
			writeText(w, http.StatusInternalServerError, "Error fetching earthquake data: %v\n", err)
			return
		}
	} else {
		earthquakes, err = h.earthquakeService.GetEarthquakes(feedType)
	}

	if err != nil {
		writeText(w, http.StatusInternalServerError, "Error fetching earthquake data: %v\n", err)
		return
	}

	// Sort and limit results
	earthquakes.SortAndLimit(sortBy, number)

	// Render ASCII output
	output := h.renderASCIIEarthquakes(earthquakes, locationName, feedType)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, output)
}

// renderASCIIEarthquakes formats earthquake data as ASCII text
func (h *EarthquakeHandler) renderASCIIEarthquakes(earthquakes *service.EarthquakeCollection, location, feedType string) string {
	var sb strings.Builder

	// ANSI color codes (Dracula theme)
	cyan := "\x1b[38;2;139;233;253m"
	green := "\x1b[38;2;80;250;123m"
	yellow := "\x1b[38;2;241;250;140m"
	orange := "\x1b[38;2;255;184;108m"
	pink := "\x1b[38;2;255;121;198m"
	purple := "\x1b[38;2;189;147;249m"
	red := "\x1b[38;2;255;85;85m"
	comment := "\x1b[38;2;98;114;164m"
	bold := "\x1b[1m"
	reset := "\x1b[0m"

	// Header
	if location != "" {
		sb.WriteString(fmt.Sprintf("%s%s🌍 Earthquakes near %s%s\n\n", bold, yellow, location, reset))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s🌍 %s%s\n\n", bold, yellow, earthquakes.Metadata.Title, reset))
	}

	sb.WriteString(fmt.Sprintf("%sTotal: %s%d%s earthquakes\n", comment, cyan, earthquakes.Metadata.Count, reset))
	sb.WriteString(fmt.Sprintf("%sUpdated: %s%s\n\n", comment, time.UnixMilli(earthquakes.Metadata.Generated).Format(time.RFC1123), reset))

	if len(earthquakes.Earthquakes) == 0 {
		sb.WriteString(fmt.Sprintf("%sNo earthquakes found.%s\n", comment, reset))
		return sb.String()
	}

	// Table header with box drawing
	sb.WriteString(fmt.Sprintf("%s┌─────────┬────────────────────────────────────────────────────┬──────────────────────┬─────────────┐%s\n",
		purple, reset))
	sb.WriteString(fmt.Sprintf("%s│%s %s%-7s%s %s│%s %s%-50s%s %s│%s %s%-20s%s %s│%s %s%-11s%s %s│%s\n",
		purple, reset, orange, "MAG", reset,
		purple, reset, orange, "LOCATION", reset,
		purple, reset, orange, "TIME", reset,
		purple, reset, orange, "DEPTH", reset,
		purple, reset))
	sb.WriteString(fmt.Sprintf("%s├─────────┼────────────────────────────────────────────────────┼──────────────────────┼─────────────┤%s\n",
		purple, reset))

	// Earthquake list
	for i, eq := range earthquakes.Earthquakes {
		tsunamiWarning := ""
		if eq.Tsunami == 1 {
			tsunamiWarning = " 🌊"
		}

		// Color code by magnitude
		var magColor string
		if eq.Magnitude < 2.0 {
			magColor = green
		} else if eq.Magnitude < 4.0 {
			magColor = yellow
		} else if eq.Magnitude < 5.0 {
			magColor = orange
		} else if eq.Magnitude < 6.0 {
			magColor = pink
		} else {
			magColor = red
		}

		sb.WriteString(fmt.Sprintf("%s│%s %s%-7.1f%s %s│%s %-50s %s│%s %s%-20s%s %s│%s %s%-7.1fkm%s%s %s│%s\n",
			purple, reset,
			magColor, eq.Magnitude, reset,
			purple, reset, truncateString(eq.Place, 50),
			purple, reset,
			cyan, eq.Time.Format("2006-01-02 15:04:05"), reset,
			purple, reset,
			comment, eq.Depth, reset, tsunamiWarning,
			purple, reset))

		// Don't add separator after last item
		if i < len(earthquakes.Earthquakes)-1 {
			sb.WriteString(fmt.Sprintf("%s├─────────┼────────────────────────────────────────────────────┼──────────────────────┼─────────────┤%s\n",
				purple, reset))
		}
	}

	sb.WriteString(fmt.Sprintf("%s└─────────┴────────────────────────────────────────────────────┴──────────────────────┴─────────────┘%s\n",
		purple, reset))

	sb.WriteString(fmt.Sprintf("\n%s🌊 = Tsunami warning%s\n", cyan, reset))
	sb.WriteString(fmt.Sprintf("\n%sView details: https://earthquake.usgs.gov/earthquakes/map/%s\n", comment, reset))

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// renderASCIIEarthquakeDetail renders detailed information for a single earthquake
func (h *EarthquakeHandler) renderASCIIEarthquakeDetail(eq *service.Earthquake) string {
	var sb strings.Builder

	// ANSI color codes (Dracula theme)
	cyan := "\x1b[38;2;139;233;253m"
	green := "\x1b[38;2;80;250;123m"
	yellow := "\x1b[38;2;241;250;140m"
	orange := "\x1b[38;2;255;184;108m"
	pink := "\x1b[38;2;255;121;198m"
	purple := "\x1b[38;2;189;147;249m"
	red := "\x1b[38;2;255;85;85m"
	comment := "\x1b[38;2;98;114;164m"
	bold := "\x1b[1m"
	reset := "\x1b[0m"

	// Magnitude color
	var magColor string
	if eq.Magnitude < 2.0 {
		magColor = green
	} else if eq.Magnitude < 4.0 {
		magColor = yellow
	} else if eq.Magnitude < 5.0 {
		magColor = orange
	} else if eq.Magnitude < 6.0 {
		magColor = pink
	} else {
		magColor = red
	}

	// Header
	sb.WriteString(fmt.Sprintf("%s%s🌍 Earthquake Details%s\n\n", bold, yellow, reset))

	// Main info box
	sb.WriteString(fmt.Sprintf("%s┌─────────────────────────────────────────────────────────────────────┐%s\n", purple, reset))
	sb.WriteString(fmt.Sprintf("%s│%s %s%-67s%s %s│%s\n",
		purple, reset, bold, eq.Place, reset, purple, reset))
	sb.WriteString(fmt.Sprintf("%s├─────────────────────────────────────────────────────────────────────┤%s\n", purple, reset))

	// Magnitude and tsunami
	tsunamiIndicator := ""
	if eq.Tsunami == 1 {
		tsunamiIndicator = fmt.Sprintf("  %s🌊 TSUNAMI WARNING%s", cyan, reset)
	}
	sb.WriteString(fmt.Sprintf("%s│%s %sMagnitude:%s %s%.1f%s %s(%s)%s%s %s│%s\n",
		purple, reset,
		orange, reset,
		magColor, eq.Magnitude, reset,
		comment, eq.MagnitudeType, reset,
		tsunamiIndicator,
		purple, reset))

	// Time
	sb.WriteString(fmt.Sprintf("%s│%s %sTime:%s     %s%s%s %s│%s\n",
		purple, reset,
		orange, reset,
		cyan, eq.Time.Format("2006-01-02 15:04:05 MST"), reset,
		purple, reset))

	// Location
	sb.WriteString(fmt.Sprintf("%s│%s %sLocation:%s  %sLat %.4f°, Lon %.4f°%s %s│%s\n",
		purple, reset,
		orange, reset,
		cyan, eq.Latitude, eq.Longitude, reset,
		purple, reset))

	// Depth
	sb.WriteString(fmt.Sprintf("%s│%s %sDepth:%s     %s%.1f km%s %s│%s\n",
		purple, reset,
		orange, reset,
		cyan, eq.Depth, reset,
		purple, reset))

	// Status
	sb.WriteString(fmt.Sprintf("%s│%s %sStatus:%s    %s%s%s %s│%s\n",
		purple, reset,
		orange, reset,
		green, eq.Status, reset,
		purple, reset))

	// Optional fields
	if eq.Felt != nil && *eq.Felt > 0 {
		sb.WriteString(fmt.Sprintf("%s│%s %sFelt:%s      %s%d reports%s %s│%s\n",
			purple, reset,
			orange, reset,
			pink, *eq.Felt, reset,
			purple, reset))
	}

	if eq.CDI != nil {
		sb.WriteString(fmt.Sprintf("%s│%s %sCDI:%s       %s%.1f%s %s│%s\n",
			purple, reset,
			orange, reset,
			pink, *eq.CDI, reset,
			purple, reset))
	}

	if eq.MMI != nil {
		sb.WriteString(fmt.Sprintf("%s│%s %sMMI:%s       %s%.1f%s %s│%s\n",
			purple, reset,
			orange, reset,
			pink, *eq.MMI, reset,
			purple, reset))
	}

	// Network and ID
	sb.WriteString(fmt.Sprintf("%s│%s %sNetwork:%s   %s%s%s %s│%s\n",
		purple, reset,
		orange, reset,
		comment, eq.Network, reset,
		purple, reset))

	sb.WriteString(fmt.Sprintf("%s│%s %sID:%s        %s%s%s %s│%s\n",
		purple, reset,
		orange, reset,
		comment, eq.ID, reset,
		purple, reset))

	sb.WriteString(fmt.Sprintf("%s└─────────────────────────────────────────────────────────────────────┘%s\n", purple, reset))

	// External link
	sb.WriteString(fmt.Sprintf("\n%sUSGS Details: %s%s%s\n", comment, cyan, eq.URL, reset))

	return sb.String()
}

// haversineDistanceCalc calculates distance between two points in km
func haversineDistanceCalc(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	deltaLat := (lat2 - lat1) * math.Pi / 180.0
	deltaLon := (lon2 - lon1) * math.Pi / 180.0

	// Haversine formula
	sinDeltaLat := math.Sin(deltaLat / 2)
	sinDeltaLon := math.Sin(deltaLon / 2)
	a := sinDeltaLat*sinDeltaLat + math.Cos(lat1Rad)*math.Cos(lat2Rad)*sinDeltaLon*sinDeltaLon
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// formatEarthquakeDistance formats distance in a human-readable way
func formatEarthquakeDistance(km float64) string {
	if km < 1 {
		return fmt.Sprintf("%.0f m", km*1000)
	} else if km < 10 {
		return fmt.Sprintf("%.1f km", km)
	} else if km < 100 {
		return fmt.Sprintf("%.0f km", km)
	}
	return fmt.Sprintf("%.0f km", km)
}
