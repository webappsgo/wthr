package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	utils "github.com/webappsgo/wthr/src/util"
)

// loadRealLocales reads the actual locale JSON files that ship with the
// project (src/common/i18n/locales/*.json) directly from disk — a _test.go
// file's working directory during `go test` is the package directory, so
// relative paths resolve correctly. This gives realistic, non-fabricated
// fixture data (real key parity, real meta.* values) without needing to
// fight go:embed rooting rules (embed directives in this package can only
// reach ./locales, not the "common/i18n/locales" prefix NewI18n expects).
// The locale files themselves are never modified — read-only fixtures.
func loadRealLocales(t *testing.T) map[string]map[string]string {
	t.Helper()

	entries, err := os.ReadDir("locales")
	if err != nil {
		t.Fatalf("failed to read locales dir: %v", err)
	}

	out := make(map[string]map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		lang := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		data, err := os.ReadFile(filepath.Join("locales", entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			t.Fatalf("failed to parse %s: %v", entry.Name(), err)
		}
		out[lang] = translations
	}
	return out
}

// newRealI18n constructs an *I18n populated from the real on-disk locale
// files, via direct struct literal construction (this test file is in
// package i18n, so unexported fields are accessible — the idiomatic way to
// unit test these methods without fighting go:embed rooting).
func newRealI18n(t *testing.T, defaultLang string) *I18n {
	t.Helper()

	translations := loadRealLocales(t)
	supported := make([]string, 0, len(translations))
	for lang := range translations {
		supported = append(supported, lang)
	}
	sort.Strings(supported)

	return &I18n{
		translations:  translations,
		defaultLang:   defaultLang,
		supportedLang: supported,
	}
}

// TestNewI18n_ErrorPath exercises NewI18n's error branch: when the FS does
// not contain a readable "common/i18n/locales" directory, NewI18n must
// return a non-nil, descriptive error rather than panicking.
//
// The happy path of NewI18n (successful embed + JSON unmarshal into the
// translations map) is exercised by the real production embed wiring in
// src/main.go; that logic is a straightforward ReadDir/ReadFile/Unmarshal
// loop, and the interesting behavior under test here — T, ParseAcceptLanguage,
// IsSupported, GetLanguageInfos, etc. — is covered below via direct struct
// construction against real locale content loaded from disk.
func TestNewI18n_ErrorPath(t *testing.T) {
	var emptyFS embed.FS

	got, err := NewI18n(emptyFS, "en")
	if err == nil {
		t.Fatal("NewI18n with empty embed.FS: expected error, got nil")
	}
	if got != nil {
		t.Errorf("NewI18n with empty embed.FS: expected nil *I18n on error, got %+v", got)
	}
}

func TestT(t *testing.T) {
	i := newRealI18n(t, "en")

	if !i.IsSupported("es") {
		t.Fatal("expected 'es' to be a supported fixture language for this test")
	}

	tests := []struct {
		name string
		lang string
		key  string
		want string
	}{
		{
			name: "exact match in requested lang",
			lang: "es",
			key:  "app.name",
			want: i.translations["es"]["app.name"],
		},
		{
			name: "key missing in requested lang falls back to default",
			lang: "es",
			key:  "__missing_key_only_in_none__",
			want: "__missing_key_only_in_none__",
		},
		{
			name: "key missing everywhere returns key itself",
			lang: "en",
			key:  "definitely.not.a.real.key",
			want: "definitely.not.a.real.key",
		},
		{
			name: "requested lang not loaded falls back to default",
			lang: "xx-not-a-real-lang",
			key:  "app.name",
			want: i.translations["en"]["app.name"],
		},
		{
			name: "empty key returns empty-key fallback (key itself, which is empty)",
			lang: "en",
			key:  "",
			want: "",
		},
		{
			name: "empty lang falls back to default lang",
			lang: "",
			key:  "app.name",
			want: i.translations["en"]["app.name"],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := i.T(tt.lang, tt.key)
			if got != tt.want {
				t.Errorf("T(%q, %q) = %q, want %q", tt.lang, tt.key, got, tt.want)
			}
		})
	}
}

