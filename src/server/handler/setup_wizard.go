// Package handlers provides setup wizard per TEMPLATE.md PART 22
package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/apimgr/weather/src/database"
	"github.com/apimgr/weather/src/path"
	"github.com/apimgr/weather/src/server/model"
	"github.com/apimgr/weather/src/util"
)

// SetupWizardRequest represents the initial setup request
// Per TEMPLATE.md PART 22: First-run admin account creation
// AI.md PART 17: Requires setup token for security
type SetupWizardRequest struct {
	SetupToken      string `json:"setup_token"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// SetupWizardResponse represents the setup response
// AI.md PART 14: Use "ok" instead of "success" for API responses
type SetupWizardResponse struct {
	Ok        bool          `json:"ok"`
	Message   string        `json:"message,omitempty"`
	Error     string        `json:"error,omitempty"`
	Admin     *models.Admin `json:"admin,omitempty"`
	SetupDone bool          `json:"setup_done"`
}

// SetupStatusHandler checks if setup is required
// Per TEMPLATE.md PART 22: Check if any admins exist
func SetupStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	count, err := adminModel.GetCount()
	if err != nil {
		log.Printf("[ERROR] " + "Failed to count admins: %v", err)
		respondJSON(w, http.StatusInternalServerError, SetupWizardResponse{
			Ok:      false,
			Error:   "SETUP_CHECK_FAILED",
			Message: "Failed to check setup status",
		})
		return
	}

	respondJSON(w, http.StatusOK, SetupWizardResponse{
		Ok:        true,
		SetupDone: count > 0,
	})
}

// SetupWizardHandler handles the initial admin account setup
// Per TEMPLATE.md PART 22: First-run admin creation with Argon2id
func SetupWizardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if setup is already done
	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	count, err := adminModel.GetCount()
	if err != nil {
		log.Printf("[ERROR] " + "Failed to count admins: %v", err)
		respondJSON(w, http.StatusInternalServerError, SetupWizardResponse{
			Ok:      false,
			Error:   "SETUP_CHECK_FAILED",
			Message: "Failed to check setup status",
		})
		return
	}

	if count > 0 {
		respondJSON(w, http.StatusForbidden, SetupWizardResponse{
			Ok:      false,
			Error:   "SETUP_COMPLETED",
			Message: "Setup already completed",
		})
		return
	}

	// Parse request
	var req SetupWizardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] " + "Failed to decode setup request: %v", err)
		respondJSON(w, http.StatusBadRequest, SetupWizardResponse{
			Ok:      false,
			Error:   "INVALID_REQUEST",
			Message: "Invalid request format",
		})
		return
	}

	// AI.md PART 17: Validate setup token before allowing admin creation
	if err := validateSetupToken(req.SetupToken); err != nil {
		log.Printf("[ERROR] " + "Invalid setup token: %v", err)
		respondJSON(w, http.StatusUnauthorized, SetupWizardResponse{
			Ok:      false,
			Error:   "INVALID_SETUP_TOKEN",
			Message: "Invalid or missing setup token. Check the server console for the token.",
		})
		return
	}

	// Validate input
	if err := validateSetupRequest(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, SetupWizardResponse{
			Ok:      false,
			Error:   "VALIDATION_FAILED",
			Message: err.Error(),
		})
		return
	}

	// Create first admin account (always a super admin)
	// Per TEMPLATE.md PART 0: Uses Argon2id for password hashing
	admin, err := adminModel.Create(req.Username, req.Email, req.Password, true)
	if err != nil {
		log.Printf("[ERROR] " + "Failed to create admin: %v", err)
		respondJSON(w, http.StatusInternalServerError, SetupWizardResponse{
			Ok:      false,
			Error:   "ADMIN_CREATE_FAILED",
			Message: "Failed to create admin account",
		})
		return
	}

	// Update setup state in database
	if err := markSetupComplete(); err != nil {
		log.Printf("[ERROR] " + "Failed to mark setup complete: %v", err)
	}

	// AI.md PART 17: Delete setup token after successful admin creation (one-time use)
	if err := deleteSetupToken(); err != nil {
		log.Printf("[ERROR] " + "Failed to delete setup token: %v", err)
	}

	log.Printf("[INFO] " + "Initial setup completed: Admin created: %s (ID: %d)", admin.Username, admin.ID)

	// Don't send password hash or API token prefix
	admin.PasswordHash = ""
	admin.APITokenPrefix = ""

	respondJSON(w, http.StatusOK, SetupWizardResponse{
		Ok:        true,
		Message:   "Setup completed successfully",
		Admin:     admin,
		SetupDone: true,
	})
}

// validateSetupRequest validates the setup request
func validateSetupRequest(req *SetupWizardRequest) error {
	// Validate username
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}

	if len(req.Username) < 3 {
		return fmt.Errorf("username must be at least 3 characters long")
	}

	if len(req.Username) > 32 {
		return fmt.Errorf("username must be at most 32 characters long")
	}

	// Username must be alphanumeric with optional underscores/hyphens
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !usernameRegex.MatchString(req.Username) {
		return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	// Validate email
	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		return fmt.Errorf("invalid email format")
	}

	// Validate password
	if req.Password == "" {
		return fmt.Errorf("password is required")
	}

	// Passwords cannot start or end with whitespace
	if req.Password != strings.TrimSpace(req.Password) {
		return fmt.Errorf("password cannot start or end with whitespace")
	}

	if len(req.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	if len(req.Password) > 128 {
		return fmt.Errorf("password must be at most 128 characters long")
	}

	// Check password complexity
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(req.Password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(req.Password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(req.Password)

	if !hasUpper || !hasLower || !hasNumber {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, and one number")
	}

	// Validate password confirmation
	if req.Password != req.ConfirmPassword {
		return fmt.Errorf("passwords do not match")
	}

	return nil
}

// validateSetupToken validates the setup token against the stored hash file
// AI.md: Setup token stored as SHA-256 hash in {config_dir}/setup_token.txt
func validateSetupToken(token string) error {
	if token == "" {
		return fmt.Errorf("setup token is required")
	}

	// Trim whitespace from token
	token = strings.TrimSpace(token)

	// Get config directory
	configDir := paths.GetConfigDir()

	// Validate against stored hash in file
	valid, err := utils.ValidateSetupToken(configDir, token)
	if err != nil {
		return fmt.Errorf("setup token not found or already used")
	}

	if !valid {
		return fmt.Errorf("invalid setup token")
	}

	return nil
}

// deleteSetupToken removes the setup token file after successful admin creation
// AI.md: File deleted after successful setup completion
func deleteSetupToken() error {
	configDir := paths.GetConfigDir()
	return utils.DeleteSetupToken(configDir)
}

// markSetupComplete updates the server_setup_state table
func markSetupComplete() error {
	_, err := database.GetServerDB().Exec(`
		INSERT OR REPLACE INTO server_setup_state (id, setup_completed, setup_completed_at)
		VALUES (1, 1, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		return fmt.Errorf("failed to mark setup complete: %w", err)
	}
	return nil
}

