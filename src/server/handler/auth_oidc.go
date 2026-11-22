// Package handler provides OIDC authentication handlers per AI.md PART 34.
package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// OIDCAuthHandler handles OIDC-based authentication per AI.md PART 34.
type OIDCAuthHandler struct {
	DB          *sql.DB
	OIDCService *service.OIDCService
	pendingMu   sync.Mutex
	pending     map[string]pendingOIDCIdentity
}

type pendingOIDCIdentity struct {
	provider string
	issuer   string
	claims   service.OIDCClaims
	expires  time.Time
}

const (
	oidcStateCookiePrefix = "oidc_state_"
	oidcPKCECookiePrefix  = "oidc_pkce_"
	oidcCookieMaxAge      = 600 // 10 minutes — enough to complete the OIDC round-trip
	oidcPendingCookie     = "oidc_pending"
)

// StartLogin initiates the OIDC authorization code flow.
//
// @Summary Start OIDC login
// @Tags auth
// @Produce html
// @Param provider path string true "OIDC provider name"
// @Success 302 {string} string "Redirect to OIDC provider"
// @Failure 400 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /server/auth/oidc/{provider} [get]
func (h *OIDCAuthHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	if !h.OIDCService.Enabled() {
		middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "page/oidc_redirect.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "OIDC authentication is not enabled.",
		}))
		return
	}

	_, err := h.OIDCService.GetProviderConfig(provider)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/oidc_redirect.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Unknown OIDC provider.",
		}))
		return
	}

	state, err := service.GenerateState()
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_redirect.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to generate state.",
		}))
		return
	}

	codeVerifier, err := service.GenerateCodeVerifier()
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_redirect.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to generate PKCE verifier.",
		}))
		return
	}

	redirectURL := buildOIDCCallbackURL(r, provider)
	authURL, err := h.OIDCService.AuthURL(r.Context(), provider, redirectURL, state, codeVerifier)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "page/oidc_redirect.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to connect to identity provider. Please try again later.",
		}))
		return
	}

	secure := r.TLS != nil
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookiePrefix + provider, Value: state, MaxAge: oidcCookieMaxAge, Path: "/", Secure: secure, HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: oidcPKCECookiePrefix + provider, Value: codeVerifier, MaxAge: oidcCookieMaxAge, Path: "/", Secure: secure, HttpOnly: true})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the OIDC provider callback, verifies the response, and creates a session.
//
// @Summary OIDC callback
// @Tags auth
// @Produce html
// @Param provider path string true "OIDC provider name"
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 302 {string} string "Redirect to dashboard"
// @Failure 400 {object} map[string]string
// @Router /server/auth/oidc/{provider}/callback [get]
func (h *OIDCAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	if !h.OIDCService.Enabled() {
		middleware.RenderHTML(w, r, http.StatusServiceUnavailable, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "OIDC authentication is not enabled.",
		}))
		return
	}

	// Verify state
	expectedStateCookie, err := r.Cookie(oidcStateCookiePrefix + provider)
	var expectedState string
	if expectedStateCookie != nil {
		expectedState = expectedStateCookie.Value
	}
	if err != nil || expectedState == "" {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Missing or expired state. Please start the login process again.",
		}))
		return
	}

	receivedState := r.URL.Query().Get("state")
	if receivedState != expectedState {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "State mismatch. Please start the login process again.",
		}))
		return
	}

	// Get PKCE verifier
	codeVerifierCookie, err := r.Cookie(oidcPKCECookiePrefix + provider)
	var codeVerifier string
	if codeVerifierCookie != nil {
		codeVerifier = codeVerifierCookie.Value
	}
	if err != nil || codeVerifier == "" {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Missing PKCE verifier. Please start the login process again.",
		}))
		return
	}

	// Clear OIDC cookies immediately — single-use
	secure := r.TLS != nil
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookiePrefix + provider, Value: "", MaxAge: -1, Path: "/", Secure: secure, HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: oidcPKCECookiePrefix + provider, Value: "", MaxAge: -1, Path: "/", Secure: secure, HttpOnly: true})

	// Check for error from provider
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		if errDesc == "" {
			errDesc = errParam
		}
		middleware.RenderHTML(w, r, http.StatusUnauthorized, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Authentication declined by identity provider: " + errDesc,
		}))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "No authorization code received.",
		}))
		return
	}

	redirectURL := buildOIDCCallbackURL(r, provider)
	claims, err := h.OIDCService.ExchangeAndVerify(r.Context(), provider, redirectURL, code, codeVerifier)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusUnauthorized, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Authentication failed. Please try again.",
		}))
		return
	}

	cfg, err := h.OIDCService.GetProviderConfig(provider)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Provider configuration error.",
		}))
		return
	}

	usersDB := database.GetUsersDB()

	// Look up existing user by external identity mapping
	var userID int64
	err = database.QueryRowContext(context.Background(), usersDB, database.TimeoutSimpleSelect, `
		SELECT user_id FROM user_oidc_mappings
		WHERE provider_name = ? AND provider_user_id = ?
	`, provider, claims.Sub).Scan(&userID)

	if err == sql.ErrNoRows {
		// New accounts require explicit username confirmation before provisioning.
		if !cfg.AutoRegister {
			middleware.RenderHTML(w, r, http.StatusForbidden, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Automatic registration is disabled for this provider. Contact your administrator.",
			}))
			return
		}
		token, tokenErr := models.GenerateSecureToken(32)
		if tokenErr != nil {
			middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{"title": "OIDC Callback", "provider": provider, "error": "Failed to begin account setup."}))
			return
		}
		h.pendingMu.Lock()
		if h.pending == nil {
			h.pending = make(map[string]pendingOIDCIdentity)
		}
		for key, pending := range h.pending {
			if time.Now().After(pending.expires) {
				delete(h.pending, key)
			}
		}
		h.pending[token] = pendingOIDCIdentity{provider: provider, issuer: cfg.Issuer, claims: *claims, expires: time.Now().Add(10 * time.Minute)}
		h.pendingMu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: oidcPendingCookie, Value: token, Path: "/server/auth/external/username", MaxAge: 600, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/server/auth/external/username", http.StatusSeeOther)
		return

	} else if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Database error.",
		}))
		return
	} else {
		// Existing user — update last login and sync identity mapping.
		// last_login_at/updated_at are bound as canonical UTC text rather than
		// produced by SQL's CURRENT_TIMESTAMP, which yields a different type and
		// zone on PostgreSQL, MySQL and SQL Server than it does on SQLite.
		now := dbtime.FormatSQLTimestamp(time.Now())
		_, _ = database.ExecContext(context.Background(), usersDB, database.TimeoutWrite, `
			UPDATE user_oidc_mappings SET last_login_at = ?, email = ?, name = ?, updated_at = ?
			WHERE provider_name = ? AND provider_user_id = ?
		`, now, claims.Email, claims.Name, now, provider, claims.Sub)

		// Verify user is still active and not banned
		var isActive, isBanned bool
		err = database.QueryRowContext(context.Background(), usersDB, database.TimeoutSimpleSelect,
			"SELECT is_active, is_banned FROM users WHERE id = ?", userID,
		).Scan(&isActive, &isBanned)
		if err != nil || !isActive || isBanned {
			middleware.RenderHTML(w, r, http.StatusForbidden, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Account is not available. Please contact your administrator.",
			}))
			return
		}
	}

	// Create session
	sessionModel := &models.UserSessionModel{DB: usersDB}
	session, err := sessionModel.CreateSession(userID, util.TrustedGetClientIP(r), r.UserAgent(), middleware.SessionSettingsFor(middleware.SessionAccountUser).MaxAge)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusInternalServerError, "page/oidc_callback.tmpl", util.TemplateData(r, map[string]interface{}{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Failed to create session.",
		}))
		return
	}

	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.SessionID, middleware.SessionSettingsFor(middleware.SessionAccountUser).MaxAge))
	http.Redirect(w, r, "/", http.StatusFound)
}

