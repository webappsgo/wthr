package handler

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	models "github.com/webappsgo/wthr/src/server/model"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyEnvelope carries the per-request inputs that the WebAuthn ceremony
// needs (RPID/origin) without coupling to gin.Context. Cookie-free callers
// (such as GraphQL resolvers) construct one from the request context.
type PasskeyEnvelope struct {
	Host  string
	HTTPS bool
}

// PasskeySummary mirrors the JSON shape returned by the REST passkey list
// endpoint so REST and GraphQL clients see identical data.
type PasskeySummary struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	CreatedAt  string      `json:"created_at"`
	LastUsedAt *string     `json:"last_used_at,omitempty"`
	Raw        *models.UserPasskey `json:"-"`
}

func buildWebAuthnFromEnvelope(env PasskeyEnvelope) (*webauthn.WebAuthn, error) {
	host := strings.TrimSpace(env.Host)
	if host == "" {
		return nil, fmt.Errorf("missing request host")
	}

	rpID := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		rpID = parsedHost
	}
	rpID = strings.TrimPrefix(strings.TrimSuffix(rpID, "]"), "[")

	scheme := "http"
	if env.HTTPS {
		scheme = "https"
	}

	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: "Weather",
		RPOrigins:     []string{fmt.Sprintf("%s://%s", scheme, host)},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true},
			Registration: webauthn.TimeoutConfig{Enforce: true},
		},
	})
}

func buildHTTPRequestFromBody(body []byte) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func issuePasskeyCeremonyToken(state *passkeyCeremonyState) (string, error) {
	token, err := models.GenerateSessionID()
	if err != nil {
		return "", fmt.Errorf("failed to generate passkey session: %w", err)
	}
	passkeyCeremonyCache.Set(token, state, passkeyCeremonyTTL)
	return token, nil
}

func loadPasskeyCeremonyByToken(token string) (*passkeyCeremonyState, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("passkey session not found")
	}
	rawState, found := passkeyCeremonyCache.Get(token)
	if !found {
		return nil, fmt.Errorf("passkey session expired")
	}
	state, ok := rawState.(*passkeyCeremonyState)
	if !ok || state == nil {
		return nil, fmt.Errorf("invalid passkey session")
	}
	return state, nil
}

func summarizePasskey(passkey *models.UserPasskey) *PasskeySummary {
	if passkey == nil {
		return nil
	}
	summary := &PasskeySummary{
		ID:        passkey.ID,
		Name:      passkey.Name,
		CreatedAt: passkey.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Raw:       passkey,
	}
	if passkey.LastUsedAt != nil {
		v := passkey.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
		summary.LastUsedAt = &v
	}
	return summary
}

// ListUserPasskeys returns the authenticated user's passkeys in the same
// order/shape as GET /api/v1/users/security/passkeys.
func ListUserPasskeys(db *sql.DB, userID int64) ([]*PasskeySummary, error) {
	model := &models.UserPasskeyModel{DB: db}
	passkeys, err := model.ListByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load passkeys: %w", err)
	}

	summaries := make([]*PasskeySummary, 0, len(passkeys))
	for _, passkey := range passkeys {
		summaries = append(summaries, summarizePasskey(passkey))
	}
	return summaries, nil
}

// DeleteUserPasskey removes one of the authenticated user's passkeys.
// Mirrors DELETE /api/v1/users/security/passkeys/:passkey_id.
func DeleteUserPasskey(db *sql.DB, userID, passkeyID int64) error {
	model := &models.UserPasskeyModel{DB: db}
	return model.DeleteByID(userID, passkeyID)
}

func loadWebAuthnUserForPasskey(db *sql.DB, user *models.User) (*passkeyUser, error) {
	model := &models.UserPasskeyModel{DB: db}
	credentials, err := model.ListCredentialsByUserID(user.ID)
	if err != nil {
		return nil, err
	}
	return &passkeyUser{user: user, credentials: credentials}, nil
}

// PasskeyRegistrationOptionsResult is the begin-registration response shared
// by REST and GraphQL callers.
type PasskeyRegistrationOptionsResult struct {
	CeremonyToken string                       `json:"ceremony_token"`
	Options       *protocol.CredentialCreation `json:"options"`
}

