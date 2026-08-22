package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/webappsgo/wthr/src/util"
)

// APIResponse represents a standardized action response per AI.md PART 14
// Canonical shape: {"ok": true, "data": {"id": "...", "message": "..."}}
type APIResponse struct {
	// Always true for successful actions
	OK bool `json:"ok"`
	// Response payload; carries id/message and any extra fields
	Data interface{} `json:"data,omitempty"`
}

// ErrorResponse represents a standardized error response per AI.md PART 14
// Canonical shape: {"ok": false, "error": "CODE", "message": "...", "details": {}}
// The HTTP status code carries the status; it is never duplicated in the body.
type ErrorResponse struct {
	// Always false for errors
	OK bool `json:"ok"`
	// Machine-readable error code (UPPER_SNAKE_CASE, e.g. INVALID_INPUT, NOT_FOUND)
	Error string `json:"error"`
	// Human-readable, user-safe explanation
	Message string `json:"message"`
	// Additional error context (validation errors, field names)
	Details map[string]interface{} `json:"details,omitempty"`
}

// PaginatedResponse represents a paginated API response per AI.md PART 14
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination contains pagination metadata per AI.md PART 14
type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// Common error codes per AI.md PART 14
const (
	// Client errors (4xx)
	ErrInvalidInput     = "INVALID_INPUT"
	ErrNotFound         = "NOT_FOUND"
	ErrUnauthorized     = "UNAUTHORIZED"
	ErrForbidden        = "FORBIDDEN"
	ErrConflict         = "CONFLICT"
	ErrValidationFailed = "VALIDATION_FAILED"
	ErrRateLimited      = "RATE_LIMITED"
	ErrBadRequest       = "BAD_REQUEST"

	// Server errors (5xx)
	ErrInternal        = "INTERNAL_ERROR"
	ErrServiceUnavail  = "SERVICE_UNAVAILABLE"
	ErrDatabaseError   = "DATABASE_ERROR"
	ErrExternalService = "EXTERNAL_SERVICE_ERROR"
)

// RespondError sends a standardized error response per AI.md PART 14
// Format: {"ok": false, "error": "ERROR_CODE", "message": "Human readable message", "details": {}}
func RespondError(c *gin.Context, status int, code string, message string, details ...map[string]interface{}) {
	// AI.md PART 14: Support .txt extension for text responses
	if shouldRespondText(c) {
		c.String(status, "%s: %s\n", code, message)
		return
	}

	response := ErrorResponse{
		OK:      false,
		Error:   code,
		Message: message,
	}

	// Add details if provided
	if len(details) > 0 {
		response.Details = details[0]
	}

	c.JSON(status, response)
}

// RespondSuccess sends a standardized action success response per AI.md PART 14
// Format: {"ok": true, "data": {"message": "Item created successfully"}}
func RespondSuccess(c *gin.Context, message string, data ...map[string]interface{}) {
	// AI.md PART 14: Support .txt extension for text responses
	if shouldRespondText(c) {
		c.String(http.StatusOK, "%s\n", message)
		return
	}

	payload := map[string]interface{}{}
	if len(data) > 0 && data[0] != nil {
		for k, v := range data[0] {
			payload[k] = v
		}
	}
	if message != "" {
		payload["message"] = message
	}

	c.JSON(http.StatusOK, APIResponse{OK: true, Data: payload})
}

// RespondCreated sends a standardized response for resource creation per AI.md PART 14
// Format: {"ok": true, "data": {"id": "item_123", "message": "Item created successfully"}}
func RespondCreated(c *gin.Context, message string, id string, data ...map[string]interface{}) {
	// AI.md PART 14: Support .txt extension for text responses
	if shouldRespondText(c) {
		c.String(http.StatusCreated, "Created: %s (ID: %s)\n", message, id)
		return
	}

	payload := map[string]interface{}{}
	if len(data) > 0 && data[0] != nil {
		for k, v := range data[0] {
			payload[k] = v
		}
	}
	if id != "" {
		payload["id"] = id
	}
	if message != "" {
		payload["message"] = message
	}

	c.JSON(http.StatusCreated, APIResponse{OK: true, Data: payload})
}

// RespondData sends a data response per AI.md PART 14
// Returns the item directly without wrapper
func RespondData(c *gin.Context, data interface{}) {
	// AI.md PART 14: Support .txt extension for text responses
	if shouldRespondText(c) {
		c.String(http.StatusOK, "%v\n", data)
		return
	}

	// Per AI.md PART 14: "Returns the item directly without wrapper"
	c.JSON(http.StatusOK, data)
}

// RespondNegotiatedData sends JSON normally and text/plain as pretty-printed JSON.
func RespondNegotiatedData(c *gin.Context, status int, data interface{}) {
	if shouldRespondText(c) {
		payload, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			c.String(status, "%s\n", fmt.Sprintf("%v", data))
			return
		}
		c.Data(status, "text/plain; charset=utf-8", append(payload, '\n'))
		return
	}

	c.JSON(status, data)
}

