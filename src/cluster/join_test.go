package cluster

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJoinHandlerRejectsInvalidRequests(t *testing.T) {
	handler := JoinHandler(nil)
	tests := []struct {
		name       string
		method     string
		remoteAddr string
		body       string
		wantStatus int
	}{
		{name: "method", method: http.MethodGet, remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusMethodNotAllowed},
		{name: "requires tls", method: http.MethodPost, remoteAddr: "192.0.2.1:1234", body: `{}`, wantStatus: http.StatusUpgradeRequired},
		{name: "malformed json", method: http.MethodPost, remoteAddr: "127.0.0.1:1234", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "invalid node id", method: http.MethodPost, remoteAddr: "127.0.0.1:1234", body: `{"token":"bad","node_id":"bad\nnode"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid token", method: http.MethodPost, remoteAddr: "127.0.0.1:1234", body: `{"token":"bad","node_id":"node-1"}`, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "/cluster/join", strings.NewReader(test.body))
			req.RemoteAddr = test.remoteAddr
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != test.wantStatus {
				t.Fatalf("JoinHandler status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestWriteTokenFailureMapsKnownErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: ErrJoinTokenInvalid, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_INVALID"},
		{name: "expired", err: ErrJoinTokenExpired, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_EXPIRED"},
		{name: "used", err: ErrJoinTokenUsed, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_USED"},
		{name: "locked", err: ErrJoinTokenLocked, wantStatus: http.StatusUnauthorized, wantCode: "TOKEN_LOCKED"},
		{name: "database", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCode: "DATABASE_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeTokenFailure(response, test.err)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"error":"`+test.wantCode+`"`) {
				t.Fatalf("writeTokenFailure() = %d %s, want status %d and code %q", response.Code, response.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestJoinTokenFormatAndUniqueness(t *testing.T) {
	first, err := newJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	if !joinTokenPattern.MatchString(first) {
		t.Fatalf("generated token has invalid format")
	}
	if first == second {
		t.Fatal("generated tokens must be unique")
	}
	if hashJoinToken(first) == first {
		t.Fatal("token storage value must be a hash")
	}
}

func TestJoinPayloadEncryptionAuthenticatesToken(t *testing.T) {
	plain := []byte(`{"database":{"password":"private"}}`)
	encrypted, err := encryptJoinPayload(plain, "node_0123456789abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decryptJoinPayload(encrypted, "node_0123456789abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatal("decrypted bootstrap differs from original")
	}
	if _, err := decryptJoinPayload(encrypted, "node_1123456789abcdefghijklmnopqrstuv"); err == nil {
		t.Fatal("bootstrap decrypted with a different token")
	}
}

func TestJoinPayloadRejectsMalformedCiphertext(t *testing.T) {
	if _, err := decryptJoinPayload("not-base64!", "node_0123456789abcdefghijklmnopqrstuv"); err == nil {
		t.Fatal("malformed ciphertext was accepted")
	}
}
