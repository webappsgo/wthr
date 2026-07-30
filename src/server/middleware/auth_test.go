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
	models "github.com/webappsgo/wthr/src/server/model"
	_ "modernc.org/sqlite"
)

// openAuthTestUsersDB opens an in-memory users.db seeded with the real
// UsersSchema and installs it as the global dual DB. This is required
// because AuthMiddleware constructs models.UserModel{DB: db} /
// models.SessionModel{DB: db} / models.TokenModel{DB: db} from the *sql.DB
// argument it's given, but every method on those model types (Create,
// GetByID, GetByToken, ...) ignores the struct's DB field entirely and
// queries database.GetUsersDB() (the package-level global) instead - see
// src/server/model/user.go:176 and session.go:70. So the db argument
// AuthMiddleware/RequireAuth/OptionalAuth accept is effectively dead;
// database.SetGlobalDualDB must be set for any of this to work, in
// production as well as in tests.
func openAuthTestUsersDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:auth_users_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open users sqlite: %v", err)
	}
	if _, err := db.Exec(database.UsersSchema); err != nil {
		t.Fatalf("apply UsersSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	database.SetGlobalDualDB(&database.DualDB{Users: db})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	return db
}

// TestAuthMiddleware_RequiredRejectsUnauthenticatedJSON verifies a required
// auth request with no session/token and a non-HTML Accept header gets a
// 401 JSON response rather than a redirect.
func TestAuthMiddleware_RequiredRejectsUnauthenticatedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usersDB := openAuthTestUsersDB(t)

	router := gin.New()
	router.Use(RequireAuth(usersDB))
	router.GET("/users/settings", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	req.Header.Set("Accept", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for unauthenticated required-auth API request", w.Code)
	}
}

// TestAuthMiddleware_RequiredRedirectsBrowser verifies an HTML request with
// no auth is redirected to the login page instead of getting a JSON 401.
func TestAuthMiddleware_RequiredRedirectsBrowser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usersDB := openAuthTestUsersDB(t)

	router := gin.New()
	router.Use(RequireAuth(usersDB))
	router.GET("/users/settings", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	req.Header.Set("Accept", "text/html")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect for unauthenticated browser request", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/login" {
		t.Errorf("Location = %q, want /server/auth/login", loc)
	}
}

// TestAuthMiddleware_OptionalAllowsUnauthenticated verifies OptionalAuth
// lets an unauthenticated request through to the handler.
func TestAuthMiddleware_OptionalAllowsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usersDB := openAuthTestUsersDB(t)

	router := gin.New()
	router.Use(OptionalAuth(usersDB))
	router.GET("/", func(c *gin.Context) {
		if IsAuthenticated(c) {
			t.Error("IsAuthenticated() = true, want false for anonymous request")
		}
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for optional-auth anonymous request", w.Code)
	}
}

// TestAuthMiddleware_ValidSessionCookieAuthenticates seeds a real user and
// session (via the real model layer, against the global users DB) and
// verifies a request carrying that session cookie is authenticated and the
// user/session context keys are populated for downstream handlers.
func TestAuthMiddleware_ValidSessionCookieAuthenticates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usersDB := openAuthTestUsersDB(t)

	userModel := &models.UserModel{DB: usersDB}
	user, err := userModel.Create("alice", "alice@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sessionModel := &models.SessionModel{DB: usersDB}
	session, err := sessionModel.Create(user.ID, 3600)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	router := gin.New()
	router.Use(RequireAuth(usersDB))
	router.GET("/users/settings", func(c *gin.Context) {
		got, ok := GetCurrentUser(c)
		if !ok {
			t.Error("GetCurrentUser: not found in context, want the seeded user")
		} else if got.Username != "alice" {
			t.Errorf("GetCurrentUser().Username = %q, want alice", got.Username)
		}
		if _, ok := GetCurrentSession(c); !ok {
			t.Error("GetCurrentSession: not found in context, want the seeded session")
		}
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session.ID})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for valid session cookie", w.Code)
	}
}

// TestAuthMiddleware_InvalidSessionCookieRejected verifies a session cookie
// value that doesn't match any stored session hash is treated as
// unauthenticated (not a crash, not a false-positive auth).
func TestAuthMiddleware_InvalidSessionCookieRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	usersDB := openAuthTestUsersDB(t)

	router := gin.New()
	router.Use(RequireAuth(usersDB))
	router.GET("/users/settings", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "totally-bogus-session-token"})
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for invalid session cookie", w.Code)
	}
}

