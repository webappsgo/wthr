package server

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Embed all templates and static files into the binary
// Following TEMPLATE.md specification lines 802-816

//go:embed all:template
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// GetTemplatesFS returns the embedded templates filesystem
func GetTemplatesFS() embed.FS {
	return templatesFS
}

// GetStaticFS returns the embedded static files filesystem
func GetStaticFS() embed.FS {
	return staticFS
}

// GetStaticSubFS returns the static files as a sub-filesystem for http.FileServer
func GetStaticSubFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

// LoadTemplates loads all templates from the embedded filesystem
//
// The templates reference a shared set of helper functions (upper, lower,
// title, add, sub, t, tf). Placeholder implementations are registered here
// so parsing succeeds standalone; callers that need real i18n translation
// output should call Funcs() on the returned template to override "t"/"tf"
// with a translation-service-backed implementation before Execute.
func LoadTemplates() (*template.Template, error) {
	funcMap := template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string { return cases.Title(language.English).String(s) },
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"t": func(_, key string) string {
			return key
		},
		"tf": func(_, key string, _ ...string) string {
			return key
		},
	}
	return template.New("").Funcs(funcMap).ParseFS(templatesFS, "template/**/*.tmpl")
}
