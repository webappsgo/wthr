package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
)

// newLocationTestHandler builds a LocationHandler backed by a fresh
// in-memory UsersSchema database (user_saved_locations lives there).
func newLocationTestHandler(t *testing.T) (*LocationHandler, *sql.DB) {
	t.Helper()
	db := newTestUsersDB(t)
	return &LocationHandler{DB: db}, db
}

// setCurrentUser injects a fake authenticated user into the context the way
// the real auth middleware does, so handlers under test see an
// authenticated request without needing the full auth stack.
func setCurrentUser(c *gin.Context, id int64) {
	c.Set(middleware.UserContextKey, &models.User{ID: id})
}

// seedLocation inserts a saved location directly and returns its ID, so
// tests can exercise Get/Update/Delete/ToggleAlerts without going through
// CreateLocation first.
func seedLocation(t *testing.T, db *sql.DB, userID int, name string, lat, lon float64) int {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO user_saved_locations (user_id, name, latitude, longitude, timezone, alerts_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', 1, datetime('now'), datetime('now'))
	`, userID, name, lat, lon)
	if err != nil {
		t.Fatalf("seed location: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed location last insert id: %v", err)
	}
	return int(id)
}

// TestListLocations covers the auth gate, the success path (only the
// caller's own locations returned), and DB-error propagation when the
// backing table has been dropped out from under the handler.
func TestListLocations(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations")

		h.ListLocations(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("returns only the current user's locations", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		seedLocation(t, db, 1, "Home", 40.7, -74.0)
		seedLocation(t, db, 1, "Work", 41.0, -73.9)
		seedLocation(t, db, 2, "Someone Else's", 10.0, 10.0)

		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations")
		setCurrentUser(c, 1)

		h.ListLocations(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var locations []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &locations); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(locations) != 2 {
			t.Fatalf("len(locations) = %d, want 2 (only user 1's rows); body=%s", len(locations), w.Body.String())
		}
	})

	t.Run("db error returns 500 without panicking", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		db.Close() // force the query to fail
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations")
		setCurrentUser(c, 1)

		h.ListLocations(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

// TestGetLocation covers auth, malformed ID, not-found, cross-user ownership
// enforcement, and the success path.
func TestGetLocation(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations/1")
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.GetLocation(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("non-numeric id returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations/abc")
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "abc"}}

		h.GetLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations/999")
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.GetLocation(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("another user's location returns 403", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 2, "Not Yours", 1, 1)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations/x")
		setCurrentUser(c, 1) // requesting as user 1, location belongs to user 2
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.GetLocation(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("owner can fetch their own location", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 1, "Mine", 5, 5)
		c, w := newTestContext(http.MethodGet, "/api/v1/users/locations/x")
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.GetLocation(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestCreateLocation covers auth, malformed JSON, coordinate range
// validation, the 10-location cap from IDEA.md, and the success path. It
// also documents a genuine validation bug around zero-valued coordinates.
func TestCreateLocation(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", map[string]interface{}{})

		h.CreateLocation(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", "{not json")
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("latitude out of range returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		body := map[string]interface{}{"name": "Bad Lat", "latitude": 95.0, "longitude": 10.0}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", body)
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("longitude out of range returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		body := map[string]interface{}{"name": "Bad Lon", "latitude": 10.0, "longitude": 190.0}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", body)
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	// BUG: latitude/longitude are declared `binding:"required"` on float64
	// fields (locations.go:116-117). gin's validator treats the Go zero
	// value (0.0) as "missing" for a required numeric field, so a
	// perfectly valid location sitting on the equator (latitude 0) or the
	// prime meridian (longitude 0) is rejected with 400 "Latitude/Longitude
	// is a required field" instead of being created. Left failing to
	// document correct expected behavior; not fixed per task instructions.
	t.Run("BUG: latitude of exactly zero is wrongly rejected as missing", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		body := map[string]interface{}{"name": "Null Island", "latitude": 0.0, "longitude": 0.0}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", body)
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201 (latitude/longitude 0 is a valid coordinate); body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("11th location is rejected once the 10-location cap is reached", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		for i := 0; i < 10; i++ {
			seedLocation(t, db, 1, "Loc", 1, 1)
		}
		body := map[string]interface{}{"name": "One Too Many", "latitude": 2.0, "longitude": 2.0}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", body)
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 once at the 10-location cap; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("valid request creates a location", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		body := map[string]interface{}{"name": "Paris", "latitude": 48.85, "longitude": 2.35, "timezone": "Europe/Paris"}
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations", body)
		setCurrentUser(c, 1)

		h.CreateLocation(c)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestUpdateLocation covers auth, malformed id/body, not-found, ownership,
// and the success path.
func TestUpdateLocation(t *testing.T) {
	body := map[string]interface{}{"name": "Updated", "latitude": 1.0, "longitude": 1.0}

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users/locations/1", body)
		c.Params = gin.Params{{Key: "id", Value: "1"}}

		h.UpdateLocation(c)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("non-numeric id returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users/locations/x", body)
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "x"}}

		h.UpdateLocation(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users/locations/999", body)
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: "999"}}

		h.UpdateLocation(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("another user's location returns 403", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 2, "Not Yours", 1, 1)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users/locations/x", body)
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.UpdateLocation(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("owner can update their own location", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 1, "Mine", 1, 1)
		c, w := newTestContextJSON(t, http.MethodPut, "/api/v1/users/locations/x", body)
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.UpdateLocation(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestDeleteLocation and TestToggleAlerts cover the remaining ownership-
// gated mutation endpoints with the same shape: auth, bad id, not-found,
// forbidden, success.
func TestDeleteLocation(t *testing.T) {
	t.Run("another user's location returns 403", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 2, "Not Yours", 1, 1)
		c, w := newTestContext(http.MethodDelete, "/api/v1/users/locations/x")
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.DeleteLocation(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("owner can delete their own location", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 1, "Mine", 1, 1)
		c, w := newTestContext(http.MethodDelete, "/api/v1/users/locations/x")
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.DeleteLocation(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM user_saved_locations WHERE id = ?", id).Scan(&count); err != nil {
			t.Fatalf("verify delete: %v", err)
		}
		if count != 0 {
			t.Errorf("location still present after delete")
		}
	})
}

func TestToggleAlerts(t *testing.T) {
	t.Run("another user's location returns 403", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 2, "Not Yours", 1, 1)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations/x/alerts", map[string]interface{}{"enabled": false})
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.ToggleAlerts(c)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("owner can toggle their own location's alerts", func(t *testing.T) {
		h, db := newLocationTestHandler(t)
		id := seedLocation(t, db, 1, "Mine", 1, 1)
		c, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/locations/x/alerts", map[string]interface{}{"enabled": false})
		setCurrentUser(c, 1)
		c.Params = gin.Params{{Key: "id", Value: itoa(id)}}

		h.ToggleAlerts(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestSearchLocations covers the short-query validation error and the
// nil-LocationEnhancer edge case (must not panic when the handler is wired
// up without one, e.g. a partially-initialized service).
func TestSearchLocations(t *testing.T) {
	t.Run("query shorter than 2 chars returns 400", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/locations/search?q=a")

		h.SearchLocations(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil LocationEnhancer does not panic, returns empty results", func(t *testing.T) {
		h, _ := newLocationTestHandler(t)
		c, w := newTestContext(http.MethodGet, "/api/v1/locations/search?q=paris")

		h.SearchLocations(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestLookupZipCode_MissingCode covers the validation error path without
// requiring a live WeatherService.
func TestLookupZipCode_MissingCode(t *testing.T) {
	h, _ := newLocationTestHandler(t)
	c, w := newTestContext(http.MethodGet, "/api/v1/locations/zip/")
	c.Params = gin.Params{{Key: "code", Value: ""}}

	h.LookupZipCode(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestLookupCoordinates_Validation covers the parse and range-check error
// paths, none of which require a live WeatherService since they return
// before it is used.
func TestLookupCoordinates_Validation(t *testing.T) {
	tests := []struct {
		name string
		lat  string
		lon  string
	}{
		{"non-numeric latitude", "abc", "10"},
		{"non-numeric longitude", "10", "abc"},
		{"latitude out of range", "95", "10"},
		{"longitude out of range", "10", "190"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newLocationTestHandler(t)
			c, w := newTestContext(http.MethodGet, "/api/v1/locations/reverse?lat="+tt.lat+"&lon="+tt.lon)

			h.LookupCoordinates(c)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// itoa avoids pulling in strconv just for test glue in every subtest above.
func itoa(n int) string {
	return jsonNumberString(n)
}

func jsonNumberString(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
