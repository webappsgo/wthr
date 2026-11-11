package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireOperatorToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := RequireOperatorToken("secret-token")(ok)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "Bearer nope", http.StatusUnauthorized},
		{"basic scheme", "Basic secret-token", http.StatusUnauthorized},
		{"correct", "Bearer secret-token", http.StatusNoContent},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
		if c.want == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: missing WWW-Authenticate", c.name)
		}
	}

	empty := RequireOperatorToken("")(ok)
	req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	empty.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("empty configured token must never authenticate, got %d", rec.Code)
	}
}

func TestNewOperatorToken(t *testing.T) {
	a, err := NewOperatorToken()
	if err != nil || len(a) != 32 {
		t.Fatalf("token %q err %v", a, err)
	}
	b, _ := NewOperatorToken()
	if a == b {
		t.Fatal("tokens must be random")
	}
}
