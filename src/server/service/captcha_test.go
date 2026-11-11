package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

func TestSimpleCaptchaSolveOnceAndExpire(t *testing.T) {
	c := NewSimpleCaptcha()
	id, a, b := c.New()
	if id == "" {
		t.Fatal("empty challenge id")
	}
	if c.Verify(id, "wrong") {
		t.Error("wrong answer accepted")
	}
	if c.Verify(id, strconv.Itoa(a+b)) {
		t.Error("challenge must be single-use even after a failed attempt")
	}

	id, a, b = c.New()
	if !c.Verify(id, " "+strconv.Itoa(a+b)+" ") {
		t.Error("correct answer rejected")
	}
	if c.Verify(id, strconv.Itoa(a+b)) {
		t.Error("challenge reused")
	}

	id, a, b = c.New()
	c.mu.Lock()
	ch := c.challenges[id]
	ch.expires = time.Now().Add(-time.Second)
	c.challenges[id] = ch
	c.mu.Unlock()
	if c.Verify(id, strconv.Itoa(a+b)) {
		t.Error("expired challenge accepted")
	}
	if c.Verify("unknown", "1") {
		t.Error("unknown id accepted")
	}
}

func TestSimpleCaptchaBounded(t *testing.T) {
	c := NewSimpleCaptcha()
	for i := 0; i < simpleCaptchaMax+50; i++ {
		c.New()
	}
	if len(c.challenges) > simpleCaptchaMax {
		t.Errorf("store grew to %d, max %d", len(c.challenges), simpleCaptchaMax)
	}
}

func TestVerifyProviderAt(t *testing.T) {
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = map[string]string{"secret": r.PostForm.Get("secret"), "response": r.PostForm.Get("response"), "remoteip": r.PostForm.Get("remoteip")}
		if r.PostForm.Get("response") == "good" {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	if err := verifyProviderAt(ctx, srv.Client(), srv.URL, config.CaptchaReCaptcha, "sec", "good", "1.2.3.4"); err != nil {
		t.Fatalf("good token: %v", err)
	}
	if gotForm["secret"] != "sec" || gotForm["remoteip"] != "1.2.3.4" {
		t.Errorf("form = %v", gotForm)
	}
	if err := verifyProviderAt(ctx, srv.Client(), srv.URL, config.CaptchaHCaptcha, "sec", "good", "1.2.3.4"); err != nil || gotForm["remoteip"] != "" {
		t.Errorf("hcaptcha must not send remoteip: err=%v form=%v", err, gotForm)
	}
	if err := verifyProviderAt(ctx, srv.Client(), srv.URL, config.CaptchaReCaptcha, "sec", "bad", ""); err != ErrCaptchaFailed {
		t.Errorf("bad token err = %v", err)
	}
	if err := verifyProviderAt(ctx, srv.Client(), srv.URL, config.CaptchaReCaptcha, "sec", "", ""); err != ErrCaptchaFailed {
		t.Errorf("empty token err = %v", err)
	}
	srv.Close()
	if err := verifyProviderAt(ctx, &http.Client{Timeout: time.Second}, srv.URL, config.CaptchaReCaptcha, "sec", "good", ""); err != ErrCaptchaUnavailable {
		t.Errorf("unreachable provider must fail closed, err = %v", err)
	}
}
