package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

func TestAddAllowlistPersistsAndActivatesEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	cfg := &config.AppConfig{}
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	request := httptest.NewRequest(http.MethodPost, "/api/v1/server/administration/security/allowlist", strings.NewReader(`{"cidr":"203.0.113.7","description":"build runner"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	AddAllowlist(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if len(config.GetGlobalConfig().Server.Security.Allowlist) != 1 || !config.IsIPAllowlisted("203.0.113.7") {
		t.Fatal("new allowlist entry was not active")
	}
}

func TestValidateAllowlistEntryRejectsMalformedValues(t *testing.T) {
	for _, input := range []struct{ cidr, description string }{
		{"203.0.113.0/24", ""},
		{"203.0.113.8/24", "host bits"},
		{"not-an-ip", "invalid"},
	} {
		if err := validateAllowlistEntry(input.cidr, input.description); err == nil {
			t.Errorf("validateAllowlistEntry(%q, %q) accepted invalid input", input.cidr, input.description)
		}
	}
}

func TestCreateIPBlockStoresTemporaryExpiryAndAdmin(t *testing.T) {
	db := newTestServerDB(t)
	setGlobalTestDualDB(t, db, db)
	request, response := newTestContextJSON(t, http.MethodPost, "/api/v1/server/administration/security/blocked-ips", map[string]string{
		"ip": "203.0.113.17", "type": "temporary", "reason": "repeated login failures",
	})
	request = request.WithContext(reqctx.SetValue(request.Context(), "admin_username", "operator"))
	request.Header.Set("Accept", "application/json")
	CreateIPBlock(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var expiry interface{}
	var blockedBy string
	if err := db.QueryRow(`SELECT expires_at, blocked_by FROM server_blocklists WHERE value = ?`, "203.0.113.17").Scan(&expiry, &blockedBy); err != nil {
		t.Fatal(err)
	}
	parsedExpiry, ok := dbtime.ParseStoredTimestamp(expiry)
	if !ok || time.Until(parsedExpiry) < 50*time.Minute || time.Until(parsedExpiry) > 70*time.Minute {
		t.Fatalf("temporary expiry = %v, valid=%t; want about one hour from now", parsedExpiry, ok)
	}
	if blockedBy != "operator" {
		t.Errorf("blocked_by = %q, want operator", blockedBy)
	}
}
