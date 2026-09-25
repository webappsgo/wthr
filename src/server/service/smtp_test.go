package service

import (
	"bufio"
	"database/sql"
	"net"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/wthr/src/database"
)

// setupSMTPServerDB opens a fresh in-memory SQLite database with the real
// production ServerSchema applied (server_config and
// server_notification_channels live there), uniquely named per test.
func setupSMTPServerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"_smtp?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(database.ServerSchema); err != nil {
		t.Fatalf("apply ServerSchema: %v", err)
	}
	return db
}

// wireSMTPGlobalDB wires database.SetGlobalDualDB so any code path that still
// falls back to database.GetServerDB() (an SMTPService constructed with a nil
// handle, or other services sharing this package) does not nil-pointer-panic,
// and restores the previous (nil) global state afterward so tests never leak
// state.
func wireSMTPGlobalDB(t *testing.T, serverDB *sql.DB) {
	t.Helper()
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })
}

// fakeSMTPResult captures what the fake SMTP responder observed for a
// single connection, so the test goroutine can assert on it.
type fakeSMTPResult struct {
	mailFrom string
	rcptTo   []string
	dataBody string
}

// startFakeSMTPResponder starts a minimal SMTP protocol responder bound to
// 127.0.0.1:0 (never a real/external host) sufficient for net/smtp.SendMail
// to complete a full unauthenticated send. If rcptResponse is non-empty, it
// is sent instead of "250 OK" after RCPT TO (e.g. to simulate a rejected
// recipient), and the DATA phase is skipped. The listener and its single
// accepted connection are torn down via t.Cleanup.
func startFakeSMTPResponder(t *testing.T, rcptResponse string) (host, port string, resultCh <-chan fakeSMTPResult) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on 127.0.0.1:0: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ch := make(chan fakeSMTPResult, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			ch <- fakeSMTPResult{}
			return
		}
		defer conn.Close()

		r := bufio.NewReader(conn)
		writeLine := func(s string) {
			conn.Write([]byte(s + "\r\n"))
		}

		var result fakeSMTPResult

		writeLine("220 localhost ESMTP ready")

		// EHLO or HELO - single-line reply, no extensions advertised so the
		// client never attempts STARTTLS/AUTH negotiation.
		if _, err := r.ReadString('\n'); err != nil {
			ch <- result
			return
		}
		writeLine("250 localhost greets you")

		// MAIL FROM
		line, err := r.ReadString('\n')
		if err != nil {
			ch <- result
			return
		}
		result.mailFrom = strings.TrimSpace(line)
		writeLine("250 OK")

		// RCPT TO
		line, err = r.ReadString('\n')
		if err != nil {
			ch <- result
			return
		}
		result.rcptTo = append(result.rcptTo, strings.TrimSpace(line))

		if rcptResponse != "" {
			writeLine(rcptResponse)
			ch <- result
			return
		}
		writeLine("250 OK")

		// DATA
		if _, err := r.ReadString('\n'); err != nil {
			ch <- result
			return
		}
		writeLine("354 End data with <CR><LF>.<CR><LF>")

		var body strings.Builder
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				break
			}
			if strings.TrimRight(l, "\r\n") == "." {
				break
			}
			body.WriteString(l)
		}
		result.dataBody = body.String()
		writeLine("250 OK: queued as 1")

		// QUIT (best-effort - client may close the connection without it)
		r.ReadString('\n')
		writeLine("221 Bye")

		ch <- result
	}()

	h, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	return h, p, ch
}

// startAcceptAndCloseListener accepts exactly one connection and closes it
// immediately, used to make a TLS handshake attempt fail fast (rather than
// hang) against a server that does not speak TLS.
func startAcceptAndCloseListener(t *testing.T) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on 127.0.0.1:0: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	h, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	return h, p
}

