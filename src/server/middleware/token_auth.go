// Package middleware provides token validation per TEMPLATE.md PART 11
package middleware

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// TokenType represents the type of API token per TEMPLATE.md PART 11
type TokenType int

const (
	TokenTypeUnknown    TokenType = iota
	TokenTypeAdmin                // adm_
	TokenTypeUser                 // usr_
	TokenTypeOrg                  // org_
	TokenTypeAdminAgent           // adm_agt_
	TokenTypeUserAgent            // usr_agt_
	TokenTypeOrgAgent             // org_agt_
)

// DetectTokenType determines the token type from prefix per TEMPLATE.md PART 11
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

// ValidateTokenPrefix validates token has correct prefix per TEMPLATE.md PART 11
func ValidateTokenPrefix(token string) error {
	tokenType := DetectTokenType(token)
	if tokenType == TokenTypeUnknown {
		return fmt.Errorf("invalid token prefix: must be adm_, usr_, org_, adm_agt_, usr_agt_, or org_agt_")
	}
	return nil
}

// writeTokenAuthError writes the non-canonical {"ok":false,"error":"<message>"}
// body TokenAuthMiddleware has always used, preserved verbatim (this shape
// predates the canonical {"ok","error":CODE,"message"} response format and is
// not upgraded here — a mechanical framework conversion must not change it).
func writeTokenAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": message})
}

// TokenAuthMiddleware validates API tokens with proper prefixes per TEMPLATE.md PART 11
func TokenAuthMiddleware(serverDB, usersDB *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeTokenAuthError(w, 401, "missing authorization header")
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeTokenAuthError(w, 401, "invalid authorization format")
				return
			}

			token := parts[1]

			// Validate token prefix
			if err := ValidateTokenPrefix(token); err != nil {
				writeTokenAuthError(w, 401, err.Error())
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
					writeTokenAuthError(w, 401, "invalid admin token")
					return
				}
				ctx = reqctx.Set(ctx, "admin", admin)
				ctx = reqctx.Set(ctx, "db", serverDB)
				ctx = reqctx.Set(ctx, "auth_type", AuthTypeAdminToken)

			case TokenTypeUser:
				// Validate user token (usr_) using new token model
				tokenModelV2 := &model.TokenModelV2{DB: usersDB}
				validatedToken, err := tokenModelV2.ValidateToken(token)
				if err != nil {
					writeTokenAuthError(w, 401, "invalid user token")
					return
				}

				// Get user
				userModel := &model.UserModel{DB: usersDB}
				user, err := userModel.GetByID(validatedToken.OwnerID)
				if err != nil {
					writeTokenAuthError(w, 401, "user not found")
					return
				}

				// Update last used timestamp
				go tokenModelV2.UpdateLastUsed(validatedToken.ID)

				ctx = reqctx.Set(ctx, UserContextKey, user)
				// Handlers read the numeric id via reqctx.GetInt(UserIDContextKey); model.User.ID is int64, which GetInt cannot assert
				ctx = reqctx.Set(ctx, UserIDContextKey, int(user.ID))
				ctx = reqctx.Set(ctx, "token", validatedToken)
				ctx = reqctx.Set(ctx, "auth_type", "user_token")

			case TokenTypeAdminAgent, TokenTypeUserAgent, TokenTypeOrgAgent, TokenTypeOrg:
				writeTokenAuthError(w, 401, "invalid or expired token")
				return

			default:
				writeTokenAuthError(w, 401, "unknown token type")
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
			authType, exists := reqctx.Get(r.Context(), "auth_type")
			if !exists || authType != AuthTypeAdminToken {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(403)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "FORBIDDEN",
					"message": "Admin access required",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
