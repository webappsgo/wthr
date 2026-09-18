package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	models "github.com/webappsgo/wthr/src/server/model"
	utils "github.com/webappsgo/wthr/src/util"
)

// newUserPublicTestHandler wires a UserPublicHandler against a fresh
// in-memory users database (real UsersSchema), and wires the global dual DB
// since UserModel.Create/Delete read database.GetUsersDB() rather than an
// injected field (see newAdminTestHandler in admin_test.go for the same
// convention).
func newUserPublicTestHandler(t *testing.T) (*UserPublicHandler, *sql.DB) {
	t.Helper()
	usersDB := newTestUsersDB(t)
	setGlobalTestDualDB(t, newTestServerDB(t), usersDB)
	return NewUserPublicHandler(usersDB), usersDB
}

// seedPublicUser creates a user via UserModel.CreateUserAccount (real Argon2id
// hashing) and returns it.
func seedPublicUser(t *testing.T, usersDB *sql.DB, username, email, password string) *models.User {
	t.Helper()
	u, err := (&models.UserModel{DB: usersDB}).CreateUserAccount(username, email, password, "user")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// withChiParam attaches a chi route context carrying a single URL param to
// r, mirroring how the chi router populates chi.URLParam during a real
// request; used in place of the legacy Params-slice assignment pattern.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestNewUserPublicHandler(t *testing.T) {
	db := newTestUsersDB(t)
	h := NewUserPublicHandler(db)
	if h == nil || h.DB != db {
		t.Fatalf("NewUserPublicHandler did not wire DB correctly")
	}
}

func TestUserPublicHandler_LoadPublicProfile(t *testing.T) {
	t.Run("public profile success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "pubuser", "pub@example.com", "password123")

		profile, err := h.loadPublicProfile("pubuser", 0)
		if err != nil {
			t.Fatalf("loadPublicProfile: %v", err)
		}
		if profile.Username != "pubuser" {
			t.Fatalf("username = %q, want pubuser", profile.Username)
		}
		if profile.Avatar.Type != "gravatar" {
			t.Fatalf("avatar type = %q, want gravatar", profile.Avatar.Type)
		}
		_ = u
	})

	t.Run("username case-insensitive and trimmed", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		seedPublicUser(t, usersDB, "caseuser", "case@example.com", "password123")

		profile, err := h.loadPublicProfile("  CaseUser  ", 0)
		if err != nil {
			t.Fatalf("loadPublicProfile: %v", err)
		}
		if profile.Username != "caseuser" {
			t.Fatalf("username = %q, want caseuser", profile.Username)
		}
	})

	t.Run("not found returns ErrNoRows", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		_, err := h.loadPublicProfile("nobody", 0)
		if err != sql.ErrNoRows {
			t.Fatalf("err = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("private profile hidden from non-owner viewer", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "privuser", "priv@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET visibility = 'private' WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("set private: %v", err)
		}

		_, err := h.loadPublicProfile("privuser", 0)
		if err != sql.ErrNoRows {
			t.Fatalf("err = %v, want sql.ErrNoRows for private profile viewed by stranger", err)
		}
	})

	t.Run("private profile visible to owner", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ownerview", "owner@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET visibility = 'private' WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("set private: %v", err)
		}

		profile, err := h.loadPublicProfile("ownerview", u.ID)
		if err != nil {
			t.Fatalf("loadPublicProfile as owner: %v", err)
		}
		if profile.Username != "ownerview" {
			t.Fatalf("username = %q, want ownerview", profile.Username)
		}
	})

	t.Run("inactive user not found", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "inactiveuser", "inactive@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET is_active = 0 WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		if _, err := h.loadPublicProfile("inactiveuser", 0); err != sql.ErrNoRows {
			t.Fatalf("err = %v, want sql.ErrNoRows for inactive user", err)
		}
	})

	t.Run("banned user not found", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "banneduser", "banned@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET is_banned = 1 WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("ban: %v", err)
		}
		if _, err := h.loadPublicProfile("banneduser", 0); err != sql.ErrNoRows {
			t.Fatalf("err = %v, want sql.ErrNoRows for banned user", err)
		}
	})
}

