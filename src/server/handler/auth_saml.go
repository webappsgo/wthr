package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

const samlStateCookie = "saml_login_state"

type pendingSAMLLogin struct {
	Provider  string
	RequestID string
	Expires   time.Time
}

type pendingSAMLIdentity struct {
	Provider        string
	ExternalID      string
	NameID          string
	Email           string
	Name            string
	Username        string
	Groups          []string
	Role            string
	IsAdmin         bool
	RequireAdminMFA bool
	AdminID         int64
	SessionIndex    string
	APIRelayState   string
	Expires         time.Time
}

// SAMLAuthHandler implements browser SSO and shares the existing external
// username confirmation URL with OIDC.
type SAMLAuthHandler struct {
	DB         *sql.DB
	Service    *service.SAMLService
	OIDC       *OIDCAuthHandler
	mu         sync.Mutex
	requests   map[string]pendingSAMLLogin
	identities map[string]pendingSAMLIdentity
}

func (h *SAMLAuthHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !h.Service.Enabled() {
		http.Error(w, "SAML authentication is disabled", http.StatusServiceUnavailable)
		return
	}
	cfg, err := h.Service.Provider(provider)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	if sp.GetSSOBindingLocation(saml.HTTPRedirectBinding) == "" {
		h.failed(w, r, provider)
		return
	}
	state, err := service.GenerateState()
	if err != nil {
		http.Error(w, "Unable to begin SAML login", http.StatusInternalServerError)
		return
	}
	request, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	h.mu.Lock()
	if h.requests == nil {
		h.requests = make(map[string]pendingSAMLLogin)
	}
	h.prune()
	h.requests[state] = pendingSAMLLogin{Provider: provider, RequestID: request.ID, Expires: time.Now().Add(10 * time.Minute)}
	h.mu.Unlock()
	redirect, err := request.Redirect(state, sp)
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: samlStateCookie, Value: state, Path: "/server/auth/saml/", MaxAge: 600, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (h *SAMLAuthHandler) StartAPILogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !h.Service.Enabled() {
		RespondNegotiatedData(w, r, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SAML_DISABLED"})
		return
	}
	if _, err := h.Service.Provider(provider); err != nil {
		RespondNegotiatedData(w, r, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND"})
		return
	}
	recorder := httptest.NewRecorder()
	h.StartLogin(recorder, r)
	if recorder.Code != http.StatusFound {
		RespondNegotiatedData(w, r, recorder.Code, map[string]interface{}{"ok": false, "error": "SAML_START_FAILED"})
		return
	}
	var relayState string
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == samlStateCookie {
			relayState = cookie.Value
			break
		}
	}
	if relayState == "" {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_START_FAILED"})
		return
	}
	h.mu.Lock()
	pending, ok := h.requests[relayState]
	h.mu.Unlock()
	if !ok || pending.Provider != provider {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_START_FAILED"})
		return
	}
	requestID, err := models.GenerateSecureToken(32)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_START_FAILED"})
		return
	}
	expires := dbtime.FormatSQLTimestamp(time.Now().Add(5 * time.Minute))
	_, err = database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite, `INSERT INTO saml_api_requests(request_id,authn_request_id,provider_name,relay_state,client_binding,expires_at) VALUES(?,?,?,?,?,?)`, requestID, pending.RequestID, provider, relayState, samlAPIClientBinding(r), expires)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_START_FAILED"})
		return
	}
	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"redirect_url": recorder.Header().Get("Location"), "request_id": requestID}})
}

func (h *SAMLAuthHandler) ListAPIProviders(w http.ResponseWriter, r *http.Request) {
	if !h.Service.Enabled() {
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": []interface{}{}})
		return
	}
	type providerItem struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	items := make([]providerItem, 0)
	for _, provider := range h.Service.Providers() {
		items = append(items, providerItem{Name: provider.Name, DisplayName: provider.DisplayName})
	}
	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": items})
}