// TestSMTP_GetProviderPreset covers case-insensitive lookup and the
// not-found error path.
func TestSMTP_GetProviderPreset(t *testing.T) {
	tests := []struct {
		name     string
		lookup   string
		wantErr  bool
		wantHost string
	}{
		{"exact match", "Gmail", false, "smtp.gmail.com"},
		{"case-insensitive match", "gMaIl", false, "smtp.gmail.com"},
		{"self-hosted preset", "Postfix (Default)", false, "localhost"},
		{"unknown provider errors", "NoSuchProvider9000", true, ""},
		{"empty name errors", "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := GetProviderPreset(tt.lookup)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetProviderPreset(%q) error = nil, want error", tt.lookup)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetProviderPreset(%q) error = %v", tt.lookup, err)
			}
			if preset.Host != tt.wantHost {
				t.Errorf("GetProviderPreset(%q).Host = %q, want %q", tt.lookup, preset.Host, tt.wantHost)
			}
		})
	}
}

// TestSMTP_ListProviderPresets is a boundary/sanity check that the full
// preset table is non-empty and internally consistent (every preset has a
// name, host and category).
func TestSMTP_ListProviderPresets(t *testing.T) {
	presets := ListProviderPresets()
	if len(presets) == 0 {
		t.Fatal("ListProviderPresets() returned empty slice")
	}
	for _, p := range presets {
		if p.Name == "" || p.Host == "" || p.Category == "" {
			t.Errorf("preset with missing field: %+v", p)
		}
	}
}

// TestSMTP_ListProvidersByCategory covers a known category, the empty
// result for an unknown category, and confirms filtering is exact-match.
func TestSMTP_ListProvidersByCategory(t *testing.T) {
	t.Run("known category returns only matching entries", func(t *testing.T) {
		got := ListProvidersByCategory("development")
		if len(got) == 0 {
			t.Fatal("expected at least one development-category preset")
		}
		for _, p := range got {
			if p.Category != "development" {
				t.Errorf("preset %q has category %q, want development", p.Name, p.Category)
			}
		}
	})

	t.Run("unknown category returns empty slice, not error", func(t *testing.T) {
		got := ListProvidersByCategory("does-not-exist")
		if len(got) != 0 {
			t.Errorf("ListProvidersByCategory(unknown) = %+v, want empty", got)
		}
	})
}

