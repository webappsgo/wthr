package handler

import (
	"fmt"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// HistoryHandler handles the historical weather page
type HistoryHandler struct {
	weatherService *service.WeatherService
	settingsModel  *model.SettingsModel
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(weatherService *service.WeatherService, settingsModel *model.SettingsModel) *HistoryHandler {
	return &HistoryHandler{
		weatherService: weatherService,
		settingsModel:  settingsModel,
	}
}

// ShowHistory displays the historical weather page
func (h *HistoryHandler) ShowHistory(w http.ResponseWriter, r *http.Request) {
	if !h.settingsModel.GetBool("history.enabled", true) {
		middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":   "Feature Disabled",
			"code":    503,
			"message": "Historical weather feature is disabled",
		}))
		return
	}

	defaultYears := h.settingsModel.GetInt("history.default_years", 10)
	minYears := h.settingsModel.GetInt("history.min_years", 5)
	maxYears := h.settingsModel.GetInt("history.max_years", 50)

	location := r.URL.Query().Get("location")
	dateStr := r.URL.Query().Get("date")
	yearsStr := r.URL.Query().Get("years")

	data := map[string]interface{}{
		"title":        "Historical Weather",
		"page":         "history",
		"defaultYears": defaultYears,
		"minYears":     minYears,
		"maxYears":     maxYears,
	}

	if location == "" && dateStr == "" {
		NegotiateResponse(w, r, "page/history.tmpl", util.TemplateData(r, data))
		return
	}

	month, day, startYear, err := parseHistoricalDate(dateStr)
	if err != nil {
		data["error"] = fmt.Sprintf("Invalid date format: %v", err)
		NegotiateResponse(w, r, "page/history.tmpl", util.TemplateData(r, data))
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
		NegotiateResponse(w, r, "page/history.tmpl", util.TemplateData(r, data))
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
		NegotiateResponse(w, r, "page/history.tmpl", util.TemplateData(r, data))
		return
	}

	data["location"] = location
	data["date"] = dateStr
	data["startYear"] = startYear
	data["historical"] = historical

	NegotiateResponse(w, r, "page/history.tmpl", util.TemplateData(r, data))
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
