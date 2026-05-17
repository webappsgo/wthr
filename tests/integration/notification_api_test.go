package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/handler"
	models "github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/service"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func setupNotificationAPITest(t *testing.T) (*gin.Engine, *sql.DB, *sql.DB, *service.NotificationService, func()) {
	gin.SetMode(gin.TestMode)

	// Create test databases with shared cache mode
	// Using file:NAME?mode=memory&cache=shared ensures all connections share the same in-memory database
	// This is required because sql.DB uses connection pooling, and with plain :memory:
	// each connection would get its own separate database
	// We use unique names per test to ensure test isolation
	testName := t.Name()
	userDB, err := sql.Open("sqlite", "file:"+testName+"_user?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open user database: %v", err)
	}

	serverDB, err := sql.Open("sqlite", "file:"+testName+"_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open server database: %v", err)
	}

	// Create tables
	createNotificationTables(t, userDB, serverDB)

	// Note: We don't set global database here to avoid test isolation issues
	// when tests run in parallel. The handler uses the explicitly provided
	// NotificationService which has its own DB references.

	// Create WebSocket hub and notification service
	wsHub := service.NewWebSocketHub()
	go wsHub.Run()

	notificationService := &service.NotificationService{
		UserDB:     userDB,
		ServerDB:   serverDB,
		WSHub:      wsHub,
		UserNotif:  &models.UserNotificationModel{DB: userDB},
		AdminNotif: &models.AdminNotificationModel{DB: serverDB},
		Prefs:      &models.NotificationPreferencesModel{UserDB: userDB, ServerDB: serverDB},
	}

	// Create handlers
	notificationAPIHandler := &handler.NotificationAPIHandlers{
		NotificationService: notificationService,
		WSHub:               wsHub,
	}

	// Create router
	r := gin.New()

	// User notification routes
	user := r.Group("/api/v1/users/notifications")
	user.Use(mockAuthMiddleware(1, false)) // Mock user ID = 1
	{
		user.GET("", notificationAPIHandler.GetUserNotifications)
		user.GET("/unread", notificationAPIHandler.GetUserUnreadNotifications)
		user.GET("/count", notificationAPIHandler.GetUserUnreadCount)
		user.GET("/stats", notificationAPIHandler.GetUserStats)
		user.PATCH("/:id/read", notificationAPIHandler.MarkUserNotificationRead)
		user.PATCH("/read", notificationAPIHandler.MarkAllUserNotificationsRead)
		user.PATCH("/:id/dismiss", notificationAPIHandler.DismissUserNotification)
		user.DELETE("/:id", notificationAPIHandler.DeleteUserNotification)
		user.GET("/preferences", notificationAPIHandler.GetUserPreferences)
		user.PATCH("/preferences", notificationAPIHandler.UpdateUserPreferences)
	}

	// Admin notification routes
	admin := r.Group("/api/v1/admin/notifications")
	admin.Use(mockAuthMiddleware(1, true)) // Mock admin ID = 1
	{
		admin.GET("", notificationAPIHandler.GetAdminNotifications)
		admin.GET("/unread", notificationAPIHandler.GetAdminUnreadNotifications)
		admin.GET("/count", notificationAPIHandler.GetAdminUnreadCount)
		admin.GET("/stats", notificationAPIHandler.GetAdminStats)
		admin.PATCH("/:id/read", notificationAPIHandler.MarkAdminNotificationRead)
		admin.PATCH("/read", notificationAPIHandler.MarkAllAdminNotificationsRead)
		admin.PATCH("/:id/dismiss", notificationAPIHandler.DismissAdminNotification)
		admin.DELETE("/:id", notificationAPIHandler.DeleteAdminNotification)
		admin.GET("/preferences", notificationAPIHandler.GetAdminPreferences)
		admin.PATCH("/preferences", notificationAPIHandler.UpdateAdminPreferences)
		admin.POST("/send", notificationAPIHandler.SendTestNotification)
	}

	cleanup := func() {
		wsHub.Stop()
		// Note: We don't close databases here since other code might reference the global
		// The databases use in-memory mode so they'll be garbage collected
	}

	return r, userDB, serverDB, notificationService, cleanup
}

func createNotificationTables(t *testing.T, userDB, serverDB *sql.DB) {
	// Use the full schemas to ensure all required tables exist
	_, err := userDB.Exec(database.UsersSchema)
	if err != nil {
		t.Fatalf("Failed to create users schema: %v", err)
	}

	_, err = serverDB.Exec(database.ServerSchema)
	if err != nil {
		t.Fatalf("Failed to create server schema: %v", err)
	}
}

func mockAuthMiddleware(id int, isAdmin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAdmin {
			c.Set("admin_id", id)
		} else {
			c.Set("user_id", id)
		}
		c.Next()
	}
}

func TestUserNotificationAPI_GetNotifications(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notifications
	_, _ = service.SendSuccessToUser(1, "Test 1", "Message 1")
	_, _ = service.SendInfoToUser(1, "Test 2", "Message 2")

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/users/notifications", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// API returns: {"notifications": [...], "limit": N, "offset": N, "count": N}
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	notifications, ok := response["notifications"].([]interface{})
	if !ok {
		t.Fatalf("Response missing 'notifications' array. Got: %v", response)
	}

	if len(notifications) != 2 {
		t.Errorf("Notifications length = %v, want 2", len(notifications))
	}
}

