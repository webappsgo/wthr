package handler

import (
	"bytes"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/metric"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

var contactAntispamState struct {
	sync.Mutex
	secret  []byte
	manager *ContactAntispam
}

func recordContactAntispamBlock(r *http.Request, manager *ContactAntispam, reason string) time.Duration {
	metric.ContactBlockedTotal.WithLabelValues(reason).Inc()
	ip := util.TrustedGetClientIP(r)
	if securityLogger != nil {
		securityLogger.Security(ip, "contact.spam_blocked", reason)
	}
	switch reason {
	case "honeypot", "captcha", "content", "markup", "duplicate", "origin", "replay", "review":
		if util.OverlayHostFromRequest(r) != "" {
			return 0
		}
		return manager.RecordContactStrike(ip, time.Now())
	default:
		return 0
	}
}

func getContactAntispam(db *database.Store, cfg *config.AppConfig) (*ContactAntispam, error) {
	if db == nil {
		db = database.GetServerStore()
		if db == nil {
			return nil, errors.New("contact antispam requires the server database")
		}
	}
	secret, err := service.InstallationSecret(db)
	if err != nil {
		return nil, err
	}
	settings := config.DefaultContactAntispamConfig()
	if cfg != nil {
		settings = cfg.Server.Pages.Contact.EffectiveAntispam()
	}
	managerConfig, err := contactAntispamConfig(settings)
	if err != nil {
		return nil, err
	}
	contactAntispamState.Lock()
	defer contactAntispamState.Unlock()
	if contactAntispamState.manager == nil || !bytes.Equal(contactAntispamState.secret, secret) {
		contactAntispamState.secret = append([]byte(nil), secret...)
		contactAntispamState.manager = NewContactAntispam(secret, managerConfig)
	} else {
		contactAntispamState.manager.SetConfig(managerConfig)
	}
	return contactAntispamState.manager, nil
}

func contactAntispamConfig(settings config.ContactAntispamConfig) (ContactAntispamConfig, error) {
	window, err := time.ParseDuration(settings.Window)
	if err != nil && settings.Window != "" {
		return ContactAntispamConfig{}, errors.New("invalid contact antispam window")
	}
	if settings.Honeypot == nil {
		value := true
		settings.Honeypot = &value
	}
	if settings.RejectMarkup == nil {
		value := true
		settings.RejectMarkup = &value
	}
	return ContactAntispamConfig{
		MinFill:      time.Duration(settings.MinFillSeconds) * time.Second,
		MaxAge:       time.Duration(settings.MaxAgeSeconds) * time.Second,
		Window:       window,
		BanDuration:  time.Duration(settings.BanHours) * time.Hour,
		ReviewMin:    time.Duration(settings.ReviewMinSeconds) * time.Second,
		ReviewMax:    time.Duration(settings.ReviewMaxSeconds) * time.Second,
		DuplicateFor: time.Duration(settings.DuplicateWindowHours) * time.Hour,
		PerIP:        settings.PerIP, PerEmail: settings.PerEmail, Global: settings.Global,
		MaxLinks: settings.MaxLinks, Strikes: settings.Strikes,
		RejectMarkup:        *settings.RejectMarkup,
		BlockedWords:        append([]string(nil), settings.BlockedWords...),
		BlockedEmailDomains: append([]string(nil), settings.BlockedEmailDomains...),
	}, nil
}
