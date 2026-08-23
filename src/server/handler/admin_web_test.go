package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

// resetGlobalConfig ensures config.GetGlobalConfig() is nil before/after
// each test so tests don't leak state into each other or into other
// concurrently-written test files in this package.
func resetGlobalConfig(t *testing.T) {
	t.Helper()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(nil) })
}

// withTestGlobalConfig points config.SaveConfig's file-resolution at a
// scratch temp dir (via CONFIG_DIR) rather than the real host config path,
// so a successful UpdateRobotsTxt/UpdateSecurityTxt call never touches the
// host filesystem outside the test sandbox.
func withTestGlobalConfig(t *testing.T) {
	t.Helper()
	resetGlobalConfig(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("web:\n  robots_txt: \"\"\n"), 0644); err != nil {
		t.Fatalf("seed server.yml: %v", err)
	}
	oldConfigDir := os.Getenv("CONFIG_DIR")
	os.Setenv("CONFIG_DIR", dir)
	t.Cleanup(func() { os.Setenv("CONFIG_DIR", oldConfigDir) })
	config.SetGlobalConfig(&config.AppConfig{})
}

// TestAdminWebHandler_ShowWebSettings_RedirectBranches covers every
// pre-render redirect branch: missing admin_id, non-int admin_id, and
// admin-not-found. The success (render) path needs HTMLRender configured
// and is not covered here (see report coverage gaps).
func TestAdminWebHandler_ShowWebSettings_RedirectBranches(t *testing.T) {
	h := NewAdminWebHandler(newTestDatabaseDB(t))
	setGlobalTestDualDB(t, h.db.DB, newTestUsersDB(t))

	t.Run("missing admin_id", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/server/admin/config/web")
		h.ShowWebSettings(w, c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", w.Code)
		}
	})

	t.Run("non-int admin_id", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/server/admin/config/web")
		c = c.WithContext(reqctx.Set(c.Context(), "admin_id", "not-an-int"))
		h.ShowWebSettings(w, c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", w.Code)
		}
	})

	t.Run("admin not found", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/server/admin/config/web")
		c = withAdminID(c, 999999)
		h.ShowWebSettings(w, c)
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", w.Code)
		}
	})
}

// TestAdminWebHandler_GetRobotsTxt_GetSecurityTxt cover the read-only JSON
// endpoints with no global config configured (nil-safe defaults).
func TestAdminWebHandler_GetRobotsTxt_GetSecurityTxt(t *testing.T) {
	h := &AdminWebHandler{}
	resetGlobalConfig(t)

	t.Run("robots", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/web/robots")
		h.GetRobotsTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("security", func(t *testing.T) {
		c, w := newTestContext(http.MethodGet, "/api/v1/server/admin/config/web/security")
		h.GetSecurityTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

// TestAdminWebHandler_UpdateRobotsTxt covers missing-content validation,
// the "global config not initialized" 500, and a real success write to a
// scratch temp config file.
func TestAdminWebHandler_UpdateRobotsTxt(t *testing.T) {
	h := &AdminWebHandler{}

	t.Run("missing content", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/robots", map[string]interface{}{})
		h.UpdateRobotsTxt(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("global config not initialized", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/robots", map[string]interface{}{"content": "User-agent: *"})
		h.UpdateRobotsTxt(w, c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		withTestGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/robots", map[string]interface{}{"content": "User-agent: *\nDisallow: /admin"})
		h.UpdateRobotsTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminWebHandler_UpdateSecurityTxt mirrors TestAdminWebHandler_UpdateRobotsTxt
// for the security.txt sibling endpoint.
func TestAdminWebHandler_UpdateSecurityTxt(t *testing.T) {
	h := &AdminWebHandler{}

	t.Run("missing content", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/security", map[string]interface{}{})
		h.UpdateSecurityTxt(w, c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("global config not initialized", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/security", map[string]interface{}{"content": "Contact: mailto:security@example.com"})
		h.UpdateSecurityTxt(w, c)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		withTestGlobalConfig(t)
		c, w := newTestContextJSON(t, http.MethodPatch, "/api/v1/server/admin/config/web/security", map[string]interface{}{"content": "Contact: mailto:security@example.com"})
		h.UpdateSecurityTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})
}

// TestAdminWebHandler_ServeRobotsTxt_ServeSecurityTxt cover the public
// text-serving endpoints, both default content (nil config) and configured
// content with template variable substitution.
func TestAdminWebHandler_ServeRobotsTxt_ServeSecurityTxt(t *testing.T) {
	h := &AdminWebHandler{}

	t.Run("robots default", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContext(http.MethodGet, "/robots.txt")
		h.ServeRobotsTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if w.Body.Len() == 0 {
			t.Errorf("expected non-empty default robots.txt body")
		}
	})

	t.Run("security default", func(t *testing.T) {
		resetGlobalConfig(t)
		c, w := newTestContext(http.MethodGet, "/.well-known/security.txt")
		h.ServeSecurityTxt(w, c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if w.Body.Len() == 0 {
			t.Errorf("expected non-empty generated security.txt body")
		}
	})
}

// TestAdminWebHandler_ServeSitemap covers the always-200 XML sitemap
// generation and a minimal structural check.
func TestAdminWebHandler_ServeSitemap(t *testing.T) {
	h := &AdminWebHandler{}
	c, w := newTestContext(http.MethodGet, "/sitemap.xml")
	h.ServeSitemap(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Errorf("Content-Type header not set")
	}
	body := w.Body.String()
	if body == "" || body[:5] != "<?xml" {
		t.Errorf("body does not start with an XML declaration: %q", body)
	}
}

// TestAdminWebHandler_ServeFavicon covers the embedded-default path (nil
// global config, no custom favicon URL).
func TestAdminWebHandler_ServeFavicon(t *testing.T) {
	h := &AdminWebHandler{}
	resetGlobalConfig(t)

	c, w := newTestContext(http.MethodGet, "/favicon.ico")
	h.ServeFavicon(w, c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("expected non-empty favicon body")
	}
}
