package model

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// SavedLocation represents a user's saved location
type SavedLocation struct {
	ID            int       `json:"id"`
	UserID        int       `json:"user_id"`
	Name          string    `json:"name"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	Timezone      string    `json:"timezone,omitempty"`
	AlertsEnabled bool      `json:"alerts_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LocationModel handles saved location database operations
type LocationModel struct {
	DB *sql.DB
}

// Create creates a new saved location
func (m *LocationModel) Create(userID int, name string, latitude, longitude float64, timezone string) (*SavedLocation, error) {
	result, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		INSERT INTO user_saved_locations (user_id, name, latitude, longitude, timezone, alerts_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, name, latitude, longitude, timezone, true, time.Now(), time.Now())

	if err != nil {
		return nil, fmt.Errorf("failed to create location: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return m.GetByID(int(id))
}

// GetByID retrieves a location by ID
func (m *LocationModel) GetByID(id int) (*SavedLocation, error) {
	location := &SavedLocation{}
	var timezone sql.NullString

	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, name, latitude, longitude, timezone, alerts_enabled, created_at, updated_at
		FROM user_saved_locations WHERE id = ?
	`, id).Scan(&location.ID, &location.UserID, &location.Name, &location.Latitude,
		&location.Longitude, &timezone, &location.AlertsEnabled, &location.CreatedAt, &location.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("location not found")
	}
	if err != nil {
		return nil, err
	}

	if timezone.Valid {
		location.Timezone = timezone.String
	}

	return location, nil
}

// GetByUserID retrieves all locations for a user
func (m *LocationModel) GetByUserID(userID int) ([]*SavedLocation, error) {
	rows, err := database.QueryContext(context.Background(), m.DB, database.TimeoutSimpleSelect, `
		SELECT id, user_id, name, latitude, longitude, timezone, alerts_enabled, created_at, updated_at
		FROM user_saved_locations WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []*SavedLocation
	for rows.Next() {
		location := &SavedLocation{}
		var timezone sql.NullString
		err := rows.Scan(&location.ID, &location.UserID, &location.Name, &location.Latitude,
			&location.Longitude, &timezone, &location.AlertsEnabled, &location.CreatedAt, &location.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if timezone.Valid {
			location.Timezone = timezone.String
		}
		locations = append(locations, location)
	}

	return locations, nil
}

// Update updates a location
func (m *LocationModel) Update(id int, name string, latitude, longitude float64, timezone string, alertsEnabled bool) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		UPDATE user_saved_locations
		SET name = ?, latitude = ?, longitude = ?, timezone = ?, alerts_enabled = ?, updated_at = ?
		WHERE id = ?
	`, name, latitude, longitude, timezone, alertsEnabled, time.Now(), id)
	return err
}

// ToggleAlerts toggles alerts for a location
func (m *LocationModel) ToggleAlerts(id int, enabled bool) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, `
		UPDATE user_saved_locations SET alerts_enabled = ?, updated_at = ?
		WHERE id = ?
	`, enabled, time.Now(), id)
	return err
}

// Delete deletes a location
func (m *LocationModel) Delete(id int) error {
	_, err := database.ExecContext(context.Background(), m.DB, database.TimeoutWrite, "DELETE FROM user_saved_locations WHERE id = ?", id)
	return err
}

// Count returns the number of locations for a user
func (m *LocationModel) Count(userID int) (int, error) {
	var count int
	err := database.QueryRowContext(context.Background(), m.DB, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM user_saved_locations WHERE user_id = ?", userID).Scan(&count)
	return count, err
}
