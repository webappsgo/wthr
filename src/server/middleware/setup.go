package middleware

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

// htmlTemplates is the page-template set used by SetupTokenRequired to
// render the setup-token entry form and the shared error page. gin.Engine
// carried its own template registry (SetHTMLTemplate); net/http has no
// built-in equivalent, so Phase 6 (main.go/router bootstrap) is expected to
// call SetHTMLTemplates once at startup with the loaded page/*.tmpl and
// admin/*.tmpl set, mirroring what router.SetHTMLTemplate(...) did.
var htmlTemplates *template.Template

// SetHTMLTemplates registers the page-template set this middleware renders
// from. Must be called during server bootstrap before SetupTokenRequired
// serves any request that reaches its HTML-rendering branches.
func SetHTMLTemplates(t *template.Template) {
	htmlTemplates = t
}

// renderHTML renders the named template from htmlTemplates, mirroring
// gin.Context.HTML(status, name, data). If no template set has been
// registered (e.g. a test that never called SetHTMLTemplates), it falls
// back to writing the status code with no body rather than panicking.
func renderHTML(w http.ResponseWriter, status int, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if htmlTemplates == nil {
		return
	}
	_ = htmlTemplates.ExecuteTemplate(w, name, data)
}

// requestLang returns the language the shared i18n middleware resolved for
// this request, defaulting to English when it did not run (AI.md PART 31).
func requestLang(r *http.Request) string {
	value, ok := reqctx.GetValue(r.Context(), "lang")
	if !ok {
		return "en"
	}
	lang, ok := value.(string)
	if !ok || lang == "" {
		return "en"
	}
	return lang
}

// setupTranslate resolves a translation key for the active request language.
// The shared i18n middleware stores the resolved language on the request
// context for every route, so the setup pages get the same fallback chain as
// the rest of the app (AI.md PART 31). A missing global instance falls back
// to the key itself rather than failing the response.
func setupTranslate(r *http.Request, key string) string {
	instance := i18n.GetGlobalI18n()
	if instance == nil {
		return key
	}
	return instance.T(requestLang(r), key)
}

// SetupTokenRequired shows setup token entry form at /server/admin when no admin exists
// AI.md: User navigates to /server/admin → User enters setup token → Redirect to /server/{admin_path}/config/setup
// AI.md: Admin panel (/server/admin) - YES (requires setup token) - accessible before setup
func SetupTokenRequired(cfg *config.AppConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqPath := r.URL.Path
			adminPath := "/server/" + cfg.GetAdminPath()

			// Only apply to admin routes
			if !strings.HasPrefix(reqPath, adminPath) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip check for setup routes (setup wizard handles its own auth)
			setupPath := adminPath + "/config/setup"
			if strings.HasPrefix(reqPath, setupPath) {
				next.ServeHTTP(w, r)
				return
			}

			// Check if any admin exists
			var count int
			err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// If admin exists, setup is complete - use normal auth flow
			if count > 0 {
				next.ServeHTTP(w, r)
				return
			}

			// No admin exists - check for setup token file
			configDir := path.GetConfigDir()
			if !util.SetupTokenExists(configDir) {
				// No setup token file - setup was somehow skipped, show error
				renderHTML(w, http.StatusServiceUnavailable, "page/error.tmpl", map[string]interface{}{
					"error":   setupTranslate(r, "errors.setup.incomplete"),
					"message": setupTranslate(r, "errors.setup.restart_required"),
				})
				return
			}

			// Check if user has valid setup token cookie
			var tokenVerified string
			if cookie, err := r.Cookie("setup_token_verified"); err == nil {
				tokenVerified = cookie.Value
			}
			if tokenVerified == "true" {
				// Token verified - redirect to setup wizard to create admin account
				http.Redirect(w, r, setupPath, http.StatusFound)
				return
			}

			// No admin, setup token exists, no verified cookie. The setup-token
			// form only exists at the admin root, so any other admin sub-path
			// redirects there instead of rendering the form at a URL the
			// wizard does not own (AI.md PART 17: admin panel reachable only
			// by typing the URL directly; PART 29: unauthenticated admin
			// access must redirect, never serve 200).
			if reqPath != adminPath {
				http.Redirect(w, r, adminPath, http.StatusFound)
				return
			}

			// No admin, setup token exists, no verified cookie - show token entry form at /admin
			// AI.md: Step 2: User navigates to /admin → Step 3: User enters setup token
			title := "Weather"
			cfgLoaded, _ := config.LoadConfig()
			if cfgLoaded != nil && cfgLoaded.Server.Branding.Title != "" {
				title = cfgLoaded.Server.Branding.Title
			}

			renderHTML(w, http.StatusOK, "admin/setup_token.tmpl", map[string]interface{}{
				"title":      title + " - " + setupTranslate(r, "admin.setup_token.page_title"),
				"lang":       requestLang(r),
				"admin_path": adminPath,
				"branding": map[string]interface{}{
					"Title": title,
				},
			})
		})
	}
}

// BlockSetupAfterComplete blocks access to setup routes after server setup is complete
// AI.md: Setup token file deleted after successful setup completion
func BlockSetupAfterComplete(cfg *config.AppConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if any admin exists - setup is complete when admin exists
			var count int
			err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
			if err == nil && count > 0 {
				// Admin exists - setup complete, redirect to admin dashboard
				adminPath := "/server/" + cfg.GetAdminPath()
				http.Redirect(w, r, adminPath+"/dashboard", http.StatusFound)
				return
			}

			// Check if setup token file still exists
			configDir := path.GetConfigDir()
			if !util.SetupTokenExists(configDir) {
				// No setup token file and no admin - should not happen
				adminPath := "/server/" + cfg.GetAdminPath()
				http.Redirect(w, r, adminPath, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSetupTokenVerified ensures the setup token has been verified before accessing setup wizard
// AI.md: Step 3: User enters setup token → Step 4: Redirect to /server/{admin_path}/config/setup
func RequireSetupTokenVerified(cfg *config.AppConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for setup_token_verified cookie
			var tokenVerified string
			if cookie, err := r.Cookie("setup_token_verified"); err == nil {
				tokenVerified = cookie.Value
			}
			if tokenVerified != "true" {
				// No verified token - redirect to /server/admin to enter token
				adminPath := "/server/" + cfg.GetAdminPath()
				http.Redirect(w, r, adminPath, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// BlockSetupAfterAdminExists blocks access to admin setup if admin account already exists
func BlockSetupAfterAdminExists(cfg *config.AppConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if admin exists in server_admin_credentials
			var count int
			err := database.QueryRowContext(context.Background(), database.GetServerDB(), database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM server_admin_credentials").Scan(&count)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "SERVER_ERROR", "Internal server error")
				return
			}

			// If admin exists, redirect to admin dashboard
			if count > 0 {
				adminPath := "/server/" + cfg.GetAdminPath()
				http.Redirect(w, r, adminPath+"/dashboard", http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
