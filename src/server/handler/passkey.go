package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/patrickmn/go-cache"
)

const (
	passkeyCeremonyCookieName = "weather_passkey_session"
	passkeyCeremonyTTL        = 15 * time.Minute
	passkeyKindLogin          = "login"
	passkeyKindTwoFactor      = "two_factor"
	passkeyKindRegistration   = "registration"
	passkeyUserHandlePrefix   = "usr:"
)

var passkeyCeremonyCache = cache.New(passkeyCeremonyTTL, 30*time.Minute)

type PasskeyHandler struct {
	DB *sql.DB
}

type passkeyCeremonyState struct {
	Kind                string
	UserID              int64
	PendingSessionToken string
	Name                string
	SessionData         webauthn.SessionData
}

type passkeyRegistrationStartRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type passkeyChallengeRequest struct {
	SessionToken string `json:"session_token"`
}

type passkeySummary struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type passkeyUser struct {
	user        *models.User
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte {
	return []byte(fmt.Sprintf("%s%d", passkeyUserHandlePrefix, u.user.ID))
}

func (u *passkeyUser) WebAuthnName() string {
	if strings.TrimSpace(u.user.DisplayName) != "" {
		return strings.TrimSpace(u.user.DisplayName)
	}
	return u.user.Username
}

func (u *passkeyUser) WebAuthnDisplayName() string {
	if strings.TrimSpace(u.user.DisplayName) != "" {
		return strings.TrimSpace(u.user.DisplayName)
	}
	return u.user.Username
}

func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func NewPasskeyHandler(db *sql.DB) *PasskeyHandler {
	return &PasskeyHandler{DB: db}
}

func (h *PasskeyHandler) loadWebAuthnUser(user *models.User) (*passkeyUser, error) {
	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	credentials, err := passkeyModel.ListCredentialsByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	return &passkeyUser{user: user, credentials: credentials}, nil
}

func (h *PasskeyHandler) buildWebAuthn(c *gin.Context) (*webauthn.WebAuthn, error) {
	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		return nil, fmt.Errorf("missing request host")
	}

	rpID := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		rpID = parsedHost
	}
	rpID = strings.TrimPrefix(strings.TrimSuffix(rpID, "]"), "[")

	scheme := "http"
	if requestUsesHTTPS(c) {
		scheme = "https"
	}

	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: "Weather",
		RPOrigins:     []string{fmt.Sprintf("%s://%s", scheme, host)},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce: true,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce: true,
			},
		},
	})
}

func setPasskeyCeremonyCookie(c *gin.Context, token string) {
	c.SetCookie(passkeyCeremonyCookieName, token, int(passkeyCeremonyTTL.Seconds()), "/", "", requestUsesHTTPS(c), true)
}

func clearPasskeyCeremonyCookie(c *gin.Context) {
	c.SetCookie(passkeyCeremonyCookieName, "", -1, "/", "", requestUsesHTTPS(c), true)
}

func loadPasskeyCeremonyState(c *gin.Context) (*passkeyCeremonyState, string, error) {
	token, err := c.Cookie(passkeyCeremonyCookieName)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, "", fmt.Errorf("passkey session not found")
	}

	rawState, found := passkeyCeremonyCache.Get(token)
	if !found {
		return nil, "", fmt.Errorf("passkey session expired")
	}

	state, ok := rawState.(*passkeyCeremonyState)
	if !ok || state == nil {
		return nil, "", fmt.Errorf("invalid passkey session")
	}

	return state, token, nil
}

func storePasskeyCeremonyState(c *gin.Context, state *passkeyCeremonyState) error {
	token, err := models.GenerateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate passkey session: %w", err)
	}

	passkeyCeremonyCache.Set(token, state, passkeyCeremonyTTL)
	setPasskeyCeremonyCookie(c, token)
	return nil
}

func parsePasskeyUserHandle(userHandle []byte) (int64, error) {
	raw := string(userHandle)
	if !strings.HasPrefix(raw, passkeyUserHandlePrefix) {
		return 0, fmt.Errorf("invalid user handle")
	}

	userID, err := strconv.ParseInt(strings.TrimPrefix(raw, passkeyUserHandlePrefix), 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid user handle")
	}

	return userID, nil
}

func cloneRequestWithBody(c *gin.Context, body []byte) *http.Request {
	req := c.Request.Clone(c.Request.Context())
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return req
}

