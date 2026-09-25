package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

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

// HandleHurricaneRequest handles hurricane tracking page requests with AI.md
// PART 14 frontend content negotiation: HTML for browsers, formatted text for
// text clients and HTTP tools, JSON for explicit API clients.
func (h *HurricaneHandler) HandleHurricaneRequest(w http.ResponseWriter, r *http.Request) {
	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		log.Printf("ERROR: active hurricanes fetch failed: %v", err)
		NegotiateErrorResponse(w, r, http.StatusInternalServerError, "page/hurricane.tmpl", ErrInternal, Translate(r, "errors.hurricane_fetch_failed"), util.TemplateData(r, map[string]interface{}{
			"Title":    Translate(r, "hurricane.title"),
			"HostInfo": util.GetHostInfo(r),
		}))
		return
	}

	if wantsExplicitJSON(r) || isOurCLIClient(r) {
		RespondNegotiatedData(w, r, http.StatusOK, data)
		return
	}

	NegotiateResponse(w, r, "page/hurricane.tmpl", util.TemplateData(r, map[string]interface{}{
		"Title":    Translate(r, "hurricane.title"),
		"Storms":   data.ActiveStorms,
		"Count":    len(data.ActiveStorms),
		"HostInfo": util.GetHostInfo(r),
	}))
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
func (h *HurricaneHandler) HandleHurricaneAPI(w http.ResponseWriter, r *http.Request) {
	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		log.Printf("ERROR: active hurricanes fetch failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, Translate(r, "errors.hurricane_fetch_failed"))
		return
	}

	RespondNegotiatedData(w, r, http.StatusOK, data)
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
func (h *HurricaneHandler) HandleHurricaneByIDAPI(w http.ResponseWriter, r *http.Request) {
	hurricaneID := chi.URLParam(r, "id")
	if hurricaneID == "" {
		RespondError(w, r, http.StatusBadRequest, ErrInvalidInput, Translate(r, "errors.hurricane_id_required"))
		return
	}

	data, err := h.hurricaneService.GetActiveStorms()
	if err != nil {
		log.Printf("ERROR: active hurricanes fetch failed: %v", err)
		RespondError(w, r, http.StatusInternalServerError, ErrInternal, Translate(r, "errors.hurricane_fetch_failed"))
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
		NotFound(w, r, Translate(r, "errors.hurricane_not_found"))
		return
	}

	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"hurricane": hurricane,
	})
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
