package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/webappsgo/wthr/src/util"
)

// I18n provides internationalization support.
// AI.md PART 31 - NON-NEGOTIABLE
type I18n struct {
	mu            sync.RWMutex
	translations  map[string]map[string]string
	defaultLang   string
	supportedLang []string
}

// NewI18n creates a new internationalization service
func NewI18n(fs embed.FS, defaultLang string) (*I18n, error) {
	i18n := &I18n{
		translations:  make(map[string]map[string]string),
		defaultLang:   defaultLang,
		supportedLang: []string{},
	}

	// Load all locale files from embedded FS
	entries, err := fs.ReadDir("common/i18n/locales")
	if err != nil {
		return nil, fmt.Errorf("failed to read locale directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := fs.ReadFile(fmt.Sprintf("common/i18n/locales/%s", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read locale file %s: %w", entry.Name(), err)
		}

		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			return nil, fmt.Errorf("failed to parse locale file %s: %w", entry.Name(), err)
		}

		i18n.translations[lang] = translations
		i18n.supportedLang = append(i18n.supportedLang, lang)
	}

	return i18n, nil
}

// T translates a key for the given language
// Falls back to default language if key not found
func (i *I18n) T(lang, key string) string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Try requested language
	if translations, ok := i.translations[lang]; ok {
		if text, ok := translations[key]; ok {
			return text
		}
	}

	// Fall back to default language
	if translations, ok := i.translations[i.defaultLang]; ok {
		if text, ok := translations[key]; ok {
			return text
		}
	}

	// AI.md PART 31: return the key itself as the last-resort fallback.
	return key
}

// ParseAcceptLanguage parses the Accept-Language header and returns the best match
// Header format: "en-US,en;q=0.9,es;q=0.8"
func (i *I18n) ParseAcceptLanguage(header string) string {
	if header == "" {
		return i.defaultLang
	}

	// Split by comma
	languages := strings.Split(header, ",")
	bestMatch := i.defaultLang
	highestQ := 0.0

	for _, lang := range languages {
		// Parse language and quality
		parts := strings.Split(strings.TrimSpace(lang), ";")
		langCode := strings.ToLower(strings.TrimSpace(parts[0]))

		// Extract base language (en-US -> en)
		if idx := strings.Index(langCode, "-"); idx > 0 {
			langCode = langCode[:idx]
		}

		// Parse quality factor (default 1.0)
		q := 1.0
		if len(parts) > 1 {
			if strings.HasPrefix(parts[1], "q=") {
				fmt.Sscanf(parts[1], "q=%f", &q)
			}
		}

		// Check if we support this language
		if i.isSupported(langCode) && q > highestQ {
			bestMatch = langCode
			highestQ = q
		}
	}

	return bestMatch
}

// IsSupported returns true if the language code is supported.
func (i *I18n) IsSupported(lang string) bool {
	return i.isSupported(lang)
}

// isSupported checks if a language is supported (internal, no lock).
func (i *I18n) isSupported(lang string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	_, ok := i.translations[lang]
	return ok
}

// GetSupportedLanguages returns list of supported language codes.
func (i *I18n) GetSupportedLanguages() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	langs := make([]string, len(i.supportedLang))
	copy(langs, i.supportedLang)
	return langs
}

// GetLanguageInfos returns metadata for all supported languages, sorted by code.
// Reads meta.name, meta.native_name, and meta.direction from each locale file.
func (i *I18n) GetLanguageInfos() []util.LanguageInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()

	infos := make([]util.LanguageInfo, 0, len(i.supportedLang))
	for _, code := range i.supportedLang {
		t := i.translations[code]
		info := util.LanguageInfo{
			Code:       code,
			Name:       t["meta.name"],
			NativeName: t["meta.native_name"],
			Direction:  t["meta.direction"],
		}
		if info.Name == "" {
			info.Name = code
		}
		if info.NativeName == "" {
			info.NativeName = code
		}
		if info.Direction == "" {
			info.Direction = "ltr"
		}
		infos = append(infos, info)
	}
	return infos
}

// GetDefaultLanguage returns the default language code.
func (i *I18n) GetDefaultLanguage() string {
	return i.defaultLang
}
