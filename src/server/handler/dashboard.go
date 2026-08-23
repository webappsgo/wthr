package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

type DashboardHandler struct {
	DB *sql.DB
}

// ShowDashboard renders the user dashboard
func (h *DashboardHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}

	// Get user's saved locations
	locationModel := &model.LocationModel{DB: h.DB}
	locations, err := locationModel.GetByUserID(int(user.ID))
	if err != nil {
		// Empty array on error
		locations = []*model.SavedLocation{}
	}

	// Get unread notification count
	notificationModel := &model.NotificationModel{DB: h.DB}
	unreadCount, err := notificationModel.GetUnreadCount(user.ID)
	if err != nil {
		unreadCount = 0
	}

	NegotiateResponse(w, r, "page/dashboard.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":         "Dashboard - Weather",
		"user":          user,
		"locations":     locations,
		"unreadCount":   unreadCount,
		"locationCount": len(locations),
		"page":          "dashboard",
	}))
}

// ShowAdminPanel renders the admin panel
func (h *DashboardHandler) ShowAdminPanel(w http.ResponseWriter, r *http.Request) {
	adminIDValue, exists := reqctx.Get(r.Context(), "admin_id")
	if !exists {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	adminID, ok := adminIDValue.(int)
	if !ok {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	adminModel := &model.AdminModel{DB: database.GetServerDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		http.Redirect(w, r, "/server/admin", http.StatusFound)
		return
	}

	// Get system statistics
	userModel := &model.UserModel{DB: h.DB}

	totalUsers, _ := userModel.Count()
	adminCount, _ := userModel.CountByRole("admin")

	// Count total locations across all users
	var totalLocations int
	database.QueryRowContext(context.Background(), h.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_saved_locations").Scan(&totalLocations)

	middleware.RenderHTML(w, r, http.StatusOK, "admin/admin.tmpl", util.TemplateData(r, map[string]interface{}{
		"title":          "Admin Panel - Weather",
		"user":           admin,
		"totalUsers":     totalUsers,
		"adminCount":     adminCount,
		"totalLocations": totalLocations,
		"page":           "admin",
	}))
}
