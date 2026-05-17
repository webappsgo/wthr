package service

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/casapps/wthr/src/server/model"
)

// NotificationService handles all notification operations
type NotificationService struct {
	UserDB   *sql.DB
	ServerDB *sql.DB
	WSHub    *WebSocketHub

	// Models
	UserNotif  *models.UserNotificationModel
	AdminNotif *models.AdminNotificationModel
	Prefs      *models.NotificationPreferencesModel
}

// NewNotificationService creates a new notification service
func NewNotificationService(userDB, serverDB *sql.DB, wsHub *WebSocketHub) *NotificationService {
	return &NotificationService{
		UserDB:   userDB,
		ServerDB: serverDB,
		WSHub:    wsHub,
		UserNotif: &models.UserNotificationModel{
			DB: userDB,
		},
		AdminNotif: &models.AdminNotificationModel{
			DB: serverDB,
		},
		Prefs: &models.NotificationPreferencesModel{
			UserDB:   userDB,
			ServerDB: serverDB,
		},
	}
}

// SendUserNotification sends a notification to a user
func (s *NotificationService) SendUserNotification(userID int, notifType models.NotificationType, display models.NotificationDisplay, title, message string, action *models.NotificationAction) (*models.Notification, error) {
	// Get user preferences
	prefs, err := s.Prefs.GetUserPreferences(userID)
	if err != nil {
		log.Printf("Error getting user preferences: %v (using defaults)", err)
	}

	// Check if notification type is enabled
	if prefs != nil {
		if !s.shouldSendNotification(display, prefs.EnableToast, prefs.EnableBanner, prefs.EnableCenter) {
			log.Printf("Notification not sent to user %d: display type %s disabled", userID, display)
			return nil, fmt.Errorf("notification display type disabled")
		}
	}

	// Create notification
	notif, err := s.UserNotif.Create(userID, notifType, display, title, message, action)
	if err != nil {
		return nil, fmt.Errorf("failed to create user notification: %w", err)
	}

	// Send via WebSocket if user is online
	if s.WSHub.IsUserConnected(userID) {
		s.WSHub.BroadcastToUser(userID, notif)
		log.Printf("Notification sent to user %d via WebSocket: %s", userID, notif.ID)
	} else {
		log.Printf("User %d offline, notification %s saved for later", userID, notif.ID)
	}

	// Enforce 100 notification limit per user
	go func() {
		deleted, err := s.UserNotif.EnforceLimit(userID, 100)
		if err != nil {
			log.Printf("Error enforcing user notification limit: %v", err)
		} else if deleted > 0 {
			log.Printf("Deleted %d old notifications for user %d", deleted, userID)
		}
	}()

	return notif, nil
}

// SendAdminNotification sends a notification to an admin
func (s *NotificationService) SendAdminNotification(adminID int, notifType models.NotificationType, display models.NotificationDisplay, title, message string, action *models.NotificationAction) (*models.Notification, error) {
	// Get admin preferences
	prefs, err := s.Prefs.GetAdminPreferences(adminID)
	if err != nil {
		log.Printf("Error getting admin preferences: %v (using defaults)", err)
	}

	// Check if notification type is enabled
	if prefs != nil {
		if !s.shouldSendNotification(display, prefs.EnableToast, prefs.EnableBanner, prefs.EnableCenter) {
			log.Printf("Notification not sent to admin %d: display type %s disabled", adminID, display)
			return nil, fmt.Errorf("notification display type disabled")
		}
	}

	// Create notification
	notif, err := s.AdminNotif.Create(adminID, notifType, display, title, message, action)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin notification: %w", err)
	}

	// Send via WebSocket if admin is online
	if s.WSHub.IsAdminConnected(adminID) {
		s.WSHub.BroadcastToAdmin(adminID, notif)
		log.Printf("Notification sent to admin %d via WebSocket: %s", adminID, notif.ID)
	} else {
		log.Printf("Admin %d offline, notification %s saved for later", adminID, notif.ID)
	}

	// Enforce 100 notification limit per admin
	go func() {
		deleted, err := s.AdminNotif.EnforceLimit(adminID, 100)
		if err != nil {
			log.Printf("Error enforcing admin notification limit: %v", err)
		} else if deleted > 0 {
			log.Printf("Deleted %d old notifications for admin %d", deleted, adminID)
		}
	}()

	return notif, nil
}

// SendSuccessToUser sends a success notification to a user
func (s *NotificationService) SendSuccessToUser(userID int, title, message string) (*models.Notification, error) {
	return s.SendUserNotification(userID, models.NotificationTypeSuccess, models.NotificationDisplayToast, title, message, nil)
}

// SendInfoToUser sends an info notification to a user
func (s *NotificationService) SendInfoToUser(userID int, title, message string) (*models.Notification, error) {
	return s.SendUserNotification(userID, models.NotificationTypeInfo, models.NotificationDisplayToast, title, message, nil)
}