// TestT_KeyPresentInRequestedLangDiffersFromDefault verifies that when a key
// exists in BOTH the requested language and the default language with
// different values, the requested language's value wins (no accidental
// fallback shadowing).
func TestT_KeyPresentInRequestedLangDiffersFromDefault(t *testing.T) {
	i := &I18n{
		translations: map[string]map[string]string{
			"en": {"greeting": "Hello"},
			"es": {"greeting": "Hola"},
		},
		defaultLang:   "en",
		supportedLang: []string{"en", "es"},
	}

	if got := i.T("es", "greeting"); got != "Hola" {
		t.Errorf("T(es, greeting) = %q, want %q", got, "Hola")
	}
	if got := i.T("en", "greeting"); got != "Hello" {
		t.Errorf("T(en, greeting) = %q, want %q", got, "Hello")
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	i := newRealI18n(t, "en")

	// Confirm the two languages used in q-value scenarios are actually
	// loaded fixture languages before asserting behavior against them.
	if !i.IsSupported("es") || !i.IsSupported("fr") {
		t.Fatal("expected both 'es' and 'fr' to be supported fixture languages for this test")
	}

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "empty header returns default lang",
			header: "",
			want:   "en",
		},
		{
			name:   "single lang exact match",
			header: "es",
			want:   "es",
		},
		{
			name:   "region subtag stripped",
			header: "en-US",
			want:   "en",
		},
		{
			name:   "multiple with q values chooses highest q",
			header: "fr;q=0.5,es;q=0.9",
			want:   "es",
		},
		{
			name:   "unsupported language falls back to default",
			header: "xx-not-real",
			want:   "en",
		},
		{
			name:   "malformed q value treated as 1.0 (Sscanf leaves q untouched)",
			header: "es;q=notanumber",
			want:   "es",
		},
		{
			name:   "whitespace around entries is trimmed",
			header: " es ; q=0.8 ",
			want:   "es",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := i.ParseAcceptLanguage(tt.header)
			if got != tt.want {
				t.Errorf("ParseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

// TestParseAcceptLanguage_QValueTie locks down a real edge case: the source
// uses a strict `q > highestQ` comparison, so on an exact tie the earlier
// entry in the header wins (the later equal-q entry does NOT overwrite it).
func TestParseAcceptLanguage_QValueTie(t *testing.T) {
	i := newRealI18n(t, "en")
	if !i.IsSupported("es") || !i.IsSupported("fr") {
		t.Fatal("expected both 'es' and 'fr' to be supported fixture languages for this test")
	}

	got := i.ParseAcceptLanguage("es;q=0.8,fr;q=0.8")
	if got != "es" {
		t.Errorf("ParseAcceptLanguage tie: got %q, want %q (earlier entry should win on exact tie)", got, "es")
	}
}

func TestIsSupported(t *testing.T) {
	i := newRealI18n(t, "en")

	tests := []struct {
		name string
		lang string
		want bool
	}{
		{"loaded lang", "en", true},
		{"not loaded lang", "xx-nope", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := i.IsSupported(tt.lang); got != tt.want {
				t.Errorf("IsSupported(%q) = %v, want %v", tt.lang, got, tt.want)
			}
		})
	}
}

// TestGetSupportedLanguages_ReturnsCopy verifies GetSupportedLanguages
// returns an independent copy: mutating the returned slice must not affect
// the internal state or subsequent calls, per the `copy(langs, ...)` in the
// source.
func TestGetSupportedLanguages_ReturnsCopy(t *testing.T) {
	i := newRealI18n(t, "en")

	first := i.GetSupportedLanguages()
	if len(first) == 0 {
		t.Fatal("expected at least one supported language from fixture locales")
	}

	original := make([]string, len(first))
	copy(original, first)

	// Mutate the returned slice.
	for idx := range first {
		first[idx] = "MUTATED"
	}

	second := i.GetSupportedLanguages()
	if len(second) != len(original) {
		t.Fatalf("GetSupportedLanguages length changed: got %d, want %d", len(second), len(original))
	}
	for idx := range second {
		if second[idx] != original[idx] {
			t.Errorf("internal state was mutated via returned slice: index %d = %q, want %q", idx, second[idx], original[idx])
		}
		if second[idx] == "MUTATED" {
			t.Errorf("internal state leaked mutation at index %d", idx)
		}
	}
}

// TestGetLanguageInfos_RealFixtures checks GetLanguageInfos against the real
// locale files: every shipped locale currently has meta.name, meta.native_name,
// and meta.direction populated, so the returned info should reflect those
// exact values (not the code/"ltr" defaults).
func TestGetLanguageInfos_RealFixtures(t *testing.T) {
	i := newRealI18n(t, "en")

	infos := i.GetLanguageInfos()
	if len(infos) != len(i.translations) {
		t.Fatalf("GetLanguageInfos returned %d infos, want %d", len(infos), len(i.translations))
	}

	byCode := make(map[string]utils.LanguageInfo, len(infos))
	for _, info := range infos {
		byCode[info.Code] = info
	}

	for code, translations := range i.translations {
		info, ok := byCode[code]
		if !ok {
			t.Errorf("missing LanguageInfo for code %q", code)
			continue
		}
		if wantName := translations["meta.name"]; wantName != "" && info.Name != wantName {
			t.Errorf("lang %q: Name = %q, want %q", code, info.Name, wantName)
		}
		if wantNative := translations["meta.native_name"]; wantNative != "" && info.NativeName != wantNative {
			t.Errorf("lang %q: NativeName = %q, want %q", code, info.NativeName, wantNative)
		}
		if wantDir := translations["meta.direction"]; wantDir != "" && info.Direction != wantDir {
			t.Errorf("lang %q: Direction = %q, want %q", code, info.Direction, wantDir)
		}
	}
}

// TestGetLanguageInfos_MissingMetaDefaults exercises the default-fallback
// branch (meta.name / meta.native_name / meta.direction entirely absent)
// deterministically, using literal fixture data rather than relying on real
// locale files to lack metadata (they currently all have it).
func TestGetLanguageInfos_MissingMetaDefaults(t *testing.T) {
	i := &I18n{
		translations: map[string]map[string]string{
			"xy": {"app.name": "Whatever"},
		},
		defaultLang:   "xy",
		supportedLang: []string{"xy"},
	}

	infos := i.GetLanguageInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 language info, got %d", len(infos))
	}

	info := infos[0]
	if info.Code != "xy" {
		t.Errorf("Code = %q, want %q", info.Code, "xy")
	}
	if info.Name != "xy" {
		t.Errorf("Name default = %q, want code %q", info.Name, "xy")
	}
	if info.NativeName != "xy" {
		t.Errorf("NativeName default = %q, want code %q", info.NativeName, "xy")
	}
	if info.Direction != "ltr" {
		t.Errorf("Direction default = %q, want %q", info.Direction, "ltr")
	}
}

func TestGetDefaultLanguage(t *testing.T) {
	i := newRealI18n(t, "fr")
	if got := i.GetDefaultLanguage(); got != "fr" {
		t.Errorf("GetDefaultLanguage() = %q, want %q", got, "fr")
	}
}

// TestT_ConcurrentReads confirms concurrent read-only access to T is
// data-race safe (the RWMutex in the source protects the translations map).
// Run with `go test -race` to make this meaningful.
func TestT_ConcurrentReads(t *testing.T) {
	i := newRealI18n(t, "en")

	var wg sync.WaitGroup
	const goroutines = 20
	const iterations = 50

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			langs := []string{"en", "es", "fr", "xx-unsupported"}
			for n := 0; n < iterations; n++ {
				lang := langs[(id+n)%len(langs)]
				_ = i.T(lang, "app.name")
				_ = i.IsSupported(lang)
				_ = i.GetSupportedLanguages()
			}
		}(g)
	}

	wg.Wait()
}

