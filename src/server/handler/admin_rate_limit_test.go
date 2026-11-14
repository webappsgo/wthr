package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

func TestApplyRateLimitConfigRequest(t *testing.T) {
	req := rateLimitConfigRequest{
		Enabled: true, GlobalBurst: 240,
		ReadRequests: 120, ReadWindow: 60,
		WriteRequests: 10, WriteWindow: 60,
		HealthRequests: 120, HealthWindow: 60,
		LoginRequests: 5, LoginWindow: 900,
		ResetRequests: 3, ResetWindow: 3600,
		RegisterRequests: 5, RegisterWindow: 3600,
	}
	got, err := applyRateLimitConfigRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Read != (config.RateLimitBucket{Requests: 120, Window: 60}) || got.Auth.PasswordReset != (config.RateLimitBucket{Requests: 3, Window: 3600}) {
		t.Fatalf("mapped rate limit config = %+v", got)
	}

	req.WriteWindow = 0
	if _, err := applyRateLimitConfigRequest(req); err == nil {
		t.Fatal("expected invalid zero window to be rejected")
	}
	maxDurationSeconds := int64((time.Duration(1<<63 - 1)) / time.Second)
	if int64(int(^uint(0)>>1)) > maxDurationSeconds {
		req.WriteWindow = 60
		req.ReadWindow = int(maxDurationSeconds + 1)
		if _, err := applyRateLimitConfigRequest(req); err == nil {
			t.Fatal("expected duration-overflowing window to be rejected")
		}
	}
}

func TestDecodeRateLimitConfigForm(t *testing.T) {
	r := httptest.NewRequest("POST", "/server/administration/settings/rate-limit", strings.NewReader("enabled=on&global_burst=240&read_requests=120&read_window=60&write_requests=10&write_window=60&health_requests=120&health_window=60&login_requests=5&login_window=900&password_reset_requests=3&password_reset_window=3600&registration_requests=5&registration_window=3600"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var got rateLimitConfigRequest
	if err := decodeRateLimitConfigForm(r, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.GlobalBurst != 240 || got.ResetWindow != 3600 || got.RegisterRequests != 5 {
		t.Fatalf("decoded rate limit form = %+v", got)
	}
}
