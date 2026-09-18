// Package middleware provides token validation per AI.md PART 11
package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// TokenType represents the type of API token per AI.md PART 11
type TokenType int

const (
	TokenTypeUnknown TokenType = iota
	// adm_
	TokenTypeAdmin
	// usr_
	TokenTypeUser
	// org_
	TokenTypeOrg
	// adm_agt_
	TokenTypeAdminAgent
	// usr_agt_
	TokenTypeUserAgent
	// org_agt_
	TokenTypeOrgAgent
)

// DetectTokenType determines the token type from prefix per AI.md PART 11
func DetectTokenType(token string) TokenType {
	// Check compound agent prefixes first (longer prefixes)
	if strings.HasPrefix(token, model.PrefixAdminAgt) {
		return TokenTypeAdminAgent
	}
	if strings.HasPrefix(token, model.PrefixUserAgt) {
		return TokenTypeUserAgent
	}
	if strings.HasPrefix(token, model.PrefixOrgAgt) {
		return TokenTypeOrgAgent
	}

	// Check standard prefixes
	if strings.HasPrefix(token, model.PrefixAdmin) {
		return TokenTypeAdmin
	}
	if strings.HasPrefix(token, model.PrefixUser) {
		return TokenTypeUser
	}
	if strings.HasPrefix(token, model.PrefixOrg) {
		return TokenTypeOrg
	}

	return TokenTypeUnknown
}

// ValidateTokenPrefix validates token has correct prefix per AI.md PART 11
func ValidateTokenPrefix(token string) error {
	tokenType := DetectTokenType(token)
	if tokenType == TokenTypeUnknown {
		return fmt.Errorf("invalid token prefix: must be adm_, usr_, org_, adm_agt_, usr_agt_, or org_agt_")
	}
	return nil
}

// TokenAuthMiddleware validates API tokens with proper prefixes per AI.md PART 11
func TokenAuthMiddleware(serverDB, usersDB *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
				return
			}

			token := parts[1]

			// Validate token prefix
			if err := ValidateTokenPrefix(token); err != nil {
				writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
				return
			}

			// Determine token type and validate
			tokenType := DetectTokenType(token)

			ctx := r.Context()

			switch tokenType {
			case TokenTypeAdmin:
				// Validate admin token (adm_)
				adminModel := &model.AdminModel{DB: serverDB}
				admin, err := adminModel.GetByAPIToken(token)
				if err != nil {
					writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
					return
				}
				ctx = reqctx.SetValue(ctx, "admin", admin)
				ctx = reqctx.SetValue(ctx, "db", serverDB)
				ctx = reqctx.SetValue(ctx, "auth_type", AuthTypeAdminToken)

			case TokenTypeUser:
				// Validate user token (usr_) using new token model
				tokenModelV2 := &model.TokenModelV2{DB: usersDB}
				validatedToken, err := tokenModelV2.ValidateToken(token)
				if err != nil {
					writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
					return
				}

				// Get user
				userModel := &model.UserModel{DB: usersDB}
				user, err := userModel.GetByID(validatedToken.OwnerID)
				if err != nil {
					writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
					return
				}

				// Update last used timestamp
				go tokenModelV2.UpdateLastUsed(validatedToken.ID)

				ctx = reqctx.SetValue(ctx, UserContextKey, user)
				// Handlers read the numeric id via reqctx.GetInt(UserIDContextKey); model.User.ID is int64, which GetInt cannot assert
				ctx = reqctx.SetValue(ctx, UserIDContextKey, int(user.ID))
				ctx = reqctx.SetValue(ctx, "token", validatedToken)
				ctx = reqctx.SetValue(ctx, "auth_type", "user_token")

			case TokenTypeAdminAgent, TokenTypeUserAgent, TokenTypeOrgAgent, TokenTypeOrg:
				writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
				return

			default:
				writeAPIError(w, http.StatusUnauthorized, "TOKEN_INVALID", "Invalid token")
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthTypeAdminToken is the auth_type context value set by TokenAuthMiddleware
// when the request presented a Server Admin token.
const AuthTypeAdminToken = "admin_token"

// RequireAdminToken rejects any request that authenticated as something other
// than a Server Admin. TokenAuthMiddleware accepts both admin (adm_) and user
// (usr_) tokens, so admin route groups must chain this after it — otherwise a
// regular user token would reach the admin API. Per AI.md PART 17 the Server
// Admin is a separate account type from a PART 34 regular user, and PART 11
// requires least privilege on every admin surface.
func RequireAdminToken() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authType, exists := reqctx.GetValue(r.Context(), "auth_type")
			if !exists || authType != AuthTypeAdminToken {
				writeAPIError(w, http.StatusForbidden, "FORBIDDEN", "Admin access required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
