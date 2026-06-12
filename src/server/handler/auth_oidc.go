// Package handler provides OIDC authentication handlers per AI.md PART 34.
package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casapps/wthr/src/database"
	models "github.com/casapps/wthr/src/server/model"
	"github.com/casapps/wthr/src/server/middleware"
	"github.com/casapps/wthr/src/server/service"
)

// OIDCAuthHandler handles OIDC-based authentication per AI.md PART 34.
type OIDCAuthHandler struct {
	DB          *sql.DB
	OIDCService *service.OIDCService
}

const (
	oidcStateCookiePrefix  = "oidc_state_"
	oidcPKCECookiePrefix   = "oidc_pkce_"
	oidcCookieMaxAge       = 600 // 10 minutes — enough to complete the OIDC round-trip
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
func (h *OIDCAuthHandler) StartLogin(c *gin.Context) {
	provider := c.Param("provider")

	if !h.OIDCService.Enabled() {
		c.HTML(http.StatusServiceUnavailable, "page/oidc_redirect.tmpl", gin.H{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "OIDC authentication is not enabled.",
		})
		return
	}

	cfg, err := h.OIDCService.GetProviderConfig(provider)
	if err != nil {
		c.HTML(http.StatusBadRequest, "page/oidc_redirect.tmpl", gin.H{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Unknown OIDC provider.",
		})
		return
	}

	state, err := service.GenerateState()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "page/oidc_redirect.tmpl", gin.H{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to generate state.",
		})
		return
	}

	codeVerifier, err := service.GenerateCodeVerifier()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "page/oidc_redirect.tmpl", gin.H{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to generate PKCE verifier.",
		})
		return
	}

	redirectURL := buildOIDCCallbackURL(c, provider)
	authURL, err := h.OIDCService.AuthURL(c.Request.Context(), provider, redirectURL, state, codeVerifier)
	if err != nil {
		c.HTML(http.StatusServiceUnavailable, "page/oidc_redirect.tmpl", gin.H{
			"title":    "OIDC Login",
			"provider": provider,
			"error":    "Failed to connect to identity provider. Please try again later.",
		})
		return
	}

	secure := c.Request.TLS != nil
	c.SetCookie(oidcStateCookiePrefix+provider, state, oidcCookieMaxAge, "/", "", secure, true)
	c.SetCookie(oidcPKCECookiePrefix+provider, codeVerifier, oidcCookieMaxAge, "/", "", secure, true)

	_ = cfg // used via OIDCService internally
	c.Redirect(http.StatusFound, authURL)
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
func (h *OIDCAuthHandler) Callback(c *gin.Context) {
	provider := c.Param("provider")

	if !h.OIDCService.Enabled() {
		c.HTML(http.StatusServiceUnavailable, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "OIDC authentication is not enabled.",
		})
		return
	}

	// Verify state
	expectedState, err := c.Cookie(oidcStateCookiePrefix + provider)
	if err != nil || expectedState == "" {
		c.HTML(http.StatusBadRequest, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Missing or expired state. Please start the login process again.",
		})
		return
	}

	receivedState := c.Query("state")
	if receivedState != expectedState {
		c.HTML(http.StatusBadRequest, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "State mismatch. Please start the login process again.",
		})
		return
	}

	// Get PKCE verifier
	codeVerifier, err := c.Cookie(oidcPKCECookiePrefix + provider)
	if err != nil || codeVerifier == "" {
		c.HTML(http.StatusBadRequest, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Missing PKCE verifier. Please start the login process again.",
		})
		return
	}

	// Clear OIDC cookies immediately — single-use
	secure := c.Request.TLS != nil
	c.SetCookie(oidcStateCookiePrefix+provider, "", -1, "/", "", secure, true)
	c.SetCookie(oidcPKCECookiePrefix+provider, "", -1, "/", "", secure, true)

	// Check for error from provider
	if errParam := c.Query("error"); errParam != "" {
		errDesc := c.Query("error_description")
		if errDesc == "" {
			errDesc = errParam
		}
		c.HTML(http.StatusUnauthorized, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Authentication declined by identity provider: " + errDesc,
		})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.HTML(http.StatusBadRequest, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "No authorization code received.",
		})
		return
	}

	redirectURL := buildOIDCCallbackURL(c, provider)
	claims, err := h.OIDCService.ExchangeAndVerify(c.Request.Context(), provider, redirectURL, code, codeVerifier)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Authentication failed. Please try again.",
		})
		return
	}

	cfg, err := h.OIDCService.GetProviderConfig(provider)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Provider configuration error.",
		})
		return
	}

	usersDB := database.GetUsersDB()

	// Look up existing user by external identity mapping
	var userID int64
	err = usersDB.QueryRow(`
		SELECT user_id FROM user_oidc_mappings
		WHERE provider_name = ? AND provider_user_id = ?
	`, provider, claims.Sub).Scan(&userID)

	if err == sql.ErrNoRows {
		// New external account — provision if auto_register is enabled
		if !cfg.AutoRegister {
			c.HTML(http.StatusForbidden, "page/oidc_callback.tmpl", gin.H{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Automatic registration is disabled for this provider. Contact your administrator.",
			})
			return
		}

		// Derive a candidate username from claims
		username := claims.PreferredUsername
		if username == "" {
			username = deriveUsernameFromClaims(claims, provider)
		}

		email := claims.Email
		if email == "" {
			email = username + "@" + provider + ".oidc"
		}
		displayName := claims.Name
		if displayName == "" {
			displayName = username
		}

		// Generate a random unusable local password — OIDC users authenticate via OIDC only
		randToken, err := models.GenerateSecureToken(32)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Failed to create account.",
			})
			return
		}
		pwHash, err := models.HashPassword(randToken)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Failed to create account.",
			})
			return
		}

		emailVerified := 0
		if claims.EmailVerified {
			emailVerified = 1
		}

		result, err := usersDB.Exec(`
			INSERT INTO user_accounts
				(username, email, display_name, password_hash, role, is_active, email_verified, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'user', 1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, username, email, displayName, pwHash, emailVerified)
		if err != nil {
			// Username or email collision — generate a unique suffix
			username = username + "_" + provider
			result, err = usersDB.Exec(`
				INSERT INTO user_accounts
					(username, email, display_name, password_hash, role, is_active, email_verified, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'user', 1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			`, username, email, displayName, pwHash, emailVerified)
			if err != nil {
				c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
					"title":    "OIDC Callback",
					"provider": provider,
					"error":    "Failed to create account. Please contact your administrator.",
				})
				return
			}
		}
		userID, _ = result.LastInsertId()

		// Store OIDC identity mapping
		_, _ = usersDB.Exec(`
			INSERT INTO user_oidc_mappings
				(user_id, provider_name, provider_user_id, issuer, email, name, created_at, updated_at, last_login_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, userID, provider, claims.Sub, cfg.Issuer, claims.Email, claims.Name)

	} else if err != nil {
		c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Database error.",
		})
		return
	} else {
		// Existing user — update last login and sync identity mapping
		_, _ = usersDB.Exec(`
			UPDATE user_oidc_mappings SET last_login_at = CURRENT_TIMESTAMP, email = ?, name = ?, updated_at = CURRENT_TIMESTAMP
			WHERE provider_name = ? AND provider_user_id = ?
		`, claims.Email, claims.Name, provider, claims.Sub)

		// Verify user is still active and not banned
		var isActive, isBanned bool
		err = usersDB.QueryRow(
			"SELECT is_active, is_banned FROM user_accounts WHERE id = ?", userID,
		).Scan(&isActive, &isBanned)
		if err != nil || !isActive || isBanned {
			c.HTML(http.StatusForbidden, "page/oidc_callback.tmpl", gin.H{
				"title":    "OIDC Callback",
				"provider": provider,
				"error":    "Account is not available. Please contact your administrator.",
			})
			return
		}
	}

	// Create session
	sessionModel := &models.UserSessionModel{DB: usersDB}
	session, err := sessionModel.CreateSession(userID, c.ClientIP(), c.Request.UserAgent(), 24*time.Hour)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "page/oidc_callback.tmpl", gin.H{
			"title":    "OIDC Callback",
			"provider": provider,
			"error":    "Failed to create session.",
		})
		return
	}

	c.SetCookie(middleware.SessionCookieName, session.SessionID, 86400, "/", "", secure, true)
	c.Redirect(http.StatusFound, "/")
}

// buildOIDCCallbackURL constructs the absolute callback URL for this request.
func buildOIDCCallbackURL(c *gin.Context, provider string) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + c.Request.Host + "/server/auth/oidc/" + provider + "/callback"
}

// deriveUsernameFromClaims creates a sanitized username candidate from OIDC claims.
func deriveUsernameFromClaims(claims *service.OIDCClaims, provider string) string {
	if claims.PreferredUsername != "" {
		return sanitizeUsername(claims.PreferredUsername)
	}
	if claims.Email != "" {
		parts := splitEmail(claims.Email)
		return sanitizeUsername(parts[0])
	}
	if claims.Name != "" {
		return sanitizeUsername(claims.Name)
	}
	return provider + "_user"
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
