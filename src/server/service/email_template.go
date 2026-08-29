package service

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server"
)

// emailTemplateFuncMap returns the template.FuncMap available inside every
// email .tmpl file: t (plain i18n lookup) and tf (i18n lookup with
// {variable} interpolation for phrases that embed a dynamic value).
func emailTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"t": func(lang, key string) string {
			instance := i18n.GetGlobalI18n()
			if instance == nil {
				return key
			}
			return instance.T(lang, key)
		},
		"tf": func(lang, key string, pairs ...string) string {
			instance := i18n.GetGlobalI18n()
			text := key
			if instance != nil {
				text = instance.T(lang, key)
			}
			for i := 0; i+1 < len(pairs); i += 2 {
				text = strings.ReplaceAll(text, "{"+pairs[i]+"}", pairs[i+1])
			}
			return text
		},
	}
}

// loadEmailTemplateSource returns the raw template source for the given
// template name, preferring an admin override on disk over the embedded
// default shipped in the binary.
func loadEmailTemplateSource(name string) (string, error) {
	overridePath := filepath.Join(path.GetConfigDir(), "template", "email", name+".tmpl")
	if content, err := os.ReadFile(overridePath); err == nil {
		return string(content), nil
	}

	embeddedPath := "template/email/" + name + ".tmpl"
	content, err := fs.ReadFile(server.GetTemplatesFS(), embeddedPath)
	if err != nil {
		return "", fmt.Errorf("email template %q not found: %w", name, err)
	}
	return string(content), nil
}

// RenderEmailTemplate loads, parses, and executes the named email template
// (admin override first, embedded default as fallback), returning the
// rendered subject and body. data should contain every {{.Field}} the
// template references; Lang is injected automatically from lang.
func RenderEmailTemplate(name, lang string, data map[string]any) (subject, body string, err error) {
	if lang == "" {
		lang = "en"
	}

	source, err := loadEmailTemplateSource(name)
	if err != nil {
		return "", "", err
	}

	renderData := make(map[string]any, len(data)+1)
	for k, v := range data {
		renderData[k] = v
	}
	renderData["Lang"] = lang

	tmpl, err := template.New(name).Funcs(emailTemplateFuncMap()).Parse(source)
	if err != nil {
		return "", "", fmt.Errorf("parsing email template %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, renderData); err != nil {
		return "", "", fmt.Errorf("executing email template %q: %w", name, err)
	}

	rendered := buf.String()
	lines := strings.SplitN(rendered, "\n", -1)
	separatorIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndex = i
			break
		}
	}
	if separatorIndex == -1 {
		return "", "", fmt.Errorf("email template %q missing '---' separator after rendering", name)
	}

	headerLines := lines[:separatorIndex]
	bodyLines := lines[separatorIndex+1:]

	for _, line := range headerLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Subject:") {
			subject = strings.TrimSpace(strings.TrimPrefix(trimmed, "Subject:"))
			break
		}
	}

	body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return subject, body, nil
}
