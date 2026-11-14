package graphql

import (
	"testing"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// --- CreateSavedLocation -----------------------------------------------

// --- ToggleLocationAlerts -----------------------------------------------

// --- MarkNotificationRead -------------------------------------------------

func seedGraphQLNotification(t *testing.T, ddb *database.DualDB, userID int64) *models.Notification {
	t.Helper()
	notification, err := (&models.UserNotificationModel{DB: ddb.Users}).CreateUserNotification(
		int(userID),
		models.NotificationTypeInfo,
		models.NotificationDisplayToast,
		"Test title",
		"Test message",
		nil,
	)
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return notification
}

// --- UpdateUserSettings ----------------------------------------------------

// --- UpdateSavedLocation ----------------------------------------------------

// --- DeleteSavedLocation ----------------------------------------------------

// --- MarkAllNotificationsRead -----------------------------------------------

// --- DeleteNotification ------------------------------------------------------

// --- UpdateUserProfile --------------------------------------------------------

// --- ChangeUserPassword -------------------------------------------------------
