package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/crewjam/saml"
	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/middleware"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

func samlSessionIndexStrings(indexes []saml.SessionIndex) []string {
	result := make([]string, len(indexes))
	for i, index := range indexes {
		result[i] = index.Value
	}
	return result
}

// StartSAMLLogout starts an SP-initiated logout for the current SAML session.
func (h *SAMLAuthHandler) StartSAMLLogout(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider, _ = h.providerForCurrentSession(r)
	}
	cfg, err := h.Service.Provider(provider)
	if err != nil || !cfg.SLOEnabled {
		http.NotFound(w, r)
		return
	}
	nameID, sessionIndex, deleteSession, err := h.samlSessionForRequest(r, provider)
	if err != nil || nameID == "" {
		http.Error(w, "No matching SAML session", http.StatusUnauthorized)
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		http.Error(w, "SAML logout is unavailable", http.StatusServiceUnavailable)
		return
	}
	sp.SignatureMethod = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	sloURL := cfg.IDPSLOURL
	if sloURL == "" {
		sloURL = sp.GetSLOBindingLocation(saml.HTTPRedirectBinding)
	}
	if sloURL == "" {
		http.Error(w, "Identity provider has no Redirect logout endpoint", http.StatusServiceUnavailable)
		return
	}
	request, err := sp.MakeLogoutRequest(sloURL, nameID, saml.WithSessionIndex(sessionIndex))
	if err != nil {
		http.Error(w, "Could not create SAML logout request", http.StatusInternalServerError)
		return
	}
	state, err := service.GenerateState()
	if err != nil {
		http.Error(w, "Could not create SAML logout state", http.StatusInternalServerError)
		return
	}
	redirect, err := sp.RedirectLogoutRequestURL(request, state)
	if err != nil {
		http.Error(w, "Could not create SAML logout redirect", http.StatusInternalServerError)
		return
	}
	now := dbtime.FormatSQLTimestamp(time.Now().Add(5 * time.Minute))
	if _, err := database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `INSERT INTO saml_logout_requests(request_id, provider_name, relay_state, expires_at) VALUES(?,?,?,?)`, request.ID, provider, state, now); err != nil {
		http.Error(w, "Could not store SAML logout state", http.StatusInternalServerError)
		return
	}
	deleteSession()
	http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountUser))
	http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountAdmin))
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (h *SAMLAuthHandler) TryStartSAMLLogout(w http.ResponseWriter, r *http.Request) bool {
	provider, err := h.providerForCurrentSession(r)
	if err != nil {
		return false
	}
	cfg, err := h.Service.Provider(provider)
	if err != nil || !cfg.SLOEnabled {
		return false
	}
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("provider", provider)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
	h.StartSAMLLogout(w, r)
	return true
}

