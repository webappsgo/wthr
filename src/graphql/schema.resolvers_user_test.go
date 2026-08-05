package graphql

import (
	"context"
	"testing"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
)

// --- CreateSavedLocation -----------------------------------------------

func TestMutationResolver_CreateSavedLocation(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.CreateSavedLocation(context.Background(), "Home", 1, 1, nil, nil, nil)
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "locationowner", "locationowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("happy path defaults alerts to enabled", func(t *testing.T) {
		loc, err := m.CreateSavedLocation(userCtx, "Home", 40.7128, -74.0060, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc == nil || loc.Name != "Home" {
			t.Fatalf("loc = %+v, want Name = %q", loc, "Home")
		}
		if loc.UserID != int(user.ID) {
			t.Fatalf("UserID = %d, want %d", loc.UserID, user.ID)
		}
		if loc.Latitude != 40.7128 || loc.Longitude != -74.0060 {
			t.Fatalf("coords = (%v, %v), want (40.7128, -74.0060)", loc.Latitude, loc.Longitude)
		}
		if !loc.AlertsEnabled {
			t.Fatal("expected AlertsEnabled to default to true")
		}
	})

	t.Run("explicit alerts=false is honored", func(t *testing.T) {
		alertsOff := false
		loc, err := m.CreateSavedLocation(userCtx, "Cabin", 44.0, -110.0, nil, nil, &alertsOff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc.AlertsEnabled {
			t.Fatal("expected AlertsEnabled to be false")
		}
	})

	t.Run("location is scoped to the creating user", func(t *testing.T) {
		other := seedGraphQLUser(t, ddb, "otherlocationowner", "otherlocationowner@example.com", "correctpass1")
		otherCtx := withGraphQLUserContext(context.Background(), other)

		loc, err := m.CreateSavedLocation(otherCtx, "Office", 10, 10, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc.UserID != int(other.ID) {
			t.Fatalf("UserID = %d, want %d", loc.UserID, other.ID)
		}
		if loc.UserID == int(user.ID) {
			t.Fatal("location leaked to the wrong user")
		}
	})
}

// --- ToggleLocationAlerts -----------------------------------------------

func TestMutationResolver_ToggleLocationAlerts(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.ToggleLocationAlerts(context.Background(), "1")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "toggleowner", "toggleowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	loc, err := m.CreateSavedLocation(ownerCtx, "Home", 1, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("seed location: %v", err)
	}
	if !loc.AlertsEnabled {
		t.Fatal("expected the seeded location to start with alerts enabled")
	}

	t.Run("toggling flips alerts_enabled", func(t *testing.T) {
		toggled, err := m.ToggleLocationAlerts(ownerCtx, formatGraphQLTestUserID(int64(loc.ID)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if toggled.AlertsEnabled {
			t.Fatal("expected AlertsEnabled to flip to false")
		}

		toggledAgain, err := m.ToggleLocationAlerts(ownerCtx, formatGraphQLTestUserID(int64(loc.ID)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !toggledAgain.AlertsEnabled {
			t.Fatal("expected AlertsEnabled to flip back to true")
		}
	})

	t.Run("cross-user isolation: cannot toggle another user's location", func(t *testing.T) {
		intruder := seedGraphQLUser(t, ddb, "toggleintruder", "toggleintruder@example.com", "correctpass1")
		intruderCtx := withGraphQLUserContext(context.Background(), intruder)

		_, err := m.ToggleLocationAlerts(intruderCtx, formatGraphQLTestUserID(int64(loc.ID)))
		if err == nil {
			t.Fatal("expected an error toggling a location owned by a different user")
		}
	})
}

// --- MarkNotificationRead -------------------------------------------------

func seedGraphQLNotification(t *testing.T, ddb *database.DualDB, userID int64) *models.Notification {
	t.Helper()
	notification, err := (&models.UserNotificationModel{DB: ddb.Users}).Create(
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

func TestMutationResolver_MarkNotificationRead(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.MarkNotificationRead(context.Background(), "some-id")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "notifyowner", "notifyowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	t.Run("unknown notification id errors", func(t *testing.T) {
		_, err := m.MarkNotificationRead(ownerCtx, "does-not-exist")
		if err == nil {
			t.Fatal("expected an error marking a nonexistent notification as read")
		}
	})

	t.Run("happy path marks the notification read", func(t *testing.T) {
		notification := seedGraphQLNotification(t, ddb, owner.ID)
		if notification.Read {
			t.Fatal("expected the seeded notification to start unread")
		}

		updated, err := m.MarkNotificationRead(ownerCtx, notification.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated.Read {
			t.Fatal("expected Read = true after marking read")
		}
		if updated.ID != notification.ID {
			t.Fatalf("ID = %q, want %q", updated.ID, notification.ID)
		}
	})

	t.Run("cross-user isolation: cannot mark another user's notification read", func(t *testing.T) {
		intruder := seedGraphQLUser(t, ddb, "notifyintruder", "notifyintruder@example.com", "correctpass1")
		intruderCtx := withGraphQLUserContext(context.Background(), intruder)

		notification := seedGraphQLNotification(t, ddb, owner.ID)
		_, err := m.MarkNotificationRead(intruderCtx, notification.ID)
		if err == nil {
			t.Fatal("expected an error marking a notification owned by a different user")
		}
	})
}

// --- UpdateUserSettings ----------------------------------------------------

func TestMutationResolver_UpdateUserSettings(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.UpdateUserSettings(context.Background(), nil, nil, nil, nil)
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "settingsowner", "settingsowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("happy path with no sections is a no-op success", func(t *testing.T) {
		result, err := m.UpdateUserSettings(userCtx, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
	})

	t.Run("happy path updates account display name and location", func(t *testing.T) {
		account := &AccountSettingsInput{
			DisplayName: "New Display Name",
			Location:    "Testville",
		}

		result, err := m.UpdateUserSettings(userCtx, account, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		var displayName, location string
		err = database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
			"SELECT display_name, location FROM user_accounts WHERE id = ?", user.ID,
		).Scan(&displayName, &location)
		if err != nil {
			t.Fatalf("reload user settings: %v", err)
		}
		if displayName != "New Display Name" {
			t.Fatalf("display_name = %q, want %q", displayName, "New Display Name")
		}
		if location != "Testville" {
			t.Fatalf("location = %q, want %q", location, "Testville")
		}
	})
}

// --- UpdateSavedLocation ----------------------------------------------------

func TestMutationResolver_UpdateSavedLocation(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.UpdateSavedLocation(context.Background(), "1", nil, nil)
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "updatelocowner", "updatelocowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	loc, err := m.CreateSavedLocation(ownerCtx, "Home", 1, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locID := formatGraphQLTestUserID(int64(loc.ID))

	t.Run("unknown location id errors", func(t *testing.T) {
		_, err := m.UpdateSavedLocation(ownerCtx, "999999", nil, nil)
		if err == nil {
			t.Fatal("expected an error updating a nonexistent location")
		}
	})

	t.Run("updating only name leaves alerts untouched", func(t *testing.T) {
		newName := "Renamed Home"
		updated, err := m.UpdateSavedLocation(ownerCtx, locID, &newName, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Name != "Renamed Home" {
			t.Fatalf("Name = %q, want %q", updated.Name, "Renamed Home")
		}
		if !updated.AlertsEnabled {
			t.Fatal("expected AlertsEnabled to remain true when alerts is nil")
		}
	})

	t.Run("updating only alerts leaves name untouched", func(t *testing.T) {
		alertsOff := false
		updated, err := m.UpdateSavedLocation(ownerCtx, locID, nil, &alertsOff)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Name != "Renamed Home" {
			t.Fatalf("Name = %q, want unchanged %q", updated.Name, "Renamed Home")
		}
		if updated.AlertsEnabled {
			t.Fatal("expected AlertsEnabled = false")
		}
	})

	t.Run("cross-user isolation: cannot update another user's location", func(t *testing.T) {
		intruder := seedGraphQLUser(t, ddb, "updatelocintruder", "updatelocintruder@example.com", "correctpass1")
		intruderCtx := withGraphQLUserContext(context.Background(), intruder)

		newName := "Hijacked"
		_, err := m.UpdateSavedLocation(intruderCtx, locID, &newName, nil)
		if err == nil {
			t.Fatal("expected an error updating a location owned by a different user")
		}
	})
}

// --- DeleteSavedLocation ----------------------------------------------------

func TestMutationResolver_DeleteSavedLocation(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.DeleteSavedLocation(context.Background(), "1")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "deletelocowner", "deletelocowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	t.Run("unknown location id returns not-found response, not an error", func(t *testing.T) {
		result, err := m.DeleteSavedLocation(ownerCtx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false for an unknown location id")
		}
		if result.Message != "Location not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "Location not found")
		}
	})

	t.Run("cross-user isolation: cannot delete another user's location", func(t *testing.T) {
		loc, err := m.CreateSavedLocation(ownerCtx, "Protected", 2, 2, nil, nil, nil)
		if err != nil {
			t.Fatalf("seed location: %v", err)
		}
		intruder := seedGraphQLUser(t, ddb, "deletelocintruder", "deletelocintruder@example.com", "correctpass1")
		intruderCtx := withGraphQLUserContext(context.Background(), intruder)

		result, err := m.DeleteSavedLocation(intruderCtx, formatGraphQLTestUserID(int64(loc.ID)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false deleting a location owned by a different user")
		}
	})

	t.Run("happy path deletes the location", func(t *testing.T) {
		loc, err := m.CreateSavedLocation(ownerCtx, "ToDelete", 3, 3, nil, nil, nil)
		if err != nil {
			t.Fatalf("seed location: %v", err)
		}

		result, err := m.DeleteSavedLocation(ownerCtx, formatGraphQLTestUserID(int64(loc.ID)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		var count int
		err = database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
			"SELECT COUNT(*) FROM user_saved_locations WHERE id = ?", loc.ID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count location rows: %v", err)
		}
		if count != 0 {
			t.Fatal("expected the location row to be gone after deletion")
		}
	})
}

// --- MarkAllNotificationsRead -----------------------------------------------

func TestMutationResolver_MarkAllNotificationsRead(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.MarkAllNotificationsRead(context.Background())
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "markallowner", "markallowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	first := seedGraphQLNotification(t, ddb, owner.ID)
	second := seedGraphQLNotification(t, ddb, owner.ID)

	intruder := seedGraphQLUser(t, ddb, "markallintruder", "markallintruder@example.com", "correctpass1")
	otherNotification := seedGraphQLNotification(t, ddb, intruder.ID)

	t.Run("happy path marks only the caller's notifications read", func(t *testing.T) {
		result, err := m.MarkAllNotificationsRead(ownerCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		for _, id := range []string{first.ID, second.ID} {
			var read bool
			err := database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
				"SELECT read FROM user_notifications WHERE id = ?", id,
			).Scan(&read)
			if err != nil {
				t.Fatalf("reload notification %s: %v", id, err)
			}
			if !read {
				t.Fatalf("notification %s: read = false, want true", id)
			}
		}

		var otherRead bool
		err = database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
			"SELECT read FROM user_notifications WHERE id = ?", otherNotification.ID,
		).Scan(&otherRead)
		if err != nil {
			t.Fatalf("reload other user's notification: %v", err)
		}
		if otherRead {
			t.Fatal("expected another user's notification to remain unread")
		}
	})

	t.Run("running again with nothing unread is still a success", func(t *testing.T) {
		result, err := m.MarkAllNotificationsRead(ownerCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
	})
}

// --- DeleteNotification ------------------------------------------------------

func TestMutationResolver_DeleteNotification(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.DeleteNotification(context.Background(), "some-id")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	owner := seedGraphQLUser(t, ddb, "deletenotifyowner", "deletenotifyowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	t.Run("unknown notification id returns not-found response, not an error", func(t *testing.T) {
		result, err := m.DeleteNotification(ownerCtx, "does-not-exist")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false for an unknown notification id")
		}
		if result.Message != "Notification not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "Notification not found")
		}
	})

	t.Run("cross-user isolation: cannot delete another user's notification", func(t *testing.T) {
		notification := seedGraphQLNotification(t, ddb, owner.ID)
		intruder := seedGraphQLUser(t, ddb, "deletenotifyintruder", "deletenotifyintruder@example.com", "correctpass1")
		intruderCtx := withGraphQLUserContext(context.Background(), intruder)

		result, err := m.DeleteNotification(intruderCtx, notification.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false deleting a notification owned by a different user")
		}
	})

	t.Run("happy path deletes the notification", func(t *testing.T) {
		notification := seedGraphQLNotification(t, ddb, owner.ID)

		result, err := m.DeleteNotification(ownerCtx, notification.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		var count int
		err = database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
			"SELECT COUNT(*) FROM user_notifications WHERE id = ?", notification.ID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count notification rows: %v", err)
		}
		if count != 0 {
			t.Fatal("expected the notification row to be gone after deletion")
		}
	})
}

// --- UpdateUserProfile --------------------------------------------------------

func TestMutationResolver_UpdateUserProfile(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.UpdateUserProfile(context.Background(), nil, nil)
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "profileowner", "profileowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("happy path updates display name and phone", func(t *testing.T) {
		name := "Profile Name"
		phone := "555-0100"
		result, err := m.UpdateUserProfile(userCtx, &name, &phone)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		var displayName, storedPhone string
		err = database.QueryRowContext(context.Background(), ddb.Users, database.TimeoutSimpleSelect,
			"SELECT display_name, phone FROM user_accounts WHERE id = ?", user.ID,
		).Scan(&displayName, &storedPhone)
		if err != nil {
			t.Fatalf("reload user profile: %v", err)
		}
		if displayName != "Profile Name" {
			t.Fatalf("display_name = %q, want %q", displayName, "Profile Name")
		}
		if storedPhone != "555-0100" {
			t.Fatalf("phone = %q, want %q", storedPhone, "555-0100")
		}
	})

	t.Run("nil fields are a no-op success", func(t *testing.T) {
		result, err := m.UpdateUserProfile(userCtx, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
	})
}