// ShowExternalUsername confirms the proposed username before a new OIDC user is created.
func (h *OIDCAuthHandler) ShowExternalUsername(w http.ResponseWriter, r *http.Request) {
	pending, ok := h.getPendingIdentity(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
		return
	}
	username := suggestOIDCUsername(r.Context(), database.GetUsersDB(), deriveUsernameFromClaims(&pending.claims, pending.provider))
	h.renderExternalUsername(w, r, pending, username, "", http.StatusOK)
}

// CompleteExternalUsername creates the local account with the username the user selected.
func (h *OIDCAuthHandler) CompleteExternalUsername(w http.ResponseWriter, r *http.Request) {
	pending, ok := h.getPendingIdentity(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	usersDB := database.GetUsersDB()
	if err := util.ValidateUsername(username); err != nil {
		h.renderExternalUsername(w, r, pending, suggestOIDCUsername(r.Context(), usersDB, username), Translate(r, "errors.auth.external_username_invalid"), http.StatusBadRequest)
		return
	}
	var exists int
	err := database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect, "SELECT 1 FROM users WHERE username = ?", username).Scan(&exists)
	if err == nil {
		h.renderExternalUsername(w, r, pending, suggestOIDCUsername(r.Context(), usersDB, username), Translate(r, "errors.auth.external_username_taken"), http.StatusConflict)
		return
	}
	if err != sql.ErrNoRows {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_unavailable"), http.StatusInternalServerError)
		return
	}

	// Recheck identity before creation so a duplicate callback cannot create a second account.
	var linkedUserID int64
	err = database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect,
		"SELECT user_id FROM user_oidc_mappings WHERE provider_name = ? AND provider_user_id = ?", pending.provider, pending.claims.Sub,
	).Scan(&linkedUserID)
	if err != sql.ErrNoRows {
		http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
		return
	}

	randToken, err := models.GenerateSecureToken(32)
	if err != nil {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	pwHash, err := models.HashPassword(randToken)
	if err != nil {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	claims := pending.claims
	email := claims.Email
	if email == "" {
		email = username + "@" + pending.provider + ".oidc"
	}
	displayName := claims.Name
	if displayName == "" {
		displayName = username
	}
	verified := 0
	if claims.EmailVerified {
		verified = 1
	}
	now := dbtime.FormatSQLTimestamp(time.Now())
	tx, err := usersDB.BeginTx(r.Context(), nil)
	if err != nil {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	result, err := tx.ExecContext(r.Context(), `
		INSERT INTO users (username, email, display_name, password_hash, role, is_active, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'user', 1, ?, ?, ?)
	`, username, email, displayName, pwHash, verified, now, now)
	if err != nil {
		_ = tx.Rollback()
		h.renderExternalUsername(w, r, pending, suggestOIDCUsername(r.Context(), usersDB, username), Translate(r, "errors.auth.external_username_taken"), http.StatusConflict)
		return
	}
	userID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `
		INSERT INTO user_oidc_mappings (user_id, provider_name, provider_user_id, issuer, email, name, created_at, updated_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, pending.provider, claims.Sub, pending.issuer, claims.Email, claims.Name, now, now, now); err != nil {
		_ = tx.Rollback()
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_create_failed"), http.StatusInternalServerError)
		return
	}
	h.consumePendingIdentity(r)
	http.SetCookie(w, &http.Cookie{Name: oidcPendingCookie, Value: "", Path: "/server/auth/external/username", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	sessionModel := &models.UserSessionModel{DB: usersDB}
	session, err := sessionModel.CreateSession(userID, util.TrustedGetClientIP(r), r.UserAgent(), middleware.SessionSettingsFor(middleware.SessionAccountUser).MaxAge)
	if err != nil {
		h.renderExternalUsername(w, r, pending, username, Translate(r, "errors.auth.external_username_session_failed"), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.SessionID, middleware.SessionSettingsFor(middleware.SessionAccountUser).MaxAge))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *OIDCAuthHandler) getPendingIdentity(r *http.Request) (pendingOIDCIdentity, bool) {
	cookie, err := r.Cookie(oidcPendingCookie)
	if err != nil || cookie.Value == "" {
		return pendingOIDCIdentity{}, false
	}
	h.pendingMu.Lock()
	defer h.pendingMu.Unlock()
	pending, ok := h.pending[cookie.Value]
	if !ok || time.Now().After(pending.expires) {
		delete(h.pending, cookie.Value)
		return pendingOIDCIdentity{}, false
	}
	return pending, true
}

func (h *OIDCAuthHandler) consumePendingIdentity(r *http.Request) {
	cookie, err := r.Cookie(oidcPendingCookie)
	if err != nil {
		return
	}
	h.pendingMu.Lock()
	delete(h.pending, cookie.Value)
	h.pendingMu.Unlock()
}

func (h *OIDCAuthHandler) renderExternalUsername(w http.ResponseWriter, r *http.Request, pending pendingOIDCIdentity, username, message string, status int) {
	middleware.RenderHTML(w, r, status, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{
		"title": Translate(r, "auth.external_username.title"), "username": username, "provider": pending.provider, "error": message,
	}))
}

func suggestOIDCUsername(ctx context.Context, db *sql.DB, raw string) string {
	base := normalizeOIDCUsernameCandidate(raw)
	if oidcUsernameAvailable(ctx, db, base) {
		return base
	}
	for i := 0; i < 32; i++ {
		digits, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			break
		}
		suffix := digits.Int64() + 1000
		trimmed := base
		if len(trimmed)+4 > util.MaxUsernameLength {
			trimmed = trimmed[:util.MaxUsernameLength-4]
		}
		candidate := trimmed + fmt.Sprint(suffix)
		if oidcUsernameAvailable(ctx, db, candidate) {
			return candidate
		}
	}
	return "user"
}

func normalizeOIDCUsernameCandidate(raw string) string {
	base := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@")))
	if strings.Contains(base, "@") {
		base = strings.SplitN(base, "@", 2)[0]
	}
	base = strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, base))
	base = strings.Trim(base, "-")
	if len(base) > util.MaxUsernameLength {
		base = base[:util.MaxUsernameLength]
	}
	if len(base) < util.MinUsernameLength {
		base = "user"
	}
	return base
}

func oidcUsernameAvailable(ctx context.Context, db *sql.DB, username string) bool {
	if util.ValidateUsername(username) != nil {
		return false
	}
	var exists int
	err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, "SELECT 1 FROM users WHERE username = ?", username).Scan(&exists)
	return err == sql.ErrNoRows
}

// buildOIDCCallbackURL constructs the absolute callback URL for this request.
// It goes through BuildURL (AI.md PART 12) rather than reading Host and
// X-Forwarded-Proto directly: this URL is registered with the external identity
// provider, so a forged Host header from an untrusted peer would otherwise send
// the provider's redirect - and the authorization code in it - to an attacker.
func buildOIDCCallbackURL(r *http.Request, provider string) string {
	return util.BuildURL(r, "/server/auth/oidc/"+provider+"/callback")
}

// deriveUsernameFromClaims creates a sanitized username candidate from OIDC claims.
func deriveUsernameFromClaims(claims *service.OIDCClaims, provider string) string {
	if claims.PreferredUsername != "" {
		return normalizeOIDCUsernameCandidate(claims.PreferredUsername)
	}
	if claims.Email != "" {
		return normalizeOIDCUsernameCandidate(claims.Email)
	}
	if claims.Name != "" {
		return normalizeOIDCUsernameCandidate(claims.Name)
	}
	return normalizeOIDCUsernameCandidate(provider + "_user")
}

// sanitizeUsername lowercases ASCII letters and keeps alphanumeric + safe punctuation.
func sanitizeUsername(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		// lowercase ASCII upper
		if ch >= 'A' && ch <= 'Z' {
			ch |= 0x20
		}
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' {
			out = append(out, ch)
		}
	}
	if len(out) == 0 {
		return "user"
	}
	return string(out)
}

// splitEmail returns [localPart, domain] for an email address.
func splitEmail(email string) []string {
	idx := len(email) - 1
	for idx >= 0 && email[idx] != '@' {
		idx--
	}
	if idx <= 0 {
		return []string{email, ""}
	}
	return []string{email[:idx], email[idx+1:]}
}
