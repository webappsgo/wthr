package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
)

// HistoryHandler handles the historical weather page
type HistoryHandler struct {
	weatherService *service.WeatherService
	settingsModel  *models.SettingsModel
	userModel      *models.UserModel
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(weatherService *service.WeatherService, settingsModel *models.SettingsModel, userModel *models.UserModel) *HistoryHandler {
	return &HistoryHandler{
		weatherService: weatherService,
		settingsModel:  settingsModel,
		userModel:      userModel,
	}
}

// HistoryPageData contains data for the history template
type HistoryPageData struct {
	Title            string
	User             *models.User
	Location         string
	Date             string
	StartYear        int
	Historical       *service.HistoricalWeather
	DefaultYears     int
	MinYears         int
	MaxYears         int
	Error            string
	Success          string
}

// ShowHistory displays the historical weather page
func (h *HistoryHandler) ShowHistory(w http.ResponseWriter, r *http.Request) {
	// Check if feature is enabled
	if !h.settingsModel.GetBool("history.enabled", true) {
		http.Error(w, "Historical weather feature is disabled", http.StatusServiceUnavailable)
		return
	}

	// Get user from session
	user, err := getUserFromSession(r, h.userModel)
	if err != nil {
		log.Printf("Error getting user from session: %v", err)
	}

	// Get settings
	defaultYears := h.settingsModel.GetInt("history.default_years", 10)
	minYears := h.settingsModel.GetInt("history.min_years", 5)
	maxYears := h.settingsModel.GetInt("history.max_years", 50)

	data := HistoryPageData{
		Title:        "Historical Weather",
		User:         user,
		DefaultYears: defaultYears,
		MinYears:     minYears,
		MaxYears:     maxYears,
	}

	// Check if we have query parameters (location, date, etc.)
	location := r.URL.Query().Get("location")
	dateStr := r.URL.Query().Get("date")
	yearsStr := r.URL.Query().Get("years")

	// If no parameters provided, show the form
	if location == "" && dateStr == "" {
		renderTemplate(w, "history", data)
		return
	}

	// Parse date with smart year detection
	month, day, startYear, err := parseHistoricalDate(dateStr)
	if err != nil {
		data.Error = fmt.Sprintf("Invalid date format: %v", err)
		renderTemplate(w, "history", data)
		return
	}

	// Parse number of years
	numberOfYears := defaultYears
	if yearsStr != "" {
		years, err := strconv.Atoi(yearsStr)
		if err == nil && years >= minYears && years <= maxYears {
			numberOfYears = years
		}
	}

	// Get coordinates for location
	coords, err := h.weatherService.GetCoordinates(location, "")
	if err != nil {
		data.Error = fmt.Sprintf("Could not find location: %v", err)
		renderTemplate(w, "history", data)
		return
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
		data.Error = fmt.Sprintf("Error fetching historical weather: %v", err)
		renderTemplate(w, "history", data)
		return
	}

	// Populate data
	data.Location = location
	data.Date = dateStr
	data.StartYear = startYear
	data.Historical = historical

	renderTemplate(w, "history", data)
}

// parseHistoricalDate parses a date string with smart year detection
// Supported formats:
// - "12/10" -> month=12, day=10, year=current year
// - "12/10/2022" -> month=12, day=10, year=2022
// - "2022-12-10" -> month=12, day=10, year=2022
// - "Dec 10" -> month=12, day=10, year=current year
// - "Dec 10, 2022" -> month=12, day=10, year=2022
func parseHistoricalDate(dateStr string) (month, day, year int, err error) {
	if dateStr == "" {
		// Default to today
		now := time.Now()
		return int(now.Month()), now.Day(), now.Year(), nil
	}

	// Try parsing various formats
	var parsedDate time.Time

	// Try ISO format with year: 2022-12-10
	parsedDate, err = time.Parse("2006-01-02", dateStr)
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

	// Try "Jan 2" format
	parsedDate, err = time.Parse("Jan 2", dateStr)
	if err == nil {
		currentYear := time.Now().Year()
		return int(parsedDate.Month()), parsedDate.Day(), currentYear, nil
	}

	// Try "Jan 2, 2006" format
	parsedDate, err = time.Parse("Jan 2, 2006", dateStr)
	if err == nil {
		return int(parsedDate.Month()), parsedDate.Day(), parsedDate.Year(), nil
	}

	// Try "January 2" format
	parsedDate, err = time.Parse("January 2", dateStr)
	if err == nil {
		currentYear := time.Now().Year()
		return int(parsedDate.Month()), parsedDate.Day(), currentYear, nil
	}

	// Try "January 2, 2006" format
	parsedDate, err = time.Parse("January 2, 2006", dateStr)
	if err == nil {
		return int(parsedDate.Month()), parsedDate.Day(), parsedDate.Year(), nil
	}

	return 0, 0, 0, fmt.Errorf("unsupported date format: %s (try MM/DD or MM/DD/YYYY)", dateStr)
}

// getUserFromSession retrieves the current user from the session
func getUserFromSession(r *http.Request, userModel *models.UserModel) (*models.User, error) {
	// Get session cookie
	_, err := r.Cookie("session_token")
	if err != nil {
		return nil, err
	}

	// Get user ID from session
	// This is simplified - in a real app you'd validate the session token
	// and look up the user ID from a sessions table
	// Placeholder
	userID := 1

	user, err := userModel.GetByID(int64(userID))
	if err != nil {
		return nil, err
	}

	return user, nil
}

// renderTemplate is a helper function to render templates
// This should match the signature used elsewhere in the codebase
func renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	// This is a placeholder - the actual implementation will be
	// in the main server package with access to template engine
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// In production, this would call the template engine:
	// templates.ExecuteTemplate(w, tmpl+".tmpl", data)

	// For now, just log that we would render
	log.Printf("Would render template %s with data: %+v", tmpl, data)
	fmt.Fprintf(w, "Historical Weather Page - template rendering not yet wired up")
}
