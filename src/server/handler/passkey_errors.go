package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
)

// Sentinel errors returned by the passkey ceremony helpers. Handlers map these
// to translation keys instead of surfacing err.Error(), so no internal error
// chain, SQL text, or WebAuthn library message ever reaches an API response
// (AI.md PART 11 / backend-rules: never expose internal error chains).
var (
	// ErrPasskeyMissingRequestHost is returned when the WebAuthn relying-party
	// host cannot be resolved from the request.
	ErrPasskeyMissingRequestHost = errors.New("missing request host")
	// ErrPasskeySessionNotFound is returned when the ceremony cookie references
	// a session that is no longer in the in-memory ceremony cache.
	ErrPasskeySessionNotFound = errors.New("passkey session not found")
	// ErrPasskeySessionExpired is returned when the ceremony cookie has passed
	// its short expiry window.
	ErrPasskeySessionExpired = errors.New("passkey session expired")
	// ErrPasskeyInvalidSession is returned for a ceremony token whose state is
	// not usable for the requested ceremony.
	ErrPasskeyInvalidSession = errors.New("invalid passkey session")
	// ErrPasskeyInvalidRegistrationSession is returned when a registration
	// ceremony token is replayed against a different user or ceremony kind.
	ErrPasskeyInvalidRegistrationSession = errors.New("invalid passkey registration session")
	// ErrPasskeyNotFound is returned when the addressed passkey does not exist
	// or does not belong to the caller.
	ErrPasskeyNotFound = errors.New("passkey not found")
	// ErrPasskeyNamePasswordRequired is returned when the start-registration
	// request omits the passkey name or the current password.
	ErrPasskeyNamePasswordRequired = errors.New("passkey name and password are required")
	// ErrPasskeyInvalidPassword is returned when the supplied password does not
	// match the authenticated account.
	ErrPasskeyInvalidPassword = errors.New("invalid password")
	// ErrPasskeyNoCredentials is returned when a discoverable-login ceremony has
	// no registered credentials for the account.
	ErrPasskeyNoCredentials = errors.New("no passkeys registered for this account")
	// ErrPasskeyInvalidRequestBody is returned when the request body is empty or
	// is not decodable as the expected JSON shape.
	ErrPasskeyInvalidRequestBody = errors.New("invalid request body")
	// ErrPasskeyInvalidSessionToken is returned when a pending two-factor
	// session token is unknown or its user no longer exists.
	ErrPasskeyInvalidSessionToken = errors.New("invalid session token")
	// ErrPasskeyCredentialNotFound is returned when a discoverable login
	// resolves to a credential the lookup callback cannot match to a user.
	ErrPasskeyCredentialNotFound = errors.New("credential not found")
	// ErrPasskeyCeremonyTokenRequired is returned when a finish request omits
	// the ceremony token issued by the begin request.
	ErrPasskeyCeremonyTokenRequired = errors.New("ceremony_token is required")
)

// isPasskeyNotFound reports whether err identifies a missing passkey. The
// model layer returns its own plain "passkey not found" error rather than this
// package's sentinel, so the sentinel match is paired with a text fallback.
func isPasskeyNotFound(err error) bool {
	if errors.Is(err, ErrPasskeyNotFound) {
		return true
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "passkey not found")
}

// writePasskeyError logs the raw error with its HTTP context and writes the
// canonical error body using the mapped translation key. The raw error never
// reaches the response; an unmapped error logs at Error level and responds with
// the generic key.
func writePasskeyError(w http.ResponseWriter, r *http.Request, status int, err error) {
	key, ok := passkeyErrorKey(err)
	if !ok {
		key = "errors.internal_error"
	}
	if status >= http.StatusInternalServerError {
		log.Printf("ERROR: passkey request failed (status %d): %v", status, err)
	} else {
		log.Printf("WARN: passkey request rejected (status %d): %v", status, err)
	}
	writeJSON(w, status, map[string]interface{}{"ok": false, "error": Translate(r, key)})
}

