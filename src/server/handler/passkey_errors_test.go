package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasskeyErrorKeySentinels(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantKey string
		wantOK  bool
	}{
		{"nil error", nil, "", false},
		{"not found", ErrPasskeyNotFound, "errors.passkey.passkey_not_found", true},
		{"session not found", ErrPasskeySessionNotFound, "errors.passkey.session_not_found", true},
		{"session expired", ErrPasskeySessionExpired, "errors.passkey.session_expired", true},
		{"invalid session", ErrPasskeyInvalidSession, "errors.passkey.invalid_session", true},
		{"invalid registration session", ErrPasskeyInvalidRegistrationSession, "errors.passkey.invalid_registration_session", true},
		{"name and password required", ErrPasskeyNamePasswordRequired, "errors.passkey.name_and_password_are_required", true},
		{"invalid password", ErrPasskeyInvalidPassword, "errors.passkey.invalid_password", true},
		{"no credentials", ErrPasskeyNoCredentials, "errors.passkey.no_passkeys_registered", true},
		{"invalid request body", ErrPasskeyInvalidRequestBody, "errors.passkey.invalid_request_body", true},
		{"invalid session token", ErrPasskeyInvalidSessionToken, "errors.passkey.invalid_session_token", true},
		{"credential not found", ErrPasskeyCredentialNotFound, "errors.passkey.credential_not_found", true},
		{"ceremony token required", ErrPasskeyCeremonyTokenRequired, "errors.passkey.ceremony_token_required", true},
		{"missing request host", ErrPasskeyMissingRequestHost, "errors.passkey.missing_request_host", true},
		{"unrecognized error", errors.New("boom"), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := passkeyErrorKey(tt.err)
			if ok != tt.wantOK {
				t.Fatalf("got ok=%v, want %v (key %q)", ok, tt.wantOK, key)
			}
			if key != tt.wantKey {
				t.Fatalf("got key %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestPasskeyErrorKeyWrapped(t *testing.T) {
	wrapped := fmt.Errorf("register passkey: %w", ErrPasskeyInvalidPassword)
	key, ok := passkeyErrorKey(wrapped)
	if !ok || key != "errors.passkey.invalid_password" {
		t.Fatalf("wrapped sentinel: got (%q, %v), want (\"errors.passkey.invalid_password\", true)", key, ok)
	}
}

func TestPasskeyErrorKeySubstrings(t *testing.T) {
	tests := []struct {
		needle  string
		wantKey string
	}{
		{"failed to load passkeys: no rows", "errors.passkey.failed_to_load_passkeys"},
		{"failed to load admin passkeys: locked", "errors.passkey.failed_to_load_passkeys"},
		{"failed to load user: not found", "errors.passkey.failed_to_load_passkeys"},
		{"failed to initialize passkeys", "errors.passkey.failed_to_initialize_passkeys"},
		{"failed to generate passkey session", "errors.passkey.failed_to_start_registration"},
		{"failed to start passkey challenge", "errors.passkey.failed_to_start_challenge"},
		{"failed to verify challenge", "errors.passkey.failed_to_verify_challenge"},
		{"failed to finish registration", "errors.passkey.failed_to_finish_registration"},
		{"failed to finish authentication", "errors.passkey.failed_to_finish_authentication"},
		{"failed to register passkey", "errors.passkey.failed_to_register_passkey"},
		{"failed to update passkey", "errors.passkey.failed_to_update_passkey"},
		{"failed to store passkey", "errors.passkey.failed_to_register_passkey"},
		{"failed to create session", "errors.passkey.failed_to_create_session"},
		{"failed to resolve passkey user", "errors.passkey.failed_to_resolve_user"},
		{"Missing Request Host", "errors.passkey.missing_request_host"},
	}
	for _, tt := range tests {
		t.Run(tt.needle, func(t *testing.T) {
			key, ok := passkeyErrorKey(errors.New(tt.needle))
			if !ok {
				t.Fatalf("expected substring %q to be recognized", tt.needle)
			}
			if key != tt.wantKey {
				t.Fatalf("got key %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestPasskeySentinelKeyDefaultArm(t *testing.T) {
	if got := passkeySentinelKey(errors.New("not a sentinel")); got != "errors.internal_error" {
		t.Fatalf("unknown sentinel: got %q, want errors.internal_error", got)
	}
}

func TestIsPasskeyNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sentinel", ErrPasskeyNotFound, true},
		{"wrapped sentinel", fmt.Errorf("delete: %w", ErrPasskeyNotFound), true},
		{"model plain error", errors.New("passkey not found"), true},
		{"model plain error different case", errors.New("Passkey Not Found"), true},
		{"unrelated error", errors.New("database is locked"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPasskeyNotFound(tt.err); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWritePasskeyError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		err        error
		wantStatus int
	}{
		{
			name:       "recognized client error",
			status:     http.StatusBadRequest,
			err:        fmt.Errorf("verify: %w", ErrPasskeyInvalidSession),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unrecognized client error falls back to internal key",
			status:     http.StatusUnauthorized,
			err:        errors.New("SQLITE_BUSY: database is locked"),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "server error",
			status:     http.StatusInternalServerError,
			err:        errors.New("failed to start passkey challenge: origin mismatch"),
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			writePasskeyError(rec, req, tt.status, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Fatalf("got content-type %q, want application/json", ct)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if ok, present := body["ok"]; !present || ok != false {
				t.Fatalf("got ok=%v, want false", ok)
			}
			msg, _ := body["error"].(string)
			if msg == "" {
				t.Fatal("expected a non-empty error message")
			}
			// The raw error chain and any SQL text must never reach the body.
			if strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked") {
				t.Fatalf("raw error text leaked into response: %q", msg)
			}
		})
	}
}