// BeginPasskeyRegistrationToken starts a passkey registration ceremony and
// returns an opaque ceremony token plus the WebAuthn options the browser
// should pass to navigator.credentials.create(). The caller is responsible
// for transporting the token (cookie for REST, mutation argument for GraphQL).
func BeginPasskeyRegistrationToken(db *sql.DB, user *models.User, env PasskeyEnvelope, name, password string) (*PasskeyRegistrationOptionsResult, error) {
	name = strings.TrimSpace(name)
	password = strings.TrimSpace(password)
	if name == "" || password == "" {
		return nil, fmt.Errorf("passkey name and password are required")
	}

	userModel := &models.UserModel{DB: db}
	if !userModel.CheckPassword(user, password) {
		return nil, fmt.Errorf("invalid password")
	}

	waUser, err := loadWebAuthnUserForPasskey(db, user)
	if err != nil {
		return nil, fmt.Errorf("failed to load passkeys: %w", err)
	}

	exclusions := make([]protocol.CredentialDescriptor, 0, len(waUser.credentials))
	for _, credential := range waUser.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	options, sessionData, err := wa.BeginRegistration(
		waUser,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
		}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		return nil, err
	}

	token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
		Kind:        passkeyKindRegistration,
		UserID:      user.ID,
		Name:        name,
		SessionData: *sessionData,
	})
	if err != nil {
		return nil, err
	}

	return &PasskeyRegistrationOptionsResult{
		CeremonyToken: token,
		Options:       options,
	}, nil
}

// PasskeyRegistrationFinalizeResult is the finish-registration response shared
// by REST and GraphQL callers. RecoveryKeys is non-nil only when this is the
// user's first passkey AND no recovery keys exist yet.
type PasskeyRegistrationFinalizeResult struct {
	Passkey      *PasskeySummary `json:"passkey"`
	RecoveryKeys []string        `json:"recovery_keys,omitempty"`
}

// FinishPasskeyRegistrationToken completes the registration ceremony begun by
// BeginPasskeyRegistrationToken. The body must be the JSON body produced by
// the browser's PublicKeyCredential.toJSON() call.
func FinishPasskeyRegistrationToken(db *sql.DB, user *models.User, env PasskeyEnvelope, ceremonyToken string, body []byte) (*PasskeyRegistrationFinalizeResult, error) {
	state, err := loadPasskeyCeremonyByToken(ceremonyToken)
	if err != nil {
		return nil, err
	}
	if state.Kind != passkeyKindRegistration || state.UserID != user.ID {
		return nil, fmt.Errorf("invalid passkey registration session")
	}

	waUser, err := loadWebAuthnUserForPasskey(db, user)
	if err != nil {
		return nil, fmt.Errorf("failed to load passkeys: %w", err)
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	credential, err := wa.FinishRegistration(waUser, state.SessionData, buildHTTPRequestFromBody(body))
	if err != nil {
		return nil, err
	}

	passkeyModel := &models.UserPasskeyModel{DB: db}
	passkey, err := passkeyModel.Create(user.ID, state.Name, credential)
	if err != nil {
		return nil, err
	}

	passkeyCeremonyCache.Delete(ceremonyToken)

	result := &PasskeyRegistrationFinalizeResult{Passkey: summarizePasskey(passkey)}

	count, err := passkeyModel.CountByUserID(user.ID)
	if err == nil && count == 1 {
		recoveryKeyModel := &models.RecoveryKeyModel{DB: db}
		existingKeys, countErr := recoveryKeyModel.GetUnusedKeysCount(int(user.ID))
		if countErr == nil && existingKeys == 0 {
			recoveryKeys, generateErr := recoveryKeyModel.GenerateRecoveryKeys(int(user.ID))
			if generateErr == nil {
				result.RecoveryKeys = recoveryKeys
			}
		}
	}

	return result, nil
}

// PasskeyChallengeOptionsResult is the begin-login-challenge response shared
// by REST and GraphQL callers.
type PasskeyChallengeOptionsResult struct {
	CeremonyToken string                        `json:"ceremony_token"`
	Options       *protocol.CredentialAssertion `json:"options"`
}

// BeginPasskeyChallengeToken starts a passkey login challenge. If
// pendingSessionToken is non-empty, the challenge is bound to that pending
// 2FA session (passkey-as-second-factor). Otherwise it begins a discoverable
// login (passkey-as-first-factor).
func BeginPasskeyChallengeToken(db *sql.DB, env PasskeyEnvelope, pendingSessionToken string) (*PasskeyChallengeOptionsResult, error) {
	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	if strings.TrimSpace(pendingSessionToken) != "" {
		pendingSession, err := loadPendingTwoFactorSession(db, pendingSessionToken)
		if err != nil {
			return nil, err
		}

		userModel := &models.UserModel{DB: db}
		user, err := userModel.GetByID(int64(pendingSession.UserID))
		if err != nil {
			return nil, fmt.Errorf("invalid session token")
		}
		if err := validateAuthUser(user); err != nil {
			return nil, err
		}

		waUser, err := loadWebAuthnUserForPasskey(db, user)
		if err != nil {
			return nil, fmt.Errorf("failed to load passkeys: %w", err)
		}
		if len(waUser.credentials) == 0 {
			return nil, fmt.Errorf("no passkeys registered for this account")
		}

		options, sessionData, err := wa.BeginLogin(waUser, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return nil, err
		}

		token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
			Kind:                passkeyKindTwoFactor,
			UserID:              user.ID,
			PendingSessionToken: pendingSession.ID,
			SessionData:         *sessionData,
		})
		if err != nil {
			return nil, err
		}

		return &PasskeyChallengeOptionsResult{CeremonyToken: token, Options: options}, nil
	}

	options, sessionData, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}

	token, err := issuePasskeyCeremonyToken(&passkeyCeremonyState{
		Kind:        passkeyKindLogin,
		SessionData: *sessionData,
	})
	if err != nil {
		return nil, err
	}

	return &PasskeyChallengeOptionsResult{CeremonyToken: token, Options: options}, nil
}

