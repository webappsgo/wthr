package graphql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	gqlhandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// contextKey is an unexported type used for context keys
type contextKey string

const (
	ctxKeyRequestIP        contextKey = "request_ip"
	ctxKeyClientIP         contextKey = "client_ip"
	ctxKeyRequestHost      contextKey = "request_host"
	ctxKeyRequestScheme    contextKey = "request_scheme"
	ctxKeyRequestUserAgent contextKey = "request_user_agent"
	ctxKeyAdminID          contextKey = "admin_id"
	ctxKeyUserRole         contextKey = "user_role"
	ctxKeyAdminEmail       contextKey = "admin_email"
	ctxKeyUserID           contextKey = "user_id"
	ctxKeyUserSession      contextKey = "user_session"
	ctxKeyUserSessionID    contextKey = "user_session_id"
)

// NewServer creates a gqlgen GraphQL server for the provided resolver tree.
func NewServer(resolver *Resolver) *gqlhandler.Server {
	srv := gqlhandler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	srv.AddTransport(transport.Websocket{KeepAlivePingInterval: 10 * time.Second})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{
		MaxUploadSize: 2 * 1024 * 1024,
		MaxMemory:     2 * 1024 * 1024,
	})
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](100)})
	return srv
}

// GraphQLHandler wraps the gqlgen handler for net/http.
func GraphQLHandler(h *gqlhandler.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx, err := buildGraphQLAuthContext(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		r = r.WithContext(authCtx)
		h.ServeHTTP(w, r)
	}
}

// PlaygroundHandler serves the GraphiQL playground with theme support, from
// this binary's own embedded assets (see playground.go) rather than a CDN.
func PlaygroundHandler(endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		theme := GetTheme(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderPlaygroundHTML(theme, endpoint)))
	}
}

func buildGraphQLAuthContext(r *http.Request) (context.Context, error) {
	ctx := r.Context()
	clientIP := util.TrustedGetClientIP(r)
	ctx = context.WithValue(ctx, ctxKeyRequestIP, clientIP)
	ctx = context.WithValue(ctx, ctxKeyClientIP, clientIP)
	ctx = context.WithValue(ctx, ctxKeyRequestHost, r.Host)

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	ctx = context.WithValue(ctx, ctxKeyRequestScheme, scheme)

	userAgent := strings.TrimSpace(r.UserAgent())
	if userAgent != "" {
		ctx = context.WithValue(ctx, ctxKeyRequestUserAgent, userAgent)
	}

	if userValue, exists := reqctx.Get(r.Context(), middleware.UserContextKey); exists {
		if user, ok := userValue.(*models.User); ok && user != nil {
			if sessionValue, sessionExists := reqctx.Get(r.Context(), middleware.SessionContextKey); sessionExists {
				if session, ok := sessionValue.(*models.Session); ok && session != nil {
					return withGraphQLUserSessionContext(ctx, user, session), nil
				}
			}
			return withGraphQLUserContext(ctx, user), nil
		}
	}

	if adminIDValue, exists := reqctx.Get(r.Context(), "admin_id"); exists {
		if adminID, ok := adminIDValue.(int); ok && adminID > 0 {
			return withGraphQLAdminContext(ctx, adminID)
		}
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		return buildGraphQLTokenContext(ctx, authHeader)
	}

	if adminSessionCookie, err := r.Cookie("admin_session"); err == nil && strings.TrimSpace(adminSessionCookie.Value) != "" {
		return buildGraphQLAdminSessionContext(ctx, adminSessionCookie.Value)
	}

	if userSessionCookie, err := r.Cookie(middleware.SessionCookieName); err == nil && strings.TrimSpace(userSessionCookie.Value) != "" {
		return buildGraphQLUserSessionContext(ctx, userSessionCookie.Value)
	}

	return ctx, nil
}

