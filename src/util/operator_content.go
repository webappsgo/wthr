package util

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/webappsgo/wthr/src/config"
)

var contactLinkPattern = regexp.MustCompile(`(?i)href\s*=\s*"/server/contact(?:[/?#][^"]*)?"|href\s*=\s*'/server/contact(?:[/?#][^']*)?'`)

// operatorHTML returns the configured branding fragment after language
// selection, format rendering, variable expansion and HTML sanitization.
func operatorHTML(cfg *config.AppConfig, name, language string, variables map[string]string) template.HTML {
	if cfg == nil {
		return ""
	}
	entries := map[string]config.OperatorPageContentConfig{
		"header":  cfg.Server.Branding.Content.Header,
		"about":   cfg.Server.Branding.Content.About,
		"privacy": cfg.Server.Branding.Content.Privacy,
		"footer":  cfg.Server.Branding.Content.Footer,
		"terms":   cfg.Server.Branding.Content.Terms,
	}
	entry, exists := entries[name]
	if !exists || entry.Body == nil {
		return ""
	}
	body := localizeOperatorBody(entry.Body, language)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	body = expandOperatorVariables(body, variables)
	var rendered []byte
	switch strings.ToLower(entry.Format) {
	case "html":
		rendered = []byte(body)
	case "md":
		rendered = blackfriday.Run([]byte(body))
	case "txt":
		paragraphs := strings.Split(strings.TrimSpace(body), "\n\n")
		for i, paragraph := range paragraphs {
			paragraphs[i] = "<p>" + template.HTMLEscapeString(strings.TrimSpace(paragraph)) + "</p>"
		}
		rendered = []byte(strings.Join(paragraphs, "\n"))
	default:
		return ""
	}
	strict := name == "header" || name == "footer"
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "br", "span", "div", "strong", "b", "em", "i", "u", "s", "small", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li")
	policy.AllowAttrs("href", "title", "target", "rel").OnElements("a")
	policy.RequireNoReferrerOnLinks(true)
	policy.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	policy.AllowURLSchemes("https", "data")
	policy.AllowAttrs("class", "id").Globally()
	if !strict {
		policy.AllowElements("table", "thead", "tbody", "tr", "th", "td", "pre", "code", "blockquote", "hr", "dl", "dt", "dd", "sup", "sub")
	}
	clean := policy.Sanitize(string(rendered))
	clean = contactLinkPattern.ReplaceAllString(clean, `href="#"`)
	return template.HTML(clean)
}

func localizeOperatorBody(body interface{}, language string) string {
	switch value := body.(type) {
	case string:
		return value
	case map[string]string:
		if text := value[language]; text != "" {
			return text
		}
		return value["default"]
	case map[string]interface{}:
		if text, ok := value[language].(string); ok && text != "" {
			return text
		}
		if text, ok := value["default"].(string); ok {
			return text
		}
	}
	return ""
}

func expandOperatorVariables(body string, variables map[string]string) string {
	values := map[string]string{
		"{current_year}": fmt.Sprint(time.Now().Year()),
		"{project_name}": ProjectName,
		"{project_org}":  "webappsgo",
	}
	for name, value := range variables {
		values["{"+name+"}"] = value
	}
	for placeholder, value := range values {
		body = strings.ReplaceAll(body, placeholder, value)
	}
	return body
}
