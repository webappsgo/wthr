package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// setAdminIDParam sets the gin :id route param, mirroring the pattern used
// elsewhere in this package (e.g. auth_oidc_test.go's :provider param).
func setAdminIDParam(c *gin.Context, id int64) {
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}
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

	c, w := newAPITestContext("/server/admin/admins")
	h.ListAdmins(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerGetAdminCount(t *testing.T) {
	h, _ := newAdminsTestHandler(t)
	newAdminsTestAdmin(t, "countadmin", "password123")

	c, w := newAPITestContext("/server/admin/admins/count")
	h.GetAdminCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerCreateInvite(t *testing.T) {
	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite", "not json")
		h.CreateInvite(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing email errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite", map[string]string{})
		h.CreateInvite(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite", map[string]string{"email": "new@example.com"})
		h.CreateInvite(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid request succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "inviterone", "password123")

		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite", map[string]string{"email": "invitee@example.com"})
		c.Set("admin_id", int(admin.ID))
		h.CreateInvite(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerAcceptInvite(t *testing.T) {
	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite/accept", "not json")
		h.AcceptInvite(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown token errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/invite/accept", map[string]string{
			"token": "no-such-token", "username": "newadmin", "password": "password123",
		})
		h.AcceptInvite(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerGetPendingInvites(t *testing.T) {
	h, _ := newAdminsTestHandler(t)

	c, w := newAPITestContext("/server/admin/admins/invites")
	h.GetPendingInvites(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestAdminsHandlerRevokeInvite(t *testing.T) {
	t.Run("missing token errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newAPITestContext("/server/admin/admins/invite/")
		h.RevokeInvite(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerUpdateAdmin(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/abc", map[string]string{
			"username": "newname", "email": "new@example.com",
		})
		c.Params = gin.Params{{Key: "id", Value: "abc"}}
		h.UpdateAdmin(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "updatetargetadmin", "password123")
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1", "not json")
		setAdminIDParam(c, admin.ID)
		h.UpdateAdmin(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid update succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "updatetargetadmin2", "password123")
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1", map[string]string{
			"username": "updatedname", "email": "updated@example.com",
		})
		setAdminIDParam(c, admin.ID)
		h.UpdateAdmin(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerDeleteAdmin(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newAPITestContext("/server/admin/admins/abc")
		c.Params = gin.Params{{Key: "id", Value: "abc"}}
		h.DeleteAdmin(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "deltargetadmin", "password123")
		c, w := newAPITestContext("/server/admin/admins/1")
		setAdminIDParam(c, admin.ID)
		h.DeleteAdmin(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cannot delete self", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "selfdeladmin", "password123")
		c, w := newAPITestContext("/server/admin/admins/1")
		setAdminIDParam(c, admin.ID)
		c.Set("admin_id", int(admin.ID))
		h.DeleteAdmin(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("deleting another admin succeeds", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		actingAdmin := newAdminsTestAdmin(t, "actingadmin", "password123")
		targetAdmin := newAdminsTestAdmin(t, "targetadmin", "password123")
		c, w := newAPITestContext("/server/admin/admins/2")
		setAdminIDParam(c, targetAdmin.ID)
		c.Set("admin_id", int(actingAdmin.ID))
		h.DeleteAdmin(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
		}
	})
}

func TestAdminsHandlerChangePassword(t *testing.T) {
	t.Run("invalid id errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/abc/password", map[string]string{
			"current_password": "old", "new_password": "newpassword123",
		})
		c.Params = gin.Params{{Key: "id", Value: "abc"}}
		h.ChangePassword(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("malformed body errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin", "password123")
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1/password", "not json")
		setAdminIDParam(c, admin.ID)
		h.ChangePassword(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not authenticated errors", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin2", "password123")
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1/password", map[string]string{
			"current_password": "old", "new_password": "newpassword123",
		})
		setAdminIDParam(c, admin.ID)
		h.ChangePassword(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
		}
	})

	// current_password is binding:"required" (admin_admins.go line 254), so an
	// empty value is rejected by ShouldBindJSON before the handler's own
	// "if request.CurrentPassword != \"\"" check is ever reached — that check
	// is unreachable dead code as currently written.
	t.Run("empty current password fails binding", func(t *testing.T) {
		h, _ := newAdminsTestHandler(t)
		admin := newAdminsTestAdmin(t, "pwtargetadmin3", "password123")
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1/password", map[string]string{
			"current_password": "", "new_password": "newpassword123",
		})
		setAdminIDParam(c, admin.ID)
		c.Set("admin_id", int(admin.ID))
		h.ChangePassword(c)
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
		c, w := newTestContextJSON(t, http.MethodPost, "/server/admin/admins/1/password", map[string]string{
			"current_password": "password123", "new_password": "newpassword123",
		})
		setAdminIDParam(c, admin.ID)
		c.Set("admin_id", int(admin.ID))
		h.ChangePassword(c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (documents admins-table bug): %s", w.Code, w.Body.String())
		}
	})
}