// RespondPaginated sends a paginated response per AI.md PART 14
// Format: {"data": [...], "pagination": {"page": 1, "limit": 250, "total": 1000, "pages": 4}}
func RespondPaginated(c *gin.Context, data interface{}, page, limit, total int) {
	pages := total / limit
	if total%limit != 0 {
		pages++
	}

	// AI.md PART 14: Support .txt extension for text responses
	if shouldRespondText(c) {
		c.String(http.StatusOK, "Page %d/%d (Total: %d items)\n", page, pages, total)
		return
	}

	response := PaginatedResponse{
		Data: data,
		Pagination: Pagination{
			Page:  page,
			Limit: limit,
			Total: total,
			Pages: pages,
		},
	}

	c.JSON(http.StatusOK, response)
}

// shouldRespondText checks if the request wants text response per AI.md PART 14
// Supports both .txt extension and Accept: text/plain header
func shouldRespondText(c *gin.Context) bool {
	// Check URL extension: /api/v1/weather.txt
	path := c.Request.URL.Path
	ext := filepath.Ext(path)
	if ext == ".txt" {
		return true
	}

	// Check Accept header
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/plain")
}

// WantsJSON checks if the request wants JSON response per AI.md PART 14
// Content negotiation: Accept: application/json returns JSON, otherwise HTML
func WantsJSON(c *gin.Context) bool {
	accept := c.GetHeader("Accept")

	// Explicit JSON request
	if strings.Contains(accept, "application/json") {
		return true
	}

	// Check query parameter ?format=json
	if c.Query("format") == "json" {
		return true
	}

	// Check if it's an API route (already JSON)
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/") {
		return true
	}

	// Check common CLI tools that prefer JSON
	userAgent := strings.ToLower(c.GetHeader("User-Agent"))
	if strings.Contains(userAgent, "curl") ||
		strings.Contains(userAgent, "wget") ||
		strings.Contains(userAgent, "httpie") {
		// CLI tools: check if Accept header is not explicitly HTML
		if !strings.Contains(accept, "text/html") {
			return true
		}
	}

	return false
}

// NegotiateResponse returns JSON or HTML based on Accept header
// AI.md PART 14: Content negotiation for all routes
func NegotiateResponse(c *gin.Context, htmlTemplate string, data gin.H) {
	if shouldRespondText(c) {
		RespondNegotiatedData(c, http.StatusOK, data)
		return
	}
	if WantsJSON(c) {
		RespondData(c, data)
		return
	}
	// Templates including the shared head/navbar/footer partials need the chrome keys.
	// util.TemplateData merges caller keys last, so it is safe for already-wrapped data.
	c.HTML(http.StatusOK, htmlTemplate, util.TemplateData(c, data))
}

// NegotiateErrorResponse returns JSON or HTML error based on Accept header
func NegotiateErrorResponse(c *gin.Context, status int, htmlTemplate string, errCode string, message string, data gin.H) {
	if shouldRespondText(c) {
		RespondError(c, status, errCode, message)
		return
	}
	if WantsJSON(c) {
		RespondError(c, status, errCode, message)
		return
	}
	// Enrich first so the error page keeps the shared chrome keys, then layer the
	// error fields on top of the enriched copy instead of mutating the caller's map.
	enriched := util.TemplateData(c, data)
	enriched["Error"] = message
	enriched["ErrorCode"] = errCode
	c.HTML(status, htmlTemplate, enriched)
}

// Helper functions for common error scenarios

// BadRequest returns a 400 Bad Request error
func BadRequest(c *gin.Context, message string, details ...map[string]interface{}) {
	RespondError(c, http.StatusBadRequest, ErrBadRequest, message, details...)
}

// InvalidInput returns a 400 Invalid Input error
func InvalidInput(c *gin.Context, message string, details ...map[string]interface{}) {
	RespondError(c, http.StatusBadRequest, ErrInvalidInput, message, details...)
}

// NotFound returns a 404 Not Found error
func NotFound(c *gin.Context, message string) {
	RespondError(c, http.StatusNotFound, ErrNotFound, message)
}

// Unauthorized returns a 401 Unauthorized error
func Unauthorized(c *gin.Context, message string) {
	RespondError(c, http.StatusUnauthorized, ErrUnauthorized, message)
}

// Forbidden returns a 403 Forbidden error
func Forbidden(c *gin.Context, message string) {
	RespondError(c, http.StatusForbidden, ErrForbidden, message)
}

// Conflict returns a 409 Conflict error
func Conflict(c *gin.Context, message string, details ...map[string]interface{}) {
	RespondError(c, http.StatusConflict, ErrConflict, message, details...)
}

// InternalError returns a 500 Internal Server Error
func InternalError(c *gin.Context, message string) {
	RespondError(c, http.StatusInternalServerError, ErrInternal, message)
}

// ValidationFailed returns a 422 Validation Failed error
func ValidationFailed(c *gin.Context, message string, details map[string]interface{}) {
	RespondError(c, http.StatusUnprocessableEntity, ErrValidationFailed, message, details)
}

// RateLimited returns a 429 Rate Limited error
func RateLimited(c *gin.Context, message string) {
	RespondError(c, http.StatusTooManyRequests, ErrRateLimited, message)
}