func (h *SAMLAuthHandler) ExchangeAPILogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	var request struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil || request.RequestID == "" {
		RespondNegotiatedData(w, r, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "BAD_REQUEST"})
		return
	}
	var clientBinding, codeHash, sessionToken, expires string
	var completed, usernameRequired, consumed bool
	err := database.QueryRowContext(r.Context(), database.GetServerDB(), database.TimeoutSimpleSelect, `SELECT client_binding,code_hash,session_token,completed,username_required,consumed,expires_at FROM saml_api_requests WHERE request_id=? AND provider_name=?`, request.RequestID, provider).Scan(&clientBinding, &codeHash, &sessionToken, &completed, &usernameRequired, &consumed, &expires)
	if err == sql.ErrNoRows {
		RespondNegotiatedData(w, r, http.StatusGone, map[string]interface{}{"ok": false, "error": "EXPIRED"})
		return
	}
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR"})
		return
	}
	if !time.Now().Before(parseSAMLAPITime(expires)) {
		_, _ = database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite, `DELETE FROM saml_api_requests WHERE request_id=?`, request.RequestID)
		RespondNegotiatedData(w, r, http.StatusGone, map[string]interface{}{"ok": false, "error": "EXPIRED"})
		return
	}
	if clientBinding != samlAPIClientBinding(r) {
		RespondNegotiatedData(w, r, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "FORBIDDEN"})
		return
	}
	if consumed {
		RespondNegotiatedData(w, r, http.StatusGone, map[string]interface{}{"ok": false, "error": "ALREADY_EXCHANGED"})
		return
	}
	if !completed {
		RespondNegotiatedData(w, r, http.StatusAccepted, map[string]interface{}{"ok": true, "data": map[string]interface{}{"status": "pending", "username_confirmation_required": usernameRequired}})
		return
	}
	if request.Code != "" {
		sum := sha256.Sum256([]byte(request.Code))
		if hex.EncodeToString(sum[:]) != codeHash {
			RespondNegotiatedData(w, r, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "FORBIDDEN"})
			return
		}
	}
	clearSessionToken, decryptErr := decryptSAMLAPISessionToken(sessionToken)
	if decryptErr != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR"})
		return
	}
	result, err := database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite, `UPDATE saml_api_requests SET consumed=1 WHERE request_id=? AND consumed=0 AND completed=1`, request.RequestID)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "INTERNAL_ERROR"})
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		RespondNegotiatedData(w, r, http.StatusGone, map[string]interface{}{"ok": false, "error": "ALREADY_EXCHANGED"})
		return
	}
	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"session_token": clearSessionToken}})
}

// APIAuthMiddleware accepts API tokens and the one-time session tokens issued
// by SAML API exchange for the SAML logout endpoint.
func (h *SAMLAuthHandler) APIAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			RespondNegotiatedData(w, r, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "UNAUTHORIZED"})
			return
		}
		token := parts[1]
		if middleware.DetectTokenType(token) == middleware.TokenTypeAdmin || middleware.DetectTokenType(token) == middleware.TokenTypeUser {
			middleware.TokenAuthMiddleware(database.GetServerDB(), database.GetUsersDB())(next).ServeHTTP(w, r)
			return
		}
		if adminSession, err := (&models.AdminSessionModel{DB: database.GetServerDB()}).GetSession(token); err == nil {
			admin, loadErr := (&models.AdminModel{DB: database.GetUsersDB()}).GetByID(adminSession.AdminID)
			if loadErr == nil && admin.IsActive {
				ctx := reqctx.SetValue(r.Context(), "admin_id", int(admin.ID))
				ctx = reqctx.SetValue(ctx, "admin", admin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if userSession, err := (&models.UserSessionModel{DB: database.GetUsersDB()}).GetSession(token); err == nil {
			user, loadErr := (&models.UserModel{DB: database.GetUsersDB()}).GetByID(int64(userSession.UserID))
			if loadErr == nil && user.IsActive && !user.IsBanned {
				ctx := reqctx.SetValue(r.Context(), middleware.UserContextKey, user)
				ctx = reqctx.SetValue(ctx, middleware.UserIDContextKey, int(user.ID))
				ctx = reqctx.SetValue(ctx, middleware.SessionContextKey, &models.Session{ID: token, UserID: int(user.ID)})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		RespondNegotiatedData(w, r, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "TOKEN_INVALID"})
	})
}

func samlAPIClientBinding(r *http.Request) string {
	sum := sha256.Sum256([]byte(util.TrustedGetClientIP(r) + "\x00" + r.UserAgent()))
	return hex.EncodeToString(sum[:])
}

func parseSAMLAPITime(value string) time.Time {
	parsed, ok := dbtime.ParseStoredTimestampText(value)
	if !ok {
		return time.Time{}
	}
	return parsed
}

func markSAMLAPIRequest(ctx context.Context, relayState, sessionToken string) (string, error) {
	if relayState == "" {
		return "", nil
	}
	code, err := models.GenerateSecureToken(32)
	if err != nil {
		return "", err
	}
	sealedSessionToken, err := encryptSAMLAPISessionToken(sessionToken)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(code))
	result, err := database.ExecContext(ctx, database.GetServerDB(), database.TimeoutWrite, `UPDATE saml_api_requests SET completed=1,username_required=0,session_token=?,code_hash=? WHERE relay_state=? AND completed=0 AND expires_at>?`, sealedSessionToken, hex.EncodeToString(sum[:]), relayState, dbtime.FormatSQLTimestamp(time.Now()))
	if err != nil {
		return "", err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return "", errors.New("SAML API request is no longer active")
	}
	return code, nil
}