func TestLoadPublicUserProfile_PackageFunc(t *testing.T) {
	_, usersDB := newUserPublicTestHandler(t)
	seedPublicUser(t, usersDB, "wrapperuser", "wrapper@example.com", "password123")

	profile, err := LoadPublicUserProfile(usersDB, "wrapperuser", 0)
	if err != nil {
		t.Fatalf("LoadPublicUserProfile: %v", err)
	}
	if profile.Username != "wrapperuser" {
		t.Fatalf("username = %q, want wrapperuser", profile.Username)
	}
}

func TestUserPublicHandler_GetPublicProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		seedPublicUser(t, usersDB, "getpub", "getpub@example.com", "password123")

		r, w := newTestContext(http.MethodGet, "/api/v1/public/users/getpub")
		r = withChiParam(r, "username", "getpub")
		h.GetPublicProfile(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/api/v1/public/users/ghost")
		r = withChiParam(r, "username", "ghost")
		h.GetPublicProfile(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("db error returns 500", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		if _, err := usersDB.Exec(`DROP TABLE user_accounts`); err != nil {
			t.Fatalf("drop table: %v", err)
		}
		r, w := newTestContext(http.MethodGet, "/api/v1/public/users/whoever")
		r = withChiParam(r, "username", "whoever")
		h.GetPublicProfile(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("viewer identity from context sees own private profile", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ctxowner", "ctxowner@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET visibility = 'private' WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("set private: %v", err)
		}
		r, w := newTestContext(http.MethodGet, "/api/v1/public/users/ctxowner")
		r = withChiParam(r, "username", "ctxowner")
		r = setAdminCurrentUser(r, u.ID)
		h.GetPublicProfile(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestGetGravatarURL(t *testing.T) {
	tests := []struct {
		name  string
		email string
		size  int
	}{
		{"simple email", "user@example.com", 128},
		{"mixed case and spaces trimmed/lowered", "  User@Example.COM  ", 64},
		{"empty email", "", 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := GetGravatarURL(tt.email, tt.size)
			if url == "" {
				t.Fatalf("GetGravatarURL returned empty string")
			}
			if !bytes.Contains([]byte(url), []byte("gravatar.com/avatar/")) {
				t.Fatalf("url = %q, want gravatar.com/avatar/ substring", url)
			}
		})
	}

	t.Run("case/whitespace insensitivity produces identical hash", func(t *testing.T) {
		a := GetGravatarURL("user@example.com", 128)
		b := GetGravatarURL("  User@Example.COM  ", 128)
		if a != b {
			t.Fatalf("gravatar urls differ for equivalent emails: %q vs %q", a, b)
		}
	})
}

func TestBuildAvatarInfo(t *testing.T) {
	t.Run("defaults to gravatar when type unset", func(t *testing.T) {
		info := buildAvatarInfo("a@example.com", sql.NullString{}, sql.NullString{})
		if info.Type != "gravatar" {
			t.Fatalf("type = %q, want gravatar", info.Type)
		}
		if len(info.URLs) != 4 {
			t.Fatalf("urls = %v, want 4 sizes", info.URLs)
		}
	})

	t.Run("url type with valid url", func(t *testing.T) {
		info := buildAvatarInfo("a@example.com", sql.NullString{String: "url", Valid: true}, sql.NullString{String: "https://example.com/a.png", Valid: true})
		if info.Type != "url" {
			t.Fatalf("type = %q, want url", info.Type)
		}
		if info.URLs["original"] != "https://example.com/a.png" {
			t.Fatalf("original url = %q", info.URLs["original"])
		}
	})

	t.Run("url type with missing url produces no entries", func(t *testing.T) {
		info := buildAvatarInfo("a@example.com", sql.NullString{String: "url", Valid: true}, sql.NullString{})
		if len(info.URLs) != 0 {
			t.Fatalf("urls = %v, want empty", info.URLs)
		}
	})

	t.Run("upload type populates all sizes with same base url", func(t *testing.T) {
		info := buildAvatarInfo("a@example.com", sql.NullString{String: "upload", Valid: true}, sql.NullString{String: "/uploads/x.png", Valid: true})
		if info.Type != "upload" {
			t.Fatalf("type = %q, want upload", info.Type)
		}
		for _, key := range []string{"original", "256", "128", "64", "32"} {
			if info.URLs[key] != "/uploads/x.png" {
				t.Fatalf("URLs[%q] = %q, want /uploads/x.png", key, info.URLs[key])
			}
		}
	})
}

func TestUserPublicHandler_AvatarLifecycle(t *testing.T) {
	h, usersDB := newUserPublicTestHandler(t)
	u := seedPublicUser(t, usersDB, "avataruser", "avatar@example.com", "password123")

	t.Run("loadCurrentUserAvatar success (default gravatar)", func(t *testing.T) {
		resp, err := h.loadCurrentUserAvatar(u.ID)
		if err != nil {
			t.Fatalf("loadCurrentUserAvatar: %v", err)
		}
		if resp.Type != "gravatar" {
			t.Fatalf("type = %q, want gravatar", resp.Type)
		}
	})

	t.Run("loadCurrentUserAvatar unknown user returns error", func(t *testing.T) {
		if _, err := h.loadCurrentUserAvatar(999999); err == nil {
			t.Fatalf("expected error for nonexistent user")
		}
	})

	t.Run("updateCurrentUserAvatar nil request errors", func(t *testing.T) {
		if err := h.updateCurrentUserAvatar(u.ID, nil); err == nil {
			t.Fatalf("expected error for nil request")
		}
	})

	t.Run("updateCurrentUserAvatar invalid type errors", func(t *testing.T) {
		if err := h.updateCurrentUserAvatar(u.ID, &UpdateAvatarRequest{Type: "bogus"}); err == nil {
			t.Fatalf("expected error for invalid avatar type")
		}
	})

	t.Run("updateCurrentUserAvatar url type missing url errors", func(t *testing.T) {
		if err := h.updateCurrentUserAvatar(u.ID, &UpdateAvatarRequest{Type: "url", URL: "  "}); err == nil {
			t.Fatalf("expected error for missing url")
		}
	})

	t.Run("updateCurrentUserAvatar url type requires https", func(t *testing.T) {
		if err := h.updateCurrentUserAvatar(u.ID, &UpdateAvatarRequest{Type: "url", URL: "http://insecure.example.com/a.png"}); err == nil {
			t.Fatalf("expected error for non-https url")
		}
	})

	t.Run("updateCurrentUserAvatar url type success", func(t *testing.T) {
		if err := h.updateCurrentUserAvatar(u.ID, &UpdateAvatarRequest{Type: "url", URL: "https://cdn.example.com/a.png"}); err != nil {
			t.Fatalf("updateCurrentUserAvatar: %v", err)
		}
		resp, err := h.loadCurrentUserAvatar(u.ID)
		if err != nil {
			t.Fatalf("loadCurrentUserAvatar: %v", err)
		}
		if resp.URLs["original"] != "https://cdn.example.com/a.png" {
			t.Fatalf("original = %q", resp.URLs["original"])
		}
	})

	t.Run("updateCurrentUserAvatar gravatar type success (idempotent)", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if err := h.updateCurrentUserAvatar(u.ID, &UpdateAvatarRequest{Type: "gravatar"}); err != nil {
				t.Fatalf("updateCurrentUserAvatar iteration %d: %v", i, err)
			}
		}
		resp, err := h.loadCurrentUserAvatar(u.ID)
		if err != nil {
			t.Fatalf("loadCurrentUserAvatar: %v", err)
		}
		if resp.Type != "gravatar" {
			t.Fatalf("type = %q, want gravatar", resp.Type)
		}
	})

	t.Run("UpdateCurrentUserAvatar package wrapper success", func(t *testing.T) {
		resp, err := UpdateCurrentUserAvatar(usersDB, u.ID, &UpdateAvatarRequest{Type: "url", URL: "https://cdn.example.com/b.png"})
		if err != nil {
			t.Fatalf("UpdateCurrentUserAvatar: %v", err)
		}
		if resp.URLs["original"] != "https://cdn.example.com/b.png" {
			t.Fatalf("original = %q", resp.URLs["original"])
		}
	})

	t.Run("resetCurrentUserAvatar clears avatar_url", func(t *testing.T) {
		if err := h.resetCurrentUserAvatar(u.ID); err != nil {
			t.Fatalf("resetCurrentUserAvatar: %v", err)
		}
		resp, err := h.loadCurrentUserAvatar(u.ID)
		if err != nil {
			t.Fatalf("loadCurrentUserAvatar: %v", err)
		}
		if resp.Type != "gravatar" {
			t.Fatalf("type = %q, want gravatar after reset", resp.Type)
		}
	})

	t.Run("ResetCurrentUserAvatar package wrapper success", func(t *testing.T) {
		if err := ResetCurrentUserAvatar(usersDB, u.ID); err != nil {
			t.Fatalf("ResetCurrentUserAvatar: %v", err)
		}
	})

	t.Run("uploadCurrentUserAvatar nil upload errors", func(t *testing.T) {
		if err := h.uploadCurrentUserAvatar(u.ID, nil); err == nil {
			t.Fatalf("expected error for nil upload")
		}
	})

	t.Run("uploadCurrentUserAvatar too large errors", func(t *testing.T) {
		if err := h.uploadCurrentUserAvatar(u.ID, &AvatarUploadRequest{Size: 3 * 1024 * 1024, ContentType: "image/png"}); err == nil {
			t.Fatalf("expected error for oversized upload")
		}
	})

	t.Run("uploadCurrentUserAvatar invalid content type errors", func(t *testing.T) {
		if err := h.uploadCurrentUserAvatar(u.ID, &AvatarUploadRequest{Size: 100, ContentType: "text/plain"}); err == nil {
			t.Fatalf("expected error for invalid content type")
		}
	})

	t.Run("uploadCurrentUserAvatar success", func(t *testing.T) {
		if err := h.uploadCurrentUserAvatar(u.ID, &AvatarUploadRequest{Size: 100, ContentType: "image/png"}); err != nil {
			t.Fatalf("uploadCurrentUserAvatar: %v", err)
		}
		resp, err := h.loadCurrentUserAvatar(u.ID)
		if err != nil {
			t.Fatalf("loadCurrentUserAvatar: %v", err)
		}
		if resp.Type != "upload" {
			t.Fatalf("type = %q, want upload", resp.Type)
		}
	})

	t.Run("UploadCurrentUserAvatar package wrapper success", func(t *testing.T) {
		resp, err := UploadCurrentUserAvatar(usersDB, u.ID, &AvatarUploadRequest{Size: 100, ContentType: "image/gif"})
		if err != nil {
			t.Fatalf("UploadCurrentUserAvatar: %v", err)
		}
		if resp.Type != "upload" {
			t.Fatalf("type = %q, want upload", resp.Type)
		}
	})
}

func TestGetExtension(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{"image/png", "png"},
		{"image/jpeg", "jpg"},
		{"image/gif", "gif"},
		{"image/webp", "webp"},
		{"image/bmp", "bmp"},
		{"image/svg+xml", "svg"},
		{"application/octet-stream", "png"},
		{"", "png"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := getExtension(tt.contentType); got != tt.want {
				t.Fatalf("getExtension(%q) = %q, want %q", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestUserPublicHandler_GetCurrentUserAvatar(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/api/v1/users/avatar")
		h.GetCurrentUserAvatar(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "curavatar", "curavatar@example.com", "password123")
		r, w := newTestContext(http.MethodGet, "/api/v1/users/avatar")
		r = setAdminCurrentUser(r, u.ID)
		h.GetCurrentUserAvatar(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("db error returns 500", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContext(http.MethodGet, "/api/v1/users/avatar")
		r = setAdminCurrentUser(r, 999999)
		h.GetCurrentUserAvatar(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserPublicHandler_UpdateAvatarSettings(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/users/avatar", map[string]interface{}{"type": "gravatar"})
		h.UpdateAvatarSettings(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("bad json returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "updavatar1", "updavatar1@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/users/avatar", "{not json")
		r = setAdminCurrentUser(r, u.ID)
		h.UpdateAvatarSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid oneof type returns 400 from binding", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "updavatar2", "updavatar2@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/users/avatar", map[string]interface{}{"type": "bogus"})
		r = setAdminCurrentUser(r, u.ID)
		h.UpdateAvatarSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	// AI.md PART 9: validation failures map to BAD_REQUEST -> 400.
	// avatarValidationKey compares via errors.Is against the ErrAvatar*
	// sentinels (see the var block above), not err.Error() text, so an
	// insecure URL correctly reaches the 400 branch.
	t.Run("url type without https returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "updavatar3", "updavatar3@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/users/avatar", map[string]interface{}{"type": "url", "url": "http://insecure.example.com/a.png"})
		r = setAdminCurrentUser(r, u.ID)
		h.UpdateAvatarSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "updavatar4", "updavatar4@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/users/avatar", map[string]interface{}{"type": "url", "url": "https://cdn.example.com/z.png"})
		r = setAdminCurrentUser(r, u.ID)
		h.UpdateAvatarSettings(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserPublicHandler_ResetAvatar(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContext(http.MethodDelete, "/api/v1/users/avatar")
		h.ResetAvatar(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "resetavatar", "resetavatar@example.com", "password123")
		r, w := newTestContext(http.MethodDelete, "/api/v1/users/avatar")
		r = setAdminCurrentUser(r, u.ID)
		h.ResetAvatar(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// newMultipartAvatarContext builds an httptest request with a multipart form
// containing a "file" field, for exercising UploadAvatar's r.FormFile path.
// Unlike multipart.Writer.CreateFormFile (which hardcodes the part's
// Content-Type to "application/octet-stream"), this uses CreatePart with an
// explicit Content-Type header so tests can control the content type that
// reaches uploadCurrentUserAvatar's validation.
func newMultipartAvatarContext(t *testing.T, filename, contentType string, data []byte) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/avatar", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r, w
}

func TestUserPublicHandler_UploadAvatar(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContext(http.MethodPost, "/api/v1/users/avatar")
		h.UploadAvatar(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("no file uploaded returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "uploadnoFile", "uploadnofile@example.com", "password123")
		r, w := newTestContext(http.MethodPost, "/api/v1/users/avatar")
		r = setAdminCurrentUser(r, u.ID)
		h.UploadAvatar(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success with real multipart file", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "uploadok", "uploadok@example.com", "password123")
		r, w := newMultipartAvatarContext(t, "avatar.png", "image/png", []byte("fake-png-bytes"))
		r = setAdminCurrentUser(r, u.ID)
		h.UploadAvatar(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	// AI.md PART 9: validation failures map to BAD_REQUEST -> 400.
	// uploadCurrentUserAvatar returns the ErrAvatarInvalidImage sentinel for
	// a disallowed content type, and avatarValidationKey compares via
	// errors.Is, so this correctly reaches the 400 branch.
	t.Run("invalid content type returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "uploadbadtype", "uploadbadtype@example.com", "password123")
		r, w := newMultipartAvatarContext(t, "avatar.txt", "text/plain", []byte("not an image"))
		r = setAdminCurrentUser(r, u.ID)
		h.UploadAvatar(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserPublicHandler_ChangeCurrentUserPassword(t *testing.T) {
	t.Run("nil request errors", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		if err := h.changeCurrentUserPassword(1, nil); err == nil {
			t.Fatalf("expected error for nil request")
		}
	})

	t.Run("unknown user errors", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		err := h.changeCurrentUserPassword(999999, &ChangePasswordRequest{CurrentPassword: "x", NewPassword: "newpassword123"})
		if err == nil {
			t.Fatalf("expected error for unknown user")
		}
	})

	t.Run("wrong current password errors", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "chpwwrong", "chpwwrong@example.com", "password123")
		err := h.changeCurrentUserPassword(u.ID, &ChangePasswordRequest{CurrentPassword: "wrongpass", NewPassword: "newpassword123"})
		if err == nil || err.Error() != "current password is incorrect" {
			t.Fatalf("err = %v, want current password is incorrect", err)
		}
	})

	t.Run("success and new password verifies", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "chpwok", "chpwok@example.com", "password123")
		if err := h.changeCurrentUserPassword(u.ID, &ChangePasswordRequest{CurrentPassword: "password123", NewPassword: "newpassword456"}); err != nil {
			t.Fatalf("changeCurrentUserPassword: %v", err)
		}

		var hash string
		if err := usersDB.QueryRow(`SELECT password_hash FROM user_accounts WHERE id = ?`, u.ID).Scan(&hash); err != nil {
			t.Fatalf("query hash: %v", err)
		}
		ok, err := utils.VerifyPassword("newpassword456", hash)
		if err != nil || !ok {
			t.Fatalf("new password did not verify: ok=%v err=%v", ok, err)
		}
	})

	t.Run("ChangeCurrentUserPassword package wrapper success", func(t *testing.T) {
		_, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "chpwwrap", "chpwwrap@example.com", "password123")
		if err := ChangeCurrentUserPassword(usersDB, u.ID, &ChangePasswordRequest{CurrentPassword: "password123", NewPassword: "newpassword789"}); err != nil {
			t.Fatalf("ChangeCurrentUserPassword: %v", err)
		}
	})
}

func TestUserPublicHandler_ChangePassword(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/password", map[string]interface{}{})
		h.ChangePassword(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("bad json returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "cp1", "cp1@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/password", "{not json")
		r = setAdminCurrentUser(r, u.ID)
		h.ChangePassword(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing required fields returns 400 from binding", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "cp2", "cp2@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/password", map[string]interface{}{})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangePassword(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong current password returns 401", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "cp3", "cp3@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/password", map[string]interface{}{
			"current_password": "wrongpass",
			"new_password":     "newpassword999",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangePassword(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "cp4", "cp4@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/password", map[string]interface{}{
			"current_password": "password123",
			"new_password":     "newpassword999",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangePassword(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUserPublicHandler_ChangeEmail(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{})
		h.ChangeEmail(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("bad json returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce1", "ce1@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", "{not json")
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing/invalid fields returns 400 from binding", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce2", "ce2@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email": "not-an-email",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong current password returns 401", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce3", "ce3@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "new-ce3@example.com",
			"current_password": "wrongpass",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("email already in use returns 409", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		seedPublicUser(t, usersDB, "ce4other", "taken-ce4@example.com", "password123")
		u := seedPublicUser(t, usersDB, "ce4", "ce4@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "taken-ce4@example.com",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success marks email unverified", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce5", "ce5@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "ce5-new@example.com",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var email string
		var verified bool
		if err := usersDB.QueryRow(`SELECT email, email_verified FROM user_accounts WHERE id = ?`, u.ID).Scan(&email, &verified); err != nil {
			t.Fatalf("query updated user: %v", err)
		}
		if email != "ce5-new@example.com" {
			t.Fatalf("email = %q, want ce5-new@example.com", email)
		}
		if verified {
			t.Fatalf("email_verified = true, want false after email change")
		}
	})

	t.Run("mixed case address is stored lowercase", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce6", "ce6@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "CE6-New@Example.COM",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var email string
		if err := usersDB.QueryRow(`SELECT email FROM user_accounts WHERE id = ?`, u.ID).Scan(&email); err != nil {
			t.Fatalf("query updated user: %v", err)
		}
		if email != "ce6-new@example.com" {
			t.Fatalf("email = %q, want ce6-new@example.com", email)
		}
	})

	t.Run("case-insensitive collision returns 409", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		seedPublicUser(t, usersDB, "ce7other", "taken-ce7@example.com", "password123")
		u := seedPublicUser(t, usersDB, "ce7", "ce7@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "Taken-CE7@Example.com",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	// util.ValidateEmailWithBlocklist blocks disposable email domains (AI.md
	// PART 33's optional blocklist), not local-part patterns — the username
	// blocklist (AI.md PART 34) is a separate check scoped to the `username`
	// field, never applied to the email address.
	t.Run("disposable domain returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce8", "ce8@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "someone@mailinator.com",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("two factor enabled without code returns 401", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "ce9", "ce9@example.com", "password123")
		if _, err := usersDB.Exec(`UPDATE user_accounts SET two_factor_enabled = 1 WHERE id = ?`, u.ID); err != nil {
			t.Fatalf("enable 2fa: %v", err)
		}
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/email", map[string]interface{}{
			"new_email":        "ce9-new@example.com",
			"current_password": "password123",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.ChangeEmail(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}

		var email string
		if err := usersDB.QueryRow(`SELECT email FROM user_accounts WHERE id = ?`, u.ID).Scan(&email); err != nil {
			t.Fatalf("query user: %v", err)
		}
		if email != "ce9@example.com" {
			t.Fatalf("email = %q, want unchanged ce9@example.com", email)
		}
	})
}

func TestAvatarValidationKey(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		wantOK bool
	}{
		{"invalid type", ErrAvatarInvalidType, true},
		{"url required", ErrAvatarURLRequired, true},
		{"url not https", ErrAvatarURLNotHTTPS, true},
		{"url svg", ErrAvatarURLSVG, true},
		{"no file", ErrAvatarNoFile, true},
		{"too large", ErrAvatarTooLarge, true},
		{"invalid image", ErrAvatarInvalidImage, true},
		{"unrelated error", sql.ErrConnDone, false},
		{"nil error", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := avatarValidationKey(tc.err)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && key == "" {
				t.Fatalf("translation key is empty for %v", tc.err)
			}
		})
	}
}

func TestUserPublicHandler_DeleteAccount(t *testing.T) {
	t.Run("unauthenticated returns 401", func(t *testing.T) {
		h, _ := newUserPublicTestHandler(t)
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/delete", map[string]interface{}{})
		h.DeleteAccount(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("bad json returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "da1", "da1@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/delete", "{not json")
		r = setAdminCurrentUser(r, u.ID)
		h.DeleteAccount(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("wrong confirmation string returns 400", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "da2", "da2@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/delete", map[string]interface{}{
			"current_password": "password123",
			"confirm":          "delete", // wrong case
		})
		r = setAdminCurrentUser(r, u.ID)
		h.DeleteAccount(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "da3", "da3@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/delete", map[string]interface{}{
			"current_password": "wrongpass",
			"confirm":          "DELETE",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.DeleteAccount(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success deletes the user", func(t *testing.T) {
		h, usersDB := newUserPublicTestHandler(t)
		u := seedPublicUser(t, usersDB, "da4", "da4@example.com", "password123")
		r, w := newTestContextJSON(t, http.MethodPost, "/api/v1/users/security/delete", map[string]interface{}{
			"current_password": "password123",
			"confirm":          "DELETE",
		})
		r = setAdminCurrentUser(r, u.ID)
		h.DeleteAccount(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}

		var count int
		if err := usersDB.QueryRow(`SELECT COUNT(*) FROM user_accounts WHERE id = ?`, u.ID).Scan(&count); err != nil {
			t.Fatalf("query count: %v", err)
		}
		if count != 0 {
			t.Fatalf("account still present after delete: count=%d", count)
		}
	})
}

// unused imports guard (keeps gofmt/govet happy if a branch above is trimmed later)
var (
	_ = time.Now
	_ = json.Marshal
)

// TestUserPublicHandler_LoadPublicProfileLegacyCreatedAt is the regression test
// for the profile lookup. loadPublicProfile used to scan created_at directly
// into a time.Time, so a row whose timestamp was written in the local-zone
// time.Time.String() layout the SQLite driver produces for a bound time.Time -
// a layout the driver's own scanner does not accept - failed the entire query
// and turned a perfectly valid public profile into a 500. created_at is now
// scanned untyped and parsed with dbtime, so a legacy or unparseable timestamp
// costs at most the join date.
func TestUserPublicHandler_LoadPublicProfileLegacyCreatedAt(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	cases := []struct {
		name      string
		username  string
		createdAt string
		wantTime  time.Time
	}{
		{
			name:      "legacy west-zone text still loads",
			username:  "legacywest",
			createdAt: now.In(handlerZoneWest).Format(handlerLocalLayout),
			wantTime:  now.UTC(),
		},
		{
			name:      "legacy east-zone text still loads",
			username:  "legacyeast",
			createdAt: now.In(handlerZoneEast).Format(handlerLocalLayout),
			wantTime:  now.UTC(),
		},
		{
			name:      "unparseable created_at loses only the join date",
			username:  "legacybroken",
			createdAt: "not-a-timestamp",
			wantTime:  time.Time{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, usersDB := newUserPublicTestHandler(t)
			u := seedPublicUser(t, usersDB, tc.username, tc.username+"@example.com", "password123")
			if _, err := usersDB.Exec(`UPDATE user_accounts SET created_at = ? WHERE id = ?`, tc.createdAt, u.ID); err != nil {
				t.Fatalf("rewrite created_at: %v", err)
			}

			profile, err := h.loadPublicProfile(tc.username, 0)
			if err != nil {
				t.Fatalf("loadPublicProfile with stored created_at %q: %v", tc.createdAt, err)
			}
			if profile.Username != tc.username {
				t.Fatalf("username = %q, want %q", profile.Username, tc.username)
			}
			if !profile.CreatedAt.Equal(tc.wantTime) {
				t.Errorf("created_at = %v, want %v", profile.CreatedAt, tc.wantTime)
			}
		})
	}
}
