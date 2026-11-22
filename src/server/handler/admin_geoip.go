package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server/middleware"
)

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// AdminGeoIPHandler handles GeoIP settings.
type AdminGeoIPHandler struct {
	DataDir   string
	Scheduler *scheduler.Scheduler
}

// ShowGeoIPSettings displays the configured country policy and saved presets.
func (h *AdminGeoIPHandler) ShowGeoIPSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	geoip := config.GeoIPConfig{}
	if cfg != nil {
		geoip = cfg.Server.GeoIP
	}
	selectedPreset := r.URL.Query().Get("preset")
	selectedMode := r.URL.Query().Get("mode")
	if selectedMode != "allow" {
		selectedMode = "deny"
	}
	if selectedPreset != "" {
		if countries, ok := geoip.Presets[selectedPreset]; ok {
			if selectedMode == "allow" {
				geoip.AllowCountries = countries
			} else {
				geoip.DenyCountries = countries
			}
		}
	}
	presetNames := make([]string, 0, len(geoip.Presets))
	for name := range geoip.Presets {
		presetNames = append(presetNames, name)
	}
	sort.Strings(presetNames)
	updateSchedule := ""
	if h.Scheduler != nil {
		for _, task := range h.Scheduler.GetTaskStatus() {
			if task["name"] == "geoip_update" {
				updateSchedule, _ = task["schedule"].(string)
				break
			}
		}
	}
	lastUpdate, databaseSize, databasesPresent := geoIPDatabaseStatus(h.DataDir)
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_geoip.tmpl", AdminTemplateData(r, map[string]interface{}{
		"Title":                 Translate(r, "admin.geoip.geoip_settings"),
		"GeoIPEnabled":          geoip.Enabled,
		"DenyCountries":         strings.Join(geoip.DenyCountries, ", "),
		"AllowCountries":        strings.Join(geoip.AllowCountries, ", "),
		"Presets":               geoip.Presets,
		"PresetNames":           presetNames,
		"SelectedPreset":        selectedPreset,
		"SelectedMode":          selectedMode,
		"GeoIPUpdateSchedule":   updateSchedule,
		"GeoIPLastUpdate":       lastUpdate,
		"GeoIPDatabaseSize":     databaseSize,
		"GeoIPDatabasesPresent": databasesPresent,
		"flash":                 TakeFlash(w, r),
	}))
}

func geoIPDatabaseStatus(dataDir string) (string, int64, int) {
	if dataDir == "" {
		return "", 0, 0
	}
	files := []string{"dbip-city-ipv4.mmdb", "dbip-city-ipv6.mmdb", "iptoasn-country.mmdb", "iptoasn-asn.mmdb"}
	var lastUpdate string
	var oldest time.Time
	var totalSize int64
	present := 0
	for _, name := range files {
		info, err := os.Lstat(filepath.Join(dataDir, "security", "geoip", name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		present++
		totalSize += info.Size()
		modified := info.ModTime()
		if oldest.IsZero() || modified.Before(oldest) {
			oldest = modified
		}
	}
	if !oldest.IsZero() {
		lastUpdate = oldest.Local().Format("2006-01-02 15:04:05 MST")
	}
	return lastUpdate, totalSize, present
}

// UpdateGeoIPSettings accepts a server-rendered form or the admin JSON API.
func (h *AdminGeoIPHandler) UpdateGeoIPSettings(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	var deny, allow, presetName, presetMode string
	var removePreset bool
	var saveFailed bool
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var request struct {
			Enabled bool     `json:"enabled"`
			Deny    []string `json:"deny_countries"`
			Allow   []string `json:"allow_countries"`
			Preset  string   `json:"preset_name"`
			SaveAs  string   `json:"preset_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			BadRequest(w, r, err.Error())
			return
		}
		enabled = request.Enabled
		deny = strings.Join(request.Deny, ",")
		allow = strings.Join(request.Allow, ",")
		presetName, presetMode = request.Preset, request.SaveAs
	} else {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/network/geoip"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		enabled = r.PostForm.Get("enabled") == "on"
		deny, allow = r.PostForm.Get("deny_countries"), r.PostForm.Get("allow_countries")
		presetName, presetMode = r.PostForm.Get("preset_name"), r.PostForm.Get("preset_mode")
		removePreset = r.PostForm.Get("remove_preset") == "1"
	}
	denyCountries, err := parseCountryCodes(deny)
	if err == nil {
		var allowCountries []string
		allowCountries, err = parseCountryCodes(allow)
		if err == nil {
			geoip := config.GeoIPConfig{Enabled: enabled, DenyCountries: denyCountries, AllowCountries: allowCountries}
			if current := config.GetGlobalConfig(); current != nil {
				geoip.Presets = current.Server.GeoIP.Presets
			}
			if geoip.Presets == nil {
				geoip.Presets = make(map[string][]string)
			}
			presetName = strings.TrimSpace(presetName)
			if len(presetName) > 64 {
				err = fmt.Errorf("preset name must be 64 characters or fewer")
			}
			if err == nil && removePreset && presetName != "" {
				delete(geoip.Presets, presetName)
			} else if err == nil && presetName != "" {
				if presetMode != "deny" && presetMode != "allow" {
					err = fmt.Errorf("choose whether the preset contains the deny or allow list")
				} else if presetMode == "allow" {
					geoip.Presets[presetName] = append([]string(nil), allowCountries...)
				} else {
					geoip.Presets[presetName] = append([]string(nil), denyCountries...)
				}
			}
			if err == nil {
				err = config.SaveGeoIPArea(geoip)
				saveFailed = err != nil
				if err == nil {
					if current := config.GetGlobalConfig(); current != nil {
						current.Server.GeoIP = geoip
						config.SetGlobalConfig(current)
					}
				}
			}
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err != nil {
			if saveFailed {
				InternalError(w, r, err.Error())
			} else {
				BadRequest(w, r, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	redirectAdminForm(w, r, adminPagePath(r, "/network/geoip"), "flash.settings_saved", "flash.settings_save_failed", err)
}

func parseCountryCodes(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return []string{}, nil
	}
	seen := make(map[string]bool)
	var countries []string
	for _, code := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' }) {
		code = strings.ToUpper(strings.TrimSpace(code))
		if !countryCodePattern.MatchString(code) {
			return nil, fmt.Errorf("invalid ISO 3166-1 alpha-2 country code: %q", code)
		}
		if !seen[code] {
			countries = append(countries, code)
			seen[code] = true
		}
	}
	return countries, nil
}
