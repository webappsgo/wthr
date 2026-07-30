package graphql

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/server/handler"
	models "github.com/webappsgo/wthr/src/server/model"
)

// --- Pure field-resolver tests -------------------------------------------
// These resolvers only read fields off the passed-in model object; they
// require no database or handler and are cheap to exercise directly.

func TestAPITokenResolver_ExpiresAt(t *testing.T) {
	res := &aPITokenResolver{&Resolver{}}
	now := time.Now()

	tests := []struct {
		name string
		obj  *models.APIToken
		want *time.Time
	}{
		{
			name: "valid expiry returns time",
			obj:  &models.APIToken{ExpiresAt: sql.NullTime{Time: now, Valid: true}},
			want: &now,
		},
		{
			name: "invalid expiry returns nil",
			obj:  &models.APIToken{ExpiresAt: sql.NullTime{Time: now, Valid: false}},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := res.ExpiresAt(context.Background(), tt.obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil || !got.Equal(*tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPITokenResolver_LastUsedIP(t *testing.T) {
	res := &aPITokenResolver{&Resolver{}}

	tests := []struct {
		name string
		obj  *models.APIToken
		want *string
	}{
		{
			name: "valid IP returned",
			obj:  &models.APIToken{LastUsedIP: sql.NullString{String: "1.2.3.4", Valid: true}},
			want: stringPtr("1.2.3.4"),
		},
		{
			name: "invalid IP returns nil",
			obj:  &models.APIToken{LastUsedIP: sql.NullString{String: "", Valid: false}},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := res.LastUsedIP(context.Background(), tt.obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNotificationResolver_Type(t *testing.T) {
	res := &notificationResolver{&Resolver{}}

	tests := []struct {
		name string
		obj  *models.Notification
		want string
	}{
		{"nil obj returns empty string", nil, ""},
		{"success type stringified", &models.Notification{Type: models.NotificationTypeSuccess}, "success"},
		{"warning type stringified", &models.Notification{Type: models.NotificationTypeWarning}, "warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := res.Type(context.Background(), tt.obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSavedLocationResolver_FieldAccessors(t *testing.T) {
	res := &savedLocationResolver{&Resolver{}}
	obj := &models.SavedLocation{
		Latitude:      41.5,
		Longitude:     -81.6,
		AlertsEnabled: true,
	}

	t.Run("Lat returns latitude", func(t *testing.T) {
		got, err := res.Lat(context.Background(), obj)
		if err != nil || got != obj.Latitude {
			t.Fatalf("got (%v, %v), want (%v, nil)", got, err, obj.Latitude)
		}
	})

	t.Run("Lon returns longitude", func(t *testing.T) {
		got, err := res.Lon(context.Background(), obj)
		if err != nil || got != obj.Longitude {
			t.Fatalf("got (%v, %v), want (%v, nil)", got, err, obj.Longitude)
		}
	})

	t.Run("Country returns empty string pointer", func(t *testing.T) {
		got, err := res.Country(context.Background(), obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || *got != "" {
			t.Fatalf("got %v, want pointer to empty string", got)
		}
	})

	t.Run("Region returns empty string pointer", func(t *testing.T) {
		got, err := res.Region(context.Background(), obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || *got != "" {
			t.Fatalf("got %v, want pointer to empty string", got)
		}
	})

	t.Run("Alerts returns AlertsEnabled", func(t *testing.T) {
		got, err := res.Alerts(context.Background(), obj)
		if err != nil || got != obj.AlertsEnabled {
			t.Fatalf("got (%v, %v), want (%v, nil)", got, err, obj.AlertsEnabled)
		}
	})
}

func TestSettingResolver_UpdatedBy(t *testing.T) {
	res := &settingResolver{&Resolver{}}
	got, err := res.UpdatedBy(context.Background(), &models.Setting{Key: "foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil (Setting has no UpdatedBy column)", got)
	}
}

// --- Resolver type-accessor tests -----------------------------------------
// The one-line wrapper accessors at the bottom of schema.resolvers.go; each
// must return a non-nil implementation wired back to the same *Resolver.

func TestResolverTypeAccessors(t *testing.T) {
	r := &Resolver{}

	if got := r.APIToken(); got == nil {
		t.Fatal("APIToken() returned nil")
	}
	if got := r.GenericResponse(); got == nil {
		t.Fatal("GenericResponse() returned nil")
	}
	if got := r.Mutation(); got == nil {
		t.Fatal("Mutation() returned nil")
	}
	if got := r.Notification(); got == nil {
		t.Fatal("Notification() returned nil")
	}
	if got := r.Query(); got == nil {
		t.Fatal("Query() returned nil")
	}
	if got := r.SavedLocation(); got == nil {
		t.Fatal("SavedLocation() returned nil")
	}
	if got := r.Setting(); got == nil {
		t.Fatal("Setting() returned nil")
	}
}

// --- Mutation guard tests --------------------------------------------------
// getUserIDFromContext(ctx) always returns 0 for a bare context.Background()
// because graphql.go stores the user ID under a typed contextKey while
// these resolvers read it back with the raw string "user_id" (see
// context_keys_test.go for the documented mismatch). That makes the
// "unauthorized" branch of every one of these mutations deterministically
// reachable without any handler/DB wiring.

func TestMutationResolver_UserAuthGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"UpdateUserProfile", func() error {
			_, err := m.UpdateUserProfile(ctx, nil, nil)
			return err
		}},
		{"UpdateUserAvatar", func() error {
			_, err := m.UpdateUserAvatar(ctx, "gravatar", nil)
			return err
		}},
		{"ResetUserAvatar", func() error {
			_, err := m.ResetUserAvatar(ctx)
			return err
		}},
		{"ChangeUserPassword", func() error {
			_, err := m.ChangeUserPassword(ctx, "old", "new")
			return err
		}},
		{"UpdateUserSettings", func() error {
			_, err := m.UpdateUserSettings(ctx, nil, nil, nil, nil)
			return err
		}},
		{"CreateUserToken", func() error {
			_, err := m.CreateUserToken(ctx, "token", nil, nil)
			return err
		}},
		{"RevokeUserToken", func() error {
			_, err := m.RevokeUserToken(ctx, "1")
			return err
		}},
		{"CreateSavedLocation", func() error {
			_, err := m.CreateSavedLocation(ctx, "home", 0, 0, nil, nil, nil)
			return err
		}},
		{"UpdateSavedLocation", func() error {
			_, err := m.UpdateSavedLocation(ctx, "1", nil, nil)
			return err
		}},
		{"DeleteSavedLocation", func() error {
			_, err := m.DeleteSavedLocation(ctx, "1")
			return err
		}},
		{"ToggleLocationAlerts", func() error {
			_, err := m.ToggleLocationAlerts(ctx, "1")
			return err
		}},
		{"MarkNotificationRead", func() error {
			_, err := m.MarkNotificationRead(ctx, "1")
			return err
		}},
		{"MarkAllNotificationsRead", func() error {
			_, err := m.MarkAllNotificationsRead(ctx)
			return err
		}},
		{"DeleteNotification", func() error {
			_, err := m.DeleteNotification(ctx, "1")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected unauthorized error, got nil")
			}
			if !strings.Contains(err.Error(), "unauthorized") {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), "unauthorized")
			}
		})
	}
}

func TestMutationResolver_AdminRoleGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"AdminUpdateUser", func() error {
			_, err := m.AdminUpdateUser(ctx, "1", nil, nil, nil)
			return err
		}},
		{"AdminDeleteUser", func() error {
			_, err := m.AdminDeleteUser(ctx, "1")
			return err
		}},
		{"AdminCreateUserInvite", func() error {
			_, err := m.AdminCreateUserInvite(ctx, "bob", "bob@example.test", nil, nil)
			return err
		}},
		{"AdminDeleteUserInvite", func() error {
			_, err := m.AdminDeleteUserInvite(ctx, "1")
			return err
		}},
		{"AdminInviteServerAdmin", func() error {
			_, err := m.AdminInviteServerAdmin(ctx, "admin@example.test", nil)
			return err
		}},
		{"AdminDeleteServerAdmin", func() error {
			_, err := m.AdminDeleteServerAdmin(ctx, "1")
			return err
		}},
		{"AdminDisableServerAdmin", func() error {
			_, err := m.AdminDisableServerAdmin(ctx, "1")
			return err
		}},
		{"AdminEnableServerAdmin", func() error {
			_, err := m.AdminEnableServerAdmin(ctx, "1")
			return err
		}},
		{"AdminUpdateSetting", func() error {
			_, err := m.AdminUpdateSetting(ctx, "key", "value")
			return err
		}},
		{"AdminUpdateSettings", func() error {
			_, err := m.AdminUpdateSettings(ctx, nil)
			return err
		}},
		{"AdminResetSettings", func() error {
			_, err := m.AdminResetSettings(ctx)
			return err
		}},
		{"AdminGenerateToken", func() error {
			_, err := m.AdminGenerateToken(ctx)
			return err
		}},
		{"AdminRevokeToken", func() error {
			_, err := m.AdminRevokeToken(ctx, "1")
			return err
		}},
		{"AdminClearAuditLogs", func() error {
			_, err := m.AdminClearAuditLogs(ctx)
			return err
		}},
		{"AdminUpdateTask", func() error {
			_, err := m.AdminUpdateTask(ctx, "task", true)
			return err
		}},
		{"AdminEnableTask", func() error {
			_, err := m.AdminEnableTask(ctx, "task")
			return err
		}},
		{"AdminDisableTask", func() error {
			_, err := m.AdminDisableTask(ctx, "task")
			return err
		}},
		{"AdminTriggerTask", func() error {
			_, err := m.AdminTriggerTask(ctx, "task")
			return err
		}},
		{"AdminUpdateChannel", func() error {
			_, err := m.AdminUpdateChannel(ctx, "email", nil, nil)
			return err
		}},
		{"AdminEnableChannel", func() error {
			_, err := m.AdminEnableChannel(ctx, "email")
			return err
		}},
		{"AdminDisableChannel", func() error {
			_, err := m.AdminDisableChannel(ctx, "email")
			return err
		}},
		{"AdminTestChannel", func() error {
			_, err := m.AdminTestChannel(ctx, "email", nil)
			return err
		}},
		{"AdminInitializeChannels", func() error {
			_, err := m.AdminInitializeChannels(ctx)
			return err
		}},
		{"AdminAutoDetectSMTP", func() error {
			_, err := m.AdminAutoDetectSMTP(ctx)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected admin-access error, got nil")
			}
			if !strings.Contains(err.Error(), "unauthorized") {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), "unauthorized")
			}
		})
	}
}

func TestMutationResolver_SessionGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	t.Run("LogoutUser requires a session", func(t *testing.T) {
		_, err := m.LogoutUser(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("RefreshUserSession requires a session", func(t *testing.T) {
		_, err := m.RefreshUserSession(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestMutationResolver_UserAuthLoadGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"EnableUserTwoFactor", func() error {
			_, err := m.EnableUserTwoFactor(ctx, "secret", "000000")
			return err
		}},
		{"DisableUserTwoFactor", func() error {
			_, err := m.DisableUserTwoFactor(ctx, "password")
			return err
		}},
		{"VerifyUserTwoFactor", func() error {
			_, err := m.VerifyUserTwoFactor(ctx, "000000")
			return err
		}},
		{"RegenerateUserRecoveryKeys", func() error {
			_, err := m.RegenerateUserRecoveryKeys(ctx, "000000")
			return err
		}},
		{"BeginUserPasskeyRegistration", func() error {
			_, err := m.BeginUserPasskeyRegistration(ctx, "device", "password")
			return err
		}},
		{"FinishUserPasskeyRegistration", func() error {
			_, err := m.FinishUserPasskeyRegistration(ctx, "token", nil)
			return err
		}},
		{"DeleteUserPasskey", func() error {
			_, err := m.DeleteUserPasskey(ctx, "1")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected error from unauthenticated user load, got nil")
			}
		})
	}
}

func TestMutationResolver_AdminLoadGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"BeginAdminPasskeyRegistration", func() error {
			_, err := m.BeginAdminPasskeyRegistration(ctx, "device", "password")
			return err
		}},
		{"FinishAdminPasskeyRegistration", func() error {
			_, err := m.FinishAdminPasskeyRegistration(ctx, "token", nil)
			return err
		}},
		{"DeleteAdminPasskey", func() error {
			_, err := m.DeleteAdminPasskey(ctx, "1")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected error from unauthenticated admin load, got nil")
			}
		})
	}
}

