package middleware

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/database"
	_ "modernc.org/sqlite"
)

// openTokenAuthTestDBs opens two in-memory SQLite databases seeded with the
// real production schemas (database.ServerSchema / database.UsersSchema) -
// the same schemas InitDBWithConfig applies in production - never the dead
// legacy database.Schema variable, so tests fail the same way production
// would if a model queries a table that doesn't actually exist.
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

// TestTokenAuthMiddleware_UserTokenFailsAgainstRealSchema documents a real
// production bug: TokenModelV2 (src/server/model/token_v2.go:187,232)
// reads/writes a table literally named "tokens" via
// `INSERT INTO tokens (...)` / `SELECT ... FROM tokens`. That table only
// exists in the dead legacy schema (database.Schema, src/database/schema.go)
// - it does NOT exist in database.UsersSchema, the schema InitDBWithConfig
// actually applies to users.db in production (backend-rules.md: "Dual
// database: server.db + users.db"). UsersSchema instead defines a
// differently-named/shaped table `user_tokens`.
//
// Consequently every usr_-prefixed API token presented to
// TokenAuthMiddleware in production hits ValidateToken's `SELECT ... FROM
// tokens` against a table that was never created, and the query fails with
// "no such table: tokens" - which TokenAuthMiddleware currently maps to a
// generic 401 "invalid user token", silently masking a schema-mismatch bug
// as an ordinary auth failure.
//
// This test proves the failure mode directly: it seeds usersDB with the
// real production UsersSchema (no hand-rolled "tokens" table) and asserts
// that querying "tokens" fails, which is the root cause of every usr_ token
// being rejected in production regardless of validity.
func TestTokenAuthMiddleware_UserTokenFailsAgainstRealSchema(t *testing.T) {
	_, usersDB := openTokenAuthTestDBs(t)

	var count int
	err := usersDB.QueryRow("SELECT COUNT(*) FROM tokens").Scan(&count)
	if err == nil {
		t.Fatalf("SELECT FROM tokens unexpectedly succeeded (count=%d) against the real "+
			"UsersSchema - if this now passes, TokenModelV2's schema mismatch "+
			"(token_v2.go queries table 'tokens', UsersSchema defines 'user_tokens') "+
			"has been fixed and this regression test should be updated/removed", count)
	}
}
