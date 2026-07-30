package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPathSecurityMiddleware_BlocksTraversal verifies the HTTP-level
// middleware rejects raw and percent-encoded traversal sequences with 400,
// and lets legitimate paths through.
func TestPathSecurityMiddleware_BlocksTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		rawPath    string
		wantStatus int
	}{
		{"legitimate path", "/api/v1/weather/london", http.StatusOK},
		{"legitimate nested path", "/users/settings/profile", http.StatusOK},
		{"dot-dot traversal", "/api/v1/../../../etc/passwd", http.StatusBadRequest},
		{"encoded dot-dot lower", "/api/v1/%2e%2e/etc/passwd", http.StatusBadRequest},
		{"encoded dot-dot upper", "/api/v1/%2E%2E/etc/passwd", http.StatusBadRequest},
		{"mixed encoded traversal", "/api/v1/..%2f..%2fetc/passwd", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(PathSecurityMiddleware())
			router.Any("/*path", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.rawPath, nil)
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("path %q: status = %d, want %d (body=%s)", tt.rawPath, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// TestURLNormalizeMiddleware_CollapsesDoubleSlashes verifies repeated
// slashes are collapsed in place (no redirect) before the handler sees the
// path.
func TestURLNormalizeMiddleware_CollapsesDoubleSlashes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenPath string
	router := gin.New()
	router.Use(URLNormalizeMiddleware())
	router.Any("/*path", func(c *gin.Context) {
		seenPath = c.Request.URL.Path
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api//v1///weather", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if seenPath == "" {
		t.Fatal("handler never observed a request path")
	}
	for i := 0; i+1 < len(seenPath); i++ {
		if seenPath[i] == '/' && seenPath[i+1] == '/' {
			t.Errorf("normalized path %q still contains a double slash", seenPath)
			break
		}
	}
}

// TestValidatePath covers the pure validatePath helper directly - the
// traversal defense used by SafePath/SafeFilePath outside the HTTP layer.
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"simple relative path", "weather/london", false},
		{"nested relative path", "cache/2026/07/data", false},
		{"empty path treated as root, not an error", "", false},
		{"parent traversal", "../etc/passwd", true},
		{"embedded traversal", "cache/../../etc/passwd", true},
		{"path with dotted segment rejected by segment regex", "cache/file.json", true},
		{"single dot segment", "./cache/file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if tt.wantErr && err == nil {
				t.Errorf("validatePath(%q) = nil error, want error", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validatePath(%q) = %v, want nil error", tt.path, err)
			}
		})
	}
}

// TestSafePath_NormalizesAndRejectsTraversal verifies SafePath normalizes a
// legitimate relative path and rejects traversal attempts.
func TestSafePath_NormalizesAndRejectsTraversal(t *testing.T) {
	t.Run("legitimate path is normalized", func(t *testing.T) {
		got, err := SafePath("weather/london")
		if err != nil {
			t.Fatalf("SafePath: %v", err)
		}
		if got == "" {
			t.Error("SafePath returned empty string for a legitimate path")
		}
	})

	t.Run("traversal attempt rejected", func(t *testing.T) {
		_, err := SafePath("../../../etc/passwd")
		if err == nil {
			t.Error("SafePath with traversal = nil error, want error")
		}
	})
}

// TestSafeFilePath_RejectsTraversal mirrors TestSafePath_JoinsWithinBase but
// for the file-path-specific helper (used for on-disk file access).
func TestSafeFilePath_RejectsTraversal(t *testing.T) {
	base := "/var/lib/casapps/wthr/uploads"

	_, err := SafeFilePath(base, "../../etc/shadow")
	if err == nil {
		t.Error("SafeFilePath with traversal = nil error, want error")
	}

	got, err := SafeFilePath(base, "avatars/user123")
	if err != nil {
		t.Fatalf("SafeFilePath legitimate file: %v", err)
	}
	if got == "" {
		t.Error("SafeFilePath returned empty path for legitimate file")
	}
}
