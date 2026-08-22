// Package middleware provides token validation per TEMPLATE.md PART 11
package middleware

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/server/model"
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

// TokenAuthMiddleware validates API tokens with proper prefixes per TEMPLATE.md PART 11
func TokenAuthMiddleware(serverDB, usersDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"ok": false, "error": "missing authorization header"})
			c.Abort()
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(401, gin.H{"ok": false, "error": "invalid authorization format"})
			c.Abort()
			return
		}

		token := parts[1]

		// Validate token prefix
		if err := ValidateTokenPrefix(token); err != nil {
			c.JSON(401, gin.H{"ok": false, "error": err.Error()})
			c.Abort()
			return
		}

		// Determine token type and validate
		tokenType := DetectTokenType(token)

		switch tokenType {
		case TokenTypeAdmin:
			// Validate admin token (adm_)
			adminModel := &model.AdminModel{DB: serverDB}
			admin, err := adminModel.GetByAPIToken(token)
			if err != nil {
				c.JSON(401, gin.H{"ok": false, "error": "invalid admin token"})
				c.Abort()
				return
			}
			c.Set("admin", admin)
			c.Set("db", serverDB)
			c.Set("auth_type", AuthTypeAdminToken)

		case TokenTypeUser:
			// Validate user token (usr_) using new token model
			tokenModelV2 := &model.TokenModelV2{DB: usersDB}
			validatedToken, err := tokenModelV2.ValidateToken(token)
			if err != nil {
				c.JSON(401, gin.H{"ok": false, "error": "invalid user token"})
				c.Abort()
				return
			}

			// Get user
			userModel := &model.UserModel{DB: usersDB}
			user, err := userModel.GetByID(validatedToken.OwnerID)
			if err != nil {
				c.JSON(401, gin.H{"ok": false, "error": "user not found"})
				c.Abort()
				return
			}

			// Update last used timestamp
			go tokenModelV2.UpdateLastUsed(validatedToken.ID)

			c.Set(UserContextKey, user)
			// Handlers read the numeric id via c.GetInt(UserIDContextKey); model.User.ID is int64, which GetInt cannot assert
			c.Set(UserIDContextKey, int(user.ID))
			c.Set("token", validatedToken)
			c.Set("auth_type", "user_token")

		case TokenTypeAdminAgent, TokenTypeUserAgent, TokenTypeOrgAgent, TokenTypeOrg:
			c.JSON(401, gin.H{"ok": false, "error": "invalid or expired token"})
			c.Abort()
			return

		default:
			c.JSON(401, gin.H{"ok": false, "error": "unknown token type"})
			c.Abort()
			return
		}

		c.Next()
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
func RequireAdminToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		authType, exists := c.Get("auth_type")
		if !exists || authType != AuthTypeAdminToken {
			c.JSON(403, gin.H{
				"ok":      false,
				"error":   "FORBIDDEN",
				"message": "Admin access required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
