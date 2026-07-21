package handler

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	models "github.com/webappsgo/wthr/src/server/model"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/patrickmn/go-cache"
)

const (
	adminPendingSessionTTL = 15 * time.Minute
	// passkeyKindAdminLogin distinguishes admin passkey login ceremonies
	// from the user-side passkeyKindLogin / passkeyKindTwoFactor states
	// already keyed in passkeyCeremonyCache. Reusing the same cache (same
	// TTL, same eviction policy) keeps one state machine for all WebAuthn
	// ceremonies, while the kind field + the "adm:" user-handle prefix
	// prevent admin/user cross-contamination.
	passkeyKindAdminLogin = "admin_login"
)

// adminPendingSession is the in-memory record kept while an admin is in the
// post-password / pre-passkey window. It is not persisted to disk because
// the admin only has 15 minutes to complete the challenge — surviving a
// restart is unnecessary, and keeping it off disk avoids leaking an
// authenticated-pending-state row if backups are restored on a different
// host.
type adminPendingSession struct {
	AdminID   int64
	IPAddress string
	UserAgent string
	CreatedAt time.Time
}

// adminPendingSessionCache holds pre-passkey admin sessions. 15-minute TTL,
// 30-minute janitor.
var adminPendingSessionCache = cache.New(adminPendingSessionTTL, 30*time.Minute)

func generateAdminPendingSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate pending session id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// CreateAdminPendingSession issues a short-lived token bound to the given
// admin. The token is opaque to the client — they pass it back to
// /api/{api_version}/auth/admin/passkey/challenge to start the WebAuthn
// ceremony.
func CreateAdminPendingSession(adminID int64, ipAddress, userAgent string) (string, error) {
	if adminID <= 0 {
		return "", fmt.Errorf("invalid admin id")
	}
	token, err := generateAdminPendingSessionID()
	if err != nil {
		return "", err
	}
	adminPendingSessionCache.Set(token, &adminPendingSession{
		AdminID:   adminID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}, adminPendingSessionTTL)
	return token, nil
}

// LoadAdminPendingSession looks up a pending session by token. Returns an
// error if the token is unknown or expired.
func LoadAdminPendingSession(token string) (*adminPendingSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("invalid session token")
	}
	raw, found := adminPendingSessionCache.Get(token)
	if !found {
		return nil, fmt.Errorf("session token expired or invalid")
	}
	pending, ok := raw.(*adminPendingSession)
	if !ok || pending == nil {
		return nil, fmt.Errorf("invalid session token")
	}
	return pending, nil
}

// DeleteAdminPendingSession removes the pending session — call this once
// the admin has either completed the passkey challenge (full session
// created) or explicitly cancelled.
func DeleteAdminPendingSession(token string) {
	adminPendingSessionCache.Delete(strings.TrimSpace(token))
}

// AdminHasPasskeys returns true if the given admin has at least one
// registered passkey. Used by the login handler to decide whether to issue
// a pending session or complete the login outright.
func AdminHasPasskeys(db *sql.DB, adminID int64) (bool, error) {
	model := &models.AdminPasskeyModel{DB: db}
	return model.HasPasskeys(adminID)
}

// AdminPasskeyLoginOptionsResult is the begin-login-challenge response
// shared by REST and (future) GraphQL admin callers.
type AdminPasskeyLoginOptionsResult struct {
	CeremonyToken string                        `json:"ceremony_token"`
	Options       *protocol.CredentialAssertion `json:"options"`
}

