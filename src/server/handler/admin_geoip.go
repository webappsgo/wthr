package handler

import (
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"

	"github.com/webappsgo/wthr/src/util"
)

// AdminGeoIPHandler handles GeoIP settings
type AdminGeoIPHandler struct {
	ConfigPath string
}

// ShowGeoIPSettings displays GeoIP settings page
func (h *AdminGeoIPHandler) ShowGeoIPSettings(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_geoip.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.geoip.geoip_settings"),
	}))
}

// UpdateGeoIPSettings updates GeoIP settings
func (h *AdminGeoIPHandler) UpdateGeoIPSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled         bool     `json:"enabled"`
		Dir             string   `json:"dir"`
		UpdateFrequency int      `json:"update_frequency"`
		DenyCountries   []string `json:"deny_countries"`
		DatabaseASN     bool     `json:"database_asn"`
		DatabaseCountry bool     `json:"database_country"`
		DatabaseCity    bool     `json:"database_city"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	updates := map[string]interface{}{
		"server.geoip.enabled":           req.Enabled,
		"server.geoip.dir":               req.Dir,
		"server.geoip.update_frequency":  req.UpdateFrequency,
		"server.geoip.deny_countries":    req.DenyCountries,
		"server.geoip.databases.asn":     req.DatabaseASN,
		"server.geoip.databases.country": req.DatabaseCountry,
		"server.geoip.databases.city":    req.DatabaseCity,
	}

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		InternalError(w, r, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
