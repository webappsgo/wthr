package handler

import (
	"database/sql"
	"net/http"

	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/util"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	DB *sql.DB
}

// ShowDashboard renders the user dashboard
func (h *DashboardHandler) ShowDashboard(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.Redirect(http.StatusFound, "/server/auth/login")
		return
	}

	// Get user's saved locations
	locationModel := &models.LocationModel{DB: h.DB}
	locations, err := locationModel.GetByUserID(int(user.ID))
	if err != nil {
		// Empty array on error
		locations = []*models.SavedLocation{}
	}

	// Get unread notification count
	notificationModel := &models.NotificationModel{DB: h.DB}
	unreadCount, err := notificationModel.GetUnreadCount(user.ID)
	if err != nil {
		unreadCount = 0
	}

	NegotiateResponse(c, "page/dashboard.tmpl", utils.TemplateData(c, gin.H{
		"title":         "Dashboard - Weather",
		"user":          user,
		"locations":     locations,
		"unreadCount":   unreadCount,
		"locationCount": len(locations),
		"page":          "dashboard",
	}))
}

// ShowAdminPanel renders the admin panel
func (h *DashboardHandler) ShowAdminPanel(c *gin.Context) {
	adminIDValue, exists := c.Get("admin_id")
	if !exists {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	adminID, ok := adminIDValue.(int)
	if !ok {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		c.Redirect(http.StatusFound, "/server/admin")
		return
	}

	// Get system statistics
	userModel := &models.UserModel{DB: h.DB}

	totalUsers, _ := userModel.Count()
	adminCount, _ := userModel.CountByRole("admin")

	// Count total locations across all users
	var totalLocations int
	h.DB.QueryRow("SELECT COUNT(*) FROM user_saved_locations").Scan(&totalLocations)

	c.HTML(http.StatusOK, "admin/admin.tmpl", utils.TemplateData(c, gin.H{
		"title":          "Admin Panel - Weather",
		"user":           admin,
		"totalUsers":     totalUsers,
		"adminCount":     adminCount,
		"totalLocations": totalLocations,
		"page":           "admin",
	}))
}