// IsSetupComplete checks if the initial setup has been completed
// Per TEMPLATE.md PART 22: Required for setup wizard flow
func IsSetupComplete() (bool, error) {
	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	count, err := adminModel.GetCount()
	if err != nil {
		return false, fmt.Errorf("failed to count admins: %w", err)
	}
	return count > 0, nil
}

// SetupRequiredMiddleware redirects to setup wizard if setup is not complete
// Per TEMPLATE.md PART 22: Force setup on first run
func SetupRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip setup check for setup endpoints
		if r.URL.Path == "/setup" || r.URL.Path == "/setup/status" || r.URL.Path == "/api/setup" || r.URL.Path == "/api/setup/status" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if setup is complete
		setupComplete, err := IsSetupComplete()
		if err != nil {
			log.Printf("[ERROR] " + "Failed to check setup status: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// If setup is not complete, redirect to setup wizard
		if !setupComplete {
			// For API requests, return JSON error (AI.md PART 14: use "ok" not "success")
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				respondJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
					"ok":             false,
					"error":          "SETUP_REQUIRED",
					"message":        "Setup required",
					"setup_required": true,
				})
				return
			}

			// For web requests, redirect to setup page
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}

		// Setup is complete, continue
		next.ServeHTTP(w, r)
	})
}