func buildGraphQLTokenContext(ctx context.Context, authHeader string) (context.Context, error) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, fmt.Errorf("invalid authorization format")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return nil, fmt.Errorf("missing bearer token")
	}

	switch middleware.DetectTokenType(token) {
	case middleware.TokenTypeAdmin:
		adminModel := &models.AdminModel{DB: database.GetServerDB()}
		admin, err := adminModel.GetByAPIToken(token)
		if err != nil {
			return nil, fmt.Errorf("invalid admin token")
		}
		return withGraphQLAdminValues(ctx, int(admin.ID), admin.Email), nil
	case middleware.TokenTypeUser:
		tokenModel := &models.TokenModelV2{DB: database.GetUsersDB()}
		validatedToken, err := tokenModel.ValidateToken(token)
		if err != nil {
			return nil, fmt.Errorf("invalid user token")
		}

		userModel := &models.UserModel{DB: database.GetUsersDB()}
		user, err := userModel.GetByID(validatedToken.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("user not found")
		}
		return withGraphQLUserContext(ctx, user), nil
	default:
		return nil, fmt.Errorf("unsupported authorization token")
	}
}

func buildGraphQLAdminSessionContext(ctx context.Context, sessionID string) (context.Context, error) {
	serverDB := database.GetServerDB()
	if serverDB == nil {
		return ctx, nil
	}

	// expires_at is fetched with the row and judged in Go rather than compared
	// in SQL. "expires_at > CURRENT_TIMESTAMP" is a lexicographic TEXT
	// comparison over a column that may hold canonical UTC text or the
	// local-zone time.Time.String() form an older build wrote, so a session
	// stored in a zone behind UTC authenticated long after it had expired,
	// while one stored ahead of UTC was rejected while still valid.
	var adminID int
	var storedExpiresAt interface{}
	err := database.QueryRowContext(ctx, serverDB, database.TimeoutSimpleSelect, `
		SELECT admin_id, expires_at
		FROM server_admin_sessions
		WHERE id = ?
	`, sessionID).Scan(&adminID, &storedExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return ctx, nil
		}
		return nil, fmt.Errorf("failed to load admin session: %w", err)
	}

	// Fail closed: an expired, NULL or unparseable expires_at leaves the
	// context unauthenticated, exactly as a missing row already did.
	if !dbtime.IsAfter(storedExpiresAt, time.Now().UTC()) {
		return ctx, nil
	}

	return withGraphQLAdminContext(ctx, adminID)
}

func buildGraphQLUserSessionContext(ctx context.Context, sessionID string) (context.Context, error) {
	sessionModel := &models.SessionModel{DB: database.GetUsersDB()}
	session, err := sessionModel.GetByID(sessionID)
	if err != nil {
		return ctx, nil
	}

	userModel := &models.UserModel{DB: database.GetUsersDB()}
	user, err := userModel.GetByID(int64(session.UserID))
	if err != nil {
		return nil, fmt.Errorf("failed to load authenticated user: %w", err)
	}

	return withGraphQLUserSessionContext(ctx, user, session), nil
}

func withGraphQLAdminContext(ctx context.Context, adminID int) (context.Context, error) {
	adminModel := &models.AdminModel{DB: database.GetServerDB()}
	admin, err := adminModel.GetByID(int64(adminID))
	if err != nil {
		return nil, fmt.Errorf("failed to load authenticated admin: %w", err)
	}

	return withGraphQLAdminValues(ctx, adminID, admin.Email), nil
}

func withGraphQLAdminValues(ctx context.Context, adminID int, email string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAdminID, adminID)
	ctx = context.WithValue(ctx, ctxKeyUserRole, "admin")
	if email != "" {
		ctx = context.WithValue(ctx, ctxKeyAdminEmail, email)
	}
	return ctx
}

func withGraphQLUserContext(ctx context.Context, user *models.User) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, int(user.ID))
	ctx = context.WithValue(ctx, ctxKeyUserRole, user.Role)
	return ctx
}

func withGraphQLUserSessionContext(ctx context.Context, user *models.User, session *models.Session) context.Context {
	ctx = withGraphQLUserContext(ctx, user)
	ctx = context.WithValue(ctx, ctxKeyUserSession, session)
	if session != nil {
		ctx = context.WithValue(ctx, ctxKeyUserSessionID, session.ID)
	}
	return ctx
}
