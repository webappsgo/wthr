// Package handlers provides HTTP handlers per TEMPLATE.md PART 22
package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/casapps/wthr/src/database"
	"github.com/casapps/wthr/src/server/model"
)

const (
	// AdminSessionDuration is the default session duration (24 hours)
	AdminSessionDuration = 24 * time.Hour
	// AdminSessionCookieName is the cookie name for admin sessions
	AdminSessionCookieName = "admin_session"
)

// AdminLoginRequest represents a login request
type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Extend session to 30 days
	Remember bool   `json:"remember"`
}

// AdminLoginResponse represents a login response
// AI.md PART 14: Use "ok" instead of "success" for API responses
type AdminLoginResponse struct {
	Ok      bool          `json:"ok"`
	Message string        `json:"message,omitempty"`
	Admin   *models.Admin `json:"admin,omitempty"`
}

// AdminLoginHandler handles admin login
// Per TEMPLATE.md PART 22: Argon2id password verification required
func AdminLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AdminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] " + "Failed to decode login request: %v", err)
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "Invalid request format",
		})
		return
	}

	// Validate input
	if req.Username == "" || req.Password == "" {
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "Username and password are required",
		})
		return
	}

	// Get admin model
	adminModel := &models.AdminModel{DB: database.GetServerDB()}

	// Verify credentials using Argon2id (TEMPLATE.md PART 0)
	admin, err := adminModel.VerifyCredentials(req.Username, req.Password)
	if err != nil {
		log.Printf("[WARN] " + "Failed login attempt for user: %s from IP: %s", req.Username, r.RemoteAddr)
		respondJSON(w, http.StatusUnauthorized, AdminLoginResponse{
			Ok: false,
			Message: "Invalid credentials",
		})
		return
	}

	// Create session
	sessionModel := &models.AdminSessionModel{DB: database.GetServerDB()}

	duration := AdminSessionDuration
	if req.Remember {
		// 30 days
		duration = 30 * 24 * time.Hour
	}

	session, err := sessionModel.CreateSession(
		admin.ID,
		r.RemoteAddr,
		r.UserAgent(),
		duration,
	)
	if err != nil {
		log.Printf("[ERROR] " + "Failed to create session: %v", err)
		respondJSON(w, http.StatusInternalServerError, AdminLoginResponse{
			Ok: false,
			Message: "Failed to create session",
		})
		return
	}

	// Update last login timestamp
	if err := adminModel.UpdateLastLogin(admin.ID); err != nil {
		log.Printf("[ERROR] " + "Failed to update last login: %v", err)
	}

	// Set secure cookie
	http.SetCookie(w, &http.Cookie{
		Name:     AdminSessionCookieName,
		Value:    session.SessionID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		// Only secure in HTTPS
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("[INFO] " + "Admin logged in: %s (ID: %d) from %s", admin.Username, admin.ID, r.RemoteAddr)

	// Don't send password hash or API token in response
	admin.PasswordHash = ""
	admin.APITokenPrefix = ""

	respondJSON(w, http.StatusOK, AdminLoginResponse{
		Ok: true,
		Message: "Login successful",
		Admin:   admin,
	})
}

// AdminLogoutHandler handles admin logout
// Per TEMPLATE.md PART 22: Session management required
func AdminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session cookie
	cookie, err := r.Cookie(AdminSessionCookieName)
	if err != nil {
		respondJSON(w, http.StatusOK, AdminLoginResponse{
			Ok: true,
			Message: "Already logged out",
		})
		return
	}

	// Delete session from database
	sessionModel := &models.AdminSessionModel{DB: database.GetServerDB()}
	if err := sessionModel.DeleteSession(cookie.Value); err != nil {
		log.Printf("[ERROR] " + "Failed to delete session: %v", err)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     AdminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	respondJSON(w, http.StatusOK, AdminLoginResponse{
		Ok: true,
		Message: "Logout successful",
	})
}

// AdminLogoutAllHandler logs out all sessions for the current admin
func AdminLogoutAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current admin from context (set by auth middleware)
	admin, ok := r.Context().Value("admin").(*models.Admin)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Delete all sessions for this admin
	sessionModel := &models.AdminSessionModel{DB: database.GetServerDB()}
	if err := sessionModel.DeleteAllSessionsForAdmin(admin.ID); err != nil {
		log.Printf("[ERROR] " + "Failed to delete all sessions: %v", err)
		respondJSON(w, http.StatusInternalServerError, AdminLoginResponse{
			Ok: false,
			Message: "Failed to logout all sessions",
		})
		return
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     AdminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("[INFO] " + "Admin logged out of all sessions: %s (ID: %d)", admin.Username, admin.ID)

	respondJSON(w, http.StatusOK, AdminLoginResponse{
		Ok: true,
		Message: "Logged out of all sessions",
	})
}

// AdminMeHandler returns the current admin's information
func AdminMeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current admin from context (set by auth middleware)
	admin, ok := r.Context().Value("admin").(*models.Admin)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Don't send password hash or API token
	admin.PasswordHash = ""
	admin.APITokenPrefix = ""

	respondJSON(w, http.StatusOK, AdminLoginResponse{
		Ok: true,
		Admin:   admin,
	})
}