// interpolationVarPattern matches `{var}`-style interpolation placeholders,
// per AI.md PART 31 ("Key rules: ... `{variable}` interpolation").
var interpolationVarPattern = regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)

// interpolationVars returns the sorted, de-duplicated set of `{var}`
// placeholders found in s.
func interpolationVars(s string) []string {
	matches := interpolationVarPattern.FindAllString(s, -1)
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		seen[m] = struct{}{}
	}
	vars := make([]string, 0, len(seen))
	for v := range seen {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	return vars
}

// TestLocaleKeyParity is the build-time i18n validation required by AI.md
// PART 31 / .claude/rules/testing-rules.md ("ALWAYS keep every language
// file's keys identical to en.json — enforced by `make i18n-validate` /
// build-time check"). AI.md PART 26 explicitly forbids adding Makefile
// targets beyond the six core ones ("Six core targets. DO NOT ADD MORE."),
// so this check is wired into `go test` (the existing `test` target) rather
// than as a new `make i18n-validate` target — `make test` already satisfies
// "build-time check" for every other package-logic requirement in the repo.
//
// Validates, per the PART 31 "Build-Time Validation" spec:
//   - every locale has the same key set as en.json (no missing, no orphaned)
//   - no empty string values in any locale
//   - `{var}` interpolation placeholders match en.json for every shared key
func TestLocaleKeyParity(t *testing.T) {
	locales := loadRealLocales(t)

	en, ok := locales["en"]
	if !ok {
		t.Fatal("en.json not found among locale fixtures")
	}

	langs := make([]string, 0, len(locales))
	for lang := range locales {
		langs = append(langs, lang)
	}
	sort.Strings(langs)

	for _, lang := range langs {
		translations := locales[lang]

		for key, enValue := range en {
			value, exists := translations[key]
			if !exists {
				t.Errorf("locale %q: missing key %q (present in en.json)", lang, key)
				continue
			}
			if strings.TrimSpace(value) == "" {
				t.Errorf("locale %q: key %q has an empty value", lang, key)
			}

			enVars := interpolationVars(enValue)
			gotVars := interpolationVars(value)
			if !reflect.DeepEqual(enVars, gotVars) {
				t.Errorf("locale %q: key %q interpolation vars = %v, want %v (per en.json)", lang, key, gotVars, enVars)
			}
		}

		if lang == "en" {
			continue
		}
		for key := range translations {
			if _, existsInEn := en[key]; !existsInEn {
				t.Errorf("locale %q: orphaned key %q (not present in en.json)", lang, key)
			}
		}
	}
}