// BeginAdminPasskeyLoginToken starts a passkey login challenge for the
// admin identified by the pending-session token. Returns an opaque
// ceremony token + the WebAuthn options the browser should pass to
// navigator.credentials.get().
func BeginAdminPasskeyLoginToken(db *sql.DB, env PasskeyEnvelope, pendingSessionToken string) (*AdminPasskeyLoginOptionsResult, error) {
	pending, err := LoadAdminPendingSession(pendingSessionToken)
	if err != nil {
		return nil, err
	}

	adminModel := &models.AdminModel{DB: db}
	admin, err := adminModel.GetByID(pending.AdminID)
	if err != nil || admin == nil {
		return nil, fmt.Errorf("invalid session token")
	}
	if !admin.IsActive {
		return nil, fmt.Errorf("admin account is disabled")
	}

	waUser, err := loadWebAuthnAdminUser(db, admin)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkeys: %w", err)
	}
	if len(waUser.credentials) == 0 {
		return nil, fmt.Errorf("no passkeys registered for this account")
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	options, sessionData, err := wa.BeginLogin(waUser, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}

	token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
		Kind:                passkeyKindAdminLogin,
		UserID:              admin.ID,
		PendingSessionToken: pendingSessionToken,
		SessionData:         *sessionData,
	})
	if err != nil {
		return nil, err
	}

	return &AdminPasskeyLoginOptionsResult{
		CeremonyToken: token,
		Options:       options,
	}, nil
}

// AdminPasskeyLoginResult is the finish-login-challenge response. The
// SessionID is the new admin_session cookie value the gin handler should
// set on its way out.
type AdminPasskeyLoginResult struct {
	SessionID string        `json:"session_id"`
	ExpiresAt time.Time     `json:"expires_at"`
	Admin     *models.Admin `json:"admin"`
}

// FinishAdminPasskeyLoginToken completes the admin login challenge.
// Verifies the WebAuthn credential, mints a full admin session, and
// invalidates the pending session token. The caller must set the
// admin_session cookie itself (this helper is gin-context-free so GraphQL
// can call it too).
func FinishAdminPasskeyLoginToken(db *sql.DB, env PasskeyEnvelope, ceremonyToken string, body []byte, clientIP, userAgent string, sessionDuration time.Duration) (*AdminPasskeyLoginResult, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("invalid request body")
	}

	state, err := loadPasskeyCeremonyByToken(ceremonyToken)
	if err != nil {
		return nil, err
	}
	if state.Kind != passkeyKindAdminLogin {
		return nil, fmt.Errorf("invalid passkey ceremony")
	}

	pending, err := LoadAdminPendingSession(state.PendingSessionToken)
	if err != nil {
		return nil, err
	}
	if pending.AdminID != state.UserID {
		return nil, fmt.Errorf("invalid session token")
	}

	adminModel := &models.AdminModel{DB: db}
	admin, err := adminModel.GetByID(pending.AdminID)
	if err != nil || admin == nil {
		return nil, fmt.Errorf("invalid session token")
	}
	if !admin.IsActive {
		return nil, fmt.Errorf("admin account is disabled")
	}

	waUser, err := loadWebAuthnAdminUser(db, admin)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkeys: %w", err)
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	credential, err := wa.FinishLogin(waUser, state.SessionData, buildHTTPRequestFromBody(body))
	if err != nil {
		return nil, err
	}

	// Verify the resolved credential actually belongs to this admin —
	// guards against a cross-admin replay if cache state ever leaks.
	matched := false
	for _, candidate := range waUser.credentials {
		if bytes.Equal(candidate.ID, credential.ID) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("credential not found")
	}

	passkeyModel := &models.AdminPasskeyModel{DB: db}
	if err := passkeyModel.UpdateCredential(admin.ID, credential); err != nil {
		return nil, fmt.Errorf("failed to update passkey: %w", err)
	}

	adminSessionModel := &models.AdminSessionModel{DB: db}
	adminSession, err := adminSessionModel.CreateSession(admin.ID, clientIP, userAgent, sessionDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin session: %w", err)
	}

	_ = adminModel.UpdateLastLogin(admin.ID)

	passkeyCeremonyCache.Delete(ceremonyToken)
	DeleteAdminPendingSession(state.PendingSessionToken)

	return &AdminPasskeyLoginResult{
		SessionID: adminSession.SessionID,
		ExpiresAt: adminSession.ExpiresAt,
		Admin:     admin,
	}, nil
}
