package graphql

import (
	"context"
	"database/sql"
	"fmt"

	gqlhandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/gin-gonic/gin"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"
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

// RegisterRoutes registers all GraphQL routes
// Per AI.md specification: /graphql for queries, GraphiQL playground at /graphql with GET
func RegisterRoutes(router *gin.Engine, resolver *Resolver) {
	srv := NewServer(resolver)

	// GraphQL endpoint for POST requests (actual queries)
	router.POST("/graphql", GraphQLHandler(srv))

	// GraphiQL playground for GET requests (interactive UI)
	router.GET("/graphql", PlaygroundHandler("/graphql"))
}

// GraphQLHandler wraps the gqlgen handler for Gin.
func GraphQLHandler(h *gqlhandler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, err := buildGraphQLAuthContext(c)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		c.Request = c.Request.WithContext(authCtx)
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// PlaygroundHandler serves the GraphiQL playground with theme support.
func PlaygroundHandler(endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get theme preference
		theme := GetTheme(c)

		// Serve GraphiQL playground with theme
		if theme == "dark" {
			// Custom GraphiQL with dark theme
			playgroundHandler := playground.Handler("GraphQL Playground", endpoint)
			playgroundHandler.ServeHTTP(c.Writer, c.Request)
		} else if theme == "light" {
			// Custom GraphiQL with light theme
			playgroundHandler := playground.Handler("GraphQL Playground", endpoint)
			playgroundHandler.ServeHTTP(c.Writer, c.Request)
		} else {
			// Auto theme (let browser decide)
			playgroundHandler := playground.Handler("GraphQL Playground", endpoint)
			playgroundHandler.ServeHTTP(c.Writer, c.Request)
		}
	}
}

func buildGraphQLAuthContext(c *gin.Context) (context.Context, error) {
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "request_ip", c.ClientIP())
	ctx = context.WithValue(ctx, "client_ip", c.ClientIP())
	ctx = context.WithValue(ctx, "request_host", c.Request.Host)

	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	ctx = context.WithValue(ctx, "request_scheme", scheme)

	userAgent := strings.TrimSpace(c.Request.UserAgent())
	if userAgent != "" {
		ctx = context.WithValue(ctx, "request_user_agent", userAgent)
	}

	if userValue, exists := c.Get(middleware.UserContextKey); exists {
		if user, ok := userValue.(*models.User); ok && user != nil {
			if sessionValue, sessionExists := c.Get(middleware.SessionContextKey); sessionExists {
				if session, ok := sessionValue.(*models.Session); ok && session != nil {
					return withGraphQLUserSessionContext(ctx, user, session), nil
				}
			}
			return withGraphQLUserContext(ctx, user), nil
		}
	}

	if adminIDValue, exists := c.Get("admin_id"); exists {
		if adminID, ok := adminIDValue.(int); ok && adminID > 0 {
			return withGraphQLAdminContext(ctx, adminID)
		}
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "" {
		return buildGraphQLTokenContext(ctx, authHeader)
	}

	if adminSessionID, err := c.Cookie("admin_session"); err == nil && strings.TrimSpace(adminSessionID) != "" {
		return buildGraphQLAdminSessionContext(ctx, adminSessionID)
	}

	if userSessionID, err := c.Cookie(middleware.SessionCookieName); err == nil && strings.TrimSpace(userSessionID) != "" {
		return buildGraphQLUserSessionContext(ctx, userSessionID)
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

	var adminID int
	err := serverDB.QueryRow(`
		SELECT admin_id
		FROM server_admin_sessions
		WHERE id = ? AND expires_at > CURRENT_TIMESTAMP
	`, sessionID).Scan(&adminID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ctx, nil
		}
		return nil, fmt.Errorf("failed to load admin session: %w", err)
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
	ctx = context.WithValue(ctx, "admin_id", adminID)
	ctx = context.WithValue(ctx, "user_role", "admin")
	if email != "" {
		ctx = context.WithValue(ctx, "admin_email", email)
	}
	return ctx
}

func withGraphQLUserContext(ctx context.Context, user *models.User) context.Context {
	ctx = context.WithValue(ctx, "user_id", int(user.ID))
	ctx = context.WithValue(ctx, "user_role", user.Role)
	return ctx
}

func withGraphQLUserSessionContext(ctx context.Context, user *models.User, session *models.Session) context.Context {
	ctx = withGraphQLUserContext(ctx, user)
	ctx = context.WithValue(ctx, "user_session", session)
	if session != nil {
		ctx = context.WithValue(ctx, "user_session_id", session.ID)
	}
	return ctx
}