// errPasskeyLoadKeys maps the internal "failed to ..." error strings that the
// ceremony helpers wrap around database and WebAuthn failures onto translation
// keys. The comparison is on a normalized lowercase substring so it stays
// correct when a helper adds a %w suffix.
var errPasskeyLoadKeys = []struct {
	needle string
	key    string
}{
	{"failed to load passkeys", "errors.passkey.failed_to_load_passkeys"},
	{"failed to load admin passkeys", "errors.passkey.failed_to_load_passkeys"},
	{"failed to load user", "errors.passkey.failed_to_load_passkeys"},
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
	{"missing request host", "errors.passkey.missing_request_host"},
	// validateAuthUser rejects an inactive, suspended, or unverified account with
	// a plain formatted error; all three collapse to the same generic message so
	// the response never reveals which account condition failed.
	{"account is disabled", "errors.passkey.not_authenticated"},
	{"account is suspended", "errors.passkey.not_authenticated"},
	{"invalid credentials", "errors.passkey.not_authenticated"},
}

// passkeyErrorKey resolves a passkey helper error to its translation key. The
// boolean reports whether the error was recognized; an unrecognized error maps
// to the generic internal-error key so the caller never falls back to
// err.Error(). Status selection is the caller's responsibility.
func passkeyErrorKey(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	for _, sentinel := range []error{
		ErrPasskeyNotFound,
		ErrPasskeySessionNotFound,
		ErrPasskeySessionExpired,
		ErrPasskeyInvalidSession,
		ErrPasskeyInvalidRegistrationSession,
		ErrPasskeyNamePasswordRequired,
		ErrPasskeyInvalidPassword,
		ErrPasskeyNoCredentials,
		ErrPasskeyInvalidRequestBody,
		ErrPasskeyInvalidSessionToken,
		ErrPasskeyCredentialNotFound,
		ErrPasskeyCeremonyTokenRequired,
		ErrPasskeyMissingRequestHost,
	} {
		if errors.Is(err, sentinel) {
			return passkeySentinelKey(sentinel), true
		}
	}

	lowered := strings.ToLower(err.Error())
	for _, entry := range errPasskeyLoadKeys {
		if strings.Contains(lowered, entry.needle) {
			return entry.key, true
		}
	}
	return "", false
}

// passkeySentinelKey returns the translation key registered for a sentinel
// error. Every sentinel in the list above is covered; the default arm exists so
// adding a sentinel without a key degrades to the generic key rather than
// panicking.
func passkeySentinelKey(sentinel error) string {
	switch sentinel {
	case ErrPasskeyNotFound:
		return "errors.passkey.passkey_not_found"
	case ErrPasskeySessionNotFound:
		return "errors.passkey.session_not_found"
	case ErrPasskeySessionExpired:
		return "errors.passkey.session_expired"
	case ErrPasskeyInvalidSession:
		return "errors.passkey.invalid_session"
	case ErrPasskeyInvalidRegistrationSession:
		return "errors.passkey.invalid_registration_session"
	case ErrPasskeyNamePasswordRequired:
		return "errors.passkey.name_and_password_are_required"
	case ErrPasskeyInvalidPassword:
		return "errors.passkey.invalid_password"
	case ErrPasskeyNoCredentials:
		return "errors.passkey.no_passkeys_registered"
	case ErrPasskeyInvalidRequestBody:
		return "errors.passkey.invalid_request_body"
	case ErrPasskeyInvalidSessionToken:
		return "errors.passkey.invalid_session_token"
	case ErrPasskeyCredentialNotFound:
		return "errors.passkey.credential_not_found"
	case ErrPasskeyCeremonyTokenRequired:
		return "errors.passkey.ceremony_token_required"
	case ErrPasskeyMissingRequestHost:
		return "errors.passkey.missing_request_host"
	}
	return "errors.internal_error"
}
