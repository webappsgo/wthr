// Package handler provides HTTP handlers
// Public user profile and avatar handler per AI.md PART 34
package handler

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"

	"github.com/gin-gonic/gin"
)

// UserPublicHandler handles public user profiles and avatars
type UserPublicHandler struct {
	DB *sql.DB
}

// NewUserPublicHandler creates a new UserPublicHandler
func NewUserPublicHandler(db *sql.DB) *UserPublicHandler {
	return &UserPublicHandler{DB: db}
}

// PublicUserProfile represents a public user profile response
// Per AI.md PART 34: Only public fields returned
type PublicUserProfile struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name,omitempty"`
	Avatar      AvatarInfo `json:"avatar"`
	Bio         string     `json:"bio,omitempty"`
	Location    string     `json:"location,omitempty"`
	Website     string     `json:"website,omitempty"`
	Verified    bool       `json:"verified"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AvatarInfo represents avatar URLs in different sizes
type AvatarInfo struct {
	Type string            `json:"type"`
	URLs map[string]string `json:"urls"`
}

// LoadPublicUserProfile returns the same public profile payload used by GET /api/v1/public/users/:username.
func LoadPublicUserProfile(db *sql.DB, username string, viewerUserID int64) (*PublicUserProfile, error) {
	h := NewUserPublicHandler(db)
	return h.loadPublicProfile(username, viewerUserID)
}

func (h *UserPublicHandler) loadPublicProfile(username string, viewerUserID int64) (*PublicUserProfile, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	var user struct {
		ID            int64
		Username      string
		DisplayName   sql.NullString
		Email         string
		Bio           sql.NullString
		Location      sql.NullString
		Website       sql.NullString
		Visibility    string
		AvatarType    sql.NullString
		AvatarURL     sql.NullString
		EmailVerified bool
		CreatedAt     time.Time
	}

	err := h.DB.QueryRow(`
		SELECT id, username, display_name, email, bio, location, website, visibility,
		       avatar_type, avatar_url, email_verified, created_at
		FROM user_accounts
		WHERE LOWER(username) = ? AND is_active = 1 AND is_banned = 0
	`, username).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.Email,
		&user.Bio, &user.Location, &user.Website, &user.Visibility,
		&user.AvatarType, &user.AvatarURL, &user.EmailVerified, &user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if user.Visibility == "private" && viewerUserID != user.ID {
		return nil, sql.ErrNoRows
	}

	return &PublicUserProfile{
		Username:    user.Username,
		DisplayName: user.DisplayName.String,
		Avatar:      buildAvatarInfo(user.Email, user.AvatarType, user.AvatarURL),
		Bio:         user.Bio.String,
		Location:    user.Location.String,
		Website:     user.Website.String,
		Verified:    user.EmailVerified,
		CreatedAt:   user.CreatedAt,
	}, nil
}

// GetPublicProfile returns a public user profile
// Route: GET /api/v1/public/users/:username
// Per AI.md PART 34: Private profiles return 404 (not 403) to prevent existence leakage
// @Summary Get public profile
// @Description Get the public profile of a user by username.
// @Tags User
// @Produce json
// @Param username path string true "Username"
// @Success 200 {object} map[string]interface{} "Public user profile"
// @Failure 404 {object} map[string]interface{} "User not found or profile is private"
// @Router /api/v1/public/users/{username} [get]
func (h *UserPublicHandler) GetPublicProfile(c *gin.Context) {
	username := strings.ToLower(c.Param("username"))
	var viewerUserID int64
	if currentUser, ok := middleware.GetCurrentUser(c); ok {
		viewerUserID = currentUser.ID
	}

	profile, err := h.loadPublicProfile(username, viewerUserID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// GetGravatarURL generates a Gravatar URL for an email address
// Per AI.md PART 34: Uses MD5 hash of lowercase trimmed email
func GetGravatarURL(email string, size int) string {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := md5.Sum([]byte(email))
	return fmt.Sprintf("https://www.gravatar.com/avatar/%x?s=%d&d=identicon", hash, size)
}

func buildAvatarInfo(email string, avatarType sql.NullString, avatarURL sql.NullString) AvatarInfo {
	aType := "gravatar"
	if avatarType.Valid && avatarType.String != "" {
		aType = avatarType.String
	}

	urls := make(map[string]string)
	switch aType {
	case "gravatar":
		urls["256"] = GetGravatarURL(email, 256)
		urls["128"] = GetGravatarURL(email, 128)
		urls["64"] = GetGravatarURL(email, 64)
		urls["32"] = GetGravatarURL(email, 32)
	case "url":
		if avatarURL.Valid {
			urls["original"] = avatarURL.String
		}
	case "upload":
		if avatarURL.Valid {
			base := avatarURL.String
			urls["original"] = base
			urls["256"] = base
			urls["128"] = base
			urls["64"] = base
			urls["32"] = base
		}
	}

	return AvatarInfo{
		Type: aType,
		URLs: urls,
	}
}

// AvatarResponse represents the response for avatar endpoints
type AvatarResponse struct {
	Type string            `json:"type"`
	URLs map[string]string `json:"urls"`
}

// AvatarUploadRequest represents avatar upload metadata shared by REST and GraphQL.
type AvatarUploadRequest struct {
	Filename    string
	Size        int64
	ContentType string
}

// LoadCurrentUserAvatar returns the same avatar payload used by GET /api/v1/users/avatar.
func LoadCurrentUserAvatar(db *sql.DB, userID int64) (*AvatarResponse, error) {
	h := NewUserPublicHandler(db)
	return h.loadCurrentUserAvatar(userID)
}

// UpdateCurrentUserAvatar applies the same avatar update used by PATCH /api/v1/users/avatar.
func UpdateCurrentUserAvatar(db *sql.DB, userID int64, req *UpdateAvatarRequest) (*AvatarResponse, error) {
	h := NewUserPublicHandler(db)
	if err := h.updateCurrentUserAvatar(userID, req); err != nil {
		return nil, err
	}
	return h.loadCurrentUserAvatar(userID)
}

// ResetCurrentUserAvatar applies the same avatar reset used by DELETE /api/v1/users/avatar.
func ResetCurrentUserAvatar(db *sql.DB, userID int64) error {
	h := NewUserPublicHandler(db)
	return h.resetCurrentUserAvatar(userID)
}

// UploadCurrentUserAvatar applies the same avatar upload used by POST /api/v1/users/avatar.
func UploadCurrentUserAvatar(db *sql.DB, userID int64, upload *AvatarUploadRequest) (*AvatarResponse, error) {
	h := NewUserPublicHandler(db)
	if err := h.uploadCurrentUserAvatar(userID, upload); err != nil {
		return nil, err
	}
	return h.loadCurrentUserAvatar(userID)
}

func (h *UserPublicHandler) loadCurrentUserAvatar(userID int64) (*AvatarResponse, error) {
	var avatarType, avatarURL sql.NullString
	var email string

	err := h.DB.QueryRow(`
		SELECT email, avatar_type, avatar_url
		FROM user_accounts WHERE id = ?
	`, userID).Scan(&email, &avatarType, &avatarURL)
	if err != nil {
		return nil, err
	}

	avatar := buildAvatarInfo(email, avatarType, avatarURL)
	return &AvatarResponse{
		Type: avatar.Type,
		URLs: avatar.URLs,
	}, nil
}

func (h *UserPublicHandler) updateCurrentUserAvatar(userID int64, req *UpdateAvatarRequest) error {
	if req == nil {
		return fmt.Errorf("invalid request")
	}

	if req.Type != "gravatar" && req.Type != "url" {
		return fmt.Errorf("avatar type must be one of: gravatar, url")
	}

	if req.Type == "url" {
		if strings.TrimSpace(req.URL) == "" {
			return fmt.Errorf("URL is required for url avatar type")
		}
		if !strings.HasPrefix(req.URL, "https://") {
			return fmt.Errorf("avatar URL must use HTTPS")
		}
	}

	var avatarURL sql.NullString
	if req.Type == "url" {
		avatarURL = sql.NullString{String: strings.TrimSpace(req.URL), Valid: true}
	}

	_, err := h.DB.Exec(`
		UPDATE user_accounts
		SET avatar_type = ?, avatar_url = ?, updated_at = ?
		WHERE id = ?
	`, req.Type, avatarURL, time.Now(), userID)
	return err
}

func (h *UserPublicHandler) resetCurrentUserAvatar(userID int64) error {
	_, err := h.DB.Exec(`
		UPDATE user_accounts
		SET avatar_type = 'gravatar', avatar_url = NULL, updated_at = ?
		WHERE id = ?
	`, time.Now(), userID)
	return err
}

func (h *UserPublicHandler) uploadCurrentUserAvatar(userID int64, upload *AvatarUploadRequest) error {
	if upload == nil {
		return fmt.Errorf("No file uploaded")
	}

	if upload.Size > 2*1024*1024 {
		return fmt.Errorf("File too large (max 2MB)")
	}

	contentType := strings.TrimSpace(upload.ContentType)
	allowedTypes := map[string]bool{
		"image/png":                true,
		"image/jpeg":               true,
		"image/gif":                true,
		"image/webp":               true,
		"image/bmp":                true,
		"image/svg+xml":            true,
		"image/x-icon":             true,
		"image/vnd.microsoft.icon": true,
	}
	if !allowedTypes[contentType] {
		return fmt.Errorf("Invalid image type")
	}

	avatarURL := fmt.Sprintf("/uploads/avatars/user_%d.%s", userID, getExtension(contentType))

	_, err := h.DB.Exec(`
		UPDATE user_accounts
		SET avatar_type = 'upload', avatar_url = ?, updated_at = ?
		WHERE id = ?
	`, avatarURL, time.Now(), userID)
	return err
}

// GetCurrentUserAvatar returns the current user's avatar info
// Route: GET /api/v1/users/avatar
// @Summary Get avatar
// @Description Get the current user's avatar info.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Avatar info"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/avatar [get]
func (h *UserPublicHandler) GetCurrentUserAvatar(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	response, err := h.loadCurrentUserAvatar(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get avatar info"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// UpdateAvatarRequest represents a request to update avatar settings
type UpdateAvatarRequest struct {
	Type string `json:"type" binding:"required,oneof=gravatar url"`
	URL  string `json:"url,omitempty"`
}

// UpdateAvatarSettings updates the user's avatar settings (type and URL)
// Route: PATCH /api/v1/users/avatar
// @Summary Update avatar settings
// @Description Update avatar type (gravatar/url/upload) and URL.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body UpdateAvatarRequest true "Avatar settings"
// @Success 200 {object} map[string]interface{} "Updated avatar info"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/avatar [patch]
func (h *UserPublicHandler) UpdateAvatarSettings(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req UpdateAvatarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := UpdateCurrentUserAvatar(h.DB, user.ID, &req)
	if err != nil {
		if err.Error() == "avatar type must be one of: gravatar, url" || err.Error() == "URL is required for url avatar type" || err.Error() == "avatar URL must use HTTPS" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update avatar"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ResetAvatar resets the user's avatar to Gravatar
// Route: DELETE /api/v1/users/avatar
// @Summary Reset avatar
// @Description Reset avatar to Gravatar default.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Avatar reset"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/avatar [delete]
func (h *UserPublicHandler) ResetAvatar(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	if err := h.resetCurrentUserAvatar(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset avatar"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Avatar reset to Gravatar"})
}

// UploadAvatar handles avatar file upload
// Route: POST /api/v1/users/avatar
// Per AI.md PART 34: Max 2MB, PNG/JPG/GIF/WEBP/etc.
// @Summary Upload avatar
// @Description Upload an avatar image (max 2 MB, PNG/JPG/GIF/WEBP).
// @Tags User
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Avatar image file"
// @Success 200 {object} map[string]interface{} "Upload successful"
// @Failure 400 {object} map[string]interface{} "Bad request or file too large"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/avatar [post]
func (h *UserPublicHandler) UploadAvatar(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	response, err := UploadCurrentUserAvatar(h.DB, user.ID, &AvatarUploadRequest{
		Filename:    file.Filename,
		Size:        file.Size,
		ContentType: file.Header.Get("Content-Type"),
	})
	if err != nil {
		switch err.Error() {
		case "No file uploaded", "File too large (max 2MB)", "Invalid image type":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save avatar"})
			return
		}
	}

	c.JSON(http.StatusOK, response)
}

// getExtension returns file extension for content type
func getExtension(contentType string) string {
	switch contentType {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	case "image/svg+xml":
		return "svg"
	default:
		return "png"
	}
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

// ChangeCurrentUserPassword applies the same password change used by POST /api/v1/users/security/password.
func ChangeCurrentUserPassword(db *sql.DB, userID int64, req *ChangePasswordRequest) error {
	h := NewUserPublicHandler(db)
	return h.changeCurrentUserPassword(userID, req)
}

func (h *UserPublicHandler) changeCurrentUserPassword(userID int64, req *ChangePasswordRequest) error {
	if req == nil {
		return fmt.Errorf("invalid request")
	}

	var passwordHash string
	err := h.DB.QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, userID).Scan(&passwordHash)
	if err != nil {
		return fmt.Errorf("failed to verify current password")
	}

	valid, err := utils.VerifyPassword(req.CurrentPassword, passwordHash)
	if err != nil || !valid {
		return fmt.Errorf("current password is incorrect")
	}

	newHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password")
	}

	_, err = h.DB.Exec(`
		UPDATE user_accounts
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, newHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update password")
	}

	return nil
}