func (h *PasskeyHandler) passkeyLookup(rawID []byte, userHandle []byte) (webauthn.User, error) {
	userID, err := parsePasskeyUserHandle(userHandle)
	if err != nil {
		return nil, err
	}

	userModel := &models.UserModel{DB: h.DB}
	user, err := userModel.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user")
	}
	if err := validateAuthUser(user); err != nil {
		return nil, err
	}

	waUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		return nil, err
	}

	for _, credential := range waUser.credentials {
		if bytes.Equal(credential.ID, rawID) {
			return waUser, nil
		}
	}

	return nil, fmt.Errorf("credential not found")
}

// @Summary List passkeys
// @Description List all registered passkeys for the authenticated user.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{} "Passkey list"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/passkeys [get]
func (h *PasskeyHandler) ListPasskeys(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	passkeys, err := passkeyModel.ListByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
		return
	}

	summaries := make([]passkeySummary, 0, len(passkeys))
	for _, passkey := range passkeys {
		summaries = append(summaries, passkeySummary{
			ID:         passkey.ID,
			Name:       passkey.Name,
			CreatedAt:  passkey.CreatedAt,
			LastUsedAt: passkey.LastUsedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"passkeys": summaries,
	})
}

// @Summary Register passkey
// @Description Begin WebAuthn passkey registration ceremony for the authenticated user.
// @Tags User
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object false "Optional passkey name"
// @Success 200 {object} map[string]interface{} "WebAuthn creation options or success on completion"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Router /api/v1/users/security/passkeys [post]
func (h *PasskeyHandler) RegisterPasskey(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	if _, hasResponse := envelope["response"]; hasResponse {
		h.finishPasskeyRegistration(c, user, body)
		return
	}

	var req passkeyRegistrationStartRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Passkey name and password are required"})
		return
	}

	userModel := &models.UserModel{DB: h.DB}
	if !userModel.CheckPassword(user, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Invalid password"})
		return
	}

	waUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
		return
	}

	exclusions := make([]protocol.CredentialDescriptor, 0, len(waUser.credentials))
	for _, credential := range waUser.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}

	wa, err := h.buildWebAuthn(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to initialize passkeys"})
		return
	}

	options, sessionData, err := wa.BeginRegistration(
		waUser,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
		}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if err := storePasskeyCeremonyState(c, &passkeyCeremonyState{
		Kind:        passkeyKindRegistration,
		UserID:      user.ID,
		Name:        req.Name,
		SessionData: *sessionData,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to start passkey registration"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"options": options,
	})
}

func (h *PasskeyHandler) finishPasskeyRegistration(c *gin.Context, user *models.User, body []byte) {
	state, token, err := loadPasskeyCeremonyState(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if state.Kind != passkeyKindRegistration || state.UserID != user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid passkey registration session"})
		return
	}

	waUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
		return
	}

	wa, err := h.buildWebAuthn(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to initialize passkeys"})
		return
	}

	credential, err := wa.FinishRegistration(waUser, state.SessionData, cloneRequestWithBody(c, body))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	passkey, err := passkeyModel.Create(user.ID, state.Name, credential)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	passkeyCeremonyCache.Delete(token)
	clearPasskeyCeremonyCookie(c)

	response := gin.H{
		"ok":      true,
		"message": "Passkey registered successfully",
		"passkey": passkeySummary{
			ID:         passkey.ID,
			Name:       passkey.Name,
			CreatedAt:  passkey.CreatedAt,
			LastUsedAt: passkey.LastUsedAt,
		},
	}

	count, err := passkeyModel.CountByUserID(user.ID)
	if err == nil && count == 1 {
		recoveryKeyModel := &models.RecoveryKeyModel{DB: h.DB}
		existingKeys, countErr := recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
		if countErr == nil && existingKeys == 0 {
			recoveryKeys, generateErr := recoveryKeyModel.GenerateRecoveryKeys(int(user.ID))
			if generateErr == nil {
				response["recovery_keys"] = recoveryKeys
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Delete passkey
// @Description Delete a registered passkey by ID.
// @Tags User
// @Security BearerAuth
// @Produce json
// @Param passkey_id path string true "Passkey ID"
// @Success 200 {object} map[string]interface{} "Passkey deleted"
// @Failure 401 {object} map[string]interface{} "Not authenticated"
// @Failure 404 {object} map[string]interface{} "Passkey not found"
// @Router /api/v1/users/security/passkeys/{passkey_id} [delete]
func (h *PasskeyHandler) DeletePasskey(c *gin.Context) {
	user, ok := middleware.GetCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Not authenticated"})
		return
	}

	passkeyID, err := strconv.ParseInt(strings.TrimSpace(c.Param("passkey_id")), 10, 64)
	if err != nil || passkeyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid passkey id"})
		return
	}

	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	if err := passkeyModel.DeleteByID(user.ID, passkeyID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "passkey not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Passkey deleted successfully",
	})
}

