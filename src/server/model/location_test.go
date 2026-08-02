package model

import (
	"testing"
)

// TestLocationModel_CreateAndGetByID covers the happy path plus the
// not-found error path for LocationModel.Create/GetByID.
func TestLocationModel_CreateAndGetByID(t *testing.T) {
	db := newModelUsersDB(t)
	userID := insertTestUser(t, db, "loc-user", "loc-user@example.com")
	model := &LocationModel{DB: db}

	tests := []struct {
		name      string
		latitude  float64
		longitude float64
		timezone  string
	}{
		{name: "with timezone", latitude: 40.7128, longitude: -74.0060, timezone: "America/New_York"},
		{name: "empty timezone", latitude: 0, longitude: 0, timezone: ""},
		{name: "negative coordinates", latitude: -33.8688, longitude: 151.2093, timezone: "Australia/Sydney"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := model.Create(int(userID), tt.name, tt.latitude, tt.longitude, tt.timezone)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if created.ID == 0 {
				t.Error("Create() returned zero ID")
			}
			if created.Latitude != tt.latitude || created.Longitude != tt.longitude {
				t.Errorf("Create() lat/lon = %v/%v, want %v/%v", created.Latitude, created.Longitude, tt.latitude, tt.longitude)
			}
			if created.Timezone != tt.timezone {
				t.Errorf("Create() timezone = %q, want %q", created.Timezone, tt.timezone)
			}
			if !created.AlertsEnabled {
				t.Error("Create() should default AlertsEnabled to true")
			}

			got, err := model.GetByID(created.ID)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			if got.Name != tt.name {
				t.Errorf("GetByID() name = %q, want %q", got.Name, tt.name)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		if _, err := model.GetByID(999999); err == nil {
			t.Error("GetByID() expected error for missing location")
		}
	})
}

// TestLocationModel_GetByUserID covers listing (empty, single, multiple)
// and confirms results are scoped to the requesting user only.
func TestLocationModel_GetByUserID(t *testing.T) {
	db := newModelUsersDB(t)
	model := &LocationModel{DB: db}
	userA := insertTestUser(t, db, "loc-a", "loc-a@example.com")
	userB := insertTestUser(t, db, "loc-b", "loc-b@example.com")

	t.Run("empty", func(t *testing.T) {
		locs, err := model.GetByUserID(int(userA))
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}
		if len(locs) != 0 {
			t.Errorf("GetByUserID() = %d locations, want 0", len(locs))
		}
	})

	if _, err := model.Create(int(userA), "Home", 1, 1, "UTC"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := model.Create(int(userA), "Work", 2, 2, "UTC"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := model.Create(int(userB), "Other", 3, 3, "UTC"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	t.Run("scoped to user", func(t *testing.T) {
		locs, err := model.GetByUserID(int(userA))
		if err != nil {
			t.Fatalf("GetByUserID() error = %v", err)
		}
		if len(locs) != 2 {
			t.Fatalf("GetByUserID() = %d locations, want 2", len(locs))
		}
		for _, l := range locs {
			if l.UserID != int(userA) {
				t.Errorf("GetByUserID() returned location for user %d, want %d", l.UserID, userA)
			}
		}
	})
}

// TestLocationModel_Update verifies fields are actually persisted, including
// toggling AlertsEnabled off (a zero-value bool, easy to accidentally no-op).
func TestLocationModel_Update(t *testing.T) {
	db := newModelUsersDB(t)
	userID := insertTestUser(t, db, "loc-upd", "loc-upd@example.com")
	model := &LocationModel{DB: db}

	created, err := model.Create(int(userID), "Original", 1, 1, "UTC")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := model.Update(created.ID, "Updated", 9.9, 8.8, "Europe/Paris", false); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := model.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "Updated" || got.Latitude != 9.9 || got.Longitude != 8.8 || got.Timezone != "Europe/Paris" {
		t.Errorf("Update() did not persist fields, got %+v", got)
	}
	if got.AlertsEnabled {
		t.Error("Update() should have disabled alerts")
	}
}

// TestLocationModel_ToggleAlerts covers both directions of the toggle.
func TestLocationModel_ToggleAlerts(t *testing.T) {
	db := newModelUsersDB(t)
	userID := insertTestUser(t, db, "loc-toggle", "loc-toggle@example.com")
	model := &LocationModel{DB: db}

	created, err := model.Create(int(userID), "Toggle", 1, 1, "UTC")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "disable", enabled: false},
		{name: "re-enable", enabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := model.ToggleAlerts(created.ID, tt.enabled); err != nil {
				t.Fatalf("ToggleAlerts() error = %v", err)
			}
			got, err := model.GetByID(created.ID)
			if err != nil {
				t.Fatalf("GetByID() error = %v", err)
			}
			if got.AlertsEnabled != tt.enabled {
				t.Errorf("AlertsEnabled = %v, want %v", got.AlertsEnabled, tt.enabled)
			}
		})
	}
}

// TestLocationModel_DeleteAndCount covers deletion (including deleting a
// non-existent row, which must not error) and the Count aggregate.
func TestLocationModel_DeleteAndCount(t *testing.T) {
	db := newModelUsersDB(t)
	userID := insertTestUser(t, db, "loc-del", "loc-del@example.com")
	model := &LocationModel{DB: db}

	count, err := model.Count(int(userID))
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}

	first, err := model.Create(int(userID), "First", 1, 1, "UTC")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := model.Create(int(userID), "Second", 2, 2, "UTC"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	count, err = model.Count(int(userID))
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}

	if err := model.Delete(first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := model.GetByID(first.ID); err == nil {
		t.Error("GetByID() expected error after delete")
	}

	count, err = model.Count(int(userID))
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Count() after delete = %d, want 1", count)
	}

	t.Run("delete non-existent is a no-op", func(t *testing.T) {
		if err := model.Delete(999999); err != nil {
			t.Errorf("Delete() of missing row should not error, got %v", err)
		}
	})
}
