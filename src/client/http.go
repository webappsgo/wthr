package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient wraps the HTTP client with config
// Per AI.md PART 33: Supports cluster failover
type HTTPClient struct {
	CLIConfig     *CLIConfig
	HTTPClient    *http.Client
	currentServer string
	failedServers map[string]bool
}

// DefaultTimeout is the default HTTP request timeout
const DefaultTimeout = 30 * time.Second

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(config *CLIConfig) *HTTPClient {
	return &HTTPClient{
		CLIConfig: config,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		currentServer: config.GetPrimaryServer(),
		failedServers: make(map[string]bool),
	}
}

// UserAgent returns the User-Agent string
// Per AI.md PART 36: User-Agent uses hardcoded project name
func UserAgent() string {
	return fmt.Sprintf("%s-cli/%s", projectName, Version)
}

// Get performs a GET request with cluster failover
// Per AI.md PART 33: Try primary, then cluster nodes on failure
func (c *HTTPClient) Get(path string) (*http.Response, error) {
	return c.doWithFailover("GET", path, nil)
}

// doWithFailover performs a request with automatic cluster failover
// Per AI.md PART 33: Silent failover to cluster nodes
func (c *HTTPClient) doWithFailover(method, path string, body []byte) (*http.Response, error) {
	servers := c.CLIConfig.GetAllServers()
	if len(servers) == 0 {
		return nil, NewConnectionError("no servers configured")
	}

	var lastErr error
	for _, server := range servers {
		// Skip servers that have already failed in this session
		if c.failedServers[server] {
			continue
		}

		url := server + path
		var req *http.Request
		var err error

		if body != nil {
			req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		} else {
			req, err = http.NewRequest(method, url, nil)
		}
		if err != nil {
			lastErr = NewConnectionError(fmt.Sprintf("failed to create request: %v", err))
			continue
		}

		c.setHeaders(req)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			// Mark server as failed and try next
			c.failedServers[server] = true
			lastErr = NewConnectionError(fmt.Sprintf("failed to connect to %s: %v", server, err))
			continue
		}

		// Update current server on success
		c.currentServer = server

		return c.handleResponse(resp)
	}

	// All servers failed
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, NewConnectionError("all servers unreachable")
}

// Post performs a POST request with JSON body and cluster failover
// Per AI.md PART 33: Try primary, then cluster nodes on failure
func (c *HTTPClient) Post(path string, body interface{}) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("failed to encode request: %v", err))
	}

	return c.doWithFailover("POST", path, jsonBody)
}

// setHeaders sets common headers on requests
// Per AI.md PART 36: Include User-Agent and auth token
func (c *HTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Accept", "application/json")

	if c.CLIConfig.Auth.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.CLIConfig.Auth.Token)
	}

	// Add user context header if specified
	// Per AI.md PART 36: --user flag translates to X-User-Context header
	if c.CLIConfig.User != "" {
		req.Header.Set("X-User-Context", c.CLIConfig.User)
	}
}

// handleResponse handles common response processing
func (c *HTTPClient) handleResponse(resp *http.Response) (*http.Response, error) {
	// Check for errors
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, NewAuthError("authentication failed - check your token")
	}

	if resp.StatusCode == 404 {
		resp.Body.Close()
		return nil, NewNotFoundError("resource not found")
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Try to parse error response
		var errorResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errorResp.Error != "" {
				return nil, NewAPIError(errorResp.Error)
			}
			if errorResp.Message != "" {
				return nil, NewAPIError(errorResp.Message)
			}
		}

		return nil, NewAPIError(fmt.Sprintf("server error (%d): %s", resp.StatusCode, string(body)))
	}

	return resp, nil
}

// GetJSON performs a GET request and decodes JSON response
func (c *HTTPClient) GetJSON(path string, result interface{}) error {
	resp, err := c.Get(path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return NewAPIError(fmt.Sprintf("failed to decode response: %v", err))
	}

	return nil
}

// PostJSON performs a POST request with JSON body and decodes JSON response
func (c *HTTPClient) PostJSON(path string, body interface{}, result interface{}) error {
	resp, err := c.Post(path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return NewAPIError(fmt.Sprintf("failed to decode response: %v", err))
	}

	return nil
}

// CheckServerVersion checks if the server version is compatible
func (c *HTTPClient) CheckServerVersion() error {
	var versionResp struct {
		Version string `json:"version"`
	}

	if err := c.GetJSON(c.CLIConfig.GetAPIPath()+"/version", &versionResp); err != nil {
		// If version endpoint doesn't exist, skip check
		if _, ok := err.(*ExitError); ok && err.(*ExitError).Code == ExitNotFound {
			return nil
		}
		return err
	}

	// Basic version compatibility check
	// Version comparison can be added in future if needed

	return nil
}