// @Summary Begin passkey auth challenge
// @Description Start WebAuthn authentication ceremony (login via passkey).
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body object false "Optional username hint"
// @Success 200 {object} map[string]interface{} "WebAuthn request options"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Router /api/v1/server/auth/passkey/challenge [post]
func (h *PasskeyHandler) BeginPasskeyChallenge(c *gin.Context) {
	var req passkeyChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil && !strings.Contains(err.Error(), "EOF") {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	wa, err := h.buildWebAuthn(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to initialize passkeys"})
		return
	}

	if strings.TrimSpace(req.SessionToken) != "" {
		pendingSession, err := loadPendingTwoFactorSession(h.DB, req.SessionToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
			return
		}

		userModel := &models.UserModel{DB: h.DB}
		user, err := userModel.GetByID(int64(pendingSession.UserID))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Invalid session token"})
			return
		}
		if err := validateAuthUser(user); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
			return
		}

		waUser, err := h.loadWebAuthnUser(user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
			return
		}
		if len(waUser.credentials) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "No passkeys registered for this account"})
			return
		}

		options, sessionData, err := wa.BeginLogin(waUser, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
			return
		}

		if err := storePasskeyCeremonyState(c, &passkeyCeremonyState{
			Kind:                passkeyKindTwoFactor,
			UserID:              user.ID,
			PendingSessionToken: pendingSession.ID,
			SessionData:         *sessionData,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to start passkey challenge"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"options": options,
		})
		return
	}

	options, sessionData, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	if err := storePasskeyCeremonyState(c, &passkeyCeremonyState{
		Kind:        passkeyKindLogin,
		SessionData: *sessionData,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to start passkey challenge"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"options": options,
	})
}

// @Summary Verify passkey auth
// @Description Complete WebAuthn authentication ceremony and return a session token.
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Session token returned"
// @Failure 400 {object} map[string]interface{} "Bad request"
// @Failure 401 {object} map[string]interface{} "Verification failed"
// @Router /api/v1/server/auth/passkey/verify [post]
func (h *PasskeyHandler) VerifyPasskey(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid request body"})
		return
	}

	state, token, err := loadPasskeyCeremonyState(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}

	wa, err := h.buildWebAuthn(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to initialize passkeys"})
		return
	}

	passkeyModel := &models.UserPasskeyModel{DB: h.DB}
	userModel := &models.UserModel{DB: h.DB}

	var (
		user       *models.User
		credential *webauthn.Credential
		response   *AuthLoginResponse
	)

	switch state.Kind {
	case passkeyKindLogin:
		parsed, parseErr := protocol.ParseCredentialRequestResponse(cloneRequestWithBody(c, body))
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": parseErr.Error()})
			return
		}

		waResolvedUser, resolvedCredential, validateErr := wa.ValidatePasskeyLogin(h.passkeyLookup, state.SessionData, parsed)
		if validateErr != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": validateErr.Error()})
			return
		}

		resolvedUser, ok := waResolvedUser.(*passkeyUser)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to resolve passkey user"})
			return
		}

		user = resolvedUser.user
		credential = resolvedCredential
		response, err = createFullAuthSession(h.DB, user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to create session"})
			return
		}
	case passkeyKindTwoFactor:
		pendingSession, sessionErr := loadPendingTwoFactorSession(h.DB, state.PendingSessionToken)
		if sessionErr != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": sessionErr.Error()})
			return
		}

		user, err = userModel.GetByID(int64(pendingSession.UserID))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "Invalid session token"})
			return
		}
		if err := validateAuthUser(user); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
			return
		}

		waUser, loadErr := h.loadWebAuthnUser(user)
		if loadErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to load passkeys"})
			return
		}

		credential, err = wa.FinishLogin(waUser, state.SessionData, cloneRequestWithBody(c, body))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": err.Error()})
			return
		}

		response, err = createFullAuthSession(h.DB, user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to create session"})
			return
		}

		sessionModel := &models.SessionModel{DB: h.DB}
		_ = sessionModel.Delete(pendingSession.ID)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Invalid passkey session"})
		return
	}

	if err := passkeyModel.UpdateCredential(user.ID, credential); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "Failed to update passkey"})
		return
	}

	_ = userModel.UpdateLastLogin(user.ID, c.ClientIP())
	passkeyCeremonyCache.Delete(token)
	clearPasskeyCeremonyCookie(c)
	setUserSessionCookie(c, response.Token, *response.ExpiresAt)

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Passkey authentication successful",
		"result":  response,
	})
}
