// Server-rendered admin settings form per AI.md PART 17 (plain HTML form POST, PRG + flash, no JavaScript required)
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
)

// settingsFormField is one rendered control on the settings page.
type settingsFormField struct {
	Key         string
	Label       string
	Description string
	Control     string
	Value       string
	Checked     bool
	Options     []string
	Restart     bool
	Secret      bool
}

// settingsFormSection groups the fields that share a key prefix.
type settingsFormSection struct {
	ID     string
	Title  string
	Fields []settingsFormField
}

// settingsSelectOptions lists the fixed choices of the string settings that are dropdowns.
var settingsSelectOptions = map[string][]string{
	"server.mode":        {"production", "development", "debug"},
	"server.date_format": {"US", "EU", "ISO"},
	"server.time_format": {"12-hour", "24-hour"},
}

// settingsTextareaSuffixes marks long free-text settings that render as a textarea.
var settingsTextareaSuffixes = []string{"description", "policy_text", "keywords"}

// settingsSecretSuffixes marks settings whose value is never echoed back to the form.
var settingsSecretSuffixes = []string{"password", "secret", "token", "private_key"}

// settingsRow is one server_config row.
type settingsRow struct {
	key, value, typ, description string
}

// loadSettingsRows reads every server_config row ordered by key.
func loadSettingsRows(ctx context.Context) ([]settingsRow, error) {
	rows, err := database.QueryContext(ctx, database.GetServerDB(), database.TimeoutSimpleSelect,
		`SELECT key, value, type, COALESCE(description, '') FROM server_config ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []settingsRow
	for rows.Next() {
		var row settingsRow
		if err := rows.Scan(&row.key, &row.value, &row.typ, &row.description); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func settingsKeyHasSuffix(key string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

// settingsFieldFor builds the form control for one row.
func settingsFieldFor(row settingsRow) settingsFormField {
	field := settingsFormField{
		Key:         row.key,
		Label:       humanizeSettingKey(row.key),
		Description: row.description,
		Value:       row.value,
		Restart:     strings.Contains(strings.ToLower(row.description), "requires restart"),
	}

	switch {
	case row.typ == "boolean":
		field.Control = "toggle"
		field.Checked = config.IsTruthy(row.value)
	case settingsKeyHasSuffix(row.key, settingsSecretSuffixes):
		field.Control = "password"
		field.Secret = true
		field.Value = ""
	case settingsSelectOptions[row.key] != nil:
		field.Control = "select"
		field.Options = settingsSelectOptions[row.key]
	case row.typ == "number":
		field.Control = "number"
	case row.typ == "json" || settingsKeyHasSuffix(row.key, settingsTextareaSuffixes):
		field.Control = "textarea"
	default:
		field.Control = "text"
	}
	return field
}

// humanizeSettingKey turns "smtp.from_address" into "From address".
func humanizeSettingKey(key string) string {
	name := key
	if idx := strings.Index(key, "."); idx >= 0 {
		name = key[idx+1:]
	}
	name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
	if name == "" {
		return key
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// buildSettingsSections groups rows by their key prefix, in stable order.
func buildSettingsSections(rows []settingsRow) []settingsFormSection {
	byID := make(map[string]*settingsFormSection)
	for _, row := range rows {
		id := row.key
		if idx := strings.Index(row.key, "."); idx > 0 {
			id = row.key[:idx]
		}
		section, ok := byID[id]
		if !ok {
			section = &settingsFormSection{ID: id, Title: humanizeSettingKey("." + id)}
			byID[id] = section
		}
		section.Fields = append(section.Fields, settingsFieldFor(row))
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	sections := make([]settingsFormSection, 0, len(ids))
	for _, id := range ids {
		sections = append(sections, *byID[id])
	}
	return sections
}

// ShowSettingsForm renders /{admin_path}/settings with one section per key prefix.
func (h *AdminHandler) ShowSettingsForm(w http.ResponseWriter, r *http.Request) {
	rows, err := loadSettingsRows(r.Context())
	if err != nil {
		InternalError(w, r, Translate(r, "errors.admin.settings.load_failed"))
		return
	}

	sections := buildSettingsSections(rows)
	selected := r.URL.Query().Get("section")
	known := false
	for _, section := range sections {
		if section.ID == selected {
			known = true
			break
		}
	}
	if !known && len(sections) > 0 {
		selected = sections[0].ID
	}

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_settings_form.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":    Translate(r, "admin.settings_form.title"),
		"sections": sections,
		"selected": selected,
		"flash":    TakeFlash(w, r),
	}))
}

// SaveSettingsForm applies one posted section, then redirects back (PRG).
func (h *AdminHandler) SaveSettingsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, adminPagePath(r, "/settings"), http.StatusSeeOther)
		return
	}

	section := strings.TrimSpace(r.PostForm.Get("section"))
	target := adminPagePath(r, "/settings")
	if section != "" {
		target += "?section=" + url.QueryEscape(section)
	}

	rows, err := loadSettingsRows(r.Context())
	if err != nil {
		SetFlash(w, r, "error", "flash.settings_save_failed")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}

	updates := make(map[string]string)
	for _, row := range rows {
		if section == "" || !strings.HasPrefix(row.key, section+".") {
			continue
		}
		values, posted := r.PostForm[row.key]
		if !posted {
			continue
		}
		value := strings.TrimSpace(values[len(values)-1])

		switch row.typ {
		case "boolean":
			parsed, parseErr := config.ParseBool(value, false)
			if parseErr != nil {
				SetFlash(w, r, "error", "flash.settings_save_failed")
				http.Redirect(w, r, target, http.StatusSeeOther)
				return
			}
			value = strconv.FormatBool(parsed)
		case "number":
			if _, parseErr := strconv.ParseFloat(value, 64); parseErr != nil {
				SetFlash(w, r, "error", "flash.settings_save_failed")
				http.Redirect(w, r, target, http.StatusSeeOther)
				return
			}
		case "json":
			if !json.Valid([]byte(value)) {
				SetFlash(w, r, "error", "flash.settings_save_failed")
				http.Redirect(w, r, target, http.StatusSeeOther)
				return
			}
		}

		if settingsKeyHasSuffix(row.key, settingsSecretSuffixes) && value == "" {
			continue
		}
		if value != row.value {
			updates[row.key] = value
		}
	}

	now := dbtime.FormatSQLTimestamp(time.Now())
	for key, value := range updates {
		if _, execErr := database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite,
			`UPDATE server_config SET value = ?, updated_at = ? WHERE key = ?`, value, now, key); execErr != nil {
			SetFlash(w, r, "error", "flash.settings_save_failed")
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}

	SetFlash(w, r, "success", "flash.settings_saved")
	http.Redirect(w, r, target, http.StatusSeeOther)
}
