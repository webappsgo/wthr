package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
	"github.com/casapps/wthr/src/util"

	"github.com/gin-gonic/gin"
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
// @Success 200 {array} models.Location "List of saved locations"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations [get]
func (h *LocationHandler) ListLocations(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}
	locations, err := locationModel.GetByUserID(int(user.ID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch locations"})
		return
	}

	c.JSON(http.StatusOK, locations)
}

// GetLocation returns a specific saved location
// @Summary Get a saved location
// @Description Get details of a specific saved location by ID
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path integer true "Location ID"
// @Success 200 {object} models.Location "Location details"
// @Failure 400 {object} map[string]interface{} "Invalid location ID"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 403 {object} map[string]interface{} "Access denied"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Router /api/v1/users/locations/{id} [get]
func (h *LocationHandler) GetLocation(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid location ID"})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}
	location, err := locationModel.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}

	// Verify ownership
	if int64(location.UserID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, location)
}

// CreateLocation creates a new saved location
// @Summary Create a saved location
// @Description Add a new location to the user's saved locations
// @Tags Locations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param location body object true "Location data" SchemaExample({"name": "New York", "latitude": 40.7128, "longitude": -74.0060, "timezone": "America/New_York"})
// @Success 201 {object} models.Location "Created location"
// @Failure 400 {object} map[string]interface{} "Invalid request data"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations [post]
func (h *LocationHandler) CreateLocation(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		Name      string  `json:"name" binding:"required"`
		Latitude  float64 `json:"latitude" binding:"required"`
		Longitude float64 `json:"longitude" binding:"required"`
		Timezone  string  `json:"timezone"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate coordinates
	if req.Latitude < -90 || req.Latitude > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Latitude must be between -90 and 90"})
		return
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Longitude must be between -180 and 180"})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}
	
	// Check location limit per IDEA.md: Save up to 10 locations per user
	count, err := locationModel.Count(int(user.ID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check location count"})
		return
	}
	if count >= 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum of 10 saved locations allowed per user"})
		return
	}
	
	location, err := locationModel.Create(int(user.ID), req.Name, req.Latitude, req.Longitude, req.Timezone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create location"})
		return
	}

	c.JSON(http.StatusCreated, location)
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
// @Success 200 {object} models.Location "Updated location"
// @Failure 400 {object} map[string]interface{} "Invalid request data"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 403 {object} map[string]interface{} "Access denied"
// @Failure 404 {object} map[string]interface{} "Location not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /api/v1/users/locations/{id} [put]
func (h *LocationHandler) UpdateLocation(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid location ID"})
		return
	}

	var req struct {
		Name          string  `json:"name" binding:"required"`
		Latitude      float64 `json:"latitude" binding:"required"`
		Longitude     float64 `json:"longitude" binding:"required"`
		Timezone      string  `json:"timezone"`
		AlertsEnabled bool    `json:"alerts_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}
	if int64(location.UserID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Update location
	if err := locationModel.Update(id, req.Name, req.Latitude, req.Longitude, req.Timezone, req.AlertsEnabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update location"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Location updated successfully"})
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
func (h *LocationHandler) DeleteLocation(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid location ID"})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}
	if int64(location.UserID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Delete location
	if err := locationModel.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete location"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Location deleted successfully"})
}

// ToggleAlerts toggles weather alerts for a location
func (h *LocationHandler) ToggleAlerts(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid location ID"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}

	// Verify ownership
	location, err := locationModel.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Location not found"})
		return
	}
	if int64(location.UserID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Toggle alerts
	if err := locationModel.ToggleAlerts(id, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alerts updated successfully", "enabled": req.Enabled})
}

// ShowAddLocationPage renders the add location page
func (h *LocationHandler) ShowAddLocationPage(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	c.HTML(http.StatusOK, "page/add_location.tmpl", utils.TemplateData(c, gin.H{
		"title": "Add Location - Weather",
		"user":  user,
		"page":  "locations",
	}))
}

// ShowEditLocationPage renders the edit location page
func (h *LocationHandler) ShowEditLocationPage(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "page/error.tmpl", gin.H{"error": "Invalid location ID"})
		return
	}

	locationModel := &models.LocationModel{DB: h.DB}
	location, err := locationModel.GetByID(id)
	if err != nil {
		c.HTML(http.StatusNotFound, "page/error.tmpl", gin.H{"error": "Location not found"})
		return
	}

	if int64(location.UserID) != user.ID {
		c.HTML(http.StatusForbidden, "page/error.tmpl", gin.H{"error": "Access denied"})
		return
	}

	c.HTML(http.StatusOK, "page/edit_location.tmpl", utils.TemplateData(c, gin.H{
		"title":    "Edit Location - Weather",
		"user":     user,
		"location": location,
		"page":     "locations",
	}))
}

