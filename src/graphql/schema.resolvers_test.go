package graphql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
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

// Fixed, non-local zones for timestamp fixtures. -11h renders wall-clock text
// that reads earlier than the true instant and +13h later, so a text-ordering
// comparison reaches the opposite conclusion from an instant comparison no
// matter what timezone the test host runs in.
// The zone abbreviations must be three to five upper-case letters ending in T:
// that is the only shape time.Parse accepts for the trailing "MST" element of
// the local layout below, so a name like "EST13" would make every fixture
// written in that zone unparseable and the tests would assert nothing.
var (
	graphqlZoneWest = time.FixedZone("WST", -11*60*60)
	graphqlZoneEast = time.FixedZone("EAST", 13*60*60)
)

// graphqlLocalLayout is the layout modernc.org/sqlite writes when a time.Time
// is bound directly, i.e. what time.Time.String() produces.
const graphqlLocalLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

// TestQueryResolver_AdminStats_ActiveUsersCountedByInstant is the regression
// test for the 30-day active-user cutoff, which used to read
// "last_login_at >= datetime('now', '-30 days')". SQLite's datetime() returns
// NULL for the local-zone layout above, so a user who logged in an hour ago
// never counted as active. The three fixtures below contradict text ordering
// and cover the fail-closed case, so this test fails against the old query.
func TestQueryResolver_AdminStats_ActiveUsersCountedByInstant(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}
	now := time.Now()

	fixtures := []struct {
		username    string
		email       string
		lastLoginAt string
	}{
		{"statsactive", "statsactive@example.com", now.Add(-time.Hour).In(graphqlZoneWest).Format(graphqlLocalLayout)},
		{"statsstale", "statsstale@example.com", now.AddDate(0, 0, -60).In(graphqlZoneEast).Format(graphqlLocalLayout)},
		{"statsunparseable", "statsunparseable@example.com", "not-a-timestamp"},
	}
	for _, fixture := range fixtures {
		user := seedGraphQLUser(t, ddb, fixture.username, fixture.email, "correctpass1")
		if _, err := ddb.Users.Exec(`
			UPDATE user_accounts SET last_login_at = ? WHERE id = ?
		`, fixture.lastLoginAt, user.ID); err != nil {
			t.Fatalf("seed last_login_at for %s: %v", fixture.username, err)
		}
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	got, err := q.AdminStats(adminCtx)
	if err != nil {
		t.Fatalf("AdminStats() error = %v", err)
	}
	if got == nil || got.Users == nil {
		t.Fatalf("got = %+v, want populated user stats", got)
	}
	if got.Users.Total != len(fixtures) {
		t.Errorf("Users.Total = %d, want %d", got.Users.Total, len(fixtures))
	}
	if got.Users.Active == nil {
		t.Fatalf("Users.Active = nil, want a populated active-user count")
	}
	if *got.Users.Active != 1 {
		t.Errorf("Users.Active = %d, want 1 (only the account whose true last login is inside 30 days)", *got.Users.Active)
	}
}

// --- Newest-first ordering regressions -----------------------------------
//
// Every test below seeds the same contradiction: the row with the newest true
// instant is stored in the legacy local-zone layout of a zone 11 hours BEHIND
// UTC, so its text reads earlier than a genuinely older row stored in a zone 13
// hours AHEAD. A third row holds text no layout can parse; in an SQL text
// ordering it sorts above both real timestamps (letters outrank digits in
// ASCII) while its true instant is unknown. Under the "ORDER BY created_at
// DESC" these queries used to run, the returned order is therefore the reverse
// of the truth, and where a LIMIT was stacked on top of it the true-newest row
// was cut from the page entirely.

// graphqlOrderingFixture is one seeded row: the name it is identified by in the
// assertions, and the exact text planted in its timestamp column.
type graphqlOrderingFixture struct {
	name      string
	timestamp string
}

// graphqlOrderingFixtures returns the three contradicting rows described above,
// listed in the insertion order the fixtures use, together with the order the
// resolver must return them in once each value is resolved to a real instant.
func graphqlOrderingFixtures(now time.Time) ([]graphqlOrderingFixture, []string) {
	fixtures := []graphqlOrderingFixture{
		{"east-older", now.Add(-2 * time.Hour).In(graphqlZoneEast).Format(graphqlLocalLayout)},
		{"west-newest", now.In(graphqlZoneWest).Format(graphqlLocalLayout)},
		{"unparseable", "not-a-timestamp"},
	}
	want := []string{"west-newest", "east-older", "unparseable"}

	return fixtures, want
}

// assertGraphQLOrder compares the names a resolver returned against the wanted
// order, reporting the whole sequence so a wrong ordering is readable at once.
func assertGraphQLOrder(t *testing.T, query string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s returned %d rows %v, want %d rows %v", query, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s order = %v, want %v", query, got, want)
		}
	}
}

