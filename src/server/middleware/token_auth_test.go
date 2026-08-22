package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	_ "modernc.org/sqlite"
)

// openTokenAuthTestDBs opens two in-memory SQLite databases seeded with the
// real production schemas (database.ServerSchema / database.UsersSchema) -
// the same schemas database.InitDualDB applies in production - so tests fail
// the same way production would if a model queries a table that doesn't
// actually exist.
func openTokenAuthTestDBs(t *testing.T) (serverDB, usersDB *sql.DB) {
	t.Helper()
	seed := time.Now().UnixNano()

	sDSN := fmt.Sprintf("file:token_auth_server_%d?mode=memory&cache=shared", seed)
	sDB, err := sql.Open("sqlite", sDSN)
	if err != nil {
		t.Fatalf("open server sqlite: %v", err)
	}
	if _, err := sDB.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	t.Cleanup(func() { sDB.Close() })

	uDSN := fmt.Sprintf("file:token_auth_users_%d?mode=memory&cache=shared", seed+1)
	uDB, err := sql.Open("sqlite", uDSN)
	if err != nil {
		t.Fatalf("open users sqlite: %v", err)
	}
	if _, err := uDB.Exec(database.UsersSchema); err != nil {
		t.Fatalf("apply UsersSchema: %v", err)
	}
	t.Cleanup(func() { uDB.Close() })

	// AdminModel.GetByAPIToken / TokenModelV2 (src/server/model/admin.go:352,
	// token_v2.go) ignore the *sql.DB they're constructed with and query
	// database.GetServerDB() / database.GetUsersDB() directly - the same dead
	// DB parameter pattern found in auth.go/server_context.go/setup.go. If
	// the global dual DB isn't set, GetServerDB() returns nil and any
	// QueryRow call on it panics with a nil pointer dereference instead of
	// failing gracefully. Registering it here keeps these tests from
	// crashing and matches what production wiring actually depends on.
	database.SetGlobalDualDB(&database.DualDB{Server: sDB, Users: uDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return sDB, uDB
}

// TestDetectTokenType covers the prefix-based token-type router, including
// the longer compound agent prefixes taking precedence over their shorter
// standalone counterparts (adm_agt_ must never be misdetected as adm_).
func TestDetectTokenType(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  TokenType
	}{
		{"admin token", "adm_abc123", TokenTypeAdmin},
		{"user token", "usr_abc123", TokenTypeUser},
		{"org token", "org_abc123", TokenTypeOrg},
		{"admin agent token", "adm_agt_abc123", TokenTypeAdminAgent},
		{"user agent token", "usr_agt_abc123", TokenTypeUserAgent},
		{"org agent token", "org_agt_abc123", TokenTypeOrgAgent},
		{"unknown prefix", "xyz_abc123", TokenTypeUnknown},
		{"empty token", "", TokenTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectTokenType(tt.token); got != tt.want {
				t.Errorf("DetectTokenType(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

// TestValidateTokenPrefix covers the accept/reject boundary directly.
func TestValidateTokenPrefix(t *testing.T) {
	if err := ValidateTokenPrefix("adm_validlookingtoken"); err != nil {
		t.Errorf("ValidateTokenPrefix(adm_...) = %v, want nil", err)
	}
	if err := ValidateTokenPrefix("not-a-real-prefix"); err == nil {
		t.Error("ValidateTokenPrefix(garbage) = nil, want error")
	}
}

// TestTokenAuthMiddleware_RejectsMissingAuthHeader verifies a request with
// no Authorization header is rejected with 401.
func TestTokenAuthMiddleware_RejectsMissingAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serverDB, usersDB := openTokenAuthTestDBs(t)

	router := gin.New()
	router.Use(TokenAuthMiddleware(serverDB, usersDB))
	router.GET("/api/v1/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for missing Authorization header", w.Code)
	}
}

// TestTokenAuthMiddleware_RejectsMalformedAuthHeader verifies a non-Bearer
// Authorization header is rejected.
func TestTokenAuthMiddleware_RejectsMalformedAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serverDB, usersDB := openTokenAuthTestDBs(t)

	router := gin.New()
	router.Use(TokenAuthMiddleware(serverDB, usersDB))
	router.GET("/api/v1/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for non-Bearer Authorization header", w.Code)
	}
}

// TestTokenAuthMiddleware_RejectsUnknownTokenPrefix verifies a bearer token
// with no recognized prefix is rejected before any DB lookup.
func TestTokenAuthMiddleware_RejectsUnknownTokenPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serverDB, usersDB := openTokenAuthTestDBs(t)

	router := gin.New()
	router.Use(TokenAuthMiddleware(serverDB, usersDB))
	router.GET("/api/v1/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token-prefix")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for unrecognized token prefix", w.Code)
	}
}

