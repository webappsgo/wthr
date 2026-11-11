package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/config"
)

type rateLimitConfigRequest struct {
	Enabled          bool `json:"enabled"`
	GlobalBurst      int  `json:"global_burst"`
	ReadRequests     int  `json:"read_requests"`
	ReadWindow       int  `json:"read_window"`
	WriteRequests    int  `json:"write_requests"`
	WriteWindow      int  `json:"write_window"`
	HealthRequests   int  `json:"health_requests"`
	HealthWindow     int  `json:"health_window"`
	LoginRequests    int  `json:"login_requests"`
	LoginWindow      int  `json:"login_window"`
	ResetRequests    int  `json:"password_reset_requests"`
	ResetWindow      int  `json:"password_reset_window"`
	RegisterRequests int  `json:"registration_requests"`
	RegisterWindow   int  `json:"registration_window"`
}

func rateLimitConfigView(c config.RateLimitConfig) map[string]interface{} {
	return map[string]interface{}{
		"enabled": c.Enabled, "global_burst": c.GlobalBurst,
		"read_requests": c.Read.Requests, "read_window": c.Read.Window,
		"write_requests": c.Write.Requests, "write_window": c.Write.Window,
		"health_requests": c.Health.Requests, "health_window": c.Health.Window,
		"login_requests": c.Auth.Login.Requests, "login_window": c.Auth.Login.Window,
		"password_reset_requests": c.Auth.PasswordReset.Requests, "password_reset_window": c.Auth.PasswordReset.Window,
		"registration_requests": c.Auth.Registration.Requests, "registration_window": c.Auth.Registration.Window,
	}
}

func UpdateRateLimitConfig(w http.ResponseWriter, r *http.Request) {
	var req rateLimitConfigRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			BadRequest(w, r, "Invalid rate limit configuration")
			return
		}
	} else if err := decodeRateLimitConfigForm(r, &req); err != nil {
		BadRequest(w, r, "Invalid rate limit configuration")
		return
	}

	current := config.GetGlobalConfig()
	if current == nil {
		var err error
		current, err = config.LoadConfig()
		if err != nil {
			InternalError(w, r, "Configuration is unavailable")
			return
		}
	}
	next, err := applyRateLimitConfigRequest(req)
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}
	updated := *current
	updated.Server.RateLimit = next
	if err := config.SaveConfig(&updated); err != nil {
		InternalError(w, r, "Failed to save rate limit configuration")
		return
	}
	config.SetGlobalConfig(&updated)
	if r.Method == http.MethodPost {
		settingsPath := strings.TrimSuffix(r.URL.Path, "/config/settings/rate-limit") + "/config/settings"
		http.Redirect(w, r, settingsPath, http.StatusSeeOther)
		return
	}
	RespondSuccess(w, r, "Rate limit settings saved and applied.", map[string]interface{}{
		"requires_restart": false,
		"rate_limit":       rateLimitConfigView(next),
	})
}

func decodeRateLimitConfigForm(r *http.Request, req *rateLimitConfigRequest) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	form := r.Form
	intFields := map[string]*int{
		"global_burst":  &req.GlobalBurst,
		"read_requests": &req.ReadRequests, "read_window": &req.ReadWindow,
		"write_requests": &req.WriteRequests, "write_window": &req.WriteWindow,
		"health_requests": &req.HealthRequests, "health_window": &req.HealthWindow,
		"login_requests": &req.LoginRequests, "login_window": &req.LoginWindow,
		"password_reset_requests": &req.ResetRequests, "password_reset_window": &req.ResetWindow,
		"registration_requests": &req.RegisterRequests, "registration_window": &req.RegisterWindow,
	}
	for name, target := range intFields {
		value, err := strconv.Atoi(form.Get(name))
		if err != nil {
			return err
		}
		*target = value
	}
	req.Enabled = form.Get("enabled") == "on"
	return nil
}

func applyRateLimitConfigRequest(req rateLimitConfigRequest) (config.RateLimitConfig, error) {
	values := []int{req.GlobalBurst, req.ReadRequests, req.ReadWindow, req.WriteRequests, req.WriteWindow, req.HealthRequests, req.HealthWindow, req.LoginRequests, req.LoginWindow, req.ResetRequests, req.ResetWindow, req.RegisterRequests, req.RegisterWindow}
	for _, value := range values {
		if value < 1 {
			return config.RateLimitConfig{}, errors.New("all rate limit values must be positive integers")
		}
	}
	maxWindow := int64((time.Duration(1<<63 - 1)) / time.Second)
	for _, window := range []int{req.ReadWindow, req.WriteWindow, req.HealthWindow, req.LoginWindow, req.ResetWindow, req.RegisterWindow} {
		if int64(window) > maxWindow {
			return config.RateLimitConfig{}, errors.New("rate limit windows must fit within time.Duration")
		}
	}
	return config.RateLimitConfig{
		Enabled: req.Enabled, GlobalBurst: req.GlobalBurst,
		Read:   config.RateLimitBucket{Requests: req.ReadRequests, Window: req.ReadWindow},
		Write:  config.RateLimitBucket{Requests: req.WriteRequests, Window: req.WriteWindow},
		Health: config.RateLimitBucket{Requests: req.HealthRequests, Window: req.HealthWindow},
		Auth: config.RateLimitAuthConfig{
			Login:         config.RateLimitBucket{Requests: req.LoginRequests, Window: req.LoginWindow},
			PasswordReset: config.RateLimitBucket{Requests: req.ResetRequests, Window: req.ResetWindow},
			Registration:  config.RateLimitBucket{Requests: req.RegisterRequests, Window: req.RegisterWindow},
		},
	}, nil
}