// --- ChangeUserPassword -------------------------------------------------------

func TestMutationResolver_ChangeUserPassword(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unauthorized without a user in context", func(t *testing.T) {
		_, err := m.ChangeUserPassword(context.Background(), "old", "newpassword1")
		if err == nil || err.Error() != "unauthorized" {
			t.Fatalf("err = %v, want %q", err, "unauthorized")
		}
	})

	user := seedGraphQLUser(t, ddb, "passwordowner", "passwordowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("wrong current password rejected", func(t *testing.T) {
		_, err := m.ChangeUserPassword(userCtx, "totally-wrong", "newpassword1")
		if err == nil {
			t.Fatal("expected an error for an incorrect current password")
		}
	})

	t.Run("happy path changes the password", func(t *testing.T) {
		result, err := m.ChangeUserPassword(userCtx, "correctpass1", "newpassword1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		// The old password must no longer work as the "current" password.
		_, err = m.ChangeUserPassword(userCtx, "correctpass1", "anotherpassword2")
		if err == nil {
			t.Fatal("expected the old password to be rejected after it was changed")
		}

		// The new password now works as the "current" password.
		result, err = m.ChangeUserPassword(userCtx, "newpassword1", "anotherpassword2")
		if err != nil {
			t.Fatalf("unexpected error re-changing with the new password: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
	})
}