// TestSMTP_LoadConfig covers the AI.md PART 18 Environment Variable Priority
// table (SMTP_* env vars override stored config), the environment path when the
// database has no value, and the defaults (port 587, from name "Weather") when
// neither is set.
func TestSMTP_LoadConfig(t *testing.T) {
	t.Run("env var overrides the stored database value", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)
		t.Setenv("SMTP_HOST", "env-host.example")

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.host", "db-host.example"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}

		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.Host != "env-host.example" {
			t.Errorf("Host = %q, want env-host.example (SMTP_HOST overrides stored config)", svc.config.Host)
		}
	})

	t.Run("env var used when database has no value", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)
		t.Setenv("SMTP_HOST", "env-host.example")
		t.Setenv("SMTP_FROM_EMAIL", "env@example.com")

		svc := NewSMTPService(db)
		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.Host != "env-host.example" {
			t.Errorf("Host = %q, want env-host.example", svc.config.Host)
		}
		if svc.config.FromAddress != "env@example.com" {
			t.Errorf("FromAddress = %q, want env@example.com", svc.config.FromAddress)
		}
	})

	t.Run("all seven SMTP env vars override stored config", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)
		t.Setenv("SMTP_HOST", "env.example.com")
		t.Setenv("SMTP_PORT", "2525")
		t.Setenv("SMTP_USERNAME", "env-user")
		t.Setenv("SMTP_PASSWORD", "env-pass")
		t.Setenv("SMTP_TLS", "true")
		t.Setenv("SMTP_FROM_NAME", "Env Name")
		t.Setenv("SMTP_FROM_EMAIL", "env@example.com")

		svc := NewSMTPService(db)
		for key, value := range map[string]string{
			"smtp.host":        "db.example.com",
			"smtp.port":        "25",
			"smtp.username":    "db-user",
			"smtp.password":    "db-pass",
			"smtp.use_tls":     "false",
			"smtp.from_name":   "DB Name",
			"smtp.from_address": "db@example.com",
		} {
			if err := svc.saveSetting(key, value); err != nil {
				t.Fatalf("saveSetting(%q): %v", key, err)
			}
		}

		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.Host != "env.example.com" {
			t.Errorf("Host = %q, want env.example.com", svc.config.Host)
		}
		if svc.config.Port != "2525" {
			t.Errorf("Port = %q, want 2525", svc.config.Port)
		}
		if svc.config.Username != "env-user" {
			t.Errorf("Username = %q, want env-user", svc.config.Username)
		}
		if svc.config.Password != "env-pass" {
			t.Errorf("Password = %q, want env-pass", svc.config.Password)
		}
		if !svc.config.UseTLS {
			t.Error("UseTLS = false, want true (SMTP_TLS=true overrides stored false)")
		}
		if svc.config.FromName != "Env Name" {
			t.Errorf("FromName = %q, want Env Name", svc.config.FromName)
		}
		if svc.config.FromAddress != "env@example.com" {
			t.Errorf("FromAddress = %q, want env@example.com", svc.config.FromAddress)
		}
	})

	t.Run("defaults applied when db and env are both empty", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.Host != "" {
			t.Errorf("Host = %q, want empty", svc.config.Host)
		}
		if svc.config.Port != "587" {
			t.Errorf("Port = %q, want default 587", svc.config.Port)
		}
		if svc.config.FromName != "Weather" {
			t.Errorf("FromName = %q, want default Weather", svc.config.FromName)
		}
	})

	t.Run("stored booleans accept the config.ParseBool truthy word list", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.use_tls", "true"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}
		if err := svc.saveSetting("smtp.auto_enable", "yes"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}
		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if !svc.config.UseTLS {
			t.Error("UseTLS = false, want true for stored value \"true\"")
		}
		if !svc.config.AutoEnable {
			t.Error("AutoEnable = false, want true for stored value \"yes\"")
		}
	})

	t.Run("stored booleans accept the config.ParseBool falsy word list", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.use_tls", "off"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}
		if err := svc.saveSetting("smtp.auto_enable", "no"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}
		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.UseTLS {
			t.Error("UseTLS = true, want false for stored value \"off\"")
		}
		if svc.config.AutoEnable {
			t.Error("AutoEnable = true, want false for stored value \"no\"")
		}
	})

	t.Run("empty stored booleans fall back to false", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if svc.config.UseTLS {
			t.Error("UseTLS = true, want false when no stored value exists")
		}
		if svc.config.AutoEnable {
			t.Error("AutoEnable = true, want false when no stored value exists")
		}
		if svc.config.Enabled {
			t.Error("Enabled = true, want false when no stored value exists")
		}
	})
}

