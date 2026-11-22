package handler

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
)

var roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

var rolePermissionOptions = []string{"read", "write", "moderate"}

type adminRoleRow struct {
	Name        string
	Count       int
	IsDefault   bool
	BuiltIn     bool
	CanRead     bool
	CanWrite    bool
	CanModerate bool
}

func readAdminRoleSettings() (map[string]interface{}, []string, string, map[string][]string, error) {
	value, exists, err := config.ReadSetting("server.users.roles")
	if err != nil {
		return nil, nil, "", nil, err
	}
	root := map[string]interface{}{}
	if exists {
		if parsed, ok := value.(map[string]interface{}); ok {
			root = parsed
		}
	}
	available := stringSlice(root["available"])
	if len(available) == 0 {
		available = []string{"admin", "user"}
	}
	defaultRole, _ := root["default"].(string)
	if defaultRole == "" {
		defaultRole = "user"
	}
	permissions := map[string][]string{}
	if values, ok := root["permissions"].(map[string]interface{}); ok {
		for name, raw := range values {
			permissions[name] = stringSlice(raw)
		}
	}
	return root, available, defaultRole, permissions, nil
}

func stringSlice(value interface{}) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

// ShowAdminRolesPage renders configured roles and their assigned user counts.
func ShowAdminRolesPage(w http.ResponseWriter, r *http.Request) {
	root, available, defaultRole, permissions, err := readAdminRoleSettings()
	if err != nil {
		renderRolePageError(w, r)
		return
	}
	users := &model.UserModel{}
	roles := make([]adminRoleRow, 0, len(available))
	for _, name := range available {
		count, err := users.CountByRole(name)
		if err != nil {
			renderRolePageError(w, r)
			return
		}
		grants := permissions[name]
		roles = append(roles, adminRoleRow{
			Name: name, Count: count, IsDefault: name == defaultRole, BuiltIn: name == "admin" || name == "user",
			CanRead: containsRolePermission(grants, "read"), CanWrite: containsRolePermission(grants, "write"), CanModerate: containsRolePermission(grants, "moderate"),
		})
	}
	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_roles.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.roles.title"), "page": "server-roles", "roles": roles,
		"available_roles": available, "default_role": defaultRole, "permissions": rolePermissionOptions,
		"flash": TakeFlash(w, r), "role_settings": root,
	}))
}

// CreateAdminRoleForm adds a custom role without replacing unrelated role settings.
func CreateAdminRoleForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.FormValue("name")))
	if !roleNamePattern.MatchString(name) {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	root, available, _, permissions, err := readAdminRoleSettings()
	if err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	if containsRolePermission(available, name) {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	grants, ok := submittedRolePermissions(r)
	if !ok {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	available = append(available, name)
	permissions[name] = grants
	root["available"] = available
	root["permissions"] = permissions
	saveRoleFormResult(w, r, config.WriteSetting("server.users.roles", root))
}

// UpdateAdminRolePermissionsForm updates permission grants for a custom role.
func UpdateAdminRolePermissionsForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	name := chi.URLParam(r, "name")
	root, available, _, permissions, err := readAdminRoleSettings()
	if err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	if !roleNamePattern.MatchString(name) || !containsRolePermission(available, name) || name == "admin" || name == "user" {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	grants, ok := submittedRolePermissions(r)
	if !ok {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	permissions[name] = grants
	root["permissions"] = permissions
	saveRoleFormResult(w, r, config.WriteSetting("server.users.roles", root))
}

// SetDefaultAdminRoleForm selects which role newly created users receive.
func SetDefaultAdminRoleForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	root, available, _, _, err := readAdminRoleSettings()
	if err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	role := r.FormValue("default_role")
	if !containsRolePermission(available, role) {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	root["default"] = role
	saveRoleFormResult(w, r, config.WriteSetting("server.users.roles", root))
}

// DeleteAdminRoleForm deletes a custom role only when no users use it and it is not default.
func DeleteAdminRoleForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	if r.FormValue("confirm_delete") != "yes" {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	name := chi.URLParam(r, "name")
	root, available, defaultRole, permissions, err := readAdminRoleSettings()
	if err != nil {
		saveRoleFormResult(w, r, err)
		return
	}
	if !roleNamePattern.MatchString(name) || name == "admin" || name == "user" || name == defaultRole || !containsRolePermission(available, name) {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	count, err := (&model.UserModel{}).CountByRole(name)
	if err != nil || count != 0 {
		saveRoleFormResult(w, r, errInvalidAdminForm)
		return
	}
	filtered := make([]string, 0, len(available)-1)
	for _, role := range available {
		if role != name {
			filtered = append(filtered, role)
		}
	}
	delete(permissions, name)
	root["available"] = filtered
	root["permissions"] = permissions
	saveRoleFormResult(w, r, config.WriteSetting("server.users.roles", root))
}

func submittedRolePermissions(r *http.Request) ([]string, bool) {
	selected := map[string]bool{}
	for _, permission := range r.Form["permissions"] {
		valid := false
		for _, option := range rolePermissionOptions {
			if option == permission {
				valid = true
				break
			}
		}
		if !valid {
			return nil, false
		}
		selected[permission] = true
	}
	grants := make([]string, 0, len(selected))
	for _, option := range rolePermissionOptions {
		if selected[option] {
			grants = append(grants, option)
		}
	}
	return grants, true
}

func containsRolePermission(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func saveRoleFormResult(w http.ResponseWriter, r *http.Request, err error) {
	redirectAdminForm(w, r, adminPagePath(r, "/roles"), "flash.settings_saved", "flash.settings_save_failed", err)
}

func renderRolePageError(w http.ResponseWriter, r *http.Request) {
	middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title": Translate(r, "admin.roles.title"), "message": Translate(r, "errors.admin.core.failed_to_fetch_settings"),
	}))
}