func encryptSAMLAPISessionToken(token string) (string, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || strings.TrimSpace(cfg.Server.Security.EncryptionKey) == "" {
		return "", errors.New("SAML API token encryption is not configured")
	}
	return util.EncryptAtRest(cfg.Server.Security.EncryptionKey, token)
}

func decryptSAMLAPISessionToken(token string) (string, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || strings.TrimSpace(cfg.Server.Security.EncryptionKey) == "" {
		return "", errors.New("SAML API token encryption is not configured")
	}
	return util.DecryptAtRest(cfg.Server.Security.EncryptionKey, token)
}

func markSAMLAPIUsernameRequired(ctx context.Context, relayState string) error {
	if relayState == "" {
		return nil
	}
	_, err := database.ExecContext(ctx, database.GetServerDB(), database.TimeoutWrite, `UPDATE saml_api_requests SET username_required=1 WHERE relay_state=? AND completed=0`, relayState)
	return err
}

func (h *SAMLAuthHandler) Metadata(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, err := h.Service.Provider(provider)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		http.Error(w, "SAML metadata is unavailable", http.StatusServiceUnavailable)
		return
	}
	data, err := service.MarshalSAMLMetadata(sp)
	if err != nil {
		http.Error(w, "SAML metadata is unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(data)
}

func (h *SAMLAuthHandler) ACS(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !h.Service.Enabled() {
		http.Error(w, "SAML authentication is disabled", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := r.ParseForm(); err != nil {
		h.failed(w, r, provider)
		return
	}
	cfg, err := h.Service.Provider(provider)
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	state := r.Form.Get("RelayState")
	apiRelayState := ""
	possibleRequestIDs := []string{}
	if state != "" {
		apiRequestID := ""
		stateCookie, cookieErr := r.Cookie(samlStateCookie)
		if cookieErr != nil || stateCookie.Value != state {
			apiErr := database.QueryRowContext(r.Context(), database.GetServerDB(), database.TimeoutSimpleSelect, `SELECT request_id FROM saml_api_requests WHERE provider_name=? AND relay_state=? AND completed=0`, provider, state).Scan(&apiRequestID)
			if apiErr != nil {
				h.failed(w, r, provider)
				return
			}
			apiRelayState = state
		}
		h.mu.Lock()
		request, ok := h.requests[state]
		delete(h.requests, state)
		h.mu.Unlock()
		if !ok && apiRelayState != "" {
			var authnRequestID, expiry string
			if dbErr := database.QueryRowContext(r.Context(), database.GetServerDB(), database.TimeoutSimpleSelect, `SELECT authn_request_id,expires_at FROM saml_api_requests WHERE request_id=? AND provider_name=?`, apiRequestID, provider).Scan(&authnRequestID, &expiry); dbErr == nil && time.Now().Before(parseSAMLAPITime(expiry)) {
				request = pendingSAMLLogin{Provider: provider, RequestID: authnRequestID, Expires: parseSAMLAPITime(expiry)}
				ok = true
			}
		}
		if !ok || request.Provider != provider || time.Now().After(request.Expires) {
			h.failed(w, r, provider)
			return
		}
		possibleRequestIDs = append(possibleRequestIDs, request.RequestID)
		http.SetCookie(w, &http.Cookie{Name: samlStateCookie, Value: "", Path: "/server/auth/saml/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	} else if !cfg.AllowIDPInitiated {
		h.failed(w, r, provider)
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	acsRequest := r.Clone(r.Context())
	acsRequest.URL, err = url.Parse(util.BuildURL(r, r.URL.Path))
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	assertion, err := h.Service.ParseResponse(sp, cfg, acsRequest, possibleRequestIDs)
	if err != nil || assertion == nil || assertion.Subject == nil || assertion.Subject.NameID == nil || assertion.Subject.NameID.Value == "" {
		h.failed(w, r, provider)
		return
	}
	externalID := assertion.Subject.NameID.Value
	if cfg.WantAssertionsSigned && assertion.Signature == nil {
		h.failed(w, r, provider)
		return
	}
	attributes := make(map[string][]string)
	for _, statement := range assertion.AttributeStatements {
		for _, attr := range statement.Attributes {
			for _, value := range attr.Values {
				attributes[attr.Name] = append(attributes[attr.Name], value.Value)
			}
		}
	}
	get := func(name string) string {
		if v := attributes[name]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if assertion.Subject.NameID.Format == string(saml.TransientNameIDFormat) {
		externalID = get(cfg.AttributeMapping.ExternalID)
		if externalID == "" {
			h.failed(w, r, provider)
			return
		}
	}
	username := get(cfg.AttributeMapping.Username)
	email := get(cfg.AttributeMapping.Email)
	name := get(cfg.AttributeMapping.Name)
	groups := attributes[cfg.AttributeMapping.Groups]
	role := mappedSAMLRole(groups, cfg.RoleMapping)
	sessionIndex := ""
	if len(assertion.AuthnStatements) > 0 {
		sessionIndex = assertion.AuthnStatements[0].SessionIndex
	}
	usersDB := database.GetUsersDB()
	if samlGroupMatch(groups, cfg.AdminGroups) {
		var adminID int64
		adminErr := database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect, `SELECT admin_id FROM admin_saml_mappings WHERE provider_name=? AND external_id=?`, provider, externalID).Scan(&adminID)
		if adminErr == sql.ErrNoRows {
			if err := markSAMLAPIUsernameRequired(r.Context(), apiRelayState); err != nil {
				h.failed(w, r, provider)
				return
			}
			pending, tokenErr := models.GenerateSecureToken(32)
			if tokenErr != nil {
				h.failed(w, r, provider)
				return
			}
			h.mu.Lock()
			if h.identities == nil {
				h.identities = make(map[string]pendingSAMLIdentity)
			}
			h.prune()
			h.identities[pending] = pendingSAMLIdentity{Provider: provider, ExternalID: externalID, NameID: assertion.Subject.NameID.Value, Email: email, Name: name, Username: username, Groups: groups, IsAdmin: true, SessionIndex: sessionIndex, APIRelayState: apiRelayState, Expires: time.Now().Add(10 * time.Minute)}
			h.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: pending, Path: "/server/auth/external/username", MaxAge: 600, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/server/auth/external/username", http.StatusSeeOther)
			return
		}
		if adminErr != nil {
			h.failed(w, r, provider)
			return
		}
		admin, loadErr := (&models.AdminModel{DB: usersDB}).GetByID(adminID)
		if loadErr != nil || !admin.IsActive {
			http.Error(w, "Admin account is unavailable", http.StatusForbidden)
			return
		}
		mfaStatus, mfaErr := models.AdminTOTPStatusFor(usersDB, adminID)
		hasPasskeys, passkeyErr := (&models.AdminPasskeyModel{DB: usersDB}).HasPasskeys(adminID)
		if mfaErr != nil || passkeyErr != nil {
			h.failed(w, r, provider)
			return
		}
		if hasPasskeys && !mfaStatus.Enabled {
			http.Error(w, "This administrator must complete the configured passkey challenge using the local login flow", http.StatusForbidden)
			return
		}
		if mfaStatus.Enabled {
			h.beginSAMLAdminMFA(w, r, pendingSAMLIdentity{Provider: provider, ExternalID: externalID, NameID: assertion.Subject.NameID.Value, Email: email, Name: name, Groups: groups, IsAdmin: true, RequireAdminMFA: true, AdminID: adminID, SessionIndex: sessionIndex, APIRelayState: apiRelayState})
			return
		}
		groupsJSON, _ := json.Marshal(groups)
		now := dbtime.FormatSQLTimestamp(time.Now())
		if _, syncErr := database.ExecContext(r.Context(), usersDB, database.TimeoutWrite, `UPDATE admin_saml_mappings SET email=?,name=?,groups_json=?,last_sync_at=?,updated_at=? WHERE provider_name=? AND external_id=? AND admin_id=?`, email, name, string(groupsJSON), now, now, provider, externalID, adminID); syncErr != nil {
			h.failed(w, r, provider)
			return
		}
		h.createAdminSession(w, r, usersDB, admin, pendingSAMLIdentity{Provider: provider, ExternalID: externalID, NameID: assertion.Subject.NameID.Value, SessionIndex: sessionIndex, APIRelayState: apiRelayState})
		return
	}
	var priorAdmin int
	if adminErr := database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect, `SELECT 1 FROM admin_saml_mappings WHERE provider_name=? AND external_id=?`, provider, externalID).Scan(&priorAdmin); adminErr == nil {
		h.failed(w, r, provider)
		return
	} else if adminErr != sql.ErrNoRows {
		h.failed(w, r, provider)
		return
	}
	var userID int64
	err = database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect, `SELECT user_id FROM user_saml_mappings WHERE provider_name = ? AND external_id = ?`, provider, externalID).Scan(&userID)
	if err == sql.ErrNoRows {
		if !cfg.AutoRegister {
			http.Error(w, "Account is not provisioned for this SAML identity", http.StatusForbidden)
			return
		}
		if err := markSAMLAPIUsernameRequired(r.Context(), apiRelayState); err != nil {
			h.failed(w, r, provider)
			return
		}
		pending, tokenErr := models.GenerateSecureToken(32)
		if tokenErr != nil {
			h.failed(w, r, provider)
			return
		}
		h.mu.Lock()
		if h.identities == nil {
			h.identities = make(map[string]pendingSAMLIdentity)
		}
		h.prune()
		h.identities[pending] = pendingSAMLIdentity{Provider: provider, ExternalID: externalID, NameID: assertion.Subject.NameID.Value, Email: email, Name: name, Username: username, Groups: groups, Role: role, SessionIndex: sessionIndex, APIRelayState: apiRelayState, Expires: time.Now().Add(10 * time.Minute)}
		h.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: pending, Path: "/server/auth/external/username", MaxAge: 600, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/server/auth/external/username", http.StatusSeeOther)
		return
	}
	if err != nil {
		h.failed(w, r, provider)
		return
	}
	var active, banned bool
	if err := database.QueryRowContext(r.Context(), usersDB, database.TimeoutSimpleSelect, "SELECT is_active, is_banned FROM users WHERE id = ?", userID).Scan(&active, &banned); err != nil || !active || banned {
		http.Error(w, "Account is unavailable", http.StatusForbidden)
		return
	}
	if err := h.syncMapping(r.Context(), usersDB, userID, provider, externalID, email, name, groups, role); err != nil {
		h.failed(w, r, provider)
		return
	}
	h.createSAMLUserSession(w, r, usersDB, userID, pendingSAMLIdentity{Provider: provider, ExternalID: externalID, NameID: assertion.Subject.NameID.Value, SessionIndex: sessionIndex, APIRelayState: apiRelayState})
}

func (h *SAMLAuthHandler) beginSAMLAdminMFA(w http.ResponseWriter, r *http.Request, pending pendingSAMLIdentity) {
	token, err := models.GenerateSecureToken(32)
	if err != nil {
		h.failed(w, r, pending.Provider)
		return
	}
	pending.Expires = time.Now().Add(10 * time.Minute)
	h.mu.Lock()
	if h.identities == nil {
		h.identities = make(map[string]pendingSAMLIdentity)
	}
	h.prune()
	h.identities[token] = pending
	h.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: token, Path: "/server/auth/external/username", MaxAge: 600, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/server/auth/external/username", http.StatusSeeOther)
}

func (h *SAMLAuthHandler) ShowExternalUsername(w http.ResponseWriter, r *http.Request) {
	pending, ok := h.pendingIdentity(r)
	if !ok {
		h.OIDC.ShowExternalUsername(w, r)
		return
	}
	username := normalizeSAMLUsername(pending.Username, pending.Email)
	if pending.IsAdmin {
		if util.ValidateAdminUsername(username) != nil || !availableSAMLAdminUsername(r.Context(), database.GetUsersDB(), username) {
			username = suggestSAMLAdminUsername(r.Context(), database.GetUsersDB(), username)
		}
	} else if util.ValidateUsername(username) != nil || !availableSAMLUsername(r.Context(), database.GetUsersDB(), username) {
		username = suggestSAMLUsername(r.Context(), database.GetUsersDB(), username)
	}
	middleware.RenderHTML(w, r, http.StatusOK, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": username, "RequireAdminMFA": pending.RequireAdminMFA, "IsAdmin": pending.IsAdmin}))
}

func (h *SAMLAuthHandler) CompleteExternalUsername(w http.ResponseWriter, r *http.Request) {
	pending, ok := h.pendingIdentity(r)
	if !ok {
		h.OIDC.CompleteExternalUsername(w, r)
		return
	}
	if pending.RequireAdminMFA {
		h.completeSAMLAdminMFA(w, r, pending)
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	usersDB := database.GetUsersDB()
	if pending.IsAdmin {
		h.completeSAMLAdminUsername(w, r, pending, username, r.PostFormValue("password"), r.PostFormValue("confirm_password"), usersDB)
		return
	}
	if util.ValidateUsername(username) != nil || !availableSAMLUsername(r.Context(), usersDB, username) {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": username, "error": "Choose a valid, available username."}))
		return
	}
	ctx := r.Context()
	token, err := models.GenerateSecureToken(32)
	if err != nil {
		h.failed(w, r, pending.Provider)
		return
	}
	hash, err := models.HashPassword(token)
	if err != nil {
		h.failed(w, r, pending.Provider)
		return
	}
	now := dbtime.FormatSQLTimestamp(time.Now())
	email := pending.Email
	if email == "" {
		email = username + "@" + pending.Provider + ".saml"
	}
	name := pending.Name
	if name == "" {
		name = username
	}
	tx, err := usersDB.BeginTx(ctx, nil)
	if err != nil {
		h.failed(w, r, pending.Provider)
		return
	}
	role := pending.Role
	if role == "" {
		role = "user"
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO users (username,email,display_name,password_hash,role,is_active,created_at,updated_at) VALUES (?,?,?,?,?,1,?,?)`, username, email, name, hash, role, now, now)
	if err != nil {
		_ = tx.Rollback()
		middleware.RenderHTML(w, r, http.StatusConflict, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": suggestSAMLUsername(ctx, usersDB, username), "error": "That username is unavailable. Choose another."}))
		return
	}
	userID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		h.failed(w, r, pending.Provider)
		return
	}
	groups, _ := json.Marshal(pending.Groups)
	_, err = tx.ExecContext(ctx, `INSERT INTO user_saml_mappings (user_id,provider_name,external_id,email,name,groups_json,last_sync_at) VALUES (?,?,?,?,?,?,?)`, userID, pending.Provider, pending.ExternalID, email, name, string(groups), now)
	if err != nil {
		_ = tx.Rollback()
		h.failed(w, r, pending.Provider)
		return
	}
	if err := tx.Commit(); err != nil {
		h.failed(w, r, pending.Provider)
		return
	}
	h.consumePending(r)
	http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: "", Path: "/server/auth/external/username", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	h.createSAMLUserSession(w, r, usersDB, userID, pending)
}

func (h *SAMLAuthHandler) completeSAMLAdminMFA(w http.ResponseWriter, r *http.Request, pending pendingSAMLIdentity) {
	valid, err := VerifyAdminTOTP(database.GetUsersDB(), pending.AdminID, strings.TrimSpace(r.PostFormValue("two_factor_code")), r.PostFormValue("use_recovery_key") == "true")
	if err != nil || !valid {
		middleware.RenderHTML(w, r, http.StatusUnauthorized, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Verify administrator sign-in", "RequireAdminMFA": true, "error": Translate(r, "errors.user.public.invalid_two_factor_code")}))
		return
	}
	h.consumePending(r)
	http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: "", Path: "/server/auth/external/username", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	admin, err := (&models.AdminModel{DB: database.GetUsersDB()}).GetByID(pending.AdminID)
	if err != nil || !admin.IsActive {
		http.Error(w, "Admin account is unavailable", http.StatusForbidden)
		return
	}
	h.createAdminSession(w, r, database.GetUsersDB(), admin, pending)
}

func (h *SAMLAuthHandler) completeSAMLAdminUsername(w http.ResponseWriter, r *http.Request, pending pendingSAMLIdentity, username, password, confirmPassword string, db *sql.DB) {
	if util.ValidateAdminUsername(username) != nil || !availableSAMLAdminUsername(r.Context(), db, username) {
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": suggestSAMLAdminUsername(r.Context(), db, username), "IsAdmin": true, "error": Translate(r, "errors.auth.external_admin_username_invalid")}))
		return
	}
	passwordMinLength := (&models.SettingsModel{DB: database.GetServerDB()}).GetInt("security.password_min_length", 8)
	if passwordMinLength < 8 {
		passwordMinLength = 8
	}
	if len(password) < passwordMinLength || password != confirmPassword {
		messageKey := "errors.auth.external_admin_password_mismatch"
		if password == confirmPassword {
			messageKey = "errors.auth.external_admin_password_too_short"
		}
		middleware.RenderHTML(w, r, http.StatusBadRequest, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": username, "IsAdmin": true, "error": Translate(r, messageKey)}))
		return
	}
	if !samlGroupMatch(pending.Groups, func() []string {
		cfg, _ := h.Service.Provider(pending.Provider)
		if cfg == nil {
			return nil
		}
		return cfg.AdminGroups
	}()) {
		h.failed(w, r, pending.Provider)
		return
	}
	var existing int
	if err := database.QueryRowContext(r.Context(), db, database.TimeoutSimpleSelect, `SELECT 1 FROM admin_saml_mappings WHERE provider_name=? AND external_id=?`, pending.Provider, pending.ExternalID).Scan(&existing); err == nil {
		h.failed(w, r, pending.Provider)
		return
	}
	email := strings.TrimSpace(pending.Email)
	if email == "" {
		email = username + "@" + pending.Provider + ".saml"
	}
	var emailExists int
	if err := database.QueryRowContext(r.Context(), db, database.TimeoutSimpleSelect, `SELECT 1 FROM admins WHERE email=?`, email).Scan(&emailExists); err == nil {
		email = username + "@" + pending.Provider + ".saml"
	}
	admin, err := (&models.AdminModel{DB: db}).CreateAdminAccount(username, email, password, false)
	if err != nil {
		middleware.RenderHTML(w, r, http.StatusConflict, "page/external_username.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Choose your username", "username": suggestSAMLAdminUsername(r.Context(), db, username), "IsAdmin": true, "error": Translate(r, "errors.auth.external_admin_create_failed")}))
		return
	}
	groups, _ := json.Marshal(pending.Groups)
	now := dbtime.FormatSQLTimestamp(time.Now())
	if _, err = database.ExecContext(r.Context(), db, database.TimeoutWrite, `INSERT INTO admin_saml_mappings (admin_id,provider_name,external_id,email,name,groups_json,created_at,updated_at,last_sync_at) VALUES (?,?,?,?,?,?,?,?,?)`, admin.ID, pending.Provider, pending.ExternalID, email, pending.Name, string(groups), now, now, now); err != nil {
		_ = (&models.AdminModel{DB: db}).DeleteAdminAccount(admin.ID)
		h.failed(w, r, pending.Provider)
		return
	}
	h.consumePending(r)
	http.SetCookie(w, &http.Cookie{Name: "saml_pending_identity", Value: "", Path: "/server/auth/external/username", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode})
	h.createAdminSession(w, r, db, admin, pending)
}

func availableSAMLAdminUsername(ctx context.Context, db *sql.DB, name string) bool {
	if util.ValidateAdminUsername(name) != nil {
		return false
	}
	var exists int
	return database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, `SELECT 1 FROM admins WHERE username=?`, name).Scan(&exists) == sql.ErrNoRows
}

func suggestSAMLAdminUsername(ctx context.Context, db *sql.DB, raw string) string {
	base := normalizeOIDCUsernameCandidate(raw)
	if availableSAMLAdminUsername(ctx, db, base) {
		return base
	}
	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			break
		}
		suffix := fmt.Sprint(n.Int64() + 1000)
		trimmed := base
		if len(trimmed)+len(suffix) > util.MaxUsernameLength {
			trimmed = trimmed[:util.MaxUsernameLength-len(suffix)]
		}
		candidate := strings.TrimRight(trimmed, "-") + suffix
		if availableSAMLAdminUsername(ctx, db, candidate) {
			return candidate
		}
	}
	return "serveradmin"
}

func samlGroupMatch(actual, allowed []string) bool {
	for _, group := range actual {
		for _, candidate := range allowed {
			if group == candidate {
				return true
			}
		}
	}
	return false
}

func (h *SAMLAuthHandler) pendingIdentity(r *http.Request) (pendingSAMLIdentity, bool) {
	cookie, err := r.Cookie("saml_pending_identity")
	if err != nil {
		return pendingSAMLIdentity{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.identities[cookie.Value]
	if !ok || time.Now().After(p.Expires) {
		delete(h.identities, cookie.Value)
		return pendingSAMLIdentity{}, false
	}
	return p, true
}
func (h *SAMLAuthHandler) consumePending(r *http.Request) {
	if c, e := r.Cookie("saml_pending_identity"); e == nil {
		h.mu.Lock()
		delete(h.identities, c.Value)
		h.mu.Unlock()
	}
}
func (h *SAMLAuthHandler) prune() {
	now := time.Now()
	for k, v := range h.requests {
		if now.After(v.Expires) {
			delete(h.requests, k)
		}
	}
	for k, v := range h.identities {
		if now.After(v.Expires) {
			delete(h.identities, k)
		}
	}
}
func (h *SAMLAuthHandler) failed(w http.ResponseWriter, r *http.Request, provider string) {
	now := dbtime.FormatSQLTimestamp(time.Now())
	_, _ = database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite, `INSERT INTO audit_log (ulid,timestamp,actor_type,actor_id,action,resource_type,resource_id,ip_address,user_agent,source,status,error) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, ulid.Make().String(), now, "external", provider, "saml.login_failed", "saml_provider", provider, util.TrustedGetClientIP(r), r.UserAgent(), "web", "failure", "SAML authentication failed")
	http.Error(w, "SAML authentication failed", http.StatusUnauthorized)
}
func (h *SAMLAuthHandler) syncMapping(ctx context.Context, db *sql.DB, userID int64, provider, externalID, email, name string, groups []string, role string) error {
	b, _ := json.Marshal(groups)
	now := dbtime.FormatSQLTimestamp(time.Now())
	if _, err := database.ExecContext(ctx, db, database.TimeoutWrite, `UPDATE user_saml_mappings SET email=?,name=?,groups_json=?,last_sync_at=? WHERE provider_name=? AND external_id=? AND user_id=?`, email, name, string(b), now, provider, externalID, userID); err != nil {
		return err
	}
	_, err := database.ExecContext(ctx, db, database.TimeoutWrite, `UPDATE users SET role=?,updated_at=? WHERE id=?`, role, now, userID)
	return err
}
func (h *SAMLAuthHandler) createSAMLUserSession(w http.ResponseWriter, r *http.Request, db *sql.DB, userID int64, identity pendingSAMLIdentity) {
	duration := middleware.SessionSettingsFor(middleware.SessionAccountUser).MaxAge
	session, err := (&models.UserSessionModel{DB: db}).CreateSession(userID, util.TrustedGetClientIP(r), r.UserAgent(), duration)
	if err != nil {
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	tokenHash := sha256.Sum256([]byte(session.SessionID))
	var sessionRowID int64
	err = database.QueryRowContext(r.Context(), db, database.TimeoutSimpleSelect, `SELECT id FROM user_sessions WHERE token_hash=?`, hex.EncodeToString(tokenHash[:])).Scan(&sessionRowID)
	if err != nil {
		_ = (&models.UserSessionModel{DB: db}).DeleteSession(session.SessionID)
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	nameID := identity.NameID
	if nameID == "" {
		nameID = identity.ExternalID
	}
	_, err = database.ExecContext(r.Context(), db, database.TimeoutWrite, `INSERT INTO user_saml_sessions(session_id,provider_name,external_id,name_id,session_index) VALUES(?,?,?,?,?)`, sessionRowID, identity.Provider, identity.ExternalID, nameID, identity.SessionIndex)
	if err != nil {
		_ = (&models.UserSessionModel{DB: db}).DeleteSession(session.SessionID)
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	code, err := markSAMLAPIRequest(r.Context(), identity.APIRelayState, session.SessionID)
	if err != nil {
		_ = (&models.UserSessionModel{DB: db}).DeleteSession(session.SessionID)
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountUser, session.SessionID, duration))
	if identity.APIRelayState != "" {
		h.renderSAMLAPIComplete(w, r, identity.APIRelayState, code)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *SAMLAuthHandler) createAdminSession(w http.ResponseWriter, r *http.Request, db *sql.DB, admin *models.Admin, identity pendingSAMLIdentity) {
	duration := middleware.SessionSettingsFor(middleware.SessionAccountAdmin).MaxAge
	session, err := (&models.AdminSessionModel{DB: database.GetServerDB()}).CreateSession(admin.ID, util.TrustedGetClientIP(r), r.UserAgent(), duration)
	if err != nil {
		http.Error(w, "Could not create admin session", http.StatusInternalServerError)
		return
	}
	nameID := identity.NameID
	if nameID == "" {
		nameID = identity.ExternalID
	}
	_, err = database.ExecContext(r.Context(), database.GetServerDB(), database.TimeoutWrite, `INSERT INTO admin_saml_sessions(session_id,provider_name,external_id,name_id,session_index) VALUES(?,?,?,?,?)`, session.ID, identity.Provider, identity.ExternalID, nameID, identity.SessionIndex)
	if err != nil {
		_ = (&models.AdminSessionModel{DB: database.GetServerDB()}).DeleteSession(session.SessionID)
		http.Error(w, "Could not create admin session", http.StatusInternalServerError)
		return
	}
	code, err := markSAMLAPIRequest(r.Context(), identity.APIRelayState, session.SessionID)
	if err != nil {
		_ = (&models.AdminSessionModel{DB: database.GetServerDB()}).DeleteSession(session.SessionID)
		http.Error(w, "Could not create admin session", http.StatusInternalServerError)
		return
	}
	_ = (&models.AdminModel{DB: db}).UpdateLastLogin(admin.ID)
	http.SetCookie(w, middleware.NewSessionCookie(r, middleware.SessionAccountAdmin, session.SessionID, duration))
	if identity.APIRelayState != "" {
		h.renderSAMLAPIComplete(w, r, identity.APIRelayState, code)
		return
	}
	cfg := config.GetGlobalConfig()
	adminPath := "/server/administration"
	if cfg != nil {
		adminPath = "/server/" + cfg.GetAdminPath()
	}
	http.Redirect(w, r, adminPath, http.StatusSeeOther)
}

func (h *SAMLAuthHandler) renderSAMLAPIComplete(w http.ResponseWriter, r *http.Request, relayState, code string) {
	var requestID string
	if err := database.QueryRowContext(r.Context(), database.GetServerDB(), database.TimeoutSimpleSelect, `SELECT request_id FROM saml_api_requests WHERE relay_state=?`, relayState).Scan(&requestID); err != nil {
		http.Error(w, "SAML sign-in completed. Return to your app to continue.", http.StatusOK)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	title := Translate(r, "auth.saml.api_complete_title")
	middleware.RenderHTML(w, r, http.StatusOK, "page/saml_api_complete.tmpl", util.TemplateData(r, map[string]interface{}{"title": title, "RequestID": requestID, "Code": code}))
}
func normalizeSAMLUsername(candidate, email string) string {
	if strings.TrimSpace(candidate) == "" {
		candidate = email
	}
	return normalizeOIDCUsernameCandidate(candidate)
}
func mappedSAMLRole(groups []string, mapping map[string][]string) string {
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		groupSet[group] = struct{}{}
	}
	for _, role := range []string{"admin", "moderator", "user"} {
		for _, group := range mapping[role] {
			if _, ok := groupSet[group]; ok {
				return role
			}
		}
	}
	return "user"
}
func availableSAMLUsername(ctx context.Context, db *sql.DB, name string) bool {
	return oidcUsernameAvailable(ctx, db, name)
}
func suggestSAMLUsername(ctx context.Context, db *sql.DB, base string) string {
	return suggestOIDCUsername(ctx, db, normalizeSAMLUsername(base, ""))
}
