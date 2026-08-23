package graphql

import (
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

// GetDarkThemeCSS returns CSS for GraphiQL dark theme (Dracula)
// Per AI.md: Project-wide theme system with Dracula dark theme as default
func GetDarkThemeCSS() string {
	return `
		/* Dracula Dark Theme for GraphiQL */
		body {
			background-color: #282a36;
			color: #f8f8f2;
		}

		.graphiql-container {
			background-color: #282a36;
		}

		.topBar {
			background-color: #44475a;
			border-bottom: 1px solid #6272a4;
		}

		.title {
			color: #8be9fd;
		}

		.execute-button {
			background-color: #50fa7b;
			color: #282a36;
			border: none;
		}

		.execute-button:hover {
			background-color: #6be99f;
		}

		.toolbar-button {
			background-color: #6272a4;
			color: #f8f8f2;
		}

		.toolbar-button:hover {
			background-color: #8be9fd;
			color: #282a36;
		}

		.CodeMirror {
			background-color: #282a36;
			color: #f8f8f2;
		}

		.CodeMirror-gutters {
			background-color: #44475a;
			border-right: 1px solid #6272a4;
		}

		.CodeMirror-linenumber {
			color: #6272a4;
		}

		.CodeMirror-cursor {
			border-left: 1px solid #f8f8f2;
		}

		.CodeMirror-selected {
			background-color: #44475a;
		}

		.cm-keyword {
			color: #ff79c6;
		}

		.cm-def {
			color: #50fa7b;
		}

		.cm-variable {
			color: #f8f8f2;
		}

		.cm-property {
			color: #8be9fd;
		}

		.cm-string {
			color: #f1fa8c;
		}

		.cm-number {
			color: #bd93f9;
		}

		.cm-atom {
			color: #bd93f9;
		}

		.cm-comment {
			color: #6272a4;
			font-style: italic;
		}

		.result-window {
			background-color: #282a36;
			color: #f8f8f2;
		}

		.history-contents {
			background-color: #282a36;
		}

		.doc-explorer-contents {
			background-color: #282a36;
			color: #f8f8f2;
		}

		.doc-explorer-title {
			color: #8be9fd;
		}

		.doc-type-description {
			color: #f8f8f2;
		}

		.doc-category-title {
			color: #bd93f9;
		}
	`
}

// GetLightThemeCSS returns CSS for GraphiQL light theme
func GetLightThemeCSS() string {
	return `
		/* Light Theme for GraphiQL */
		body {
			background-color: #ffffff;
			color: #333333;
		}

		.graphiql-container {
			background-color: #ffffff;
		}

		.topBar {
			background-color: #f5f5f5;
			border-bottom: 1px solid #e0e0e0;
		}

		.title {
			color: #0066cc;
		}

		.execute-button {
			background-color: #00aa00;
			color: #ffffff;
			border: none;
		}

		.execute-button:hover {
			background-color: #00cc00;
		}

		.toolbar-button {
			background-color: #e0e0e0;
			color: #333333;
		}

		.toolbar-button:hover {
			background-color: #0066cc;
			color: #ffffff;
		}

		.CodeMirror {
			background-color: #ffffff;
			color: #333333;
		}

		.CodeMirror-gutters {
			background-color: #f5f5f5;
			border-right: 1px solid #e0e0e0;
		}

		.CodeMirror-linenumber {
			color: #999999;
		}

		.CodeMirror-cursor {
			border-left: 1px solid #333333;
		}

		.CodeMirror-selected {
			background-color: #e5e5e5;
		}

		.result-window {
			background-color: #ffffff;
			color: #333333;
		}

		.history-contents {
			background-color: #ffffff;
		}

		.doc-explorer-contents {
			background-color: #ffffff;
			color: #333333;
		}

		.doc-explorer-title {
			color: #0066cc;
		}
	`
}