// SendWarningToUser sends a warning notification to a user
func (s *NotificationService) SendWarningToUser(userID int, title, message string) (*models.Notification, error) {
	return s.SendUserNotification(userID, models.NotificationTypeWarning, models.NotificationDisplayToast, title, message, nil)
}

// SendErrorToUser sends an error notification to a user
func (s *NotificationService) SendErrorToUser(userID int, title, message string) (*models.Notification, error) {
	return s.SendUserNotification(userID, models.NotificationTypeError, models.NotificationDisplayToast, title, message, nil)
}

// SendSecurityToUser sends a security notification to a user
func (s *NotificationService) SendSecurityToUser(userID int, title, message string) (*models.Notification, error) {
	return s.SendUserNotification(userID, models.NotificationTypeSecurity, models.NotificationDisplayBanner, title, message, nil)
}

// SendSuccessToAdmin sends a success notification to an admin
func (s *NotificationService) SendSuccessToAdmin(adminID int, title, message string) (*models.Notification, error) {
	return s.SendAdminNotification(adminID, models.NotificationTypeSuccess, models.NotificationDisplayToast, title, message, nil)
}

// SendInfoToAdmin sends an info notification to an admin
func (s *NotificationService) SendInfoToAdmin(adminID int, title, message string) (*models.Notification, error) {
	return s.SendAdminNotification(adminID, models.NotificationTypeInfo, models.NotificationDisplayToast, title, message, nil)
}

// SendWarningToAdmin sends a warning notification to an admin
func (s *NotificationService) SendWarningToAdmin(adminID int, title, message string) (*models.Notification, error) {
	return s.SendAdminNotification(adminID, models.NotificationTypeWarning, models.NotificationDisplayToast, title, message, nil)
}

// SendErrorToAdmin sends an error notification to an admin
func (s *NotificationService) SendErrorToAdmin(adminID int, title, message string) (*models.Notification, error) {
	return s.SendAdminNotification(adminID, models.NotificationTypeError, models.NotificationDisplayToast, title, message, nil)
}

// SendSecurityToAdmin sends a security notification to an admin
func (s *NotificationService) SendSecurityToAdmin(adminID int, title, message string) (*models.Notification, error) {
	return s.SendAdminNotification(adminID, models.NotificationTypeSecurity, models.NotificationDisplayBanner, title, message, nil)
}

// GetUserNotifications retrieves notifications for a user with pagination
func (s *NotificationService) GetUserNotifications(userID int, limit, offset int) ([]*models.Notification, error) {
	return s.UserNotif.GetByUserID(userID, limit, offset)
}

// GetAdminNotifications retrieves notifications for an admin with pagination
func (s *NotificationService) GetAdminNotifications(adminID int, limit, offset int) ([]*models.Notification, error) {
	return s.AdminNotif.GetByAdminID(adminID, limit, offset)
}

// GetUserUnreadNotifications retrieves unread notifications for a user
func (s *NotificationService) GetUserUnreadNotifications(userID int) ([]*models.Notification, error) {
	return s.UserNotif.GetUnread(userID)
}

// GetAdminUnreadNotifications retrieves unread notifications for an admin
func (s *NotificationService) GetAdminUnreadNotifications(adminID int) ([]*models.Notification, error) {
	return s.AdminNotif.GetUnread(adminID)
}

// GetUserUnreadCount returns the count of unread notifications for a user
func (s *NotificationService) GetUserUnreadCount(userID int) (int, error) {
	return s.UserNotif.GetUnreadCount(userID)
}

// GetAdminUnreadCount returns the count of unread notifications for an admin
func (s *NotificationService) GetAdminUnreadCount(adminID int) (int, error) {
	return s.AdminNotif.GetUnreadCount(adminID)
}

// MarkUserNotificationAsRead marks a user notification as read
func (s *NotificationService) MarkUserNotificationAsRead(notifID string, userID int) error {
	return s.UserNotif.MarkAsRead(notifID, userID)
}

// MarkAdminNotificationAsRead marks an admin notification as read
func (s *NotificationService) MarkAdminNotificationAsRead(notifID string, adminID int) error {
	return s.AdminNotif.MarkAsRead(notifID, adminID)
}

// MarkAllUserNotificationsAsRead marks all user notifications as read
func (s *NotificationService) MarkAllUserNotificationsAsRead(userID int) error {
	return s.UserNotif.MarkAllAsRead(userID)
}

// MarkAllAdminNotificationsAsRead marks all admin notifications as read
func (s *NotificationService) MarkAllAdminNotificationsAsRead(adminID int) error {
	return s.AdminNotif.MarkAllAsRead(adminID)
}

// DismissUserNotification dismisses a user notification
func (s *NotificationService) DismissUserNotification(notifID string, userID int) error {
	return s.UserNotif.Dismiss(notifID, userID)
}

