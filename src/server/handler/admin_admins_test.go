package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
)

// newAdminsAPIRequest builds a bare GET request/recorder pair. Kept local
// (rather than reusing the package's shared newAPITestContext/newTestContextJSON
// gin-based helpers, which have not been converted to net/http yet) to avoid
// depending on unexported helpers owned by another file, mirroring the pattern
// documented in api_test.go's newAPITestRequest.
func newAdminsAPIRequest(target string) (*http.Request, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r, w
}

// newAdminsJSONRequest builds a request/recorder pair with a JSON (or raw,
// for malformed-body cases) body, mirroring newTestContextJSON's exact
// body-encoding behavior: a string body is used as raw bytes verbatim (this
// is how the "not json" malformed-body test cases produce invalid JSON), a
// []byte body is used as-is, and anything else is json.Marshal'd.
func newAdminsJSONRequest(t *testing.T, method, target string, body interface{}) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()

	var raw []byte
	switch v := body.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	r := httptest.NewRequest(method, target, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	return r, w
}

// setAdminIDParam sets the chi :id route param on r, mirroring the pattern
// used elsewhere in this package for chi-converted handlers. It accepts the
// raw string value so both valid numeric ids and deliberately-invalid values
// (e.g. "abc") can be exercised.
func setAdminIDParam(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// withAdminID stores admin_id on r's context, mirroring the reqctx key set
// by the RequireAdminAuth middleware for authenticated admin requests.
func withAdminID(r *http.Request, id int) *http.Request {
	return r.WithContext(reqctx.Set(r.Context(), "admin_id", id))
}

// newAdminsTestHandler wires the global dual-DB (required since AdminModel
// and AdminInviteModel both read through database.GetServerDB() rather than
// an injected field) and returns a ready-to-use AdminsHandler plus the
// backing DB for direct assertions.
func newAdminsTestHandler(t *testing.T) (*AdminsHandler, *sql.DB) {
	t.Helper()
	serverDB := newTestServerDB(t)
	setGlobalTestDualDB(t, serverDB, serverDB)
	inviteService := service.NewAdminInviteService(serverDB, "https://example.com", nil)
	return NewAdminsHandler(serverDB, inviteService), serverDB
}

// newAdminsTestAdmin creates a real admin row for tests that need one.
func newAdminsTestAdmin(t *testing.T, username, password string) *models.Admin {
	t.Helper()
	admin, err := (&models.AdminModel{}).Create(username, username+"@example.com", password, false)
	if err != nil {
		t.Fatalf("failed to create test admin: %v", err)
	}
	return admin
}

func TestAdminsHandlerListAdmins(t *testing.T) {
	h, _ := newAdminsTestHandler(t)
	newAdminsTestAdmin(t, "listadminsadmin", "password123")

	r, w := newAdminsAPIRequest("/server/admin/config/admins")
	h.ListAdmins(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerGetAdminCount(t *testing.T) {
	h, _ := newAdminsTestHandler(t)
	newAdminsTestAdmin(t, "countadmin", "password123")

	r, w := newAdminsAPIRequest("/server/admin/config/admins/count")
	h.GetAdminCount(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerCreateInvite(t *testing.T) {
	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite", "not json")
		h.CreateInvite(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing email errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite", map[string]string{})
		h.CreateInvite(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite", map[string]string{"email": "new@example.com"})
		h.CreateInvite(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid request succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "inviterone", "password123")

		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite", map[string]string{"email": "invitee@example.com"})
		r = withAdminID(r, int(admin.ID))
		h.CreateInvite(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerAcceptInvite(t *testing.T) {
	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite/accept", "not json")
		h.AcceptInvite(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown token errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/invite/accept", map[string]string{
			"token": "no-such-token", "username": "newadmin", "password": "password123",
		})
		h.AcceptInvite(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerGetPendingInvites(t *testing.T) {
	h, _ := newAdminsTestHandler(t)

	r, w := newAdminsAPIRequest("/server/admin/config/admins/invites")
	h.GetPendingInvites(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerRevokeInvite(t *testing.T) {
	t.Run("missing token errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsAPIRequest("/server/admin/config/admins/invite/")
		h.RevokeInvite(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerUpdateAdmin(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/abc", map[string]string{
			"username": "newname", "email": "new@example.com",
		})
		r = setAdminIDParam(r, "abc")
		h.UpdateAdmin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "updatetargetadmin", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1", "not json")
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		h.UpdateAdmin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid update succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "updatetargetadmin2", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1", map[string]string{
			"username": "updatedname", "email": "updated@example.com",
		})
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		h.UpdateAdmin(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerDeleteAdmin(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsAPIRequest("/server/admin/config/admins/abc")
		r = setAdminIDParam(r, "abc")
		h.DeleteAdmin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "deltargetadmin", "password123")
		r, w := newAdminsAPIRequest("/server/admin/config/admins/1")
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		h.DeleteAdmin(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cannot delete self", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "selfdeladmin", "password123")
		r, w := newAdminsAPIRequest("/server/admin/config/admins/1")
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		r = withAdminID(r, int(admin.ID))
		h.DeleteAdmin(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("deleting another admin succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		actingAdmin := newAdminsTestAdmin(t, "actingadmin", "password123")
		targetAdmin := newAdminsTestAdmin(t, "targetadmin", "password123")
		r, w := newAdminsAPIRequest("/server/admin/config/admins/2")
		r = setAdminIDParam(r, strconv.FormatInt(targetAdmin.ID, 10))
		r = withAdminID(r, int(actingAdmin.ID))
		h.DeleteAdmin(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerChangePassword(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/abc/password", map[string]string{
			"current_password": "old", "new_password": "newpassword123",
		})
		r = setAdminIDParam(r, "abc")
		h.ChangePassword(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1/password", "not json")
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		h.ChangePassword(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin2", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1/password", map[string]string{
			"current_password": "old", "new_password": "newpassword123",
		})
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		h.ChangePassword(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	// current_password was binding:"required" in the original gin handler
	// (admin_admins.go), so an empty value was rejected by ShouldBindJSON
	// before the handler body ran, making the "if request.CurrentPassword !=
	// \"\"" branch below dead code as originally written - preserved verbatim
	// here, now enforced by the manual validation check that replaced the
	// gin binding tag.
	t.Run("empty current password fails binding", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin3", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1/password", map[string]string{
			"current_password": "", "new_password": "newpassword123",
		})
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		r = withAdminID(r, int(admin.ID))
		h.ChangePassword(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	// A non-empty current_password triggers the verification query
	// `SELECT password FROM admins WHERE id = ?`, but the schema only
	// defines a `server_admin_credentials` table, never `admins` — so this
	// path always fails with a 500, documenting a real production bug
	// rather than exercising the intended success path.
	t.Run("non-empty current password hits missing admins table", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin4", "password123")
		r, w := newAdminsJSONRequest(t, http.MethodPost, "/server/admin/config/admins/1/password", map[string]string{
			"current_password": "password123", "new_password": "newpassword123",
		})
		r = setAdminIDParam(r, strconv.FormatInt(admin.ID, 10))
		r = withAdminID(r, int(admin.ID))
		h.ChangePassword(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (documents admins-table bug): %s", w.Code, w.Body.String())
		}
	})
}

// TestAdminsHandlerShowAdminsPage_LoadsData verifies ShowAdminsPage pulls
// admins, count, and pending invites from the real DB before attempting
// to render (the pre-render guard/data-loading logic under test here).
func TestAdminsHandlerShowAdminsPage_LoadsData(t *testing.T) {
	h, _ := newAdminsTestHandler(t)
	newAdminsTestAdmin(t, "showadminspage", "password123")

	r, w := newAdminsAPIRequest("/server/admin/config/admins")
	defer htmlRenderGuard(t)
	h.ShowAdminsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

// TestAdminsHandlerShowInviteAcceptPage_MissingToken verifies the
// missing-token guard renders the error page with 400 rather than
// attempting to verify an empty token.
func TestAdminsHandlerShowInviteAcceptPage_MissingToken(t *testing.T) {
	h, _ := newAdminsTestHandler(t)

	r, w := newAdminsAPIRequest("/server/admin/config/admins/invite/accept")
	defer htmlRenderGuard(t)
	h.ShowInviteAcceptPage(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

// TestAdminsHandlerShowInviteAcceptPage_InvalidToken verifies an
// unrecognized token reaches VerifyInvite, gets rejected, and renders
// the error page with 400 (not a panic/500).
func TestAdminsHandlerShowInviteAcceptPage_InvalidToken(t *testing.T) {
	h, _ := newAdminsTestHandler(t)

	r, w := newAdminsAPIRequest("/server/admin/config/admins/invite/accept?token=does-not-exist")
	defer htmlRenderGuard(t)
	h.ShowInviteAcceptPage(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

// TestAdminsHandlerShowInviteAcceptPage_ValidToken verifies a genuine,
// unexpired, unused invite token passes VerifyInvite and proceeds to the
// success-render branch (200) rather than the error branch.
func TestAdminsHandlerShowInviteAcceptPage_ValidToken(t *testing.T) {
	h, serverDB := newAdminsTestHandler(t)
	inviterID := newAdminsTestAdmin(t, "invitercreator", "password123")
	inviteService := service.NewAdminInviteService(serverDB, "https://example.com", nil)
	created, _, err := inviteService.CreateInvite("invitee@example.com", int(inviterID.ID), "24h")
	if err != nil {
		t.Fatalf("CreateInvite() unexpected error: %v", err)
	}

	r, w := newAdminsAPIRequest("/server/admin/config/admins/invite/accept?token=" + created.Token)
	defer htmlRenderGuard(t)
	h.ShowInviteAcceptPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}