// SAMLLogout handles signed IdP LogoutRequests and SP-initiated LogoutResponses.
func (h *SAMLAuthHandler) SAMLLogout(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, err := h.Service.Provider(provider)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		http.Error(w, "SAML logout is unavailable", http.StatusServiceUnavailable)
		return
	}
	sp.SignatureMethod = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	switch r.Method {
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid SAML logout message", http.StatusBadRequest)
			return
		}
	case http.MethodGet:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid SAML logout message", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
		return
	}
	if r.Form.Get("SAMLRequest") != "" {
		logout, err := sp.ValidateLogoutRequest(r)
		identity, identityErr := service.ParseSAMLLogoutRequestIdentity(r)
		if err != nil || identityErr != nil || logout == nil || identity == nil || logout.NameID == nil || logout.NameID.Value == "" || logout.ID != identity.ID || logout.NameID.Value != identity.NameID {
			http.Error(w, "Invalid signed SAML LogoutRequest", http.StatusBadRequest)
			return
		}
		if err := h.endSAMLNameIDSessions(r, provider, identity.NameID, samlSessionIndexStrings(logout.AllSessionIndexes())); err != nil {
			http.Error(w, "Could not end SAML sessions", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountUser))
		http.SetCookie(w, middleware.ClearSessionCookie(r, middleware.SessionAccountAdmin))
		if r.Method == http.MethodPost {
			destination := cfg.IDPSLOURL
			if destination == "" {
				destination = sp.GetSLOBindingLocation(saml.HTTPPostBinding)
			}
			if err := writeSAMLPostLogoutResponse(w, r, sp, destination, logout.ID, r.Form.Get("RelayState")); err != nil {
				http.Error(w, "Could not create SAML LogoutResponse", http.StatusInternalServerError)
				return
			}
			return
		}
		response, err := sp.MakeRedirectLogoutResponse(logout.ID, r.Form.Get("RelayState"))
		if err != nil {
			http.Error(w, "Could not create SAML LogoutResponse", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, response.String(), http.StatusFound)
		return
	}
	if r.Form.Get("SAMLResponse") == "" {
		h.StartSAMLLogout(w, r)
		return
	}
	if _, err := database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `DELETE FROM saml_logout_requests WHERE expires_at <= ?`, dbtime.FormatSQLTimestamp(time.Now())); err != nil {
		http.Error(w, "SAML logout state is unavailable", http.StatusInternalServerError)
		return
	}
	rows, err := database.QueryContext(r.Context(), database.GetServerStore(), database.TimeoutSimpleSelect, `SELECT request_id, relay_state FROM saml_logout_requests WHERE provider_name=? AND expires_at>?`, provider, dbtime.FormatSQLTimestamp(time.Now()))
	if err != nil {
		http.Error(w, "SAML logout state is unavailable", http.StatusInternalServerError)
		return
	}
	var ids []string
	validRelayStates := make(map[string]struct{})
	for rows.Next() {
		var id string
		var relay string
		if err := rows.Scan(&id, &relay); err != nil {
			_ = rows.Close()
			http.Error(w, "SAML logout state is unavailable", http.StatusInternalServerError)
			return
		}
		ids = append(ids, id)
		validRelayStates[relay] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		http.Error(w, "SAML logout state is unavailable", http.StatusInternalServerError)
		return
	}
	_ = rows.Close()
	if len(ids) == 0 {
		http.Error(w, "Unsolicited SAML LogoutResponse", http.StatusBadRequest)
		return
	}
	if _, ok := validRelayStates[r.Form.Get("RelayState")]; !ok || r.Form.Get("RelayState") == "" {
		http.Error(w, "Invalid SAML logout state", http.StatusBadRequest)
		return
	}
	response, err := sp.ParseLogoutResponse(r, ids)
	if err != nil {
		http.Error(w, "Invalid signed SAML LogoutResponse", http.StatusBadRequest)
		return
	}
	if _, err := database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `DELETE FROM saml_logout_requests WHERE request_id=? AND provider_name=?`, response.InResponseTo, provider); err != nil {
		http.Error(w, "Could not consume SAML logout state", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *SAMLAuthHandler) StartAPISAMLLogout(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, err := h.Service.Provider(provider)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND"})
		return
	}
	nameID, sessionIndex, deleteSession, err := h.samlSessionForRequest(r, provider)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "UNAUTHORIZED"})
		return
	}
	if !cfg.SLOEnabled {
		deleteSession()
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "logged_out"}})
		return
	}
	sp, err := h.Service.ServiceProvider(r.Context(), cfg, util.BuildURL(r, "/"))
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SAML_LOGOUT_UNAVAILABLE"})
		return
	}
	sp.SignatureMethod = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	sloURL := cfg.IDPSLOURL
	if sloURL == "" {
		sloURL = sp.GetSLOBindingLocation(saml.HTTPRedirectBinding)
	}
	if sloURL == "" {
		deleteSession()
		RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "logged_out"}})
		return
	}
	request, err := sp.MakeLogoutRequest(sloURL, nameID, saml.WithSessionIndex(sessionIndex))
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_LOGOUT_FAILED"})
		return
	}
	state, err := service.GenerateState()
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_LOGOUT_FAILED"})
		return
	}
	redirect, err := sp.RedirectLogoutRequestURL(request, state)
	if err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_LOGOUT_FAILED"})
		return
	}
	if _, err := database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `INSERT INTO saml_logout_requests(request_id,provider_name,relay_state,expires_at) VALUES(?,?,?,?)`, request.ID, provider, state, dbtime.FormatSQLTimestamp(time.Now().Add(5*time.Minute))); err != nil {
		RespondNegotiatedData(w, r, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SAML_LOGOUT_FAILED"})
		return
	}
	deleteSession()
	RespondNegotiatedData(w, r, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"redirect_url": redirect.String()}})
}

