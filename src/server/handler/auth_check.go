package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/server/model"
)

// authCheckUnauthorized answers 401 with the RFC 6750 challenge; invalid_token is
// named only when a token was presented and rejected.
func authCheckUnauthorized(w http.ResponseWriter, r *http.Request, code string) {
	w.Header().Set("WWW-Authenticate", middleware.BearerChallenge(code))
	RespondError(w, r, http.StatusUnauthorized, code, Translate(r, "errors.auth.invalid_token"))
}

// HandleAuthCheck is the machine-only token check (AI.md PART 16 "/server/"
// API endpoints): 200 with {"kind"} when the Bearer token is valid, 401
// otherwise. The kind is `operator` for server.token, `owner` for a Server
// Admin token and `user` for a regular user token. It has no side effects and
// never echoes the token.
// GET /api/{api_version}/server/auth/check
func HandleAuthCheck(serverDB, usersDB *database.Store, cfg *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			authCheckUnauthorized(w, r, "UNAUTHORIZED")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			authCheckUnauthorized(w, r, "UNAUTHORIZED")
			return
		}

		if cfg != nil && cfg.Server.Token != "" {
			want := sha256.Sum256([]byte(cfg.Server.Token))
			got := sha256.Sum256([]byte(token))
			if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
				RespondData(w, r, map[string]string{"kind": "operator"})
				return
			}
		}

		switch middleware.DetectTokenType(token) {
		case middleware.TokenTypeAdmin:
			if serverDB != nil {
				adminStore := database.GetUsersStore()
				if adminStore == nil {
					adminStore = serverDB
				}
				if admin, err := (&model.AdminModel{Store: adminStore}).GetByAPIToken(token); err == nil && admin != nil {
					RespondData(w, r, map[string]string{"kind": "owner"})
					return
				}
			}
		case middleware.TokenTypeUser:
			if usersDB != nil {
				if valid, err := (&model.TokenModelV2{Store: usersDB}).ValidateToken(token); err == nil && valid != nil {
					RespondData(w, r, map[string]string{"kind": "user"})
					return
				}
			}
		}
		authCheckUnauthorized(w, r, "TOKEN_INVALID")
	}
}
