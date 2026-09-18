package mode

import (
	"os"
	"testing"
)

func TestSetAppMode(t *testing.T) {
	tests := []struct {
		input    string
		expected AppMode
	}{
		{"development", Development},
		{"devel", Development},
		{"dev", Development},
		{"debug", Debug},
		{"production", Production},
		{"prod", Production},
		{"DEVELOPMENT", Development},
		{"PRODUCTION", Production},
		// Default to production
		{"anything", Production},
	}

	for _, tt := range tests {
		SetDebugEnabled(false)
		SetAppMode(tt.input)
		if GetCurrentAppMode() != tt.expected {
			t.Errorf("SetAppMode(%q): got %v, want %v", tt.input, GetCurrentAppMode(), tt.expected)
		}
	}
}

func TestSetAppModeDebugAliasEnablesDebug(t *testing.T) {
	SetDebugEnabled(false)
	SetAppMode("debug")
	if !IsDebugEnabled() {
		t.Error(`SetAppMode("debug") should default the debug flag on`)
	}
}

func TestIsAppModeDev(t *testing.T) {
	SetAppMode("development")
	if !IsAppModeDev() {
		t.Error("IsAppModeDev() should return true when mode is development")
	}
	if IsAppModeProd() {
		t.Error("IsAppModeProd() should return false when mode is development")
	}
}

func TestIsAppModeProd(t *testing.T) {
	SetAppMode("production")
	if !IsAppModeProd() {
		t.Error("IsAppModeProd() should return true when mode is production")
	}
	if IsAppModeDev() {
		t.Error("IsAppModeDev() should return false when mode is production")
	}
}

func TestSetDebugEnabled(t *testing.T) {
	SetDebugEnabled(true)
	if !IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return true when debug is enabled")
	}

	SetDebugEnabled(false)
	if IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return false when debug is disabled")
	}
}

func TestGetAppModeString(t *testing.T) {
	SetAppMode("production")
	SetDebugEnabled(false)
	if GetAppModeString() != "production" {
		t.Errorf("GetAppModeString() = %q, want %q", GetAppModeString(), "production")
	}

	SetDebugEnabled(true)
	if GetAppModeString() != "production [debugging]" {
		t.Errorf("GetAppModeString() = %q, want %q", GetAppModeString(), "production [debugging]")
	}

	SetAppMode("development")
	SetDebugEnabled(false)
	if GetAppModeString() != "development" {
		t.Errorf("GetAppModeString() = %q, want %q", GetAppModeString(), "development")
	}

	SetDebugEnabled(true)
	if GetAppModeString() != "development [debugging]" {
		t.Errorf("GetAppModeString() = %q, want %q", GetAppModeString(), "development [debugging]")
	}

	if ModeString() != GetAppModeString() {
		t.Errorf("ModeString() = %q, want %q", ModeString(), GetAppModeString())
	}
}

func TestFromEnv(t *testing.T) {
	// Save original env vars
	origMode, hadMode := os.LookupEnv("MODE")
	origDebug, hadDebug := os.LookupEnv("DEBUG")
	defer func() {
		restoreEnv(t, "MODE", origMode, hadMode)
		restoreEnv(t, "DEBUG", origDebug, hadDebug)
	}()

	// MODE selects the mode
	t.Setenv("MODE", "development")
	os.Unsetenv("DEBUG")
	SetDebugEnabled(false)
	FromEnv()
	if !IsAppModeDev() {
		t.Error("FromEnv() should set development mode from MODE env var")
	}

	// DEBUG enables debug independently of MODE
	t.Setenv("MODE", "production")
	t.Setenv("DEBUG", "true")
	FromEnv()
	if !IsDebugEnabled() {
		t.Error("FromEnv() should enable debug from DEBUG env var")
	}

	// An explicit DEBUG=false wins over the MODE=debug default
	t.Setenv("MODE", "debug")
	t.Setenv("DEBUG", "false")
	FromEnv()
	if GetCurrentAppMode() != Debug {
		t.Errorf("FromEnv(): got mode %v, want %v", GetCurrentAppMode(), Debug)
	}
	if IsDebugEnabled() {
		t.Error("explicit DEBUG=false must override the MODE=debug default")
	}

	// An empty DEBUG is not an explicit value and leaves the alias default alone
	t.Setenv("MODE", "debug")
	t.Setenv("DEBUG", "")
	FromEnv()
	if !IsDebugEnabled() {
		t.Error("empty DEBUG must not clear the MODE=debug default")
	}
}

func restoreEnv(t *testing.T, key, value string, existed bool) {
	t.Helper()
	if existed {
		os.Setenv(key, value)
		return
	}
	os.Unsetenv(key)
}

func TestAppModeString(t *testing.T) {
	tests := []struct {
		mode AppMode
		want string
	}{
		{Production, "production"},
		{Development, "development"},
		{Debug, "debug"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("AppMode.String() = %q, want %q", got, tt.want)
		}
	}
}