// TestRequireAdmin covers both the missing-user and non-admin-role reject
// paths plus the admin-role accept path.
func TestRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setContext func(c *gin.Context)
		wantStatus int
	}{
		{
			name:       "no user in context",
			setContext: func(c *gin.Context) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "regular user role",
			setContext: func(c *gin.Context) {
				c.Set(UserContextKey, &models.User{ID: 1, Username: "bob", Role: "user"})
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "admin role",
			setContext: func(c *gin.Context) {
				c.Set(UserContextKey, &models.User{ID: 2, Username: "root", Role: "admin"})
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(tt.setContext)
			router.Use(RequireAdmin())
			router.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestIsAdmin covers the true/false boundary for the IsAdmin helper.
func TestIsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if IsAdmin(c) {
		t.Error("IsAdmin() = true, want false with no user in context")
	}

	c.Set(UserContextKey, &models.User{ID: 1, Role: "user"})
	if IsAdmin(c) {
		t.Error("IsAdmin() = true, want false for a regular-role user")
	}

	c.Set(UserContextKey, &models.User{ID: 2, Role: "admin"})
	if !IsAdmin(c) {
		t.Error("IsAdmin() = false, want true for an admin-role user")
	}
}

// TestRestrictAdminToAdminRoutes_ClearsAdminContextOnNonAdminRoute verifies
// an admin user hitting a non-admin, non-exempt path has their user/session
// context wiped so they're treated as anonymous.
//
// This documents a real production bug: auth.go:192-193 "clears" the admin
// by calling c.Set(UserContextKey, nil) / c.Set(SessionContextKey, nil)
// rather than removing the key. gin.Context has no Delete, so the key still
// "exists" in the context map with a nil value. IsAuthenticated (auth.go:
// 149-152) only checks existence via c.Get's second return value, so it
// still reports true for an admin that was supposedly cleared to anonymous -
// even though GetCurrentUser correctly returns (nil, false) for the same
// context, because its type assertion on a bare nil interface fails. Any
// caller that gates on IsAuthenticated (rather than GetCurrentUser) after
// RestrictAdminToAdminRoutes runs will incorrectly treat a "cleared" admin
// as still logged in. This test encodes the CORRECT expected behavior and is
// expected to FAIL against the current implementation.
func TestRestrictAdminToAdminRoutes_ClearsAdminContextOnNonAdminRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(UserContextKey, &models.User{ID: 1, Role: "admin"})
		c.Next()
	})
	router.Use(RestrictAdminToAdminRoutes())
	router.GET("/weather/today", func(c *gin.Context) {
		if IsAuthenticated(c) {
			t.Error("admin user context should have been cleared on a non-admin route")
		}
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/weather/today", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestRestrictAdminToAdminRoutes_ExemptsAPIRoutes verifies /api paths are
// skipped entirely (admin context left untouched).
func TestRestrictAdminToAdminRoutes_ExemptsAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(UserContextKey, &models.User{ID: 1, Role: "admin"})
		c.Next()
	})
	router.Use(RestrictAdminToAdminRoutes())
	router.GET("/api/v1/weather", func(c *gin.Context) {
		if !IsAuthenticated(c) {
			t.Error("admin user context should be preserved on an exempted /api route")
		}
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestBlockAdminFromUserRoutes covers: non-/users route passes through
// untouched, admin hitting /users gets blocked (JSON for API, redirect for
// HTML), and a regular user hitting /users passes through.
func TestBlockAdminFromUserRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		role       string
		accept     string
		wantStatus int
	}{
		{"non-users route always passes", "/weather/today", "admin", "application/json", http.StatusOK},
		{"admin blocked from users route (API)", "/users/settings", "admin", "application/json", http.StatusForbidden},
		{"admin blocked from users route (HTML)", "/users/settings", "admin", "text/html", http.StatusFound},
		{"regular user allowed on users route", "/users/settings", "user", "application/json", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(UserContextKey, &models.User{ID: 1, Role: tt.role})
				c.Next()
			})
			router.Use(BlockAdminFromUserRoutes())
			router.GET(tt.path, func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", tt.accept)
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
