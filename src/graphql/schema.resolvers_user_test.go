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
