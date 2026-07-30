package service

import (
	"database/sql"
	"testing"
)

// seedEmailChannelSMTPConfig inserts host/from_address rows into
// server_config so SMTPService.IsEnabled() reports true without ever
// touching the network.
func seedEmailChannelSMTPConfig(t *testing.T, db *sql.DB, host, fromAddress string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO server_config (key, value) VALUES (?, ?)`, "smtp.host", host)
	if err != nil {
		t.Fatalf("seed smtp.host: %v", err)
	}
	_, err = db.Exec(`INSERT INTO server_config (key, value) VALUES (?, ?)`, "smtp.from_address", fromAddress)
	if err != nil {
		t.Fatalf("seed smtp.from_address: %v", err)
	}
}

// TestEmailChannel_NewEmailChannel_NilSMTP covers the nil-dependency boundary:
// constructing a channel with a nil SMTP service must not panic and must
// leave the channel disabled.
func TestEmailChannel_NewEmailChannel_NilSMTP(t *testing.T) {
	ch := NewEmailChannel(nil)
	if ch == nil {
		t.Fatal("NewEmailChannel(nil) returned nil")
	}
	if ch.IsEnabled() {
		t.Error("expected channel to be disabled when smtp is nil")
	}
}

// TestEmailChannel_GetType_GetName covers the two trivial constant-return
// accessors.
func TestEmailChannel_GetType_GetName(t *testing.T) {
	ch := NewEmailChannel(nil)
	if got := ch.GetType(); got != "email" {
		t.Errorf("GetType() = %q, want %q", got, "email")
	}
	if got := ch.GetName(); got != "Email (SMTP)" {
		t.Errorf("GetName() = %q, want %q", got, "Email (SMTP)")
	}
}

// TestEmailChannel_IsEnabled_DisabledSMTP verifies that a non-nil SMTP
// service which is not configured (no host/from_address) is treated as
// disabled, without hitting the network.
func TestEmailChannel_IsEnabled_DisabledSMTP(t *testing.T) {
	serverDB := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, serverDB)
	smtp := NewSMTPService(serverDB)
	ch := NewEmailChannel(smtp)
	if ch.IsEnabled() {
		t.Error("expected channel to be disabled with unconfigured SMTP")
	}
}

// TestEmailChannel_IsEnabled_EnabledSMTP verifies the happy path: once the
// server_config table has host + from_address populated, the channel
// reports enabled.
func TestEmailChannel_IsEnabled_EnabledSMTP(t *testing.T) {
	serverDB := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, serverDB)
	seedEmailChannelSMTPConfig(t, serverDB, "smtp.example.com", "noreply@example.com")

	smtp := NewSMTPService(serverDB)
	ch := NewEmailChannel(smtp)
	if !ch.IsEnabled() {
		t.Error("expected channel to be enabled with configured SMTP")
	}
}

// TestEmailChannel_Send_NotEnabled ensures Send fails fast with a clear
// error and never attempts to reach the network when the channel is
// disabled (nil smtp, no config).
func TestEmailChannel_Send_NotEnabled(t *testing.T) {
	ch := NewEmailChannel(nil)
	err := ch.Send("user@example.com", "subject", "body", nil)
	if err == nil {
		t.Fatal("expected error when channel is not enabled, got nil")
	}
	const want = "email channel not enabled: SMTP not configured"
	if err.Error() != want {
		t.Errorf("Send() error = %q, want %q", err.Error(), want)
	}
}

// TestEmailChannel_Test_NilSMTP covers the guard clause in Test() that
// short-circuits before touching a nil smtp field.
func TestEmailChannel_Test_NilSMTP(t *testing.T) {
	ch := NewEmailChannel(nil)
	err := ch.Test("user@example.com")
	if err == nil {
		t.Fatal("expected error for nil SMTP service, got nil")
	}
	const want = "SMTP service not initialized"
	if err.Error() != want {
		t.Errorf("Test() error = %q, want %q", err.Error(), want)
	}
}

// TestEmailChannel_ValidateConfig is table-driven over the pure validation
// logic: required fields present/missing, and the port type-assertion
// branch (int/int64/float64 accepted, everything else rejected).
func TestEmailChannel_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config with int port",
			config: map[string]interface{}{
				"host":         "smtp.example.com",
				"port":         587,
				"from_address": "noreply@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid config with int64 port",
			config: map[string]interface{}{
				"host":         "smtp.example.com",
				"port":         int64(587),
				"from_address": "noreply@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid config with float64 port",
			config: map[string]interface{}{
				"host":         "smtp.example.com",
				"port":         float64(587),
				"from_address": "noreply@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing port key is a required-field error",
			config: map[string]interface{}{
				"host":         "smtp.example.com",
				"from_address": "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name:    "empty config missing all required fields",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "missing host",
			config: map[string]interface{}{
				"port":         587,
				"from_address": "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing both port and from_address",
			config: map[string]interface{}{
				"host": "smtp.example.com",
			},
			wantErr: true,
		},
		{
			name: "port is a string",
			config: map[string]interface{}{
				"host":         "smtp.example.com",
				"port":         "587",
				"from_address": "noreply@example.com",
			},
			wantErr: true,
		},
		{
			name:    "nil config map",
			config:  nil,
			wantErr: true,
		},
	}

	ch := NewEmailChannel(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ch.ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig(%v) error = %v, wantErr %v", tt.config, err, tt.wantErr)
			}
		})
	}
}

// TestEmailChannel_Refresh_NilSMTP verifies Refresh is a safe no-op when
// smtp is nil (guard clause), and idempotent when called repeatedly.
func TestEmailChannel_Refresh_NilSMTP(t *testing.T) {
	ch := NewEmailChannel(nil)
	ch.Refresh()
	ch.Refresh()
	if ch.IsEnabled() {
		t.Error("expected channel to remain disabled after Refresh with nil smtp")
	}
}

// TestEmailChannel_Refresh_ReflectsConfigChange verifies Refresh re-checks
// the underlying SMTP config, so a channel created before SMTP was
// configured picks up the change afterward (config hot-reload path).
func TestEmailChannel_Refresh_ReflectsConfigChange(t *testing.T) {
	serverDB := setupSMTPServerDB(t)
	wireSMTPGlobalDB(t, serverDB)
	smtp := NewSMTPService(serverDB)
	ch := NewEmailChannel(smtp)
	if ch.IsEnabled() {
		t.Fatal("expected channel to start disabled")
	}

	seedEmailChannelSMTPConfig(t, serverDB, "smtp.example.com", "noreply@example.com")
	// smtp.config is cached from the first IsEnabled() call above, so force
	// a reload the same way LoadConfig would be re-invoked in production.
	if err := smtp.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ch.Refresh()

	if !ch.IsEnabled() {
		t.Error("expected channel to be enabled after Refresh following config change")
	}

	// Idempotency: calling Refresh again with no config change keeps state stable.
	ch.Refresh()
	if !ch.IsEnabled() {
		t.Error("expected channel to remain enabled after second Refresh")
	}
}