// AdminSessionsHandler lists all active sessions for the current admin
func AdminSessionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current admin from context
	admin, ok := r.Context().Value("admin").(*models.Admin)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionModel := &models.AdminSessionModel{DB: database.GetServerDB()}
	sessions, err := sessionModel.GetActiveSessions(admin.ID)
	if err != nil {
		log.Printf("[ERROR] " + "Failed to get sessions: %v", err)
		http.Error(w, "Failed to get sessions", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"sessions": sessions,
	})
}

// AdminChangePasswordRequest represents a password change request
type AdminChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

// AdminChangePasswordHandler handles admin password changes
// Per TEMPLATE.md PART 0: Uses Argon2id for password hashing
func AdminChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current admin from context
	admin, ok := r.Context().Value("admin").(*models.Admin)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req AdminChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "Invalid request format",
		})
		return
	}

	// Validate input
	if req.CurrentPassword == "" || req.NewPassword == "" || req.ConfirmPassword == "" {
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "All fields are required",
		})
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "Passwords do not match",
		})
		return
	}

	// Validate password strength (minimum 8 characters)
	if len(req.NewPassword) < 8 {
		respondJSON(w, http.StatusBadRequest, AdminLoginResponse{
			Ok: false,
			Message: "Password must be at least 8 characters long",
		})
		return
	}

	adminModel := &models.AdminModel{DB: database.GetServerDB()}

	// Verify current password using Argon2id
	fullAdmin, err := adminModel.GetByID(admin.ID)
	if err != nil {
		log.Printf("[ERROR] " + "Failed to get admin: %v", err)
		respondJSON(w, http.StatusInternalServerError, AdminLoginResponse{
			Ok: false,
			Message: "Failed to verify current password",
		})
		return
	}

	valid, err := models.VerifyPassword(req.CurrentPassword, fullAdmin.PasswordHash)
	if err != nil || !valid {
		respondJSON(w, http.StatusUnauthorized, AdminLoginResponse{
			Ok: false,
			Message: "Current password is incorrect",
		})
		return
	}

	// Update password using Argon2id (TEMPLATE.md PART 0)
	if err := adminModel.UpdatePassword(admin.ID, req.NewPassword); err != nil {
		log.Printf("[ERROR] " + "Failed to update password: %v", err)
		respondJSON(w, http.StatusInternalServerError, AdminLoginResponse{
			Ok: false,
			Message: "Failed to update password",
		})
		return
	}

	log.Printf("[INFO] " + "Admin password changed: %s (ID: %d)", admin.Username, admin.ID)

	respondJSON(w, http.StatusOK, AdminLoginResponse{
		Ok: true,
		Message: "Password changed successfully",
	})
}

// AdminRegenerateAPITokenHandler regenerates the API token for the current admin
func AdminRegenerateAPITokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current admin from context
	admin, ok := r.Context().Value("admin").(*models.Admin)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	newToken, err := adminModel.RegenerateAPIToken(admin.ID)
	if err != nil {
		log.Printf("[ERROR] " + "Failed to regenerate API token: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":      false,
			"message": "Failed to regenerate API token",
		})
		return
	}

	log.Printf("[INFO] " + "Admin API token regenerated: %s (ID: %d)", admin.Username, admin.ID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "API token regenerated successfully",
		"token":   newToken,
	})
}

// respondJSON is a helper function to send JSON responses
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] " + "Failed to encode JSON response: %v", err)
	}
}