// graphQLPasskeyEnvelope(ctx) requires "request_host" in the context; a bare
// context.Background() always fails that check before any handler/DB call.

func TestMutationResolver_PasskeyChallengeEnvelopeGuards(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"BeginUserPasskeyChallenge", func() error {
			_, err := m.BeginUserPasskeyChallenge(ctx, nil)
			return err
		}},
		{"FinishUserPasskeyChallenge", func() error {
			_, err := m.FinishUserPasskeyChallenge(ctx, "token", nil)
			return err
		}},
		{"BeginAdminPasskeyChallenge", func() error {
			_, err := m.BeginAdminPasskeyChallenge(ctx, "token")
			return err
		}},
		{"FinishAdminPasskeyChallenge", func() error {
			_, err := m.FinishAdminPasskeyChallenge(ctx, "token", nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected 'missing request host' error, got nil")
			}
			if !strings.Contains(err.Error(), "request host") {
				t.Fatalf("error = %q, want it to mention the missing request host", err.Error())
			}
		})
	}
}

func TestMutationResolver_SubmitContactForm_NilServerDB(t *testing.T) {
	m := &mutationResolver{&Resolver{}}
	_, err := m.SubmitContactForm(context.Background(), "Jane", "jane@example.test", "hi", "hello there")
	if err == nil {
		t.Fatal("expected error with nil ServerDB, got nil")
	}
	if !strings.Contains(err.Error(), "server database unavailable") {
		t.Fatalf("error = %q, want it to mention server database unavailable", err.Error())
	}
}

