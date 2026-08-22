package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
	utils "github.com/webappsgo/wthr/src/util"
)

// TestAccessLogger_WritesRequestLine verifies AccessLogger produces an
// access.log entry containing the method, path, and status code for a
// completed request.
func TestAccessLogger_WritesRequestLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()
	logger, err := utils.NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	router := gin.New()
	router.Use(AccessLogger(logger))
	router.GET("/api/v1/weather", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, err := os.ReadFile(filepath.Join(logDir, "access.log"))
	if err != nil {
		t.Fatalf("read access.log: %v", err)
	}
	got := string(data)
	if got == "" {
		t.Fatal("access.log is empty, want a logged request line")
	}
	for _, want := range []string{"GET", "/api/v1/weather", "200"} {
		if !strings.Contains(got, want) {
			t.Errorf("access.log = %q, want it to contain %q", got, want)
		}
	}
}

// TestAccessLogger_UsernameNeverPopulatedForRealAuthenticatedUser documents a
// real production bug: AccessLogger (access_log.go:36-42) extracts the
// username by type-asserting c.Get("user") as map[string]interface{}, then
// indexing ["username"]. But every middleware that actually authenticates a
// request (auth.go's AuthMiddleware, token_auth.go's TokenAuthMiddleware)
// sets the "user" context key to a *model.User struct, never a map. The type
// assertion therefore always fails silently, and the access log's username
// field is always empty - even for fully authenticated requests - defeating
// the purpose of access logging for security/audit review.
//
// This test encodes CORRECT expected behavior (a real authenticated user's
// username should appear in the access log) and is expected to FAIL against
// the current implementation.
func TestAccessLogger_UsernameNeverPopulatedForRealAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()
	logger, err := utils.NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Mirrors exactly what auth.go / token_auth.go set: a *model.User,
		// not a map[string]interface{}.
		c.Set(UserContextKey, &model.User{ID: 1, Username: "alice"})
		c.Next()
	})
	router.Use(AccessLogger(logger))
	router.GET("/users/settings", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, err := os.ReadFile(filepath.Join(logDir, "access.log"))
	if err != nil {
		t.Fatalf("read access.log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "alice") {
		t.Errorf("access.log = %q, want it to contain username %q - "+
			"access_log.go:36-42 type-asserts c.Get(\"user\") as "+
			"map[string]interface{}, but auth.go/token_auth.go always set it to "+
			"a *model.User, so the assertion always fails and usernames never "+
			"appear in the access log", got, "alice")
	}
}

// TestAccessLoggerWithFormat_WritesFormattedLine verifies the
// format-configurable variant also produces output via the formatter and
// logger.Write path, for a representative format.
func TestAccessLoggerWithFormat_WritesFormattedLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()
	logger, err := utils.NewLogger(logDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	formatter := service.NewLogFormatter(service.LogFormatText)

	router := gin.New()
	router.Use(AccessLoggerWithFormat(logger, formatter))
	router.GET("/api/v1/weather", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weather", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// AccessLoggerWithFormat writes through logger.Write, which appends to
	// access.log per utils.Logger's Write implementation.
	data, err := os.ReadFile(filepath.Join(logDir, "access.log"))
	if err != nil {
		t.Fatalf("read access.log: %v", err)
	}
	if len(data) == 0 {
		t.Error("access.log is empty, want a formatted access log line written via logger.Write")
	}
}
