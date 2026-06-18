package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/casapps/wthr/src/config"
	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/handler"
)

// initTestDualDB creates in-memory dual databases for testing
func initTestDualDB(t *testing.T) (*database.DualDB, func()) {
	// Create in-memory server database with shared cache mode
	// Using file::memory:?cache=shared ensures all connections share the same in-memory database
	// This is required because sql.DB uses connection pooling, and with plain :memory:
	// each connection would get its own separate database
	serverDB, err := sql.Open("sqlite", "file:authtest_server?mode=memory&cache=shared")
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
	usersDB, err := sql.Open("sqlite", "file:authtest_users?mode=memory&cache=shared")
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

	// Set a minimal global config with open registration so handler tests can
	// exercise the registration path. The project default is invite-only (per
	// IDEA.md), but unit tests need to verify the registration handler itself.
	cfg := &config.AppConfig{}
	cfg.Users.Enabled = true
	cfg.Users.Registration.Mode = "open"
	cfg.Users.Registration.RequireEmailVerification = false
	config.SetGlobalConfig(cfg)

	cleanup := func() {
		database.SetGlobalDualDB(nil)
		config.SetGlobalConfig(nil)
		serverDB.Close()
		usersDB.Close()
	}

	return dualDB, cleanup
}

func setupTestRouter(t *testing.T) (*gin.Engine, *database.DualDB, func()) {
	gin.SetMode(gin.TestMode)

	dualDB, cleanup := initTestDualDB(t)

	r := gin.New()
	return r, dualDB, cleanup
}

func TestAuthHandler_Register(t *testing.T) {
	r, dualDB, cleanup := setupTestRouter(t)
	defer cleanup()

	authHandler := &handler.AuthHandler{DB: dualDB.Users}

	r.POST("/register", authHandler.HandleRegister)

	tests := []struct {
		name       string
		payload    map[string]string
		wantStatus int
	}{
		{
			name: "Valid registration",
			payload: map[string]string{
				"username":         "testuser",
				"email":            "test@example.com",
				"password":         "ValidPass123",
				"confirm_password": "ValidPass123",
			},
			wantStatus: http.StatusCreated, // 201 for successful creation
		},
		{
			name: "Missing username",
			payload: map[string]string{
				"email":    "test@example.com",
				"password": "ValidPass123",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Missing email",
			payload: map[string]string{
				"username": "testuser2",
				"password": "ValidPass123",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Short password",
			payload: map[string]string{
				"username": "testuser3",
				"email":    "test3@example.com",
				"password": "short",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid email",
			payload: map[string]string{
				"username": "testuser4",
				"email":    "invalid-email",
				"password": "ValidPass123",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/register", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAuthHandler_Login(t *testing.T) {
	r, dualDB, cleanup := setupTestRouter(t)
	defer cleanup()

	authHandler := &handler.AuthHandler{DB: dualDB.Users}

	// Setup routes
	r.POST("/register", authHandler.HandleRegister)
	r.POST("/login", authHandler.HandleLogin)

	// First, create a user
	registerPayload := map[string]string{
		"username":         "logintest",
		"email":            "login@example.com",
		"password":         "ValidPass123",
		"confirm_password": "ValidPass123",
	}
	jsonData, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest("POST", "/register", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("Failed to create test user: %s", w.Body.String())
	}

	tests := []struct {
		name       string
		payload    map[string]string
		wantStatus int
	}{
		{
			name: "Valid login",
			payload: map[string]string{
				"identifier": "logintest",
				"password":   "ValidPass123",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Wrong password",
			payload: map[string]string{
				"identifier": "logintest",
				"password":   "WrongPassword",
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "Non-existent user",
			payload: map[string]string{
				"identifier": "nonexistent",
				"password":   "ValidPass123",
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "Missing password",
			payload: map[string]string{
				"identifier": "logintest",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}