func (h *SAMLAuthHandler) samlSessionForRequest(r *http.Request, provider string) (string, string, func(), error) {
	if adminID := reqctx.GetInt(r.Context(), "admin_id"); adminID > 0 {
		var nameID, index, externalID string
		err := database.QueryRowContext(r.Context(), database.GetServerStore(), database.TimeoutSimpleSelect, `SELECT ss.name_id,ss.session_index,ss.external_id FROM admin_saml_sessions ss JOIN admin_sessions a ON a.id=ss.session_id WHERE ss.provider_name=? AND a.admin_id=? ORDER BY a.created_at DESC`, provider, adminID).Scan(&nameID, &index, &externalID)
		if err == nil {
			return nameID, index, func() {
				_, _ = database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `DELETE FROM admin_sessions WHERE id IN (SELECT session_id FROM admin_saml_sessions WHERE provider_name=? AND external_id=?)`, provider, externalID)
			}, nil
		}
	}
	if userID := reqctx.GetInt(r.Context(), middleware.UserIDContextKey); userID > 0 {
		var nameID, index, externalID string
		err := database.QueryRowContext(r.Context(), database.GetUsersStore(), database.TimeoutSimpleSelect, `SELECT ss.name_id,ss.session_index,ss.external_id FROM user_saml_sessions ss JOIN user_sessions us ON us.id=ss.session_id WHERE ss.provider_name=? AND us.user_id=? ORDER BY us.created_at DESC`, provider, userID).Scan(&nameID, &index, &externalID)
		if err == nil {
			return nameID, index, func() {
				_, _ = database.ExecContext(r.Context(), database.GetUsersStore(), database.TimeoutWrite, `DELETE FROM user_sessions WHERE id IN (SELECT session_id FROM user_saml_sessions WHERE provider_name=? AND external_id=?)`, provider, externalID)
			}, nil
		}
	}
	if session, ok := middleware.GetCurrentSession(r); ok {
		hash := sha256.Sum256([]byte(session.ID))
		var rowID int64
		var nameID, index string
		err := database.QueryRowContext(r.Context(), database.GetUsersStore(), database.TimeoutSimpleSelect, `SELECT us.id, ss.name_id, ss.session_index FROM user_sessions us JOIN user_saml_sessions ss ON ss.session_id=us.id WHERE us.token_hash=? AND ss.provider_name=?`, hex.EncodeToString(hash[:]), provider).Scan(&rowID, &nameID, &index)
		if err == nil {
			return nameID, index, func() {
				_, _ = database.ExecContext(r.Context(), database.GetUsersStore(), database.TimeoutWrite, `DELETE FROM user_sessions WHERE id=?`, rowID)
			}, nil
		}
	}
	if cookie, err := r.Cookie(middleware.UserSessionCookieName()); err == nil && cookie.Value != "" {
		hash := sha256.Sum256([]byte(cookie.Value))
		var rowID int64
		var nameID, index string
		err := database.QueryRowContext(r.Context(), database.GetUsersStore(), database.TimeoutSimpleSelect, `SELECT us.id, ss.name_id, ss.session_index FROM user_sessions us JOIN user_saml_sessions ss ON ss.session_id=us.id WHERE us.token_hash=? AND ss.provider_name=?`, hex.EncodeToString(hash[:]), provider).Scan(&rowID, &nameID, &index)
		if err == nil {
			return nameID, index, func() {
				_, _ = database.ExecContext(r.Context(), database.GetUsersStore(), database.TimeoutWrite, `DELETE FROM user_sessions WHERE id=?`, rowID)
			}, nil
		}
	}
	if cookie, err := r.Cookie(middleware.AdminSessionCookieName()); err == nil && cookie.Value != "" {
		hash := models.HashAdminSessionToken(cookie.Value)
		var rowID, nameID, index string
		err := database.QueryRowContext(r.Context(), database.GetServerStore(), database.TimeoutSimpleSelect, `SELECT ss.session_id, ss.name_id, ss.session_index FROM admin_sessions a JOIN admin_saml_sessions ss ON ss.session_id=a.id WHERE a.token_hash=? AND ss.provider_name=?`, hash, provider).Scan(&rowID, &nameID, &index)
		if err == nil {
			return nameID, index, func() {
				_, _ = database.ExecContext(r.Context(), database.GetServerStore(), database.TimeoutWrite, `DELETE FROM admin_sessions WHERE id=?`, rowID)
			}, nil
		}
	}
	return "", "", nil, errors.New("no SAML session for provider")
}

