package main

import (
	"testing"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		actual   int
	}{
		{"ExitSuccess", 0, ExitSuccess},
		{"ExitGeneralError", 1, ExitGeneralError},
		{"ExitConfigError", 2, ExitConfigError},
		{"ExitConnError", 3, ExitConnError},
		{"ExitAuthError", 4, ExitAuthError},
		{"ExitNotFound", 5, ExitNotFound},
		{"ExitUsageError", 64, ExitUsageError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("Expected %s to be %d, got %d", tt.name, tt.expected, tt.actual)
			}
		})
	}
}

func TestExitErrorError(t *testing.T) {
	err := &ExitError{
		Message: "test error",
		Code:    ExitGeneralError,
	}

	if err.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", err.Error())
	}
}

func TestNewExitError(t *testing.T) {
	err := NewExitError("custom error", 99)

	if err.Message != "custom error" {
		t.Errorf("Expected message 'custom error', got '%s'", err.Message)
	}

	if err.Code != 99 {
		t.Errorf("Expected code 99, got %d", err.Code)
	}

	if err.Error() != "custom error" {
		t.Errorf("Expected error string 'custom error', got '%s'", err.Error())
	}
}

func TestNewConfigError(t *testing.T) {
	err := NewConfigError("config not found")

	if err.Message != "config not found" {
		t.Errorf("Expected message 'config not found', got '%s'", err.Message)
	}

	if err.Code != ExitConfigError {
		t.Errorf("Expected exit code %d, got %d", ExitConfigError, err.Code)
	}

	// Verify it's an *ExitError
	if _, ok := interface{}(err).(*ExitError); !ok {
		t.Error("Expected *ExitError type")
	}
}

func TestNewConnectionError(t *testing.T) {
	err := NewConnectionError("connection refused")

	if err.Message != "connection refused" {
		t.Errorf("Expected message 'connection refused', got '%s'", err.Message)
	}

	if err.Code != ExitConnError {
		t.Errorf("Expected exit code %d, got %d", ExitConnError, err.Code)
	}
}

func TestNewAuthError(t *testing.T) {
	err := NewAuthError("invalid token")

	if err.Message != "invalid token" {
		t.Errorf("Expected message 'invalid token', got '%s'", err.Message)
	}

	if err.Code != ExitAuthError {
		t.Errorf("Expected exit code %d, got %d", ExitAuthError, err.Code)
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("resource not found")

	if err.Message != "resource not found" {
		t.Errorf("Expected message 'resource not found', got '%s'", err.Message)
	}

	if err.Code != ExitNotFound {
		t.Errorf("Expected exit code %d, got %d", ExitNotFound, err.Code)
	}
}

func TestNewUsageError(t *testing.T) {
	err := NewUsageError("invalid command")

	if err.Message != "invalid command" {
		t.Errorf("Expected message 'invalid command', got '%s'", err.Message)
	}

	if err.Code != ExitUsageError {
		t.Errorf("Expected exit code %d, got %d", ExitUsageError, err.Code)
	}
}

func TestExitErrorImplementsError(t *testing.T) {
	var err error = NewConfigError("test")

	// Verify it implements error interface
	if err.Error() != "test" {
		t.Errorf("Expected error message 'test', got '%s'", err.Error())
	}

	// Verify type assertion works
	if exitErr, ok := err.(*ExitError); !ok {
		t.Error("Expected to be able to type assert to *ExitError")
	} else {
		if exitErr.Code != ExitConfigError {
			t.Errorf("Expected code %d, got %d", ExitConfigError, exitErr.Code)
		}
	}
}

func TestExitErrorCodes(t *testing.T) {
	// Verify all error constructors use correct exit codes
	tests := []struct {
		name     string
		errFunc  func(string) *ExitError
		expected int
	}{
		{"ConfigError", NewConfigError, ExitConfigError},
		{"ConnectionError", NewConnectionError, ExitConnError},
		{"AuthError", NewAuthError, ExitAuthError},
		{"NotFoundError", NewNotFoundError, ExitNotFound},
		{"UsageError", NewUsageError, ExitUsageError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.errFunc("test message")
			if err.Code != tt.expected {
				t.Errorf("Expected exit code %d, got %d", tt.expected, err.Code)
			}
		})
	}
}
