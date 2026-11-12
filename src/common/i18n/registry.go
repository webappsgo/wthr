package i18n

// LanguageSpec is one registry entry per AI.md PART 31: the language code, its
// English and native names, text direction and CLDR plural categories.
type LanguageSpec struct {
	Code       string
	Name       string
	NativeName string
	Direction  string
	Plurals    []string
}

// Registry is the single source of truth for the supported languages. A
// language is supported only when its entry here, its embedded locale file and
// the config default list all agree; the tests in this package and in
// src/config enforce that at build time.
var Registry = []LanguageSpec{
	{Code: "en", Name: "English", NativeName: "English", Direction: "ltr", Plurals: []string{"one", "other"}},
	{Code: "es", Name: "Spanish", NativeName: "Español", Direction: "ltr", Plurals: []string{"one", "other"}},
	{Code: "fr", Name: "French", NativeName: "Français", Direction: "ltr", Plurals: []string{"one", "other"}},
	{Code: "de", Name: "German", NativeName: "Deutsch", Direction: "ltr", Plurals: []string{"one", "other"}},
	{Code: "it", Name: "Italian", NativeName: "Italiano", Direction: "ltr", Plurals: []string{"one", "many", "other"}},
	{Code: "ar", Name: "Arabic", NativeName: "العربية", Direction: "rtl", Plurals: []string{"zero", "one", "two", "few", "many", "other"}},
	{Code: "zh", Name: "Chinese", NativeName: "中文", Direction: "ltr", Plurals: []string{"other"}},
	{Code: "ja", Name: "Japanese", NativeName: "日本語", Direction: "ltr", Plurals: []string{"other"}},
}

// RegistryCodes returns the registry language codes in registry order.
func RegistryCodes() []string {
	codes := make([]string, 0, len(Registry))
	for _, spec := range Registry {
		codes = append(codes, spec.Code)
	}

	return codes
}

// IsRegisteredCode reports whether code is a registry language code.
func IsRegisteredCode(code string) bool {
	for _, spec := range Registry {
		if spec.Code == code {
			return true
		}
	}

	return false
}
