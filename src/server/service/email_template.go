package service

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/server"
)

// emailTemplateSource carries the raw subject/body pair an email template
// resolves to, before {variable} substitution is applied.
type emailTemplateSource struct {
	subject string
	body    string
}

// splitEmailTemplateFile parses the "Subject: ...\n---\n...body..." structure
// shared by every embedded default and admin override .tmpl file on disk.
func splitEmailTemplateFile(name, raw string) (emailTemplateSource, error) {
	lines := strings.Split(raw, "\n")
	separatorIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndex = i
			break
		}
	}
	if separatorIndex == -1 {
		return emailTemplateSource{}, fmt.Errorf("email template %q missing '---' separator", name)
	}

	var subject string
	for _, line := range lines[:separatorIndex] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Subject:") {
			subject = strings.TrimSpace(strings.TrimPrefix(trimmed, "Subject:"))
			break
		}
	}

	body := strings.TrimSpace(strings.Join(lines[separatorIndex+1:], "\n"))
	return emailTemplateSource{subject: subject, body: body}, nil
}

// loadEmbeddedEmailTemplate reads and parses the embedded default .tmpl file
// for name, used only as a last-resort fallback when neither an admin
// override file nor an i18n subject/body key pair is available.
func loadEmbeddedEmailTemplate(name string) (emailTemplateSource, error) {
	embeddedPath := "template/email/" + name + ".tmpl"
	content, err := fs.ReadFile(server.GetTemplatesFS(), embeddedPath)
	if err != nil {
		return emailTemplateSource{}, fmt.Errorf("email template %q not found: %w", name, err)
	}
	return splitEmailTemplateFile(name, string(content))
}

// loadAdminOverrideEmailTemplate reads the admin-authored override file for
// name, if one has been saved to disk. It returns ok=false (no error) when
// no override exists, so callers fall through to the next source.
func loadAdminOverrideEmailTemplate(name string) (source emailTemplateSource, ok bool, err error) {
	overridePath := filepath.Join(path.GetConfigDir(), "template", "email", name+".tmpl")
	content, readErr := os.ReadFile(overridePath)
	if readErr != nil {
		return emailTemplateSource{}, false, nil
	}
	source, err = splitEmailTemplateFile(name, string(content))
	if err != nil {
		return emailTemplateSource{}, false, err
	}
	return source, true, nil
}

// loadLocalizedEmailTemplate looks up the full subject/body text for name in
// the given language via the two i18n keys email.<name>.subject and
// email.<name>.body. ok is false when neither key resolved to anything
// (i.e. the global i18n instance isn't loaded), so callers fall through to
// the embedded .tmpl file.
func loadLocalizedEmailTemplate(name, lang string) (source emailTemplateSource, ok bool) {
	instance := i18n.GetGlobalI18n()
	if instance == nil {
		return emailTemplateSource{}, false
	}

	subjectKey := "email." + name + ".subject"
	bodyKey := "email." + name + ".body"
	subject := instance.T(lang, subjectKey)
	body := instance.T(lang, bodyKey)

	// I18n.T falls back to returning the key itself when no translation
	// exists anywhere (not even in the default language) - that means the
	// keys are genuinely missing, not just untranslated for this language.
	if subject == subjectKey && body == bodyKey {
		return emailTemplateSource{}, false
	}
	if subject == subjectKey {
		subject = ""
	}
	if body == bodyKey {
		body = ""
	}
	return emailTemplateSource{subject: subject, body: body}, true
}

// substituteVariables replaces every {key} occurrence in text with the
// corresponding value from data. Matching is exact and case-sensitive -
// same {variable} substitution style the AI.md PART 18 admin template
// editor exposes to Server Admins.
func substituteVariables(text string, data map[string]any) string {
	for key, value := range data {
		text = strings.ReplaceAll(text, "{"+key+"}", fmt.Sprint(value))
	}
	return text
}

// RenderEmailTemplate resolves the named email template to a subject/body
// pair and substitutes {variable} placeholders from data.
//
// Resolution order:
//  1. Admin override file at {config_dir}/template/email/{name}.tmpl, if
//     present - a single English-language file that applies regardless of
//     lang, since admin customization (PART 18) is not per-language.
//  2. Localized full text from the email.<name>.subject / email.<name>.body
//     i18n keys for lang, which already fall back to the default language
//     per PART 31.
//  3. The embedded default .tmpl file shipped in the binary, parsed the
//     same "Subject: ...\n---\n...body..." way as an admin override, used
//     only if the i18n keys are entirely missing.
//
// AI.md PART 18 requires plain {variable} substitution for email templates,
// not Go's text/template - so no template engine is involved anywhere in
// this function.
func RenderEmailTemplate(name, lang string, data map[string]any) (subject, body string, err error) {
	if lang == "" {
		lang = "en"
	}

	var source emailTemplateSource

	if override, ok, overrideErr := loadAdminOverrideEmailTemplate(name); overrideErr != nil {
		return "", "", overrideErr
	} else if ok {
		source = override
	} else if localized, ok := loadLocalizedEmailTemplate(name, lang); ok {
		source = localized
	} else if embedded, embeddedErr := loadEmbeddedEmailTemplate(name); embeddedErr == nil {
		source = embedded
	} else {
		return "", "", embeddedErr
	}

	subject = substituteVariables(source.subject, data)
	body = substituteVariables(source.body, data)
	return subject, body, nil
}