// ChangePassword allows authenticated user to change their password
// Route: POST /api/v1/users/security/password
// @Summary Change password
// @Description Change the current user's password.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body ChangePasswordRequest true "Current and new password"
// @Success 200 {object} map[string]interface{} "Password changed"
// @Failure 400 {object} map[string]interface{} "Validation error"
// @Failure 401 {object} map[string]interface{} "Wrong current password or not authenticated"
// @Router /api/v1/users/security/password [post]
func (h *UserPublicHandler) ChangePassword(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.changeCurrentUserPassword(user.ID, &req); err != nil {
		if err.Error() == "current password is incorrect" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

// ChangeEmail allows an authenticated user to change their email address.
// The new email is marked unverified until the user completes email verification.
func (h *UserPublicHandler) ChangeEmail(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		NewEmail        string `json:"new_email" binding:"required,email"`
		CurrentPassword string `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify current password before allowing email change
	var passwordHash string
	if err := h.DB.QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, user.ID).Scan(&passwordHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify credentials"})
		return
	}
	valid, err := utils.VerifyPassword(req.CurrentPassword, passwordHash)
	if err != nil || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Check the new email is not already in use
	var existing int64
	_ = h.DB.QueryRow(`SELECT id FROM user_accounts WHERE email = ? AND id != ?`, req.NewEmail, user.ID).Scan(&existing)
	if existing != 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Email address is already in use"})
		return
	}

	// Update email and mark as unverified until re-verified
	if _, err := h.DB.Exec(
		`UPDATE user_accounts SET email = ?, email_verified = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		req.NewEmail, user.ID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email address updated. Please verify your new email address."})
}

// DeleteAccount permanently deletes the authenticated user's account.
// Requires current password and the literal string "DELETE" as confirmation.
func (h *UserPublicHandler) DeleteAccount(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		Confirm         string `json:"confirm" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Confirm != "DELETE" {
		c.JSON(http.StatusBadRequest, gin.H{"error": `Confirmation must be the exact string "DELETE"`})
		return
	}

	// Verify password before allowing deletion
	var passwordHash string
	if err := h.DB.QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, user.ID).Scan(&passwordHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify credentials"})
		return
	}
	valid, err := utils.VerifyPassword(req.CurrentPassword, passwordHash)
	if err != nil || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Delete the account (cascades to sessions, preferences, locations, etc.)
	userModel := &models.UserModel{DB: h.DB}
	if err := userModel.Delete(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}