// SearchLocations searches for cities by name
func (h *LocationHandler) SearchLocations(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if len(query) < 2 {
		InvalidInput(c, "Query must be at least 2 characters")
		return
	}

	// Search in cities database
	results := h.searchCities(query, 10)

	RespondNegotiatedData(c, http.StatusOK, results)
}

// LookupZipCode looks up a location by ZIP/postal code
func (h *LocationHandler) LookupZipCode(c *gin.Context) {
	zipCode := strings.TrimSpace(c.Param("code"))
	if zipCode == "" {
		InvalidInput(c, "ZIP code is required")
		return
	}

	// Use weather service to geocode the ZIP code
	coords, err := h.WeatherService.ParseAndResolveLocation(zipCode, "")
	if err != nil {
		NotFound(c, "ZIP code not found")
		return
	}

	// Enhance the location with full details
	enhanced := h.LocationEnhancer.EnhanceLocation(coords)

	// Return location data
	result := gin.H{
		"name":      enhanced.Name,
		"latitude":  enhanced.Latitude,
		"longitude": enhanced.Longitude,
		"timezone":  enhanced.Timezone,
		"country":   enhanced.Country,
		"admin1":    enhanced.Admin1,
	}

	RespondNegotiatedData(c, http.StatusOK, result)
}

// LookupCoordinates reverse geocodes coordinates to get location info
func (h *LocationHandler) LookupCoordinates(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		InvalidInput(c, "Invalid latitude")
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		InvalidInput(c, "Invalid longitude")
		return
	}

	// Validate coordinate ranges
	if lat < -90 || lat > 90 {
		InvalidInput(c, "Latitude must be between -90 and 90")
		return
	}
	if lon < -180 || lon > 180 {
		InvalidInput(c, "Longitude must be between -180 and 180")
		return
	}

	// Use weather service to reverse geocode
	coords, err := h.WeatherService.ReverseGeocode(lat, lon)
	if err != nil {
		NotFound(c, "Location not found")
		return
	}

	// Enhance the location with full details
	enhanced := h.LocationEnhancer.EnhanceLocation(coords)

	// Return location data
	result := gin.H{
		"name":      enhanced.Name,
		"latitude":  enhanced.Latitude,
		"longitude": enhanced.Longitude,
		"timezone":  enhanced.Timezone,
		"country":   enhanced.Country,
		"admin1":    enhanced.Admin1,
	}

	RespondNegotiatedData(c, http.StatusOK, result)
}

// searchCities searches the cities database for matching names
func (h *LocationHandler) searchCities(query string, limit int) []gin.H {
	if h.LocationEnhancer == nil {
		return []gin.H{}
	}

	queryLower := strings.ToLower(query)
	var results []gin.H

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
func (h *LocationHandler) formatCityResult(city service.City) gin.H {
	// Format the display name
	displayParts := []string{city.Name}
	if city.State != "" {
		displayParts = append(displayParts, city.State)
	}
	if city.Country != "" {
		displayParts = append(displayParts, city.Country)
	}

	return gin.H{
		"name":      city.Name,
		"latitude":  city.Coord.Lat,
		"longitude": city.Coord.Lon,
		"country":   city.Country,
		"admin1":    city.State,
		"timezone":  "",
		"display":   strings.Join(displayParts, ", "),
	}
}
