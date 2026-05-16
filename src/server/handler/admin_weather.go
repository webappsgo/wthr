package handler

import (
	"net/http"

	"github.com/apimgr/weather/src/util"
	"github.com/gin-gonic/gin"
)

// AdminWeatherHandler handles weather-specific settings
type AdminWeatherHandler struct {
	ConfigPath string
}

// ShowWeatherSettings displays weather settings page
func (h *AdminWeatherHandler) ShowWeatherSettings(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_weather.tmpl", gin.H{
		"title": "Weather Settings",
	})
}

// UpdateWeatherSettings updates weather settings in server.yml
func (h *AdminWeatherHandler) UpdateWeatherSettings(c *gin.Context) {
	var req struct {
		// Sources
		OpenMeteoEnabled       bool   `json:"openmeteo_enabled"`
		OpenMeteoBaseURL       string `json:"openmeteo_base_url"`
		OpenMeteoTimeout       int    `json:"openmeteo_timeout"`
		OpenMeteoRetryAttempts int    `json:"openmeteo_retry_attempts"`
		USGSEarthquakeEnabled  bool   `json:"usgs_earthquake_enabled"`
		NHCHurricaneEnabled    bool   `json:"nhc_hurricane_enabled"`
		// Cache
		CacheEnabled           bool   `json:"cache_enabled"`
		CacheTTL               int    `json:"cache_ttl"`
		CacheMaxSize           int    `json:"cache_max_size"`
		// Features
		ForecastEnabled        bool   `json:"forecast_enabled"`
		CurrentWeatherEnabled  bool   `json:"current_weather_enabled"`
		HistoricalDataEnabled  bool   `json:"historical_data_enabled"`
		// Alerts
		AlertsEnabled          bool   `json:"alerts_enabled"`
		AlertsCheckInterval    int    `json:"alerts_check_interval"`
		AlertsSeverityThreshold string `json:"alerts_severity_threshold"`
		// API Limits
		APIRateLimit           int    `json:"api_rate_limit"`
		APIMaxForecastDays     int    `json:"api_max_forecast_days"`
		APIMaxHistoricalDays   int    `json:"api_max_historical_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"weather.sources.openmeteo.enabled":        req.OpenMeteoEnabled,
		"weather.sources.openmeteo.base_url":       req.OpenMeteoBaseURL,
		"weather.sources.openmeteo.timeout":        req.OpenMeteoTimeout,
		"weather.sources.openmeteo.retry_attempts": req.OpenMeteoRetryAttempts,
		"weather.sources.usgs_earthquake.enabled":  req.USGSEarthquakeEnabled,
		"weather.sources.nhc_hurricane.enabled":    req.NHCHurricaneEnabled,
		"weather.cache.enabled":                    req.CacheEnabled,
		"weather.cache.ttl":                        req.CacheTTL,
		"weather.cache.max_size":                   req.CacheMaxSize,
		"weather.features.forecast":                req.ForecastEnabled,
		"weather.features.current_weather":         req.CurrentWeatherEnabled,
		"weather.features.historical_data":         req.HistoricalDataEnabled,
		"weather.alerts.enabled":                   req.AlertsEnabled,
		"weather.alerts.check_interval":            req.AlertsCheckInterval,
		"weather.alerts.severity_threshold":        req.AlertsSeverityThreshold,
		"weather.api.rate_limit":                   req.APIRateLimit,
		"weather.api.max_forecast_days":            req.APIMaxForecastDays,
		"weather.api.max_historical_days":          req.APIMaxHistoricalDays,
	}

	if err := utils.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
