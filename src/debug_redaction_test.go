package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRedactedPPROFCmdline(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"/usr/local/bin/wthr", "--config", "/private/secrets.yml"}
	t.Cleanup(func() { os.Args = originalArgs })

	response := httptest.NewRecorder()
	redactedPPROFCmdline(response, httptest.NewRequest("GET", "/debug/pprof/cmdline", nil))

	if got, want := response.Body.String(), "wthr\x00"; got != want {
		t.Fatalf("cmdline response = %q, want only binary name %q", got, want)
	}
}

func TestRedactedExpvarHandler(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"/usr/local/bin/wthr", "--token", "sensitive-value"}
	t.Cleanup(func() { os.Args = originalArgs })

	response := httptest.NewRecorder()
	redactedExpvarHandler().ServeHTTP(response, httptest.NewRequest("GET", "/debug/vars", nil))

	var variables map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &variables); err != nil {
		t.Fatalf("decode expvar response: %v", err)
	}
	var commandLine []string
	if err := json.Unmarshal(variables["cmdline"], &commandLine); err != nil {
		t.Fatalf("decode cmdline expvar: %v", err)
	}
	if want := []string{"wthr"}; !reflect.DeepEqual(commandLine, want) {
		t.Fatalf("cmdline expvar = %q, want %q", commandLine, want)
	}
	if strings.Contains(response.Body.String(), "sensitive-value") {
		t.Fatal("expvar response exposed a command-line secret")
	}
}