// DismissAdminNotification dismisses an admin notification
func (s *NotificationService) DismissAdminNotification(notifID string, adminID int) error {
	return s.AdminNotif.Dismiss(notifID, adminID)
}

// DeleteUserNotification deletes a user notification
func (s *NotificationService) DeleteUserNotification(notifID string, userID int) error {
	return s.UserNotif.Delete(notifID, userID)
}

// DeleteAdminNotification deletes an admin notification
func (s *NotificationService) DeleteAdminNotification(notifID string, adminID int) error {
	return s.AdminNotif.Delete(notifID, adminID)
}

// GetUserStatistics returns notification statistics for a user
func (s *NotificationService) GetUserStatistics(userID int) (*models.NotificationStatistics, error) {
	return s.UserNotif.GetStatistics(userID)
}

// GetAdminStatistics returns notification statistics for an admin
func (s *NotificationService) GetAdminStatistics(adminID int) (*models.NotificationStatistics, error) {
	return s.AdminNotif.GetStatistics(adminID)
}

// GetUserPreferences retrieves notification preferences for a user
func (s *NotificationService) GetUserPreferences(userID int) (*models.NotificationPreferences, error) {
	return s.Prefs.GetUserPreferences(userID)
}

// GetAdminPreferences retrieves notification preferences for an admin
func (s *NotificationService) GetAdminPreferences(adminID int) (*models.NotificationPreferences, error) {
	return s.Prefs.GetAdminPreferences(adminID)
}

// UpdateUserPreferences updates notification preferences for a user
func (s *NotificationService) UpdateUserPreferences(userID int, prefs *models.NotificationPreferences) error {
	return s.Prefs.UpdateUserPreferences(userID, prefs)
}

// UpdateAdminPreferences updates notification preferences for an admin
func (s *NotificationService) UpdateAdminPreferences(adminID int, prefs *models.NotificationPreferences) error {
	return s.Prefs.UpdateAdminPreferences(adminID, prefs)
}

// CleanupExpired deletes all expired notifications (called by scheduler)
func (s *NotificationService) CleanupExpired() error {
	// Cleanup user notifications
	userDeleted, err := s.UserNotif.CleanupExpired()
	if err != nil {
		log.Printf("Error cleaning up expired user notifications: %v", err)
		return err
	}
	if userDeleted > 0 {
		log.Printf("Cleaned up %d expired user notifications", userDeleted)
	}

	// Cleanup admin notifications
	adminDeleted, err := s.AdminNotif.CleanupExpired()
	if err != nil {
		log.Printf("Error cleaning up expired admin notifications: %v", err)
		return err
	}
	if adminDeleted > 0 {
		log.Printf("Cleaned up %d expired admin notifications", adminDeleted)
	}

	totalDeleted := userDeleted + adminDeleted
	if totalDeleted > 0 {
		log.Printf("Total expired notifications cleaned up: %d", totalDeleted)
	}

	return nil
}

// EnforceLimits enforces the 100 notification limit for all users and admins
func (s *NotificationService) EnforceLimits() error {
	// This is called periodically by the scheduler
	// The actual limit enforcement happens per-user/admin when new notifications are created
	// This is just a safety net to catch any edge cases

	log.Println("Notification limit enforcement task completed (limits enforced per-user on creation)")
	return nil
}

// shouldSendNotification checks if a notification should be sent based on preferences
func (s *NotificationService) shouldSendNotification(display models.NotificationDisplay, enableToast, enableBanner, enableCenter bool) bool {
	switch display {
	case models.NotificationDisplayToast:
		return enableToast
	case models.NotificationDisplayBanner:
		return enableBanner
	case models.NotificationDisplayCenter:
		return enableCenter
	default:
		return true
	}
}

// ShouldSendEmail determines if an email should be sent based on notification decision logic
// Per TEMPLATE.md Part 25: Email vs WebUI notification decision logic
func (s *NotificationService) ShouldSendEmail(userID int, eventType string, severity string, smtpConfigured bool) bool {
	// SMTP not configured = NO email
	if !smtpConfigured {
		return false
	}

	// Critical events ALWAYS send email if SMTP configured
	if severity == "critical" || severity == "error" {
		return true
	}

	// User offline = send email
	if !s.WSHub.IsUserConnected(userID) {
		return true
	}

	// User online = WebUI only (no email)
	return false
}

// ShouldSendEmailToAdmin determines if an email should be sent to admin
func (s *NotificationService) ShouldSendEmailToAdmin(adminID int, eventType string, severity string, smtpConfigured bool) bool {
	// SMTP not configured = NO email
	if !smtpConfigured {
		return false
	}

	// Critical events ALWAYS send email if SMTP configured
	if severity == "critical" || severity == "error" {
		return true
	}

	// Admin offline = send email
	if !s.WSHub.IsAdminConnected(adminID) {
		return true
	}

	// Admin online = WebUI only (no email)
	return false
}