// FinishPasskeyChallengeToken completes the login challenge begun by
// BeginPasskeyChallengeToken and returns a fully-authenticated AuthLoginResponse.
// The body must be the JSON body produced by the browser's
// PublicKeyCredential.toJSON() call.
func FinishPasskeyChallengeToken(db *sql.DB, env PasskeyEnvelope, ceremonyToken string, body []byte, clientIP string) (*AuthLoginResponse, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("invalid request body")
	}

	state, err := loadPasskeyCeremonyByToken(ceremonyToken)
	if err != nil {
		return nil, err
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	passkeyModel := &models.UserPasskeyModel{DB: db}
	userModel := &models.UserModel{DB: db}

	var (
		user       *models.User
		credential *webauthn.Credential
		response   *AuthLoginResponse
		req        = buildHTTPRequestFromBody(body)
	)

	switch state.Kind {
	case passkeyKindLogin:
		parsed, parseErr := protocol.ParseCredentialRequestResponse(req)
		if parseErr != nil {
			return nil, parseErr
		}

		lookup := func(rawID []byte, userHandle []byte) (webauthn.User, error) {
			userID, err := parsePasskeyUserHandle(userHandle)
			if err != nil {
				return nil, err
			}

			candidate, err := userModel.GetByID(userID)
			if err != nil {
				return nil, fmt.Errorf("failed to load user")
			}
			if err := validateAuthUser(candidate); err != nil {
				return nil, err
			}

			waUser, err := loadWebAuthnUserForPasskey(db, candidate)
			if err != nil {
				return nil, err
			}
			for _, credential := range waUser.credentials {
				if bytes.Equal(credential.ID, rawID) {
					return waUser, nil
				}
			}
			return nil, fmt.Errorf("credential not found")
		}

		waResolvedUser, resolvedCredential, validateErr := wa.ValidatePasskeyLogin(lookup, state.SessionData, parsed)
		if validateErr != nil {
			return nil, validateErr
		}

		resolvedUser, ok := waResolvedUser.(*passkeyUser)
		if !ok {
			return nil, fmt.Errorf("failed to resolve passkey user")
		}
		user = resolvedUser.user
		credential = resolvedCredential

		response, err = createFullAuthSession(db, user)
		if err != nil {
			return nil, fmt.Errorf("failed to create session")
		}

	case passkeyKindTwoFactor:
		pendingSession, sessionErr := loadPendingTwoFactorSession(db, state.PendingSessionToken)
		if sessionErr != nil {
			return nil, sessionErr
		}

		user, err = userModel.GetByID(int64(pendingSession.UserID))
		if err != nil {
			return nil, fmt.Errorf("invalid session token")
		}
		if err := validateAuthUser(user); err != nil {
			return nil, err
		}

		waUser, loadErr := loadWebAuthnUserForPasskey(db, user)
		if loadErr != nil {
			return nil, fmt.Errorf("failed to load passkeys: %w", loadErr)
		}

		credential, err = wa.FinishLogin(waUser, state.SessionData, req)
		if err != nil {
			return nil, err
		}

		response, err = createFullAuthSession(db, user)
		if err != nil {
			return nil, fmt.Errorf("failed to create session")
		}

		sessionModel := &models.SessionModel{DB: db}
		_ = sessionModel.Delete(pendingSession.ID)

	default:
		return nil, fmt.Errorf("invalid passkey session")
	}

	if err := passkeyModel.UpdateCredential(user.ID, credential); err != nil {
		return nil, fmt.Errorf("failed to update passkey: %w", err)
	}

	_ = userModel.UpdateLastLogin(user.ID, clientIP)
	passkeyCeremonyCache.Delete(ceremonyToken)

	return response, nil
}
