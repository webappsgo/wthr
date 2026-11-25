package handler

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
)

// ShowServerAdminsPage renders only the signed-in admin's details and the
// aggregate account count, matching the admin privacy rules in AI.md PART 17.
func ShowServerAdminsPage(w http.ResponseWriter, r *http.Request) {
	admins := &model.AdminModel{Store: database.GetUsersStore()}
	admin, err := admins.GetByID(AdminID(r))
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title":   Translate(r, "admin.admins.title"),
			"message": Translate(r, "errors.admin.admins.failed_to_fetch_admins"),
		}))
		return
	}

	count, err := admins.GetCount()
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/error.tmpl", AdminTemplateData(r, map[string]interface{}{
			"title":   Translate(r, "admin.admins.title"),
			"message": Translate(r, "errors.admin.admins.failed_to_count_admins"),
		}))
		return
	}

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin_admins.tmpl", AdminTemplateData(r, map[string]interface{}{
		"title":       Translate(r, "admin.admins.title"),
		"page":        "server-admins",
		"admin_count": count,
		"current_admin": struct {
			Username string
			Email    string
		}{Username: admin.Username, Email: admin.Email},
	}))
}

// ShowAdminDetailPage prevents the legacy numeric route from exposing or
// implying access to another administrator's details. The signed-in admin's
// account lives under their own username route.
func ShowAdminDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id != AdminID(r) {
		http.NotFound(w, r)
		return
	}

	username := AdminUsername(r)
	if username == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, adminPagePath(r, "/"+url.PathEscape(username)+"/profile"), http.StatusSeeOther)
}
