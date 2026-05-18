package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
	"github.com/casapps/wthr/src/util"
)

// HistoryHandler handles the historical weather page
type HistoryHandler struct {
	weatherService *service.WeatherService
	settingsModel  *models.SettingsModel
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(weatherService *service.WeatherService, settingsModel *models.SettingsModel) *HistoryHandler {
	return &HistoryHandler{
		weatherService: weatherService,
		settingsModel:  settingsModel,
	}
}

// ShowHistory displays the historical weather page
func (h *HistoryHandler) ShowHistory(c *gin.Context) {
	if !h.settingsModel.GetBool("history.enabled", true) {
		c.HTML(http.StatusServiceUnavailable, "page/error.tmpl", utils.TemplateData(c, gin.H{
			"title":   "Feature Disabled",
			"code":    503,
			"message": "Historical weather feature is disabled",
		}))
		return
	}

	defaultYears := h.settingsModel.GetInt("history.default_years", 10)
	minYears := h.settingsModel.GetInt("history.min_years", 5)
	maxYears := h.settingsModel.GetInt("history.max_years", 50)

	location := c.Query("location")
	dateStr := c.Query("date")
	yearsStr := c.Query("years")

	data := gin.H{
		"title":        "Historical Weather",
		"page":         "history",
		"defaultYears": defaultYears,
		"minYears":     minYears,
		"maxYears":     maxYears,
	}

	if location == "" && dateStr == "" {
		NegotiateResponse(c, "page/history.tmpl", utils.TemplateData(c, data))
		return
	}

	month, day, startYear, err := parseHistoricalDate(dateStr)
	if err != nil {
		data["error"] = fmt.Sprintf("Invalid date format: %v", err)
		NegotiateResponse(c, "page/history.tmpl", utils.TemplateData(c, data))
		return
	}

	numberOfYears := defaultYears
	if yearsStr != "" {
		if years, err := strconv.Atoi(yearsStr); err == nil && years >= minYears && years <= maxYears {
			numberOfYears = years
		}
	}

	coords, err := h.weatherService.GetCoordinates(location, "")
	if err != nil {
		data["error"] = fmt.Sprintf("Could not find location: %v", err)
		NegotiateResponse(c, "page/history.tmpl", utils.TemplateData(c, data))
		return
	}

	historical, err := h.weatherService.GetHistoricalWeather(
		coords.Latitude,
		coords.Longitude,
		month,
		day,
		startYear,
		numberOfYears,
	)
	if err != nil {
		data["error"] = fmt.Sprintf("Error fetching historical weather: %v", err)
		NegotiateResponse(c, "page/history.tmpl", utils.TemplateData(c, data))
		return
	}

	data["location"] = location
	data["date"] = dateStr
	data["startYear"] = startYear
	data["historical"] = historical

	NegotiateResponse(c, "page/history.tmpl", utils.TemplateData(c, data))
}

// parseHistoricalDate parses a date string with smart year detection.
// Supported formats: MM/DD, MM/DD/YYYY, YYYY-MM-DD, "Jan 2", "Jan 2, 2006",
// "January 2", "January 2, 2006".
func parseHistoricalDate(dateStr string) (month, day, year int, err error) {
	if dateStr == "" {
		now := time.Now()
		return int(now.Month()), now.Day(), now.Year(), nil
	}

	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"Jan 2, 2006",
		"January 2, 2006",
	}
	for _, f := range formats {
		if t, e := time.Parse(f, dateStr); e == nil {
			return int(t.Month()), t.Day(), t.Year(), nil
		}
	}

	yearlessFormats := []string{
		"01/02",
		"Jan 2",
		"January 2",
	}
	currentYear := time.Now().Year()
	for _, f := range yearlessFormats {
		if t, e := time.Parse(f, dateStr); e == nil {
			return int(t.Month()), t.Day(), currentYear, nil
		}
	}

	return 0, 0, 0, fmt.Errorf("unsupported date format: %s (try MM/DD or MM/DD/YYYY)", dateStr)
}
