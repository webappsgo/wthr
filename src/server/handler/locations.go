package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

type LocationHandler struct {
	DB               *sql.DB
	WeatherService   *service.WeatherService
	LocationEnhancer *service.LocationEnhancer
}

// ListLocations returns all saved locations for the current user
// @Summary List user's saved locations
// @Description Get all saved locations for the authenticated user
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.Location "List of saved locations"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations [get]
func (h *LocationHandler) ListLocations(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}
	locations, err := locationModel.GetByUserID(int(user.ID))
	if err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_fetch_locations"))
		return
	}

	writeJSON(w, http.StatusOK, locations)
}

// GetLocation returns a specific saved location
// @Summary Get a saved location
// @Description Get details of a specific saved location by ID
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Location ID"
// @Success 200 {object} model.Location "Location details"
// @Failure 400 {object} map[string]interface{} "Invalid location ID"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 403 {object} map[string]interface{} "Access denied"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Router /api/v1/users/locations/{id} [get]
func (h *LocationHandler) GetLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.locations.invalid_location_id"))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}
	location, err := locationModel.GetByID(id)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.location_not_found"))
		return
	}

	// Verify ownership
	if int64(location.UserID) != user.ID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	writeJSON(w, http.StatusOK, location)
}

// CreateLocation creates a new saved location
// @Summary Create a saved location
// @Description Add a new location to the user's saved locations
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param location body object true "Location data" SchemaExample({"name": "New York", "latitude": 40.7128, "longitude": -74.0060, "timezone": "America/New_York"})
// @Success 201 {object} model.Location "Created location"
// @Failure 400 {object} map[string]interface{} "Invalid request data"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations [post]
func (h *LocationHandler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
		// Latitude/Longitude intentionally have no "required" binding: 0.0 is
		// a legitimate coordinate (e.g. Null Island / equator / prime
		// meridian) and gin's "required" rejects the zero value for
		// non-pointer numeric fields. Range validation below is the real
		// correctness check.
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	// Validate coordinates
	if req.Latitude < -90 || req.Latitude > 90 {
		BadRequest(w, r, Translate(r, "errors.locations.latitude_must_be_between_90_and_90"))
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		BadRequest(w, r, Translate(r, "errors.locations.longitude_must_be_between_180_and_180"))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}

	// Check location limit per IDEA.md: Save up to 10 locations per user
	count, err := locationModel.Count(int(user.ID))
	if err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_check_location_count"))
		return
	}
	if count >= 10 {
		BadRequest(w, r, Translate(r, "errors.locations.maximum_of_10_saved_locations_allowed_per_user"))
		return
	}

	location, err := locationModel.Create(int(user.ID), req.Name, req.Latitude, req.Longitude, req.Timezone)
	if err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_create_location"))
		return
	}

	writeJSON(w, http.StatusCreated, location)
}

// UpdateLocation updates a saved location
// @Summary Update a saved location
// @Description Update details of an existing saved location
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Location ID"
// @Param location body object true "Updated location data" SchemaExample({"name": "Updated Name", "latitude": 40.7128, "longitude": -74.0060})
// @Success 200 {object} model.Location "Updated location"
// @Failure 400 {object} map[string]interface{} "Invalid request data"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 403 {object} map[string]interface{} "Access denied"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations/{id} [put]
func (h *LocationHandler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.locations.invalid_location_id"))
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
		// See CreateLocation: 0.0 is a legitimate coordinate, so no
		// "required" binding on the float fields.
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Timezone      string  `json:"timezone"`
		AlertsEnabled bool    `json:"alerts_enabled"`
	}

	if !DecodeAndValidate(w, r, &req) {
		return
	}

	if req.Latitude < -90 || req.Latitude > 90 {
		BadRequest(w, r, Translate(r, "errors.locations.latitude_must_be_between_90_and_90"))
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		BadRequest(w, r, Translate(r, "errors.locations.longitude_must_be_between_180_and_180"))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.location_not_found"))
		return
	}
	if int64(location.UserID) != user.ID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	// Update location
	if err := locationModel.Update(id, req.Name, req.Latitude, req.Longitude, req.Timezone, req.AlertsEnabled); err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_update_location"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.locations.location_updated_successfully"),
	})
}

// DeleteLocation deletes a saved location
// @Summary Delete a saved location
// @Description Remove a location from the user's saved locations
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Location ID"
// @Success 204 "Location deleted successfully"
// @Failure 400 {object} map[string]interface{} "Invalid location ID"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 403 {object} map[string]interface{} "Access denied"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations/{id} [delete]
func (h *LocationHandler) DeleteLocation(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.locations.invalid_location_id"))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.location_not_found"))
		return
	}
	if int64(location.UserID) != user.ID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	// Delete location
	if err := locationModel.Delete(id); err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_delete_location"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": Translate(r, "success.locations.location_deleted_successfully"),
	})
}

// ToggleAlerts toggles weather alerts for a location
func (h *LocationHandler) ToggleAlerts(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		Unauthorized(w, r, Translate(r, "errors.admin.admins.not_authenticated"))
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		BadRequest(w, r, Translate(r, "errors.locations.invalid_location_id"))
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.location_not_found"))
		return
	}
	if int64(location.UserID) != user.ID {
		Forbidden(w, r, Translate(r, "errors.locations.access_denied"))
		return
	}

	// Toggle alerts
	if err := locationModel.ToggleAlerts(id, req.Enabled); err != nil {
		InternalError(w, r, Translate(r, "errors.locations.failed_to_toggle_alerts"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"message": Translate(r, "success.locations.alerts_updated_successfully"), "enabled": req.Enabled})
}

