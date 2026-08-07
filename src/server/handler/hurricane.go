package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// HurricaneHandler handles hurricane tracking requests
type HurricaneHandler struct {
	hurricaneService *service.HurricaneService
}

// NewHurricaneHandler creates a new hurricane handler
func NewHurricaneHandler(hurricaneService *service.HurricaneService) *HurricaneHandler {
	return &HurricaneHandler{
		hurricaneService: hurricaneService,
	}
}

// ListActiveStorms returns active storm data for non-HTTP callers such as GraphQL.
func (h *HurricaneHandler) ListActiveStorms() ([]service.Storm, error) {
	if h == nil || h.hurricaneService == nil {
		return nil, fmt.Errorf("hurricane service not initialized")
	}

	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		return nil, err
	}

	return data.ActiveStorms, nil
}

// HandleHurricaneRequest handles hurricane tracking page requests
func (h *HurricaneHandler) HandleHurricaneRequest(c *gin.Context) {
	// Check if user wants JSON
	accept := c.GetHeader("Accept")
	wantsJSON := strings.Contains(accept, "application/json")

	// Get active storms
	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		if wantsJSON {
			RespondError(c, http.StatusInternalServerError, ErrInternal, "Failed to fetch hurricane data")
		} else {
			c.String(http.StatusInternalServerError, "Failed to fetch hurricane data: %v", err)
		}
		return
	}

	// Return JSON if requested
	if wantsJSON {
		RespondNegotiatedData(c, http.StatusOK, data)
		return
	}

	// Check user agent to determine if browser or console
	isBrowser := util.IsBrowser(c)

	if isBrowser {
		// Render HTML template
		hostInfo := util.GetHostInfo(c)
		c.HTML(http.StatusOK, "page/hurricane.tmpl", util.TemplateData(c, gin.H{
			"Title":    "Active Hurricanes & Tropical Storms",
			"Storms":   data.ActiveStorms,
			"Count":    len(data.ActiveStorms),
			"HostInfo": hostInfo,
		}))
	} else {
		// Render console output
		output := h.renderConsoleOutput(data)
		c.String(http.StatusOK, output)
	}
}

// HandleHurricaneAPI handles JSON API requests for hurricane data
// @Summary Get active hurricanes (deprecated)
// @Description Get active hurricanes and tropical storms from NOAA NHC. Deprecated: use /api/v1/severe-weather instead
// @Tags hurricanes
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Active storms data"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Deprecated
// @Router /api/v1/hurricanes [get]
func (h *HurricaneHandler) HandleHurricaneAPI(c *gin.Context) {
	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, ErrInternal, "Failed to fetch hurricane data")
		return
	}

	RespondNegotiatedData(c, http.StatusOK, data)
}

// HandleHurricaneByIDAPI handles JSON API requests for a specific hurricane by ID
// @Summary Get hurricane by ID (deprecated)
// @Description Get detailed information for a specific hurricane by ID or name. Deprecated: use /api/v1/severe-weather instead
// @Tags hurricanes
// @Accept json
// @Produce json
// @Param id path string true "Hurricane ID or name"
// @Success 200 {object} map[string]interface{} "Hurricane details"
// @Failure 400 {object} map[string]interface{} "Bad request - ID required"
// @Failure 404 {object} map[string]interface{} "Hurricane not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Deprecated
// @Router /api/v1/hurricanes/{id} [get]
func (h *HurricaneHandler) HandleHurricaneByIDAPI(c *gin.Context) {
	hurricaneID := c.Param("id")
	if hurricaneID == "" {
		RespondError(c, http.StatusBadRequest, ErrInvalidInput, "Hurricane ID required")
		return
	}

	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, ErrInternal, "Failed to fetch hurricane data")
		return
	}

	// Find hurricane by ID or name (case-insensitive)
	var hurricane *service.Storm
	for i := range data.ActiveStorms {
		if data.ActiveStorms[i].ID == hurricaneID ||
			strings.EqualFold(data.ActiveStorms[i].Name, hurricaneID) {
			hurricane = &data.ActiveStorms[i]
			break
		}
	}

	if hurricane == nil {
		NotFound(c, "Hurricane not found")
		return
	}

	RespondNegotiatedData(c, http.StatusOK, gin.H{
		"ok":        true,
		"hurricane": hurricane,
	})
}

// renderConsoleOutput renders hurricane data for console/terminal
func (h *HurricaneHandler) renderConsoleOutput(data *service.HurricaneData) string {
	if len(data.ActiveStorms) == 0 {
		return "🌊 No active tropical storms or hurricanes at this time.\n\n"
	}

	output := "🌀 Active Tropical Storms & Hurricanes\n"
	output += "═══════════════════════════════════════\n\n"

	for _, storm := range data.ActiveStorms {
		icon := h.hurricaneService.GetStormIcon(storm.Classification, storm.WindSpeed)
		category := h.hurricaneService.GetStormCategory(storm.WindSpeed)

		output += icon + " " + storm.Name + "\n"
		output += "   Category: " + category + "\n"
		output += "   Wind Speed: " + formatInt(storm.WindSpeed) + " mph\n"
		output += "   Pressure: " + formatInt(storm.Pressure) + " mb\n"
		output += "   Location: " + formatFloat(storm.Latitude) + ", " + formatFloat(storm.Longitude) + "\n"

		if storm.MovementSpeed > 0 {
			output += "   Movement: " + storm.MovementDir + " at " + formatInt(storm.MovementSpeed) + " mph\n"
		}

		output += "   Last Update: " + storm.LastUpdate + "\n"

		if storm.PublicAdvisory != "" {
			output += "   Advisory: " + storm.PublicAdvisory + "\n"
		}

		output += "\n"
	}

	output += "Data from NOAA National Hurricane Center\n"
	output += "Updates every 10 minutes\n\n"

	return output
}

// Helper functions
func formatInt(val int) string {
	if val == 0 {
		return "N/A"
	}
	return formatIntToStr(val)
}

func formatFloat(val float64) string {
	if val == 0 {
		return "N/A"
	}
	return formatFloatToStr(val)
}

func formatIntToStr(val int) string {
	// Simple int to string
	if val < 0 {
		return "-" + formatIntToStr(-val)
	}
	if val < 10 {
		return string(rune('0' + val))
	}
	return formatIntToStr(val/10) + string(rune('0'+val%10))
}

func formatFloatToStr(val float64) string {
	// Simple float to string with 2 decimals
	intPart := int(val)
	fracPart := int((val - float64(intPart)) * 100)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	result := formatIntToStr(intPart) + "."
	if fracPart < 10 {
		result += "0"
	}
	result += formatIntToStr(fracPart)
	return result
}