// --- Query guard tests -----------------------------------------------------

func TestQueryResolver_UserAuthGuards(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"CurrentUserAvatar", func() error { _, err := q.CurrentUserAvatar(ctx); return err }},
		{"UserSettings", func() error { _, err := q.UserSettings(ctx); return err }},
		{"UserTokens", func() error { _, err := q.UserTokens(ctx); return err }},
		{"SavedLocations", func() error { _, err := q.SavedLocations(ctx); return err }},
		{"SavedLocation", func() error { _, err := q.SavedLocation(ctx, "1"); return err }},
		{"Notifications", func() error { _, err := q.Notifications(ctx); return err }},
		{"UnreadNotifications", func() error { _, err := q.UnreadNotifications(ctx); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected unauthorized error, got nil")
			}
			if !strings.Contains(err.Error(), "unauthorized") {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), "unauthorized")
			}
		})
	}
}

func TestQueryResolver_UserAuthLoadGuards(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"CurrentUser", func() error { _, err := q.CurrentUser(ctx); return err }},
		{"CurrentUserTwoFactorStatus", func() error { _, err := q.CurrentUserTwoFactorStatus(ctx); return err }},
		{"CurrentUserTwoFactorSetup", func() error { _, err := q.CurrentUserTwoFactorSetup(ctx); return err }},
		{"CurrentUserPasskeys", func() error { _, err := q.CurrentUserPasskeys(ctx); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected error from unauthenticated user load, got nil")
			}
		})
	}
}

