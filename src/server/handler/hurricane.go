package handler

import (
	"fmt"
	"log"
	"net/http"

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
