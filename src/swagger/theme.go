package swagger

import (
	"html/template"
	"net/http"
)

// Theme represents the UI theme preference
type Theme string

const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
	ThemeAuto  Theme = "auto"
)

// GetTheme retrieves the theme preference from cookie, query param, or returns default
// Per AI.md specification: dark is the default theme
func GetTheme(r *http.Request) Theme {
	// Check query parameter first (for theme switching)
	if theme := r.URL.Query().Get("theme"); theme != "" {
		switch theme {
		case "dark", "light", "auto":
			return Theme(theme)
		}
	}

	// Check cookie
	if cookie, err := r.Cookie("theme"); err == nil {
		switch cookie.Value {
		case "dark", "light", "auto":
			return Theme(cookie.Value)
		}
	}

	// Default to dark theme per AI.md specification
	return ThemeDark
}

// GetDarkThemeCSS returns CSS for Swagger UI dark theme (Dracula)
// Per AI.md: Project-wide theme system with Dracula dark theme as default
func GetDarkThemeCSS() template.CSS {
	return template.CSS(`
		/* Dracula Dark Theme for Swagger UI */
		.swagger-ui {
			background-color: #282a36;
			color: #f8f8f2;
		}

		.swagger-ui .topbar {
			background-color: #44475a;
			border-bottom: 1px solid #6272a4;
		}

		.swagger-ui .info {
			background-color: #282a36;
		}

		.swagger-ui .info .title {
			color: #8be9fd;
		}

		.swagger-ui .info .description,
		.swagger-ui .info p {
			color: #f8f8f2;
		}

		.swagger-ui .opblock {
			background-color: #44475a;
			border: 1px solid #6272a4;
		}

		.swagger-ui .opblock .opblock-summary {
			border-color: #6272a4;
		}

		.swagger-ui .opblock .opblock-summary-path {
			color: #50fa7b;
		}

		.swagger-ui .opblock .opblock-summary-description {
			color: #f8f8f2;
		}

		.swagger-ui .opblock.opblock-get {
			background-color: rgba(139, 233, 253, 0.1);
			border-color: #8be9fd;
		}

		.swagger-ui .opblock.opblock-post {
			background-color: rgba(80, 250, 123, 0.1);
			border-color: #50fa7b;
		}

		.swagger-ui .opblock.opblock-put {
			background-color: rgba(255, 184, 108, 0.1);
			border-color: #ffb86c;
		}

		.swagger-ui .opblock.opblock-delete {
			background-color: rgba(255, 85, 85, 0.1);
			border-color: #ff5555;
		}

		.swagger-ui .opblock.opblock-patch {
			background-color: rgba(189, 147, 249, 0.1);
			border-color: #bd93f9;
		}

		.swagger-ui .btn {
			background-color: #6272a4;
			color: #f8f8f2;
			border: 1px solid #44475a;
		}

		.swagger-ui .btn:hover {
			background-color: #8be9fd;
			color: #282a36;
		}

		.swagger-ui .btn.authorize {
			background-color: #50fa7b;
			color: #282a36;
		}

		.swagger-ui .btn.execute {
			background-color: #8be9fd;
			color: #282a36;
		}

		.swagger-ui input,
		.swagger-ui textarea,
		.swagger-ui select {
			background-color: #44475a;
			color: #f8f8f2;
			border: 1px solid #6272a4;
		}

		.swagger-ui .model-box {
			background-color: #44475a;
			border: 1px solid #6272a4;
		}

		.swagger-ui .model-title {
			color: #8be9fd;
		}

		.swagger-ui .prop-type {
			color: #bd93f9;
		}

		.swagger-ui .response-col_status {
			color: #50fa7b;
		}

		.swagger-ui .response-col_description {
			color: #f8f8f2;
		}

		.swagger-ui table thead tr th {
			background-color: #44475a;
			color: #f8f8f2;
			border-bottom: 1px solid #6272a4;
		}

		.swagger-ui table tbody tr td {
			border-bottom: 1px solid #44475a;
			color: #f8f8f2;
		}
	`)
}

// GetLightThemeCSS returns CSS for Swagger UI light theme
func GetLightThemeCSS() template.CSS {
	return template.CSS(`
		/* Light Theme for Swagger UI */
		.swagger-ui {
			background-color: #ffffff;
			color: #333333;
		}

		.swagger-ui .topbar {
			background-color: #f5f5f5;
			border-bottom: 1px solid #e0e0e0;
		}

		.swagger-ui .info {
			background-color: #ffffff;
		}

		.swagger-ui .info .title {
			color: #0066cc;
		}

		.swagger-ui .info .description,
		.swagger-ui .info p {
			color: #333333;
		}

		.swagger-ui .opblock {
			background-color: #fafafa;
			border: 1px solid #e0e0e0;
		}

		.swagger-ui .opblock .opblock-summary {
			border-color: #e0e0e0;
		}

		.swagger-ui .opblock .opblock-summary-path {
			color: #006600;
		}

		.swagger-ui .opblock .opblock-summary-description {
			color: #333333;
		}

		.swagger-ui .btn {
			background-color: #e0e0e0;
			color: #333333;
			border: 1px solid #cccccc;
		}

		.swagger-ui .btn:hover {
			background-color: #0066cc;
			color: #ffffff;
		}

		.swagger-ui .btn.authorize {
			background-color: #00aa00;
			color: #ffffff;
		}

		.swagger-ui .btn.execute {
			background-color: #0066cc;
			color: #ffffff;
		}

		.swagger-ui input,
		.swagger-ui textarea,
		.swagger-ui select {
			background-color: #ffffff;
			color: #333333;
			border: 1px solid #cccccc;
		}

		.swagger-ui .model-box {
			background-color: #fafafa;
			border: 1px solid #e0e0e0;
		}

		.swagger-ui .model-title {
			color: #0066cc;
		}

		.swagger-ui .prop-type {
			color: #8800cc;
		}

		.swagger-ui table thead tr th {
			background-color: #f5f5f5;
			color: #333333;
			border-bottom: 1px solid #e0e0e0;
		}

		.swagger-ui table tbody tr td {
			border-bottom: 1px solid #f5f5f5;
			color: #333333;
		}
	`)
}