func TestQueryResolver_AdminRoleGuards(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"AdminUsers", func() error { _, err := q.AdminUsers(ctx); return err }},
		{"AdminServerAdmins", func() error { _, err := q.AdminServerAdmins(ctx); return err }},
		{"AdminServerAdmin", func() error { _, err := q.AdminServerAdmin(ctx, "1"); return err }},
		{"AdminUserInvites", func() error { _, err := q.AdminUserInvites(ctx); return err }},
		{"AdminUserInvite", func() error { _, err := q.AdminUserInvite(ctx, "1"); return err }},
		{"AdminSettings", func() error { _, err := q.AdminSettings(ctx); return err }},
		{"AdminSetting", func() error { _, err := q.AdminSetting(ctx, "key"); return err }},
		{"AdminTokens", func() error { _, err := q.AdminTokens(ctx); return err }},
		{"AdminAuditLogs", func() error { _, err := q.AdminAuditLogs(ctx, nil, nil); return err }},
		{"AdminStats", func() error { _, err := q.AdminStats(ctx); return err }},
		{"AdminTasks", func() error { _, err := q.AdminTasks(ctx); return err }},
		{"AdminTaskHistory", func() error { _, err := q.AdminTaskHistory(ctx, "task", nil); return err }},
		{"AdminChannels", func() error { _, err := q.AdminChannels(ctx); return err }},
		{"AdminChannel", func() error { _, err := q.AdminChannel(ctx, "email"); return err }},
		{"AdminChannelStats", func() error { _, err := q.AdminChannelStats(ctx, "email"); return err }},
		{"AdminQueueStats", func() error { _, err := q.AdminQueueStats(ctx); return err }},
		{"AdminSMTPProviders", func() error { _, err := q.AdminSMTPProviders(ctx); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected admin-access error, got nil")
			}
			if !strings.Contains(err.Error(), "unauthorized") {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), "unauthorized")
			}
		})
	}
}

