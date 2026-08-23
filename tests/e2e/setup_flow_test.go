package e2e_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/handler"
	_ "modernc.org/sqlite"
)

// initTestDualDB creates in-memory dual databases for testing
func initTestDualDB(t *testing.T) (*database.DualDB, func()) {
	// Create in-memory server database with shared cache mode
	// Using file::memory:?cache=shared ensures all connections share the same in-memory database
	// This is required because sql.DB uses connection pooling, and with plain :memory:
	// each connection would get its own separate database
	serverDB, err := sql.Open("sqlite", "file:e2etest_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open server database: %v", err)
	}
	if _, err := serverDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if _, err := serverDB.Exec(database.ServerSchema); err != nil {
		t.Fatalf("Failed to create server schema: %v", err)
	}

	// Create in-memory users database with shared cache mode
	usersDB, err := sql.Open("sqlite", "file:e2etest_users?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open users database: %v", err)
	}
	if _, err := usersDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if _, err := usersDB.Exec(database.UsersSchema); err != nil {
		t.Fatalf("Failed to create users schema: %v", err)
	}

	dualDB := &database.DualDB{
		Server: serverDB,
		Users:  usersDB,
	}

	// Set global dual database
	database.SetGlobalDualDB(dualDB)

	cleanup := func() {
		database.SetGlobalDualDB(nil)
		serverDB.Close()
		usersDB.Close()
	}

	return dualDB, cleanup
}

// TestCompleteSetupFlow tests the admin setup wizard account step end-to-end.
func TestCompleteSetupFlow(t *testing.T) {
	// Initialize fresh database
	dualDB, cleanup := initTestDualDB(t)
	defer cleanup()

	// Create router
	r := chi.NewRouter()
	setupHandler := &handler.SetupHandler{DB: dualDB.Server}

	// Setup route for the admin account step
	r.Post("/admin/server/setup", setupHandler.CreateAdmin)

	// Step 1: Create administrator
	t.Run("1. Create administrator", func(t *testing.T) {
		payload := map[string]string{
			"username":         "administrator",
			"email":            "admin@example.com",
			"password":         "AdminPass12345",
			"confirm_password": "AdminPass12345",
		}
		jsonData, _ := json.Marshal(payload)

		req := httptest.NewRequest("POST", "/admin/server/setup", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.AddCookie(&http.Cookie{Name: "setup_token_verified", Value: "true"})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// Accept 200 or 201 for successful creation
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("Failed to create administrator: %s", w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Log response for debugging
		t.Logf("Admin creation response: %v", response)

		// Check redirect URL if present
		if redirect, ok := response["redirect"].(string); ok && redirect != "" {
			t.Logf("Redirect URL: %s", redirect)
		}
	})

	// Step 2: Verify admin was created
	t.Run("2. Verify admin in database", func(t *testing.T) {
		var adminCount int
		err := dualDB.Server.QueryRow("SELECT COUNT(*) FROM server_admin_credentials").Scan(&adminCount)
		if err != nil {
			t.Fatalf("Failed to query admins: %v", err)
		}

		if adminCount != 1 {
			t.Errorf("Expected 1 admin, got %d", adminCount)
		}
	})
}

// TestAdminSetupValidation tests validation in admin creation
func TestAdminSetupValidation(t *testing.T) {
	dualDB, cleanup := initTestDualDB(t)
	defer cleanup()

	r := chi.NewRouter()
	setupHandler := &handler.SetupHandler{DB: dualDB.Server}
	r.Post("/admin/server/setup", setupHandler.CreateAdmin)

	tests := []struct {
		name       string
		payload    map[string]string
		wantStatus int
	}{
		{
			name: "Valid admin",
			payload: map[string]string{
				"username":         "myadmin",
				"email":            "admin@test.com",
				"password":         "AdminPass12345",
				"confirm_password": "AdminPass12345",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Password too short",
			payload: map[string]string{
				"username":         "admin2",
				"email":            "admin2@test.com",
				"password":         "Short1",
				"confirm_password": "Short1",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Passwords don't match",
			payload: map[string]string{
				"username":         "admin3",
				"email":            "admin3@test.com",
				"password":         "AdminPass12345",
				"confirm_password": "DifferentPass12345",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Missing email",
			payload: map[string]string{
				"username":         "admin4",
				"password":         "AdminPass12345",
				"confirm_password": "AdminPass12345",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid email",
			payload: map[string]string{
				"username":         "admin5",
				"email":            "not-an-email",
				"password":         "AdminPass12345",
				"confirm_password": "AdminPass12345",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/admin/server/setup", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			req.AddCookie(&http.Cookie{Name: "setup_token_verified", Value: "true"})
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			// For successful creation, accept 200 or 201
			if tt.wantStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusCreated {
					t.Errorf("Expected status 200 or 201, got %d. Body: %s", w.Code, w.Body.String())
				}
			} else if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}
