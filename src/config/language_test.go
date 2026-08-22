package config

import (
	"os"
	"testing"
)

// clearLanguageEnv unsets every environment variable ResolveLanguage consults
// and restores the previous values when the test finishes.
func clearLanguageEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CLI_LANG", "LC_ALL", "LANG"} {
		previous, existed := os.LookupEnv(name)
		os.Unsetenv(name)
		t.Cleanup(func() {
			if existed {
				os.Setenv(name, previous)
				return
			}
			os.Unsetenv(name)
		})
	}
}

// TestResolveLanguage covers the AI.md PART 31 server language chain:
// --lang (CLI_LANG) > server.yml lang > LC_ALL > LANG > en.
func TestResolveLanguage(t *testing.T) {
	t.Run("defaults to english with no signal", func(t *testing.T) {
		clearLanguageEnv(t)
		if got := ResolveLanguage(nil); got != "en" {
			t.Fatalf("ResolveLanguage(nil) = %q, want %q", got, "en")
		}
	})

	t.Run("config value beats the environment", func(t *testing.T) {
		clearLanguageEnv(t)
		os.Setenv("LANG", "fr_FR.UTF-8")
		cfg := &AppConfig{}
		cfg.Server.Lang = "de"
		if got := ResolveLanguage(cfg); got != "de" {
			t.Fatalf("ResolveLanguage() = %q, want %q", got, "de")
		}
	})

	t.Run("flag beats the config value", func(t *testing.T) {
		clearLanguageEnv(t)
		os.Setenv("CLI_LANG", "ja")
		cfg := &AppConfig{}
		cfg.Server.Lang = "de"
		if got := ResolveLanguage(cfg); got != "ja" {
			t.Fatalf("ResolveLanguage() = %q, want %q", got, "ja")
		}
	})

	t.Run("auto config falls through to LC_ALL then LANG", func(t *testing.T) {
		clearLanguageEnv(t)
		os.Setenv("LC_ALL", "es_ES.UTF-8")
		os.Setenv("LANG", "fr_FR.UTF-8")
		cfg := &AppConfig{}
		cfg.Server.Lang = "auto"
		if got := ResolveLanguage(cfg); got != "es" {
			t.Fatalf("ResolveLanguage() = %q, want %q", got, "es")
		}

		os.Unsetenv("LC_ALL")
		if got := ResolveLanguage(cfg); got != "fr" {
			t.Fatalf("ResolveLanguage() = %q, want %q", got, "fr")
		}
	})

	t.Run("posix locales carry no language preference", func(t *testing.T) {
		clearLanguageEnv(t)
		os.Setenv("LANG", "C.UTF-8")
		if got := ResolveLanguage(nil); got != "en" {
			t.Fatalf("ResolveLanguage() = %q, want %q", got, "en")
		}
	})
}

// TestNormalizeLanguageCode covers locale-string reduction to a base code.
func TestNormalizeLanguageCode(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"auto":        "",
		"C":           "",
		"POSIX":       "",
		"en":          "en",
		"EN":          "en",
		" es ":        "es",
		"en-US":       "en",
		"zh_CN.UTF-8": "zh",
		"ar@latin":    "ar",
	}
	for input, want := range cases {
		if got := normalizeLanguageCode(input); got != want {
			t.Errorf("normalizeLanguageCode(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestIsHealthzRootAliasEnabled covers the AI.md PART 13 gate for the optional
// root /healthz alias, which must default to false.
func TestIsHealthzRootAliasEnabled(t *testing.T) {
	if (*AppConfig)(nil).IsHealthzRootAliasEnabled() {
		t.Error("nil config reported the root /healthz alias as enabled")
	}

	cfg := &AppConfig{}
	if cfg.IsHealthzRootAliasEnabled() {
		t.Error("zero-value config reported the root /healthz alias as enabled")
	}

	cfg.Server.Healthz.Root.Enabled = true
	if !cfg.IsHealthzRootAliasEnabled() {
		t.Error("config with server.healthz.root.enabled=true reported the alias as disabled")
	}
}