// ShowAddLocationPage renders the add location page
func (h *LocationHandler) ShowAddLocationPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/add_location.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "locations.add_location_page_title"),
		"user":  user,
		"page":  "locations",
	}))
}

// ShowEditLocationPage renders the edit location page
func (h *LocationHandler) ShowEditLocationPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{"error": Translate(r, "errors.locations.invalid_location_id")}))
		return
	}

	locationModel := &model.LocationModel{DB: h.DB}
	location, err := locationModel.GetByID(id)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusNotFound, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{"error": Translate(r, "errors.locations.location_not_found")}))
		return
	}

	if int64(location.UserID) != user.ID {
		middleware.RenderHTML(w, r, http.StatusForbidden, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{"error": Translate(r, "errors.locations.access_denied")}))
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "page/edit_location.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":    Translate(r, "locations.edit_location_page_title"),
		"user":     user,
		"location": location,
		"page":     "locations",
	}))
}

// SearchLocations searches for cities by name
func (h *LocationHandler) SearchLocations(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		InvalidInput(w, r, Translate(r, "errors.locations.query_must_be_at_least_2_characters"))
		return
	}

	// Search in cities database
	results := h.searchCities(query, 10)

	RespondNegotiatedData(w, r, http.StatusOK, results)
}

// LookupZipCode looks up a location by ZIP/postal code
func (h *LocationHandler) LookupZipCode(w http.ResponseWriter, r *http.Request) {
	zipCode := strings.TrimSpace(chi.URLParam(r, "code"))
	if zipCode == "" {
		InvalidInput(w, r, Translate(r, "errors.locations.zip_code_is_required"))
		return
	}

	// Use weather service to geocode the ZIP code
	coords, err := h.WeatherService.ParseAndResolveLocation(zipCode, "")
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.zip_code_not_found"))
		return
	}

	// Enhance the location with full details
	enhanced := h.LocationEnhancer.EnhanceLocation(coords)

	// Return location data
	result := map[string]interface{}{
		"name":      enhanced.Name,
		"latitude":  enhanced.Latitude,
		"longitude": enhanced.Longitude,
		"timezone":  enhanced.Timezone,
		"country":   enhanced.Country,
		"admin1":    enhanced.Admin1,
	}

	RespondNegotiatedData(w, r, http.StatusOK, result)
}

// LookupCoordinates reverse geocodes coordinates to get location info
func (h *LocationHandler) LookupCoordinates(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		InvalidInput(w, r, Translate(r, "errors.locations.invalid_latitude"))
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		InvalidInput(w, r, Translate(r, "errors.locations.invalid_longitude"))
		return
	}

	// Validate coordinate ranges
	if lat < -90 || lat > 90 {
		InvalidInput(w, r, Translate(r, "errors.locations.latitude_must_be_between_90_and_90"))
		return
	}
	if lon < -180 || lon > 180 {
		InvalidInput(w, r, Translate(r, "errors.locations.longitude_must_be_between_180_and_180"))
		return
	}

	// Use weather service to reverse geocode
	coords, err := h.WeatherService.ReverseGeocode(lat, lon)
	if err != nil {
		NotFound(w, r, Translate(r, "errors.locations.location_not_found"))
		return
	}

	// Enhance the location with full details
	enhanced := h.LocationEnhancer.EnhanceLocation(coords)

	// Return location data
	result := map[string]interface{}{
		"name":      enhanced.Name,
		"latitude":  enhanced.Latitude,
		"longitude": enhanced.Longitude,
		"timezone":  enhanced.Timezone,
		"country":   enhanced.Country,
		"admin1":    enhanced.Admin1,
	}

	RespondNegotiatedData(w, r, http.StatusOK, result)
}

// searchCities searches the cities database for matching names
func (h *LocationHandler) searchCities(query string, limit int) []map[string]interface{} {
	if h.LocationEnhancer == nil {
		return []map[string]interface{}{}
	}

	queryLower := strings.ToLower(query)
	var results []map[string]interface{}

	// Search through cities data
	for _, city := range h.LocationEnhancer.GetCitiesData() {
		// Check if city name starts with query (higher priority)
		if strings.HasPrefix(strings.ToLower(city.Name), queryLower) {
			results = append(results, h.formatCityResult(city))
			if len(results) >= limit {
				break
			}
			continue
		}

		// Check if city name contains query
		if strings.Contains(strings.ToLower(city.Name), queryLower) {
			results = append(results, h.formatCityResult(city))
			if len(results) >= limit {
				break
			}
		}
	}

	return results
}

// formatCityResult formats a city for the search results
func (h *LocationHandler) formatCityResult(city service.City) map[string]interface{} {
	// Format the display name
	displayParts := []string{city.Name}
	if city.State != "" {
		displayParts = append(displayParts, city.State)
	}
	if city.Country != "" {
		displayParts = append(displayParts, city.Country)
	}

	return map[string]interface{}{
		"name":      city.Name,
		"latitude":  city.Coord.Lat,
		"longitude": city.Coord.Lon,
		"country":   city.Country,
		"admin1":    city.State,
		"timezone":  "",
		"display":   strings.Join(displayParts, ", "),
	}
}