func (h *SAMLAuthHandler) providerForCurrentSession(r *http.Request) (string, error) {
	if cookie, err := r.Cookie(middleware.UserSessionCookieName()); err == nil && cookie.Value != "" {
		hash := sha256.Sum256([]byte(cookie.Value))
		var provider string
		err := database.QueryRowContext(r.Context(), database.GetUsersStore(), database.TimeoutSimpleSelect, `SELECT ss.provider_name FROM user_sessions us JOIN user_saml_sessions ss ON ss.session_id=us.id WHERE us.token_hash=?`, hex.EncodeToString(hash[:])).Scan(&provider)
		if err == nil {
			return provider, nil
		}
	}
	if cookie, err := r.Cookie(middleware.AdminSessionCookieName()); err == nil && cookie.Value != "" {
		var provider string
		err := database.QueryRowContext(r.Context(), database.GetServerStore(), database.TimeoutSimpleSelect, `SELECT ss.provider_name FROM admin_sessions a JOIN admin_saml_sessions ss ON ss.session_id=a.id WHERE a.token_hash=?`, models.HashAdminSessionToken(cookie.Value)).Scan(&provider)
		if err == nil {
			return provider, nil
		}
	}
	return "", errors.New("no SAML session for current account")
}

func (h *SAMLAuthHandler) endSAMLNameIDSessions(r *http.Request, provider, nameID string, indexes []string) error {
	if nameID == "" {
		return errors.New("SAML NameID is required")
	}
	usersDB, serverDB := database.GetUsersStore(), database.GetServerStore()
	if len(indexes) == 0 {
		_, err := database.ExecContext(r.Context(), usersDB, database.TimeoutWrite, `DELETE FROM user_sessions WHERE id IN (SELECT session_id FROM user_saml_sessions WHERE provider_name=? AND name_id=?)`, provider, nameID)
		if err != nil {
			return err
		}
		_, err = database.ExecContext(r.Context(), serverDB, database.TimeoutWrite, `DELETE FROM admin_sessions WHERE id IN (SELECT session_id FROM admin_saml_sessions WHERE provider_name=? AND name_id=?)`, provider, nameID)
		return err
	}
	for _, index := range indexes {
		if _, err := database.ExecContext(r.Context(), usersDB, database.TimeoutWrite, `DELETE FROM user_sessions WHERE id IN (SELECT session_id FROM user_saml_sessions WHERE provider_name=? AND name_id=? AND session_index=?)`, provider, nameID, index); err != nil {
			return err
		}
		if _, err := database.ExecContext(r.Context(), serverDB, database.TimeoutWrite, `DELETE FROM admin_sessions WHERE id IN (SELECT session_id FROM admin_saml_sessions WHERE provider_name=? AND name_id=? AND session_index=?)`, provider, nameID, index); err != nil {
			return err
		}
	}
	return nil
}

func writeSAMLPostLogoutResponse(w http.ResponseWriter, r *http.Request, sp *saml.ServiceProvider, destination, requestID, relayState string) error {
	if destination == "" {
		return errors.New("identity provider has no POST logout endpoint")
	}
	response, err := sp.MakeLogoutResponse(destination, requestID)
	if err != nil {
		return err
	}
	message, err := xml.Marshal(response)
	if err != nil {
		return err
	}
	view := struct{ Destination, Message, RelayState string }{
		Destination: destination,
		Message:     base64.StdEncoding.EncodeToString(message),
		RelayState:  relayState,
	}
	target, err := url.Parse(destination)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") || target.Host == "" {
		return errors.New("invalid identity provider POST logout endpoint")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; form-action "+target.Scheme+"://"+target.Host+"; base-uri 'none'; frame-ancestors 'none'")
	middleware.RenderHTML(w, r, http.StatusOK, "page/saml_post_logout_response.tmpl", util.TemplateData(r, map[string]interface{}{"title": "Complete sign-out", "Destination": view.Destination, "Message": view.Message, "RelayState": view.RelayState}))
	return nil
}
