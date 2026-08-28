package cli

import (
	"fmt"

	"github.com/webappsgo/wthr/src/common/i18n"
	"github.com/webappsgo/wthr/src/config"
)

// T translates a cli.* key per AI.md PART 31 (Server CLI Output: help text,
// status messages, error messages, startup banners must all be translatable).
//
// Language resolution follows the CLI fallback chain (--lang flag → config
// file → LANG/LC_ALL env → auto-detect → en): config.ResolveLanguage(nil)
// already implements that chain, reading CLI_LANG (set by the --lang flag
// in cli.go) ahead of LC_ALL/LANG, with a nil config since most CLI command
// structs carry no *config.AppConfig.
//
// Locale values store raw Go printf verbs (%s, %d, ...); Sprintf is applied
// only when args are given, avoiding %-escaping issues on verb-free strings.
func T(key string, args ...interface{}) string {
	lang := config.ResolveLanguage(nil)

	instance := i18n.GetGlobalI18n()
	if instance == nil {
		return key
	}
	if !instance.IsSupported(lang) {
		lang = "en"
	}

	text := instance.T(lang, key)
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}
