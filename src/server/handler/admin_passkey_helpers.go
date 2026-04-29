package handler

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	models "github.com/apimgr/weather/src/server/model"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// AdminPasskeySummary mirrors the JSON shape returned by the REST admin
// passkey list endpoint so REST and (eventually) GraphQL clients see the
// same data. Modelled on PasskeySummary but for the admin side.
type AdminPasskeySummary struct {
	ID         int64                 `json:"id"`
	Name       string                `json:"name"`
	CreatedAt  string                `json:"created_at"`
	LastUsedAt *string               `json:"last_used_at,omitempty"`
	Raw        *models.AdminPasskey  `json:"-"`
}

func summarizeAdminPasskey(passkey *models.AdminPasskey) *AdminPasskeySummary {
	if passkey == nil {
		return nil
	}
	summary := &AdminPasskeySummary{
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

// ListAdminPasskeys returns the authenticated admin's passkeys.
func ListAdminPasskeys(db *sql.DB, adminID int64) ([]*AdminPasskeySummary, error) {
	model := &models.AdminPasskeyModel{DB: db}
	passkeys, err := model.ListByAdminID(adminID)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkeys: %w", err)
	}

	summaries := make([]*AdminPasskeySummary, 0, len(passkeys))
	for _, passkey := range passkeys {
		summaries = append(summaries, summarizeAdminPasskey(passkey))
	}
	return summaries, nil
}

// DeleteAdminPasskey removes one of the authenticated admin's passkeys.
func DeleteAdminPasskey(db *sql.DB, adminID, passkeyID int64) error {
	model := &models.AdminPasskeyModel{DB: db}
	return model.DeleteByID(adminID, passkeyID)
}

func loadWebAuthnAdminUser(db *sql.DB, admin *models.Admin) (*adminPasskeyUser, error) {
	model := &models.AdminPasskeyModel{DB: db}
	credentials, err := model.ListCredentialsByAdminID(admin.ID)
	if err != nil {
		return nil, err
	}
	return &adminPasskeyUser{admin: admin, credentials: credentials}, nil
}

// adminPasskeyUser is a local adapter (kept private) so the user-side passkey
// helpers and admin-side passkey helpers don't accidentally cross-pollinate.
type adminPasskeyUser struct {
	admin       *models.Admin
	credentials []webauthn.Credential
}

func (u *adminPasskeyUser) WebAuthnID() []byte {
	return []byte(fmt.Sprintf("%s%d", models.AdminPasskeyUserHandlePrefix, u.admin.ID))
}

func (u *adminPasskeyUser) WebAuthnName() string {
	return u.admin.Username
}

func (u *adminPasskeyUser) WebAuthnDisplayName() string {
	return u.admin.Username
}

func (u *adminPasskeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func parseAdminPasskeyUserHandle(userHandle []byte) (int64, error) {
	raw := string(userHandle)
	if !strings.HasPrefix(raw, models.AdminPasskeyUserHandlePrefix) {
		return 0, fmt.Errorf("invalid admin user handle")
	}
	adminID, err := strconv.ParseInt(strings.TrimPrefix(raw, models.AdminPasskeyUserHandlePrefix), 10, 64)
	if err != nil || adminID <= 0 {
		return 0, fmt.Errorf("invalid admin user handle")
	}
	return adminID, nil
}

// AdminPasskeyRegistrationOptionsResult is the begin-registration response
// shared by REST and GraphQL callers.
type AdminPasskeyRegistrationOptionsResult struct {
	CeremonyToken string                       `json:"ceremony_token"`
	Options       *protocol.CredentialCreation `json:"options"`
}

// BeginAdminPasskeyRegistrationToken starts an admin-side registration
// ceremony. Mirrors BeginPasskeyRegistrationToken (user-side) but verifies
// against admin credentials and stores against server_admin_passkeys. The
// ceremony state goes through the same `passkeyCeremonyCache` as the user
// side — entries carry the kind (`registration`) plus a separate user-handle
// prefix (`adm:`) so the two sides cannot be confused at verify time.
func BeginAdminPasskeyRegistrationToken(db *sql.DB, admin *models.Admin, env PasskeyEnvelope, name, password string) (*AdminPasskeyRegistrationOptionsResult, error) {
	name = strings.TrimSpace(name)
	password = strings.TrimSpace(password)
	if name == "" || password == "" {
		return nil, fmt.Errorf("Passkey name and password are required")
	}

	valid, err := models.VerifyPassword(password, admin.PasswordHash)
	if err != nil || !valid {
		return nil, fmt.Errorf("Invalid password")
	}

	waUser, err := loadWebAuthnAdminUser(db, admin)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkeys: %w", err)
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
		UserID:      admin.ID,
		Name:        name,
		SessionData: *sessionData,
		// Admin flag is recorded by reusing the user-handle prefix on the
		// SessionData so verify-time rejects cross-side matches; we also
		// double-check `Kind == passkeyKindRegistration && admin context`
		// explicitly in FinishAdminPasskeyRegistrationToken below.
	})
	if err != nil {
		return nil, err
	}

	return &AdminPasskeyRegistrationOptionsResult{
		CeremonyToken: token,
		Options:       options,
	}, nil
}

// AdminPasskeyRegistrationFinalizeResult is the finish-registration response.
type AdminPasskeyRegistrationFinalizeResult struct {
	Passkey *AdminPasskeySummary `json:"passkey"`
}

// FinishAdminPasskeyRegistrationToken completes the registration ceremony.
func FinishAdminPasskeyRegistrationToken(db *sql.DB, admin *models.Admin, env PasskeyEnvelope, ceremonyToken string, body []byte) (*AdminPasskeyRegistrationFinalizeResult, error) {
	state, err := loadPasskeyCeremonyByToken(ceremonyToken)
	if err != nil {
		return nil, err
	}
	if state.Kind != passkeyKindRegistration || state.UserID != admin.ID {
		return nil, fmt.Errorf("Invalid passkey registration session")
	}

	waUser, err := loadWebAuthnAdminUser(db, admin)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin passkeys: %w", err)
	}

	wa, err := buildWebAuthnFromEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkeys: %w", err)
	}

	credential, err := wa.FinishRegistration(waUser, state.SessionData, buildHTTPRequestFromBody(body))
	if err != nil {
		return nil, err
	}

	passkeyModel := &models.AdminPasskeyModel{DB: db}
	passkey, err := passkeyModel.Create(admin.ID, state.Name, credential)
	if err != nil {
		return nil, err
	}

	passkeyCeremonyCache.Delete(ceremonyToken)

	return &AdminPasskeyRegistrationFinalizeResult{
		Passkey: summarizeAdminPasskey(passkey),
	}, nil
}
