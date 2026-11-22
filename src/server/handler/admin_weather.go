package handler

import (
	"encoding/json"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"
	"strconv"

	"github.com/webappsgo/wthr/src/util"
)

// AdminWeatherHandler handles weather-specific settings
type AdminWeatherHandler struct {
	ConfigPath string
}

// ShowWeatherSettings displays weather settings page
func (h *AdminWeatherHandler) ShowWeatherSettings(w http.ResponseWriter, r *http.Request) {
	c := readAdminConfig(h.ConfigPath)
	settings := map[string]interface{}{"openmeteo_enabled": true, "openmeteo_base_url": "https://api.open-meteo.com", "openmeteo_timeout": 30, "openmeteo_retry_attempts": 3, "cache_enabled": true, "cache_ttl": 900, "cache_max_size": 100, "forecast_enabled": true, "current_weather_enabled": true, "alerts_check_interval": 15, "alerts_severity_threshold": "moderate", "api_rate_limit": 60, "api_max_forecast_days": 16, "api_max_historical_days": 365}
	for key, path := range map[string]string{
		"openmeteo_enabled": "weather.sources.openmeteo.enabled", "openmeteo_base_url": "weather.sources.openmeteo.base_url", "openmeteo_timeout": "weather.sources.openmeteo.timeout", "openmeteo_retry_attempts": "weather.sources.openmeteo.retry_attempts",
		"usgs_earthquake_enabled": "weather.sources.usgs_earthquake.enabled", "nhc_hurricane_enabled": "weather.sources.nhc_hurricane.enabled", "cache_enabled": "weather.cache.enabled", "cache_ttl": "weather.cache.ttl", "cache_max_size": "weather.cache.max_size",
		"forecast_enabled": "weather.features.forecast", "current_weather_enabled": "weather.features.current_weather", "historical_data_enabled": "weather.features.historical_data", "alerts_enabled": "weather.alerts.enabled", "alerts_check_interval": "weather.alerts.check_interval", "alerts_severity_threshold": "weather.alerts.severity_threshold",
		"api_rate_limit": "weather.api.rate_limit", "api_max_forecast_days": "weather.api.max_forecast_days", "api_max_historical_days": "weather.api.max_historical_days",
	} {
		v := adminConfigValue(c, path)
		if v != nil {
			settings[key] = v
		}
	}
	data := map[string]interface{}{
		"title":    Translate(r, "admin.weather.weather_settings"),
		"settings": settings,
		"flash":    TakeFlash(w, r),
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_weather.tmpl", AdminTemplateData(r, data))
}

// UpdateWeatherSettingsForm handles session-authenticated HTML form submissions.
func (h *AdminWeatherHandler) UpdateWeatherSettingsForm(w http.ResponseWriter, r *http.Request) {
	target := adminPagePath(r, "/settings/weather")
	if err := r.ParseForm(); err != nil {
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	ints := map[string]int{}
	for _, name := range []string{"openmeteo_timeout", "openmeteo_retry_attempts", "cache_ttl", "cache_max_size", "alerts_check_interval", "api_rate_limit", "api_max_forecast_days", "api_max_historical_days"} {
		value, err := strconv.Atoi(r.FormValue(name))
		if err != nil || value < 0 {
			redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
			return
		}
		ints[name] = value
	}
	severity := r.FormValue("alerts_severity_threshold")
	switch severity {
	case "minor", "moderate", "severe", "extreme":
	default:
		redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", errInvalidAdminForm)
		return
	}
	updates := map[string]interface{}{
		"weather.sources.openmeteo.enabled": r.FormValue("openmeteo_enabled") == "true", "weather.sources.openmeteo.base_url": r.FormValue("openmeteo_base_url"), "weather.sources.openmeteo.timeout": ints["openmeteo_timeout"], "weather.sources.openmeteo.retry_attempts": ints["openmeteo_retry_attempts"],
		"weather.sources.usgs_earthquake.enabled": r.FormValue("usgs_earthquake_enabled") == "true", "weather.sources.nhc_hurricane.enabled": r.FormValue("nhc_hurricane_enabled") == "true", "weather.cache.enabled": r.FormValue("cache_enabled") == "true", "weather.cache.ttl": ints["cache_ttl"], "weather.cache.max_size": ints["cache_max_size"],
		"weather.features.forecast": r.FormValue("forecast_enabled") == "true", "weather.features.current_weather": r.FormValue("current_weather_enabled") == "true", "weather.features.historical_data": r.FormValue("historical_data_enabled") == "true", "weather.alerts.enabled": r.FormValue("alerts_enabled") == "true", "weather.alerts.check_interval": ints["alerts_check_interval"], "weather.alerts.severity_threshold": severity,
		"weather.api.rate_limit": ints["api_rate_limit"], "weather.api.max_forecast_days": ints["api_max_forecast_days"], "weather.api.max_historical_days": ints["api_max_historical_days"],
	}
	err := util.UpdateYAMLConfig(h.ConfigPath, updates)
	redirectAdminForm(w, r, target, "flash.settings_saved", "flash.settings_save_failed", err)
}

// UpdateWeatherSettings updates weather settings in server.yml
func (h *AdminWeatherHandler) UpdateWeatherSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// Sources
		OpenMeteoEnabled       bool   `json:"openmeteo_enabled"`
		OpenMeteoBaseURL       string `json:"openmeteo_base_url"`
		OpenMeteoTimeout       int    `json:"openmeteo_timeout"`
		OpenMeteoRetryAttempts int    `json:"openmeteo_retry_attempts"`
		USGSEarthquakeEnabled  bool   `json:"usgs_earthquake_enabled"`
		NHCHurricaneEnabled    bool   `json:"nhc_hurricane_enabled"`
		// Cache
		CacheEnabled bool `json:"cache_enabled"`
		CacheTTL     int  `json:"cache_ttl"`
		CacheMaxSize int  `json:"cache_max_size"`
		// Features
		ForecastEnabled       bool `json:"forecast_enabled"`
		CurrentWeatherEnabled bool `json:"current_weather_enabled"`
		HistoricalDataEnabled bool `json:"historical_data_enabled"`
		// Alerts
		AlertsEnabled           bool   `json:"alerts_enabled"`
		AlertsCheckInterval     int    `json:"alerts_check_interval"`
		AlertsSeverityThreshold string `json:"alerts_severity_threshold"`
		// API Limits
		APIRateLimit         int `json:"api_rate_limit"`
		APIMaxForecastDays   int `json:"api_max_forecast_days"`
		APIMaxHistoricalDays int `json:"api_max_historical_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, err.Error())
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

	if err := util.UpdateYAMLConfig(h.ConfigPath, updates); err != nil {
		InternalError(w, r, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
