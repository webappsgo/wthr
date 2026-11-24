package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/server/service"
)

func adminSAMLProviderRequest(target string, provider string, body []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if provider != "" {
		ctx := chi.NewRouteContext()
		ctx.URLParams.Add("provider", provider)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
	}
	return r
}

func TestAdminSAMLProviderAPILifecycle(t *testing.T) {
	newAdminTestHandler(t)
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	metadataServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not SAML metadata")
	}))
	t.Cleanup(metadataServer.Close)
	valid := service.SAMLProviderConfig{Name: "  example  ", IDPMetadataURL: metadataServer.URL + "/metadata"}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.CreateSAMLProviderAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers", "", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Data service.SAMLProviderConfig `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.Name != "example" || created.Data.CertExpiryDays != 365 || !created.Data.AutoGenerateCert {
		t.Fatalf("created provider defaults = %#v", created.Data)
	}

	w = httptest.NewRecorder()
	h.ListSAMLProvidersAPI(w, httptest.NewRequest(http.MethodGet, "/api/v1/server/administration/auth/saml/providers", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"name": "example"`)) {
		t.Fatalf("list response = %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.GetSAMLProviderAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/example", "example", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.CreateSAMLProviderAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers", "", body))
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, body=%s", w.Code, w.Body.String())
	}

	valid.SPEntityID = "https://service.example.test/entity"
	updateBody, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.UpdateSAMLProviderAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/example", "example", updateBody))
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.SAMLProviderMetadataAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/missing/metadata", "missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing metadata provider status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.SAMLProviderMetadataAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/example/metadata", "example", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unreachable provider metadata status = %d, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.DeleteSAMLProvider(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/example", "example", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.GetSAMLProviderAPI(w, adminSAMLProviderRequest("/api/v1/server/administration/auth/saml/providers/example", "example", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted provider status = %d", w.Code)
	}
}

func TestAdminSAMLProviderRejectsInvalidInputAndUnknownUpdate(t *testing.T) {
	newAdminTestHandler(t)
	h := &AdminAuthSettingsHandler{ConfigPath: newAuthSettingsTestConfigFile(t)}
	for _, body := range [][]byte{[]byte("{"), []byte(`{"name":"!bad","idp_metadata_url":"file:///etc/passwd"}`)} {
		w := httptest.NewRecorder()
		h.CreateSAMLProviderAPI(w, adminSAMLProviderRequest("/providers", "", body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid create status = %d, body=%s", w.Code, w.Body.String())
		}
	}
	validBody := []byte(`{"name":"unused","idp_metadata_url":"https://idp.example.test/metadata"}`)
	w := httptest.NewRecorder()
	h.UpdateSAMLProviderAPI(w, adminSAMLProviderRequest("/providers/missing", "missing", validBody))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown update status = %d, body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.DeleteSAMLProvider(w, adminSAMLProviderRequest("/providers/missing", "missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown delete status = %d", w.Code)
	}
}
