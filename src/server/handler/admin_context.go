package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/util"
)

// adminUsernameKey is the gin context key holding the resolved admin username.
const adminUsernameKey = "admin_username"

// AdminLang returns the active request language, defaulting to English when the
// i18n middleware has not resolved one (AI.md PART 31 fallback chain).
func AdminLang(c *gin.Context) string {
	if value, ok := c.Get("lang"); ok {
		if lang, ok := value.(string); ok && lang != "" {
			return lang
		}
	}
	return "en"
}

// AdminTranslate resolves a translation key for the active request language.
// A missing global i18n instance falls back to the key itself so a page never
// fails to render because translations are unavailable.
func AdminTranslate(c *gin.Context, key string) string {
	if instance := i18n.GetGlobalI18n(); instance != nil {
		return instance.T(AdminLang(c), key)
	}
	return key
}

// AdminUsername returns the username of the currently authenticated Server
// Admin. The API group carries the full admin record on the context; the HTML
// session group only carries the admin id, so that case is resolved from the
// server database and memoized on the context for the rest of the request.
func AdminUsername(c *gin.Context) string {
	if value, ok := c.Get(adminUsernameKey); ok {
		if username, ok := value.(string); ok && username != "" {
			return username
		}
	}

	if value, ok := c.Get("admin"); ok {
		if admin, ok := value.(*model.Admin); ok && admin != nil && admin.Username != "" {
			c.Set(adminUsernameKey, admin.Username)
			return admin.Username
		}
	}

	adminID, ok := c.Get("admin_id")
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
	c.Set(adminUsernameKey, username)
	return username
}

// AdminTemplateData enriches the shared template data with the admin's own
// account paths. AI.md PART 17 puts the admin's profile, preferences and
// notifications under /{admin_path}/{admin_username}/, so admin chrome needs
// the resolved username to build those links.
func AdminTemplateData(c *gin.Context, data gin.H) gin.H {
	enriched := util.TemplateData(c, data)
	username := AdminUsername(c)
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
func RequireAdminSelf() gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminSelfMatches(c) {
			c.Next()
			return
		}
		c.HTML(http.StatusNotFound, "page/error.tmpl", util.TemplateData(c, gin.H{
			"title": AdminTranslate(c, "error.not_found"),
			"error": AdminTranslate(c, "error.not_found"),
		}))
		c.Abort()
	}
}

// RequireAdminSelfAPI is the JSON counterpart of RequireAdminSelf, returning
// the canonical error shape from AI.md PART 14.
func RequireAdminSelfAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminSelfMatches(c) {
			c.Next()
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"ok":      false,
			"error":   "NOT_FOUND",
			"message": AdminTranslate(c, "error.not_found"),
		})
		c.Abort()
	}
}

// adminSelfMatches reports whether the {admin_username} path segment matches
// the authenticated admin.
func adminSelfMatches(c *gin.Context) bool {
	requested := c.Param("admin_username")
	if requested == "" {
		return false
	}
	current := AdminUsername(c)
	return current != "" && requested == current
}