func TestQueryResolver_UserTokens_OrderedByInstant(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	user := seedGraphQLUser(t, ddb, "tokenorder", "tokenorder@example.com", "correctpass1")
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	fixtures, want := graphqlOrderingFixtures(time.Now().UTC().Truncate(time.Second))
	for i, fixture := range fixtures {
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_tokens (user_id, token_hash, token_prefix, name, scopes, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, user.ID, fmt.Sprintf("token-hash-%d", i), fmt.Sprintf("usr_%04d", i), fixture.name, "read", fixture.timestamp); err != nil {
			t.Fatalf("seed token %s: %v", fixture.name, err)
		}
	}

	got, err := q.UserTokens(withGraphQLUserContext(context.Background(), user))
	if err != nil {
		t.Fatalf("UserTokens() error = %v", err)
	}

	names := make([]string, 0, len(got))
	for _, token := range got {
		if token.Name == nil {
			t.Fatalf("token %s has no name, fixtures always set one", token.ID)
		}
		names = append(names, *token.Name)
	}
	assertGraphQLOrder(t, "UserTokens", names, want)
}

func TestQueryResolver_SavedLocations_OrderedByInstant(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	user := seedGraphQLUser(t, ddb, "locationorder", "locationorder@example.com", "correctpass1")
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	fixtures, want := graphqlOrderingFixtures(time.Now().UTC().Truncate(time.Second))
	for _, fixture := range fixtures {
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_saved_locations (user_id, name, latitude, longitude, timezone, alerts_enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, user.ID, fixture.name, 41.5, -81.6, "UTC", 1, fixture.timestamp, fixture.timestamp); err != nil {
			t.Fatalf("seed location %s: %v", fixture.name, err)
		}
	}

	got, err := q.SavedLocations(withGraphQLUserContext(context.Background(), user))
	if err != nil {
		t.Fatalf("SavedLocations() error = %v", err)
	}

	names := make([]string, 0, len(got))
	for _, loc := range got {
		names = append(names, loc.Name)
	}
	assertGraphQLOrder(t, "SavedLocations", names, want)
}

func TestQueryResolver_AdminUsers_OrderedByInstant(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	fixtures, want := graphqlOrderingFixtures(time.Now().UTC().Truncate(time.Second))
	for _, fixture := range fixtures {
		user := seedGraphQLUser(t, ddb, fixture.name, fixture.name+"@example.com", "correctpass1")
		if _, err := ddb.Users.Exec(`
			UPDATE user_accounts SET created_at = ? WHERE id = ?
		`, fixture.timestamp, user.ID); err != nil {
			t.Fatalf("seed created_at for %s: %v", fixture.name, err)
		}
	}

	got, err := q.AdminUsers(withGraphQLAdminValues(context.Background(), 1, "admin@example.com"))
	if err != nil {
		t.Fatalf("AdminUsers() error = %v", err)
	}

	names := make([]string, 0, len(got))
	for _, user := range got {
		names = append(names, user.Username)
	}
	assertGraphQLOrder(t, "AdminUsers", names, want)
}

