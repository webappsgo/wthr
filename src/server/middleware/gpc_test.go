package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGlobalPrivacyControlExposesRequestFlag(t *testing.T) {
	handler := GlobalPrivacyControl(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !GPCOptOut(r) {
			t.Error("GPCOptOut() = false inside request with Sec-GPC: 1")
		}
		if _, err := r.Cookie("theme"); err == nil {
			t.Error("optional theme cookie remained available on a GPC request")
		}
		if _, err := r.Cookie("user_lat"); err == nil {
			t.Error("personal location cookie remained available on a GPC request")
		}
		if _, err := r.Cookie("weather_session"); err != nil {
			t.Error("required session cookie was removed on a GPC request")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/server/preferences", nil)
	req.Header.Set("Sec-GPC", "1")
	req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	req.AddCookie(&http.Cookie{Name: "user_lat", Value: "51.5"})
	req.AddCookie(&http.Cookie{Name: "weather_session", Value: "session-id"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestGlobalPrivacyControlDoesNotMarkOrdinaryRequest(t *testing.T) {
	handler := GlobalPrivacyControl(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GPCOptOut(r) {
			t.Error("GPCOptOut() = true without Sec-GPC: 1")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
