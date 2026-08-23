package handler

import (
	"context"
	"github.com/webappsgo/wthr/src/server/middleware"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// adminUsernameKey is the reqctx key holding the resolved admin username.
const adminUsernameKey = "admin_username"

// AdminLang returns the active request language, defaulting to English when the
// i18n middleware has not resolved one (AI.md PART 31 fallback chain).
func AdminLang(r *http.Request) string {
	if value, ok := reqctx.Get(r.Context(), "lang"); ok {
		if lang, ok := value.(string); ok && lang != "" {
			return lang
		}
	}
	return "en"
}

// AdminTranslate resolves a translation key for the active request language.
// A missing global i18n instance falls back to the key itself so a page never
// fails to render because translations are unavailable.
func AdminTranslate(r *http.Request, key string) string {
	if instance := i18n.GetGlobalI18n(); instance != nil {
		return instance.T(AdminLang(r), key)
	}
	return key
}

// AdminUsername returns the username of the currently authenticated Server
// Admin. The API group carries the full admin record on the context; the HTML
// session group only carries the admin id, so that case is resolved from the
// server database on every call (net/http's immutable *http.Request context
// cannot be memoized the way gin.Context.Set could).
func AdminUsername(r *http.Request) string {
	if value, ok := reqctx.Get(r.Context(), adminUsernameKey); ok {
		if username, ok := value.(string); ok && username != "" {
			return username
		}
	}

	if value, ok := reqctx.Get(r.Context(), "admin"); ok {
		if admin, ok := value.(*model.Admin); ok && admin != nil && admin.Username != "" {
			return admin.Username
		}
	}

	adminID, ok := reqctx.Get(r.Context(), "admin_id")
	if !ok {
		return ""
	}
	var id int64
	switch typed := adminID.(type) {
	case int:
		id = int64(typed)
	case int64:
		id = typed
	default:
		return ""
	}

	db := database.GetServerDB()
	if db == nil {
		return ""
	}
	var username string
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		"SELECT username FROM server_admin_credentials WHERE id = ?", id).Scan(&username)
	if err != nil || username == "" {
		return ""
	}
	return username
}

// AdminTemplateData enriches the shared template data with the admin's own
// account paths. AI.md PART 17 puts the admin's profile, preferences and
// notifications under /{admin_path}/{admin_username}/, so admin chrome needs
// the resolved username to build those links.
func AdminTemplateData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	enriched := util.TemplateData(r, data)
	username := AdminUsername(r)
	enriched[adminUsernameKey] = username

	selfPath := ""
	if username != "" {
		if adminPath, ok := enriched["admin_path"].(string); ok && adminPath != "" {
			selfPath = adminPath + "/" + username
		}
	}
	enriched["admin_self_path"] = selfPath

	selfAPIPath := ""
	if username != "" {
		if adminAPIPath, ok := enriched["admin_api_path"].(string); ok && adminAPIPath != "" {
			selfAPIPath = adminAPIPath + "/" + username
		}
	}
	enriched["admin_self_api_path"] = selfAPIPath

	return enriched
}

// RequireAdminSelf rejects HTML requests whose {admin_username} segment does
// not belong to the authenticated admin. AI.md PART 17 scopes that subtree to
// the admin's OWN account, and a mismatch must not disclose whether another
// admin exists, so it answers 404 rather than 403.
func RequireAdminSelf() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminSelfMatches(r) {
				next.ServeHTTP(w, r)
				return
			}
			middleware.RenderHTML(w, r, http.StatusNotFound, "page/error.tmpl", util.TemplateData(r, map[string]interface{}{
				"title": AdminTranslate(r, "error.not_found"),
				"error": AdminTranslate(r, "error.not_found"),
			}))
		})
	}
}

// RequireAdminSelfAPI is the JSON counterpart of RequireAdminSelf, returning
// the canonical error shape from AI.md PART 14.
func RequireAdminSelfAPI() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminSelfMatches(r) {
				next.ServeHTTP(w, r)
				return
			}
			NotFound(w, r, AdminTranslate(r, "error.not_found"))
		})
	}
}

// adminSelfMatches reports whether the {admin_username} path segment matches
// the authenticated admin.
func adminSelfMatches(r *http.Request) bool {
	requested := chi.URLParam(r, "admin_username")
	if requested == "" {
		return false
	}
	current := AdminUsername(r)
	return current != "" && requested == current
}