// TestSMTP_IsEnabled covers the happy path (host+from address configured and
// a live handshake verified), the false path when unconfigured, and confirms
// IsEnabled triggers an implicit LoadConfig when config has never been loaded.
// AI.md PART 18: "SMTP configured and working" is the only enabling state, so a
// configured-but-unverified host must still report false.
func TestSMTP_IsEnabled(t *testing.T) {
	t.Run("false when never configured", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if svc.IsEnabled() {
			t.Error("IsEnabled() = true, want false with empty database")
		}
	})

	t.Run("false when host and from address configured but not yet verified", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.host", "smtp.example.com"); err != nil {
			t.Fatalf("saveSetting host: %v", err)
		}
		if err := svc.saveSetting("smtp.from_address", "noreply@example.com"); err != nil {
			t.Fatalf("saveSetting from_address: %v", err)
		}

		if svc.IsEnabled() {
			t.Error("IsEnabled() = true, want false when the connection has not been verified")
		}
	})

	t.Run("true when a working SMTP server was verified", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		host, port, _ := startFakeSMTPResponder(t, "")

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.host", host); err != nil {
			t.Fatalf("saveSetting host: %v", err)
		}
		if err := svc.saveSetting("smtp.port", port); err != nil {
			t.Fatalf("saveSetting port: %v", err)
		}
		if err := svc.saveSetting("smtp.from_address", "noreply@example.com"); err != nil {
			t.Fatalf("saveSetting from_address: %v", err)
		}

		if err := svc.VerifyConfiguredConnection(); err != nil {
			t.Fatalf("VerifyConfiguredConnection() error = %v, want nil", err)
		}
		if !svc.IsEnabled() {
			t.Error("IsEnabled() = false, want true after a successful handshake")
		}
	})

	t.Run("false when the configured host refuses connections", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.host", "127.0.0.1"); err != nil {
			t.Fatalf("saveSetting host: %v", err)
		}
		// port 1 is privileged and unbound in the container; dial fails fast
		if err := svc.saveSetting("smtp.port", "1"); err != nil {
			t.Fatalf("saveSetting port: %v", err)
		}
		if err := svc.saveSetting("smtp.from_address", "noreply@example.com"); err != nil {
			t.Fatalf("saveSetting from_address: %v", err)
		}

		if err := svc.VerifyConfiguredConnection(); err == nil {
			t.Fatal("VerifyConfiguredConnection() error = nil, want a dial failure")
		}
		if svc.IsEnabled() {
			t.Error("IsEnabled() = true, want false after a failed handshake")
		}
	})

	t.Run("false when host set but from address missing", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.saveSetting("smtp.host", "smtp.example.com"); err != nil {
			t.Fatalf("saveSetting host: %v", err)
		}

		if svc.IsEnabled() {
			t.Error("IsEnabled() = true, want false when from_address is missing")
		}
	})
}

// TestSMTP_VerifyConfiguredConnection covers the empty-host short circuit,
// which must not dial, and the empty-host result of leaving verified false.
func TestSMTP_VerifyConfiguredConnection(t *testing.T) {
	t.Run("empty host disables email without dialing", func(t *testing.T) {
		db := setupSMTPServerDB(t)
		wireSMTPGlobalDB(t, db)

		svc := NewSMTPService(db)
		if err := svc.VerifyConfiguredConnection(); err != nil {
			t.Errorf("VerifyConfiguredConnection() error = %v, want nil for an unconfigured host", err)
		}
		if svc.verified {
			t.Error("verified = true, want false when no host is configured")
		}
	})
}

// TestSMTP_TestConnection never dials a real external host: it uses either
// an explicit "unconfigured" config, a local 127.0.0.1 listener, or a
// deliberately-closed port to exercise every branch deterministically.
func TestSMTP_TestConnection(t *testing.T) {
	t.Run("unconfigured host returns error without dialing", func(t *testing.T) {
		svc := &SMTPService{}
		err := svc.TestConnection(&SMTPConfig{Host: ""})
		if err == nil {
			t.Fatal("TestConnection() error = nil, want error for empty host")
		}
	})

	t.Run("nil config parameter falls back to service config", func(t *testing.T) {
		svc := &SMTPService{config: &SMTPConfig{Host: ""}}
		err := svc.TestConnection(nil)
		if err == nil {
			t.Fatal("TestConnection(nil) error = nil, want error using service's empty config")
		}
	})

	// AI.md PART 18 requires a real EHLO handshake, not a bare TCP connect,
	// so the success path must run against a responder that actually answers
	// with a 220 greeting and a 2xx EHLO reply.
	t.Run("plain connection to a live SMTP responder succeeds", func(t *testing.T) {
		host, port, _ := startFakeSMTPResponder(t, "")

		svc := &SMTPService{}
		if err := svc.TestConnection(&SMTPConfig{Host: host, Port: port, UseTLS: false}); err != nil {
			t.Errorf("TestConnection() error = %v, want nil", err)
		}
	})

	t.Run("connection refused when nothing is listening", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		host, port, splitErr := net.SplitHostPort(ln.Addr().String())
		if splitErr != nil {
			t.Fatalf("split addr: %v", splitErr)
		}
		// free the port so the subsequent dial is refused
		ln.Close()

		svc := &SMTPService{}
		err = svc.TestConnection(&SMTPConfig{Host: host, Port: port, UseTLS: false})
		if err == nil {
			t.Fatal("TestConnection() error = nil, want dial error against closed port")
		}
		if !strings.Contains(err.Error(), "SMTP handshake with") {
			t.Errorf("TestConnection() error = %q, want to contain \"SMTP handshake with\"", err.Error())
		}
	})

	// A TLS dial against a plaintext listener fails inside the shared
	// smtpHandshake helper, so the error carries the same wrapper prefix as
	// every other handshake failure.
	t.Run("TLS dial against a non-TLS listener fails", func(t *testing.T) {
		host, port := startAcceptAndCloseListener(t)

		svc := &SMTPService{}
		err := svc.TestConnection(&SMTPConfig{Host: host, Port: port, UseTLS: true})
		if err == nil {
			t.Fatal("TestConnection() error = nil, want TLS handshake failure")
		}
		if !strings.Contains(err.Error(), "SMTP handshake with") {
			t.Errorf("TestConnection() error = %q, want to contain \"SMTP handshake with\"", err.Error())
		}
	})
}

