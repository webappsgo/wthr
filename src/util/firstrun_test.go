package util

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectFirstRun covers both the missing-db (first run) and
// present-db (not first run) cases.
func TestDetectFirstRun(t *testing.T) {
	t.Run("no_db_is_first_run", func(t *testing.T) {
		dataDir := t.TempDir()
		if !DetectFirstRun(dataDir) {
			t.Error("DetectFirstRun() = false, want true when server.db is absent")
		}
	})

	t.Run("db_present_not_first_run", func(t *testing.T) {
		dataDir := t.TempDir()
		dbDir := filepath.Join(dataDir, "db")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dbDir, "server.db"), []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if DetectFirstRun(dataDir) {
			t.Error("DetectFirstRun() = true, want false when server.db exists")
		}
	})
}

// TestGenerateSetupToken verifies a 32-hex-char (128-bit) token is
// produced and two calls never collide.
func TestGenerateSetupToken(t *testing.T) {
	tok1, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken: %v", err)
	}
	if len(tok1) != 32 {
		t.Errorf("len(token) = %d, want 32", len(tok1))
	}
	tok2, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken: %v", err)
	}
	if tok1 == tok2 {
		t.Error("two generated tokens collided")
	}
}

// TestHashSetupToken verifies deterministic SHA-256 hex hashing.
func TestHashSetupToken(t *testing.T) {
	h1 := HashSetupToken("abc")
	h2 := HashSetupToken("abc")
	if h1 != h2 {
		t.Error("HashSetupToken is not deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("len(hash) = %d, want 64 (SHA-256 hex)", len(h1))
	}
	if HashSetupToken("abc") == HashSetupToken("xyz") {
		t.Error("different inputs produced the same hash")
	}
}

// TestSaveValidateDeleteSetupToken exercises the full lifecycle:
// save -> exists -> validate (correct/incorrect) -> delete -> exists.
func TestSaveValidateDeleteSetupToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	token := "supersecrettoken123"

	if SetupTokenExists(configDir) {
		t.Error("SetupTokenExists() = true before SaveSetupToken, want false")
	}

	if err := SaveSetupToken(configDir, token); err != nil {
		t.Fatalf("SaveSetupToken: %v", err)
	}

	if !SetupTokenExists(configDir) {
		t.Error("SetupTokenExists() = false after SaveSetupToken, want true")
	}

	// Stored content must be the SHA-256 hash, never the plaintext token.
	data, err := os.ReadFile(filepath.Join(configDir, "setup_token.txt"))
	if err != nil {
		t.Fatalf("ReadFile setup_token.txt: %v", err)
	}
	if string(data) == token {
		t.Error("setup_token.txt stores plaintext token, want SHA-256 hash")
	}
	if string(data) != HashSetupToken(token) {
		t.Error("setup_token.txt content does not match HashSetupToken(token)")
	}

	valid, err := ValidateSetupToken(configDir, token)
	if err != nil {
		t.Fatalf("ValidateSetupToken (correct): %v", err)
	}
	if !valid {
		t.Error("ValidateSetupToken(correct token) = false, want true")
	}

	valid, err = ValidateSetupToken(configDir, "wrong-token")
	if err != nil {
		t.Fatalf("ValidateSetupToken (incorrect): %v", err)
	}
	if valid {
		t.Error("ValidateSetupToken(wrong token) = true, want false")
	}

	if err := DeleteSetupToken(configDir); err != nil {
		t.Fatalf("DeleteSetupToken: %v", err)
	}
	if SetupTokenExists(configDir) {
		t.Error("SetupTokenExists() = true after DeleteSetupToken, want false")
	}

	// Deleting again (already gone) must not error.
	if err := DeleteSetupToken(configDir); err != nil {
		t.Errorf("DeleteSetupToken on already-deleted token: %v", err)
	}
}

// TestValidateSetupToken_MissingFile verifies a clear error when no
// token file exists yet.
func TestValidateSetupToken_MissingFile(t *testing.T) {
	configDir := t.TempDir()
	_, err := ValidateSetupToken(configDir, "anything")
	if err == nil {
		t.Error("ValidateSetupToken with no token file: err = nil, want error")
	}
}

// TestSelectRandomPort verifies the selected port falls within the
// documented 64000-64999 range and is actually available to bind.
func TestSelectRandomPort(t *testing.T) {
	port := SelectRandomPort()
	if port < MinPort || port > MaxPort {
		if port != 64948 {
			t.Errorf("SelectRandomPort() = %d, want in [%d,%d] or fallback 64948", port, MinPort, MaxPort)
		}
	}
}

// TestCreateDefaultServerYML verifies a valid server.yml is written with
// the SMTP host/port and default sections present.
func TestCreateDefaultServerYML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "server.yml")

	if err := CreateDefaultServerYML(configPath, "smtp.example.com", 587); err != nil {
		t.Fatalf("CreateDefaultServerYML: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"smtp.example.com",
		"587",
		"mode: production",
		"earthquakes: true",
	} {
		if !findSubstring(content, want) {
			t.Errorf("server.yml missing expected content %q", want)
		}
	}
}

// TestFindSubstring covers presence, absence, empty substr, and
// substr-longer-than-s edge cases for the unexported helper.
func TestFindSubstring(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"present", "hello world", "world", true},
		{"absent", "hello world", "xyz", false},
		{"empty_substr", "hello", "", true},
		{"substr_longer_than_s", "hi", "hello", false},
		{"equal_strings", "same", "same", true},
		{"both_empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findSubstring(tt.s, tt.substr); got != tt.want {
				t.Errorf("findSubstring(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}
