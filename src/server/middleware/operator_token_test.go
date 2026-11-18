package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireOperatorToken(t *testing.T) {
	SetOperatorToken("tok_secretvalue")
	t.Cleanup(func() { SetOperatorToken("") })
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := RequireOperatorToken()(ok)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "Bearer nope", http.StatusUnauthorized},
		{"basic scheme", "Basic tok_secretvalue", http.StatusUnauthorized},
		{"correct", "Bearer tok_secretvalue", http.StatusNoContent},
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
}

func TestOperatorTokenNeverAuthenticatesWhenUnset(t *testing.T) {
	SetOperatorToken("")
	h := RequireOperatorToken()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("an unset operator token must never authenticate, got %d", rec.Code)
	}
	if IsOperatorToken("") || IsOperatorToken("anything") {
		t.Error("IsOperatorToken must be false while no token is set")
	}
}

func TestRegeneratedTokenReplacesThePreviousOneAtOnce(t *testing.T) {
	SetOperatorToken("tok_first")
	t.Cleanup(func() { SetOperatorToken("") })
	if !IsOperatorToken("tok_first") {
		t.Fatal("the first token must work")
	}
	SetOperatorToken("tok_second")
	if IsOperatorToken("tok_first") || !IsOperatorToken("tok_second") {
		t.Error("the previous value must stop working immediately")
	}
}

func TestNewOperatorTokenFormat(t *testing.T) {
	a, err := NewOperatorToken()
	if err != nil || !IsOperatorTokenFormat(a) {
		t.Fatalf("token %q err %v", a, err)
	}
	if len(a) != len("tok_")+32 {
		t.Errorf("length = %d, want 36", len(a))
	}
	b, _ := NewOperatorToken()
	if a == b {
		t.Fatal("tokens must be random")
	}
}

func TestNewOperatorTokenUsesTheWholeAlphabet(t *testing.T) {
	seen := map[rune]bool{}
	for i := 0; i < 400; i++ {
		token, _ := NewOperatorToken()
		for _, r := range strings.TrimPrefix(token, "tok_") {
			seen[r] = true
		}
	}
	if len(seen) != len(base62Alphabet) {
		t.Errorf("only %d of %d characters ever appeared", len(seen), len(base62Alphabet))
	}
}

func TestIsOperatorTokenFormat(t *testing.T) {
	good, _ := NewOperatorToken()
	if !IsOperatorTokenFormat(good) {
		t.Error("a generated token must have the format")
	}
	for _, bad := range []string{"", "tok_", "tok_short", "abc_" + strings.Repeat("a", 32), "tok_" + strings.Repeat("a", 31) + "-", "tok_" + strings.Repeat("a", 33)} {
		if IsOperatorTokenFormat(bad) {
			t.Errorf("%q must not have the format", bad)
		}
	}
}

func TestMaskOperatorTokenNeverShowsTheValue(t *testing.T) {
	token, _ := NewOperatorToken()
	masked := MaskOperatorToken(token)
	if strings.Contains(masked, token[4:len(token)-4]) || !strings.HasPrefix(masked, "tok_") || !strings.HasSuffix(masked, token[len(token)-4:]) {
		t.Errorf("masked form = %q", masked)
	}
	if strings.Contains(MaskOperatorToken("tok_a"), "tok_a") {
		t.Error("a short value must be fully masked")
	}
}