// TestSMTP_SendEmail_NotConfigured is the error path when no host is set;
// it must not attempt any network I/O.
func TestSMTP_SendEmail_NotConfigured(t *testing.T) {
	svc := &SMTPService{config: &SMTPConfig{}}
	err := svc.SendEmail("dest@example.com", "subject", "body")
	if err == nil {
		t.Fatal("SendEmail() error = nil, want error when Host is empty")
	}
	if !strings.Contains(err.Error(), "SMTP not configured") {
		t.Errorf("SendEmail() error = %q, want to contain \"SMTP not configured\"", err.Error())
	}
}

// TestSMTP_SendEmail_HappyPath drives a full send against a local fake SMTP
// responder bound to 127.0.0.1 - no real network I/O to any external host.
func TestSMTP_SendEmail_HappyPath(t *testing.T) {
	host, port, resultCh := startFakeSMTPResponder(t, "")

	// verified records the working handshake AI.md PART 18 requires before
	// any email is attempted; the fake responder is that live server.
	svc := &SMTPService{verified: true, config: &SMTPConfig{
		Host:        host,
		Port:        port,
		FromAddress: "sender@example.com",
		FromName:    "Test Sender",
		UseTLS:      false,
	}}

	if err := svc.SendEmail("dest@example.com", "Hello There", "<p>body text</p>"); err != nil {
		t.Fatalf("SendEmail() error = %v", err)
	}

	select {
	case result := <-resultCh:
		if !strings.Contains(result.mailFrom, "sender@example.com") {
			t.Errorf("server saw MAIL FROM %q, want to contain sender@example.com", result.mailFrom)
		}
		if len(result.rcptTo) != 1 || !strings.Contains(result.rcptTo[0], "dest@example.com") {
			t.Errorf("server saw RCPT TO %v, want to contain dest@example.com", result.rcptTo)
		}
		if !strings.Contains(result.dataBody, "Subject: Hello There") {
			t.Errorf("data body = %q, want to contain Subject header", result.dataBody)
		}
		if !strings.Contains(result.dataBody, "<p>body text</p>") {
			t.Errorf("data body = %q, want to contain message body", result.dataBody)
		}
		if !strings.Contains(result.dataBody, "Test Sender <sender@example.com>") {
			t.Errorf("data body = %q, want to contain formatted From header", result.dataBody)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake SMTP responder result")
	}
}

// TestSMTP_SendEmail_RecipientRejected covers the error path where the
// remote server rejects the recipient - SendEmail must surface the wrapped
// error rather than silently succeeding.
func TestSMTP_SendEmail_RecipientRejected(t *testing.T) {
	host, port, resultCh := startFakeSMTPResponder(t, "550 no such user")

	// verified is set because the local responder answers the real handshake;
	// the rejection under test is the RCPT reply, not a disabled server.
	svc := &SMTPService{verified: true, config: &SMTPConfig{
		Host:        host,
		Port:        port,
		FromAddress: "sender@example.com",
		UseTLS:      false,
	}}

	err := svc.SendEmail("nobody@example.com", "subject", "body")
	if err == nil {
		t.Fatal("SendEmail() error = nil, want error on recipient rejection")
	}
	if !strings.Contains(err.Error(), "failed to send email") {
		t.Errorf("SendEmail() error = %q, want to contain \"failed to send email\"", err.Error())
	}

	select {
	case <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake SMTP responder result")
	}
}

// TestSMTP_SendTestEmail confirms SendTestEmail composes a body referencing
// the current config and delegates to SendEmail, exercised end-to-end
// against the local fake responder.
func TestSMTP_SendTestEmail(t *testing.T) {
	host, port, resultCh := startFakeSMTPResponder(t, "")

	// verified is set because the local responder answers the real handshake
	// AI.md PART 18 requires before any send is attempted.
	svc := &SMTPService{verified: true, config: &SMTPConfig{
		Host:        host,
		Port:        port,
		FromAddress: "sender@example.com",
		UseTLS:      false,
	}}

	if err := svc.SendTestEmail("dest@example.com"); err != nil {
		t.Fatalf("SendTestEmail() error = %v", err)
	}

	select {
	case result := <-resultCh:
		if !strings.Contains(result.dataBody, "Subject: Weather SMTP Test") {
			t.Errorf("data body = %q, want SMTP test subject", result.dataBody)
		}
		if !strings.Contains(result.dataBody, "Host: "+host) {
			t.Errorf("data body = %q, want to reference configured host", result.dataBody)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake SMTP responder result")
	}
}

// TestSMTP_EnableChannel_Idempotent verifies calling EnableChannel twice
// upserts a single server_notification_channels row rather than creating
// duplicates, honoring the "safe to repeat" requirement for this operation.
func TestSMTP_EnableChannel_Idempotent(t *testing.T) {
	db := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, db)

	svc := &SMTPService{config: &SMTPConfig{Host: "smtp.example.com", FromAddress: "a@example.com"}}

	if err := svc.EnableChannel(); err != nil {
		t.Fatalf("EnableChannel() call 1 error = %v", err)
	}
	if err := svc.EnableChannel(); err != nil {
		t.Fatalf("EnableChannel() call 2 error = %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM server_notification_channels WHERE channel_type = 'email'").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1 (EnableChannel must upsert, not insert duplicates)", count)
	}

	var enabled int
	var state string
	if err := db.QueryRow("SELECT enabled, state FROM server_notification_channels WHERE channel_type = 'email'").Scan(&enabled, &state); err != nil {
		t.Fatalf("select query: %v", err)
	}
	if enabled != 1 || state != "enabled" {
		t.Errorf("enabled=%d state=%q, want enabled=1 state=enabled", enabled, state)
	}
}

// TestSMTP_getSetting_saveSetting_RoundTrip covers the raw DB helpers used
// throughout this file: save then get returns the same value, overwriting
// an existing key updates rather than duplicates, and a missing key returns
// sql.ErrNoRows.
func TestSMTP_getSetting_saveSetting_RoundTrip(t *testing.T) {
	db := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, db)
	svc := NewSMTPService(db)

	t.Run("missing key returns sql.ErrNoRows", func(t *testing.T) {
		_, err := svc.getSetting("smtp.does_not_exist")
		if err != sql.ErrNoRows {
			t.Errorf("getSetting() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("save then get round-trips the value", func(t *testing.T) {
		if err := svc.saveSetting("smtp.host", "first.example.com"); err != nil {
			t.Fatalf("saveSetting: %v", err)
		}
		got, err := svc.getSetting("smtp.host")
		if err != nil {
			t.Fatalf("getSetting: %v", err)
		}
		if got != "first.example.com" {
			t.Errorf("getSetting() = %q, want first.example.com", got)
		}
	})

	t.Run("saving the same key twice updates rather than duplicates", func(t *testing.T) {
		if err := svc.saveSetting("smtp.host", "second.example.com"); err != nil {
			t.Fatalf("saveSetting (update): %v", err)
		}
		got, err := svc.getSetting("smtp.host")
		if err != nil {
			t.Fatalf("getSetting: %v", err)
		}
		if got != "second.example.com" {
			t.Errorf("getSetting() = %q, want second.example.com (last write wins)", got)
		}

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM server_config WHERE key = 'smtp.host'").Scan(&count); err != nil {
			t.Fatalf("count query: %v", err)
		}
		if count != 1 {
			t.Errorf("row count = %d, want 1 (saveSetting must upsert)", count)
		}
	})
}

// TestSMTPService_UsesInjectedServerDB proves the *sql.DB passed to
// NewSMTPService is the database actually read and written, not the
// process-global one. The global is deliberately wired to a DIFFERENT server
// database holding a contradicting smtp.host: if the injected handle were
// ignored again, LoadConfig would return the decoy's host and EnableChannel
// would write the decoy's server_notification_channels.
func TestSMTPService_UsesInjectedServerDB(t *testing.T) {
	injected := setupSMTPServerDB(t)
	decoy := openDecoyServerDB(t)
	wireSMTPGlobalDB(t, decoy)

	if _, err := injected.Exec(`INSERT INTO server_config (key, value) VALUES ('smtp.host', 'injected.example')`); err != nil {
		t.Fatalf("seed injected server_config: %v", err)
	}
	if _, err := decoy.Exec(`INSERT INTO server_config (key, value) VALUES ('smtp.host', 'decoy.example')`); err != nil {
		t.Fatalf("seed decoy server_config: %v", err)
	}

	svc := NewSMTPService(injected)
	if err := svc.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := svc.GetConfig().Host; got != "injected.example" {
		t.Errorf("expected host from the injected database, got %q (decoy.example means the global was read)", got)
	}

	if err := svc.EnableChannel(); err != nil {
		t.Fatalf("EnableChannel: %v", err)
	}

	var injectedRows, decoyRows int
	if err := injected.QueryRow(`SELECT COUNT(*) FROM server_notification_channels`).Scan(&injectedRows); err != nil {
		t.Fatalf("count injected server_notification_channels: %v", err)
	}
	if err := decoy.QueryRow(`SELECT COUNT(*) FROM server_notification_channels`).Scan(&decoyRows); err != nil {
		t.Fatalf("count decoy server_notification_channels: %v", err)
	}
	if injectedRows != 1 {
		t.Errorf("expected 1 row in the injected server_notification_channels, got %d", injectedRows)
	}
	if decoyRows != 0 {
		t.Errorf("expected 0 rows in the global (decoy) server_notification_channels, got %d", decoyRows)
	}
}

// TestSMTPService_NilInjectedDBFallsBackToGlobal documents the nil-handle
// fallback so callers constructing the service before the dual DB is wired
// keep the previous behavior instead of panicking.
func TestSMTPService_NilInjectedDBFallsBackToGlobal(t *testing.T) {
	serverDB := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, serverDB)

	if _, err := serverDB.Exec(`INSERT INTO server_config (key, value) VALUES ('smtp.host', 'global.example')`); err != nil {
		t.Fatalf("seed server_config: %v", err)
	}

	svc := NewSMTPService(nil)
	if err := svc.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig with nil injected handle: %v", err)
	}
	if got := svc.GetConfig().Host; got != "global.example" {
		t.Errorf("expected the nil handle to fall back to the global server db, got %q", got)
	}
}