// TestQueryResolver_AdminAuditLogs_OrderedByInstantAndPaged also pins the page
// boundary: with limit 1 the single entry served must be the true newest, which
// under the old text ordering was the entry sorted last of the three.
func TestQueryResolver_AdminAuditLogs_OrderedByInstantAndPaged(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	fixtures, want := graphqlOrderingFixtures(time.Now().UTC().Truncate(time.Second))
	for i, fixture := range fixtures {
		if _, err := ddb.Server.Exec(`
			INSERT INTO server_audit_log (ulid, action, timestamp)
			VALUES (?, ?, ?)
		`, fmt.Sprintf("audit-ulid-%d", i), fixture.name, fixture.timestamp); err != nil {
			t.Fatalf("seed audit log %s: %v", fixture.name, err)
		}
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	got, err := q.AdminAuditLogs(adminCtx, nil, nil)
	if err != nil {
		t.Fatalf("AdminAuditLogs() error = %v", err)
	}
	actions := make([]string, 0, len(got))
	for _, entry := range got {
		actions = append(actions, entry.Action)
	}
	assertGraphQLOrder(t, "AdminAuditLogs", actions, want)

	firstPageSize := 1
	firstPage, err := q.AdminAuditLogs(adminCtx, &firstPageSize, nil)
	if err != nil {
		t.Fatalf("AdminAuditLogs(limit=1) error = %v", err)
	}
	if len(firstPage) != 1 {
		t.Fatalf("AdminAuditLogs(limit=1) returned %d rows, want 1", len(firstPage))
	}
	if firstPage[0].Action != want[0] {
		t.Errorf("AdminAuditLogs(limit=1) served %q, want %q (the entry with the newest true instant)", firstPage[0].Action, want[0])
	}

	secondPage, err := q.AdminAuditLogs(adminCtx, &firstPageSize, &firstPageSize)
	if err != nil {
		t.Fatalf("AdminAuditLogs(limit=1, offset=1) error = %v", err)
	}
	if len(secondPage) != 1 || secondPage[0].Action != want[1] {
		t.Errorf("AdminAuditLogs(limit=1, offset=1) = %+v, want the single entry %q", secondPage, want[1])
	}

	zeroLimit := 0
	emptyPage, err := q.AdminAuditLogs(adminCtx, &zeroLimit, nil)
	if err != nil {
		t.Fatalf("AdminAuditLogs(limit=0) error = %v", err)
	}
	if len(emptyPage) != 0 {
		t.Errorf("AdminAuditLogs(limit=0) returned %d rows, want 0 to match the old SQL LIMIT 0 contract", len(emptyPage))
	}
}

// TestQueryResolver_Notifications_NewestByInstantSurvivesLimit is the sharpest
// of this group: the notifications query caps its result, so a wrong ordering
// did not merely reorder the list, it decided which rows existed in it. The
// true-newest notification is stored in the legacy layout of a zone behind UTC,
// so its text sorts below all graphQLNotificationListLimit canonical-UTC
// fillers; the old "ORDER BY created_at DESC LIMIT 50" kept every filler and
// dropped it. The cut must now fall on the oldest filler instead.
func TestQueryResolver_Notifications_NewestByInstantSurvivesLimit(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	user := seedGraphQLUser(t, ddb, "notiforder", "notiforder@example.com", "correctpass1")
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	now := time.Now().UTC().Truncate(time.Second)
	insertNotification := func(id, timestamp string) {
		t.Helper()
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_notifications (id, user_id, type, display, title, message, created_at)
			VALUES (?, ?, 'info', 'center', ?, 'fixture', ?)
		`, id, user.ID, id, timestamp); err != nil {
			t.Fatalf("seed notification %s: %v", id, err)
		}
	}

	insertNotification("newest", now.In(graphqlZoneWest).Format(graphqlLocalLayout))

	// The fillers sit within the hour before "newest", close enough together
	// that the newest row's local-zone text (11 hours behind its real instant)
	// reads older than all of them.
	var oldestFiller string
	for i := 1; i <= graphQLNotificationListLimit; i++ {
		oldestFiller = fmt.Sprintf("filler-%02d", i)
		insertNotification(oldestFiller, dbtime.FormatSQLTimestamp(now.Add(-time.Duration(i)*time.Minute)))
	}

	got, err := q.Notifications(withGraphQLUserContext(context.Background(), user))
	if err != nil {
		t.Fatalf("Notifications() error = %v", err)
	}
	if len(got) != graphQLNotificationListLimit {
		t.Fatalf("Notifications() returned %d rows, want %d", len(got), graphQLNotificationListLimit)
	}
	if got[0].ID != "newest" {
		t.Errorf("Notifications()[0].ID = %q, want %q (the notification with the newest true instant)", got[0].ID, "newest")
	}
	for _, notif := range got {
		if notif.ID == oldestFiller {
			t.Errorf("Notifications() kept %q, want it cut as the oldest row by true instant", oldestFiller)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.After(got[i-1].CreatedAt) {
			t.Fatalf("Notifications() is not newest-first: %q (%s) precedes %q (%s)",
				got[i-1].ID, got[i-1].CreatedAt, got[i].ID, got[i].CreatedAt)
		}
	}
}

// TestQueryResolver_Notifications_UnparseableTimestampIsListedLast proves the
// other half of the contract: a notification whose created_at cannot be read
// still appears - it sorts to the end, where it can neither be reported as the
// newest nor displace a genuinely recent row - rather than being dropped, which
// is what the old code did when such a value failed to scan into a time.Time.
func TestQueryResolver_Notifications_UnparseableTimestampIsListedLast(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	user := seedGraphQLUser(t, ddb, "notifbroken", "notifbroken@example.com", "correctpass1")
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}

	fixtures, want := graphqlOrderingFixtures(time.Now().UTC().Truncate(time.Second))
	for _, fixture := range fixtures {
		if _, err := ddb.Users.Exec(`
			INSERT INTO user_notifications (id, user_id, type, display, title, message, created_at)
			VALUES (?, ?, 'info', 'center', ?, 'fixture', ?)
		`, fixture.name, user.ID, fixture.name, fixture.timestamp); err != nil {
			t.Fatalf("seed notification %s: %v", fixture.name, err)
		}
	}

	got, err := q.Notifications(withGraphQLUserContext(context.Background(), user))
	if err != nil {
		t.Fatalf("Notifications() error = %v", err)
	}

	ids := make([]string, 0, len(got))
	for _, notif := range got {
		ids = append(ids, notif.ID)
	}
	assertGraphQLOrder(t, "Notifications", ids, want)
}