// TestTokenAuthMiddleware_RejectsUnvalidatedAdminToken verifies an adm_
// token with no matching row in server_admin_credentials'
// api_token/api_token_hash is rejected with 401 rather than panicking or
// succeeding.
func TestTokenAuthMiddleware_RejectsUnvalidatedAdminToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serverDB, usersDB := openTokenAuthTestDBs(t)

	router := gin.New()
	router.Use(TokenAuthMiddleware(serverDB, usersDB))
	router.GET("/api/v1/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	req.Header.Set("Authorization", "Bearer adm_doesnotexist000000000000")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for unknown admin token", w.Code)
	}
}

// TestTokenAuthMiddleware_RejectsAgentAndOrgTokens verifies the
// not-yet-implemented token types (agent/org - PART 35/36 explicitly not
// implemented per this project's optional-rules.md) are always rejected
// rather than accidentally authenticating.
func TestTokenAuthMiddleware_RejectsAgentAndOrgTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokens := []string{
		"adm_agt_00000000000000000000000000000000",
		"usr_agt_00000000000000000000000000000000",
		"org_agt_00000000000000000000000000000000",
		"org_00000000000000000000000000000000",
	}

	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			serverDB, usersDB := openTokenAuthTestDBs(t)
			router := gin.New()
			router.Use(TokenAuthMiddleware(serverDB, usersDB))
			router.GET("/api/v1/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 for unimplemented token type %q", w.Code, tok)
			}
		})
	}
}

// TestTokenAuthMiddleware_UserTokenValidatesAgainstRealSchema proves that a
// usr_ token issued by TokenModelV2 authenticates end-to-end against the
// real production schema. TokenModelV2 used to read and write a table
// literally named "tokens", which existed only in a legacy single-database
// schema that has since been deleted and never in database.UsersSchema - the
// schema actually applied to users.db - so every usr_ token failed with
// "no such table: tokens", surfaced as a generic 401. Both sides now agree on the canonical
// user_tokens table, so this asserts the positive case: seed usersDB with
// the real UsersSchema, mint a token through the model, and require the
// middleware to accept it and populate the auth context.
func TestTokenAuthMiddleware_UserTokenValidatesAgainstRealSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	serverDB, usersDB := openTokenAuthTestDBs(t)

	// The canonical token table must exist in the applied schema - a missing
	// table here is the exact regression this test guards against.
	var count int
	if err := usersDB.QueryRow("SELECT COUNT(*) FROM user_tokens").Scan(&count); err != nil {
		t.Fatalf("SELECT FROM user_tokens failed against the real UsersSchema: %v", err)
	}

	// The middleware resolves the token owner through UserModel.GetByID, so
	// the token needs a real owning account row.
	if _, err := usersDB.Exec(`
		INSERT INTO user_accounts (id, username, email, password_hash)
		VALUES (42, 'tokenuser', 'tokenuser@example.com', 'argon2id$placeholder')
	`); err != nil {
		t.Fatalf("insert user account: %v", err)
	}

	tokenModel := &model.TokenModelV2{DB: usersDB}
	created, err := tokenModel.CreateToken(model.OwnerTypeUser, 42, "middleware test", model.ScopeRead, 0)
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	var gotAuthType any
	router := gin.New()
	router.Use(TokenAuthMiddleware(serverDB, usersDB))
	router.GET("/api/v1/protected", func(c *gin.Context) {
		gotAuthType, _ = c.Get("auth_type")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a valid user token: %s", w.Code, w.Body.String())
	}
	if gotAuthType != "user_token" {
		t.Errorf("auth_type in context = %v, want \"user_token\"", gotAuthType)
	}
}

// TestRequireAdminToken covers the admin-only gate that must sit behind
// TokenAuthMiddleware on every admin route group. TokenAuthMiddleware accepts
// both adm_ and usr_ tokens, so without this gate a regular user token would
// reach the whole admin API (config writes, scheduler control, server
// restart). The three cases are the full decision surface: admin passes,
// authenticated-but-not-admin is refused, unauthenticated is refused.
func TestRequireAdminToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		authType any
		setAuth  bool
		want     int
	}{
		{"admin token passes", AuthTypeAdminToken, true, http.StatusOK},
		{"user token refused", "user_token", true, http.StatusForbidden},
		{"unauthenticated refused", nil, false, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.setAuth {
					c.Set("auth_type", tt.authType)
				}
				c.Next()
			})
			router.Use(RequireAdminToken())
			router.GET("/admin", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))

			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
			if tt.want == http.StatusForbidden && !strings.Contains(w.Body.String(), `"error":"FORBIDDEN"`) {
				t.Errorf("body = %q, want canonical FORBIDDEN error shape", w.Body.String())
			}
		})
	}
}