func TestUserNotificationAPI_GetUnreadCount(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notifications
	notif1, _ := service.SendSuccessToUser(1, "Test 1", "Message 1")
	_, _ = service.SendInfoToUser(1, "Test 2", "Message 2")

	// Mark one as read
	_ = service.MarkUserNotificationAsRead(notif1.ID, 1)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/users/notifications/count", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// API returns: {"count": N}
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	count, ok := response["count"].(float64)
	if !ok {
		t.Fatalf("Response missing 'count' field. Got: %v", response)
	}

	if int(count) != 1 {
		t.Errorf("Unread count = %v, want 1", int(count))
	}
}

func TestUserNotificationAPI_MarkAsRead(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notification
	notif, _ := service.SendSuccessToUser(1, "Test", "Message")

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/users/notifications/"+notif.ID+"/read", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify it's marked as read
	retrieved, _ := service.UserNotif.GetByID(notif.ID)
	if !retrieved.Read {
		t.Error("Notification should be marked as read")
	}
}

func TestUserNotificationAPI_MarkAllAsRead(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notifications
	_, _ = service.SendSuccessToUser(1, "Test 1", "Message 1")
	_, _ = service.SendInfoToUser(1, "Test 2", "Message 2")

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/users/notifications/read", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify all are marked as read
	count, _ := service.GetUserUnreadCount(1)
	if count != 0 {
		t.Errorf("Unread count = %v, want 0", count)
	}
}

func TestUserNotificationAPI_DismissNotification(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notification
	notif, _ := service.SendSuccessToUser(1, "Test", "Message")

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/users/notifications/"+notif.ID+"/dismiss", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify it's dismissed
	retrieved, _ := service.UserNotif.GetByID(notif.ID)
	if !retrieved.Dismissed {
		t.Error("Notification should be dismissed")
	}
}

func TestUserNotificationAPI_DeleteNotification(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notification
	notif, _ := service.SendSuccessToUser(1, "Test", "Message")

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/users/notifications/"+notif.ID, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify it's deleted
	_, err := service.UserNotif.GetByID(notif.ID)
	if err == nil {
		t.Error("Notification should be deleted")
	}
}

func TestUserNotificationAPI_GetPreferences(t *testing.T) {
	r, _, _, _, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/users/notifications/preferences", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	var prefs models.NotificationPreferences
	err := json.Unmarshal(w.Body.Bytes(), &prefs)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Check defaults
	if !prefs.EnableToast {
		t.Error("Default EnableToast should be true")
	}
	if prefs.ToastDurationSuccess != 5 {
		t.Errorf("Default ToastDurationSuccess = %v, want 5", prefs.ToastDurationSuccess)
	}
}

func TestUserNotificationAPI_UpdatePreferences(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Prepare request body
	updateData := models.NotificationPreferences{
		EnableToast:           false,
		EnableBanner:          true,
		EnableCenter:          true,
		EnableSound:           true,
		ToastDurationSuccess:  3,
		ToastDurationInfo:     7,
		ToastDurationWarning:  15,
	}

	body, _ := json.Marshal(updateData)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/users/notifications/preferences", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	// Verify preferences were updated
	prefs, err := service.GetUserPreferences(1)
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	if prefs.EnableToast {
		t.Error("EnableToast should be false after update")
	}
	if !prefs.EnableSound {
		t.Error("EnableSound should be true after update")
	}
	if prefs.ToastDurationSuccess != 3 {
		t.Errorf("ToastDurationSuccess = %v, want 3", prefs.ToastDurationSuccess)
	}
}

func TestUserNotificationAPI_GetStatistics(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Create test notifications
	notif1, err1 := service.SendSuccessToUser(1, "Success", "Message")
	notif2, err2 := service.SendInfoToUser(1, "Info", "Message")
	notif3, err3 := service.SendWarningToUser(1, "Warning", "Message")

	// Check for creation errors
	if err1 != nil || notif1 == nil {
		t.Fatalf("Failed to create success notification: %v", err1)
	}
	if err2 != nil || notif2 == nil {
		t.Fatalf("Failed to create info notification: %v", err2)
	}
	if err3 != nil || notif3 == nil {
		t.Fatalf("Failed to create warning notification: %v", err3)
	}

	// Mark one as read
	_ = service.MarkUserNotificationAsRead(notif3.ID, 1)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/users/notifications/stats", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusOK)
	}

	var stats models.NotificationStatistics
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %v, want 3", stats.Total)
	}
	if stats.Unread != 2 {
		t.Errorf("Unread = %v, want 2", stats.Unread)
	}
	if stats.Read != 1 {
		t.Errorf("Read = %v, want 1", stats.Read)
	}
}

func TestAdminNotificationAPI_SendTestNotification(t *testing.T) {
	r, _, _, service, cleanup := setupNotificationAPITest(t)
	defer cleanup()

	// Prepare request body
	testData := map[string]interface{}{
		"type":    "success",
		"display": "toast",
		"title":   "Test Notification",
		"message": "This is a test notification",
	}

	body, _ := json.Marshal(testData)

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/notifications/send", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status code = %v, want %v", w.Code, http.StatusCreated)
		t.Logf("Response body: %s", w.Body.String())
	}

	// Verify notification was created
	count, _ := service.GetAdminUnreadCount(1)
	if count != 1 {
		t.Errorf("Admin unread count = %v, want 1", count)
	}
}
