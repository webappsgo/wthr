package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
)

func TestToolsListUsesRegisteredAPIRoutes(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/v1/weather/{location}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router.Get("/api/v1/users/settings", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router.Get("/api/v1/server/administration/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := NewServer(router, nil, nil, "test")
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Result.Tools) != 1 || body.Result.Tools[0].Name != "getWeatherLocation" {
		t.Fatalf("tools = %#v, want only the public weather operation and no user-only operation", body.Result.Tools)
	}
}

func TestCallToolDispatchesIntoRegisteredRouter(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/v1/weather/{location}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"location":"` + chi.URLParam(r, "location") + `"}`))
	})
	server := NewServer(router, nil, nil, "test")
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"c1","method":"tools/call","params":{"name":"getWeatherLocation","arguments":{"location":"New York"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Cookie", "weather_session=must-not-be-forwarded")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/call status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `New York`) {
		t.Fatalf("tool result did not contain dispatched response: %s", response.Body.String())
	}
}

func TestMethodAndOriginValidation(t *testing.T) {
	server := NewServer(chi.NewRouter(), nil, nil, "test")
	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", get.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.invalid")
	request.Header.Set("Accept", "application/json, text/event-stream")
	post := httptest.NewRecorder()
	server.ServeHTTP(post, request)
	if post.Code != http.StatusForbidden {
		t.Fatalf("invalid Origin status = %d, want 403", post.Code)
	}
}

func TestConfiguredOriginMatchesSchemeAndDefaultPort(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Server.FQDN = "weather.example"
	cfg.Server.SSL.Enabled = true
	server := NewServer(chi.NewRouter(), cfg, nil, "test")
	request := httptest.NewRequest(http.MethodPost, "http://backend.internal:8080/mcp", nil)
	request.Header.Set("Origin", "https://weather.example")
	if !server.validOrigin(request) {
		t.Fatal("configured HTTPS origin should be accepted")
	}
	request.Header.Set("Origin", "https://weather.example:8443")
	if server.validOrigin(request) {
		t.Fatal("configured FQDN on an unconfigured port should be rejected")
	}
	request.Header.Set("Origin", "http://weather.example")
	if server.validOrigin(request) {
		t.Fatal("configured FQDN on the wrong scheme should be rejected")
	}
}

func TestModernDiscoveryAndStatelessToolsList(t *testing.T) {
	server := NewServer(chi.NewRouter(), nil, nil, "test")
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "server/discover")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("server/discover status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body struct {
		Result struct {
			SupportedVersions []string                   `json:"supportedVersions"`
			ResultType        string                     `json:"resultType"`
			TTL               int                        `json:"ttlMs"`
			CacheScope        string                     `json:"cacheScope"`
			Meta              map[string]json.RawMessage `json:"_meta"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !contains(body.Result.SupportedVersions, modernVersion) || body.Result.ResultType != "complete" || body.Result.TTL != 0 || body.Result.CacheScope != "private" {
		t.Fatalf("discover result missing modern protocol fields: %#v", body.Result)
	}
	if _, ok := body.Result.Meta["io.modelcontextprotocol/serverInfo"]; !ok {
		t.Fatalf("discover result missing server identity metadata: %#v", body.Result.Meta)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "tools/list")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ttlMs":0`) || !strings.Contains(response.Body.String(), `"resultType":"complete"`) {
		t.Fatalf("modern tools/list response missing protocol fields: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestModernProtocolRejectsHeaderMismatchAndUnsupportedVersion(t *testing.T) {
	server := NewServer(chi.NewRouter(), nil, nil, "test")
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "tools/call")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":-32020`) {
		t.Fatalf("mismatched header status/body = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2099-01-01")
	request.Header.Set("Mcp-Method", "server/discover")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":-32022`) || !strings.Contains(response.Body.String(), modernVersion) {
		t.Fatalf("unsupported version status/body = %d %s", response.Code, response.Body.String())
	}

	for _, method := range []string{"initialize", "ping"} {
		request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"`+method+`","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("MCP-Protocol-Version", "2026-07-28")
		request.Header.Set("Mcp-Method", method)
		response = httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":-32601`) {
			t.Errorf("modern %s status/body = %d %s", method, response.Code, response.Body.String())
		}
	}
}

func TestLLMSUsesPublicRouteCatalog(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/v1/weather/{location}", func(http.ResponseWriter, *http.Request) {})
	router.Get("/api/v1/server/administration/status", func(http.ResponseWriter, *http.Request) {})
	router.Get("/api/v1/server/metrics", func(http.ResponseWriter, *http.Request) {})
	cfg := &config.AppConfig{}
	cfg.Server.Branding.Title = "Weather"
	cfg.Server.Branding.Description = "Forecasts for everyone"
	cfg.Web.LLMS.Enabled = true
	cfg.Web.LLMS.IncludeEndpoints = true
	server := NewServer(router, cfg, nil, "test")
	response := httptest.NewRecorder()
	server.LLMS(response, httptest.NewRequest(http.MethodGet, "/llms.txt", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("LLMS status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Weather", "Base URL: http://" + hostname + "/api/v1", "GET /weather/{location}", "MCP: http://" + hostname + "/mcp"} {
		if !strings.Contains(body, want) {
			t.Errorf("llms.txt missing %q: %s", want, body)
		}
	}
	for _, excluded := range []string{"/server/administration", "/server/metrics"} {
		if strings.Contains(body, excluded) {
			t.Errorf("llms.txt must omit %q: %s", excluded, body)
		}
	}
}

func TestLLMSCanBeDisabled(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Web.LLMS.Enabled = false
	server := NewServer(chi.NewRouter(), cfg, nil, "test")
	response := httptest.NewRecorder()
	server.LLMS(response, httptest.NewRequest(http.MethodGet, "/llms.txt", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled llms status = %d, want 404", response.Code)
	}
}
