package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	// Per AI.md PART 33 line 45267-45326: Nested config structure
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: "http://localhost:8080",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}

	client := NewHTTPClient(config)

	if client.CLIConfig.Server.Primary != config.Server.Primary {
		t.Errorf("Expected server %s, got %s", config.Server.Primary, client.CLIConfig.Server.Primary)
	}

	// DefaultTimeout is used (30s)
	if client.HTTPClient.Timeout != DefaultTimeout {
		t.Errorf("Expected timeout %v, got %v", DefaultTimeout, client.HTTPClient.Timeout)
	}
}

func TestHTTPClientGet(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify headers
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization header with Bearer token")
		}

		if r.Header.Get("User-Agent") == "" {
			t.Error("Expected User-Agent header")
		}

		// Send response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create client with nested config
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	resp, err := client.GetResource("/test")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHTTPClientGetJSON(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"temperature": 72.5,
			"location":    "New York",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	var result map[string]interface{}
	err := client.GetJSON("/weather", &result)
	if err != nil {
		t.Fatalf("GetJSON() failed: %v", err)
	}

	// Verify response
	if temp, ok := result["temperature"].(float64); !ok || temp != 72.5 {
		t.Errorf("Expected temperature 72.5, got %v", result["temperature"])
	}

	if loc, ok := result["location"].(string); !ok || loc != "New York" {
		t.Errorf("Expected location 'New York', got %v", result["location"])
	}
}

func TestHTTPClientAuthError(t *testing.T) {
	// Create test server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "invalid-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected error for 401 response")
	}

	// Verify it's an auth error
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitAuthError {
			t.Errorf("Expected ExitAuthError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type")
	}
}

func TestHTTPClientForbiddenError(t *testing.T) {
	// Create test server that returns 403
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected error for 403 response")
	}

	// Verify it's an auth error
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitAuthError {
			t.Errorf("Expected ExitAuthError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type")
	}
}

func TestHTTPClientNotFoundError(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}

	// Verify it's a not found error
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitNotFound {
			t.Errorf("Expected ExitNotFound, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type")
	}
}

func TestHTTPClientServerError(t *testing.T) {
	// Create test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected error for 500 response")
	}

	// Verify it's an API error with general exit code
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitGeneralError {
			t.Errorf("Expected ExitGeneralError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type for server error")
	}
}

func TestHTTPClientConnectionError(t *testing.T) {
	// Create client with invalid server URL
	config := &CLIConfig{
		Server: ServerConfig{
			// Invalid port
			Primary: "http://localhost:1",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected connection error")
	}

	// Verify it's a connection error
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitConnError {
			t.Errorf("Expected ExitConnError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type")
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	// Create test server that sleeps
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client - uses DefaultTimeout (30s) but we need shorter for test
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)
	// Override timeout for test
	client.HTTPClient.Timeout = 1 * time.Second

	// Make request
	_, err := client.GetResource("/test")
	if err == nil {
		t.Fatal("Expected timeout error")
	}

	// Should be a connection error
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitConnError {
			t.Errorf("Expected ExitConnError, got exit code %d", exitErr.Code)
		}
	}
}

func TestHTTPClientInvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	var result map[string]interface{}
	err := client.GetJSON("/test", &result)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	// Verify it's an API error about decoding
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Code != ExitGeneralError {
			t.Errorf("Expected ExitGeneralError, got exit code %d", exitErr.Code)
		}
	} else {
		t.Error("Expected ExitError type for decode error")
	}
}

func TestHTTPClientNoToken(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no Authorization header
		if r.Header.Get("Authorization") != "" {
			t.Error("Expected no Authorization header when token is empty")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create client without token
	config := &CLIConfig{
		Server: ServerConfig{
			Primary: server.URL,
		},
		Auth: AuthConfig{
			// No token
			Token: "",
		},
	}
	client := NewHTTPClient(config)

	// Make request
	resp, err := client.GetResource("/test")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCheckServerVersion(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary:    server.URL,
			APIVersion: "v1",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Check server version
	err := client.CheckServerVersion()
	if err != nil {
		t.Fatalf("CheckServerVersion() failed: %v", err)
	}
}

func TestCheckServerVersionNoEndpoint(t *testing.T) {
	// Create test server that doesn't have version endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create client
	config := &CLIConfig{
		Server: ServerConfig{
			Primary:    server.URL,
			APIVersion: "v1",
		},
		Auth: AuthConfig{
			Token: "test-token",
		},
	}
	client := NewHTTPClient(config)

	// Check server version - should not error if endpoint doesn't exist
	err := client.CheckServerVersion()
	if err != nil {
		t.Fatalf("CheckServerVersion() should not error when endpoint doesn't exist: %v", err)
	}
}
