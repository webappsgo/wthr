package handler

import (
	"net/http"

	"github.com/casapps/wthr/src/util"
	"github.com/gin-gonic/gin"
)

// AdminGeoIPHandler handles GeoIP settings
type AdminGeoIPHandler struct {
	ConfigPath string
}

// ShowGeoIPSettings displays GeoIP settings page
func (h *AdminGeoIPHandler) ShowGeoIPSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_geoip.tmpl", gin.H{
		"title": "GeoIP Settings",
	})
}

// UpdateGeoIPSettings updates GeoIP settings
func (h *AdminGeoIPHandler) UpdateGeoIPSettings(c *gin.Context) {
	var req struct {
		Enabled          bool     `json:"enabled"`
		Dir              string   `json:"dir"`
		UpdateFrequency  int      `json:"update_frequency"`
		DenyCountries    []string `json:"deny_countries"`
		DatabaseASN      bool     `json:"database_asn"`
		DatabaseCountry  bool     `json:"database_country"`
		DatabaseCity     bool     `json:"database_city"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

	if err := utils.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