func TestQueryResolver_AdminPasskeys_NoCurrentAdmin(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	if _, err := q.AdminPasskeys(context.Background()); err == nil {
		t.Fatal("expected error resolving current admin, got nil")
	}
}

func TestQueryResolver_WeatherServiceGuards(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"Weather", func() error { _, err := q.Weather(ctx, nil, nil, nil); return err }},
		{"Forecast", func() error { _, err := q.Forecast(ctx, nil, nil, nil, nil); return err }},
		{"SearchLocations", func() error { _, err := q.SearchLocations(ctx, "cleveland"); return err }},
		{"IPGeolocation", func() error { _, err := q.IPGeolocation(ctx, nil); return err }},
		{"CurrentLocation", func() error { _, err := q.CurrentLocation(ctx); return err }},
		{"HistoricalWeather", func() error { _, err := q.HistoricalWeather(ctx, "cleveland", "2024-01-01"); return err }},
		{"LookupZipCode", func() error { _, err := q.LookupZipCode(ctx, "44101"); return err }},
		{"LookupCoordinates", func() error { _, err := q.LookupCoordinates(ctx, 41.5, -81.6); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected weather-service-not-initialized error, got nil")
			}
			if !strings.Contains(err.Error(), "not initialized") {
				t.Fatalf("error = %q, want it to mention the service is not initialized", err.Error())
			}
		})
	}
}

func TestQueryResolver_HandlerNilGuards(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"Earthquakes", func() error { _, err := q.Earthquakes(ctx, nil, nil); return err }},
		{"Hurricanes", func() error { _, err := q.Hurricanes(ctx, nil); return err }},
		{"SevereWeather", func() error { _, err := q.SevereWeather(ctx, nil); return err }},
		{"MoonPhase", func() error { _, err := q.MoonPhase(ctx, nil); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected service-not-initialized error, got nil")
			}
			if !strings.Contains(err.Error(), "not initialized") {
				t.Fatalf("error = %q, want it to mention the service is not initialized", err.Error())
			}
		})
	}
}

func TestQueryResolver_ValidateInvite_EmptyToken(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	t.Run("ValidateUserInvite rejects empty token before touching the DB", func(t *testing.T) {
		_, err := q.ValidateUserInvite(ctx, "")
		if err == nil {
			t.Fatal("expected 'token required' error, got nil")
		}
		if !strings.Contains(err.Error(), "token required") {
			t.Fatalf("error = %q, want it to mention token required", err.Error())
		}
	})

	t.Run("ValidateServerInvite rejects empty token before touching global state", func(t *testing.T) {
		_, err := q.ValidateServerInvite(ctx, "")
		if err == nil {
			t.Fatal("expected 'token required' error, got nil")
		}
		if !strings.Contains(err.Error(), "token required") {
			t.Fatalf("error = %q, want it to mention token required", err.Error())
		}
	})
}

// Health has no auth guard and must tolerate a nil ServerDB (it only pings
// the DB when ServerDB != nil). It reads package-level init state from the
// handler package, so pin that state first for a deterministic assertion.
func TestQueryResolver_Health(t *testing.T) {
	handler.SetInitStatus(true, true, true)
	t.Cleanup(func() { handler.SetInitStatus(false, false, false) })

	q := &queryResolver{&Resolver{}}
	got, err := q.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Health() returned nil status")
	}
	if got.Status != "healthy" {
		t.Fatalf("Status = %q, want %q (ServerDB is nil so the DB check is skipped and init state is fully ready)", got.Status, "healthy")
	}
	if got.Database != nil {
		t.Fatalf("Database = %+v, want nil because ServerDB is nil", got.Database)
	}
	if got.Memory == nil {
		t.Fatal("Memory stats should never be nil")
	}
	if got.LocationDatabases == nil || got.LocationDatabases.Countries == nil || !got.LocationDatabases.Countries.Loaded {
		t.Fatalf("LocationDatabases.Countries.Loaded = false, want true")
	}
}

func TestQueryResolver_Health_Initializing(t *testing.T) {
	handler.SetInitStatus(false, false, false)
	t.Cleanup(func() { handler.SetInitStatus(false, false, false) })

	q := &queryResolver{&Resolver{}}
	got, err := q.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "initializing" {
		t.Fatalf("Status = %q, want %q", got.Status, "initializing")
	}
}
