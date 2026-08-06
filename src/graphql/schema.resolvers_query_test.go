package graphql

import (
	"context"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/webappsgo/wthr/src/database"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// This file covers the happy-path and cross-user-isolation behavior of the
// Query resolvers whose unauthorized/guard paths are already exercised by
// TestQueryResolver_UserAuthGuards / TestQueryResolver_UserAuthLoadGuards /
// TestQueryResolver_AdminRoleGuards in schema.resolvers_test.go. Those tests
// are not duplicated here.

// --- CurrentUser -------------------------------------------------------

func TestQueryResolver_CurrentUser(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "currentuserqry", "currentuserqry@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.CurrentUser(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != user.ID {
		t.Fatalf("got = %+v, want ID = %d", got, user.ID)
	}
	if got.Username != "currentuserqry" {
		t.Fatalf("Username = %q, want %q", got.Username, "currentuserqry")
	}
}

// --- CurrentUserAvatar ---------------------------------------------------

func TestQueryResolver_CurrentUserAvatar(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "avatarqryuser", "avatarqryuser@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.CurrentUserAvatar(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Type == "" {
		t.Fatalf("got = %+v, want a non-empty avatar Type (gravatar fallback)", got)
	}
}

// --- CurrentUserTwoFactorStatus / CurrentUserTwoFactorSetup -------------

func TestQueryResolver_CurrentUserTwoFactorStatus(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "twofastatususer", "twofastatususer@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.CurrentUserTwoFactorStatus(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil status")
	}
	if got.Enabled {
		t.Fatal("expected Enabled = false for a freshly created user")
	}
	if got.RecoveryKeysCount != 0 {
		t.Fatalf("RecoveryKeysCount = %d, want 0", got.RecoveryKeysCount)
	}
}

func TestQueryResolver_CurrentUserTwoFactorSetup(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "twofasetupuser", "twofasetupuser@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.CurrentUserTwoFactorSetup(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Secret == "" || got.QrCode == "" {
		t.Fatalf("got = %+v, want a non-empty Secret and QrCode", got)
	}
	if got.Account != "twofasetupuser@example.com" {
		t.Fatalf("Account = %q, want the user's email", got.Account)
	}
}

// --- CurrentUserPasskeys -------------------------------------------------

func TestQueryResolver_CurrentUserPasskeys(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "passkeylistuser", "passkeylistuser@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.CurrentUserPasskeys(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d passkeys for a fresh user, want 0", len(got))
	}
}

// --- UserSettings --------------------------------------------------------

func TestQueryResolver_UserSettings(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "settingsqryuser", "settingsqryuser@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	got, err := q.UserSettings(userCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Account == nil {
		t.Fatalf("got = %+v, want a non-nil Account block", got)
	}
}

// --- UserTokens ------------------------------------------------------------

func TestQueryResolver_UserTokens(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	user := seedGraphQLUser(t, ddb, "tokenqryowner", "tokenqryowner@example.com", "correctpass1")
	userCtx := withGraphQLUserContext(context.Background(), user)

	t.Run("empty for a fresh user", func(t *testing.T) {
		tokens, err := q.UserTokens(userCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 0 {
			t.Fatalf("got %d tokens, want 0", len(tokens))
		}
	})

	t.Run("lists a token scoped to the caller and hides another user's token", func(t *testing.T) {
		if _, err := database.ExecContext(context.Background(), ddb.Users, database.TimeoutWrite,
			`INSERT INTO user_tokens (user_id, name, token_hash, token_prefix, scopes) VALUES (?, ?, ?, ?, ?)`,
			user.ID, "CI token", "deadbeefdeadbeefdeadbeefdeadbeef", "usr_deadbeef", "read",
		); err != nil {
			t.Fatalf("seed token: %v", err)
		}

		other := seedGraphQLUser(t, ddb, "tokenqryother", "tokenqryother@example.com", "correctpass1")
		if _, err := database.ExecContext(context.Background(), ddb.Users, database.TimeoutWrite,
			`INSERT INTO user_tokens (user_id, name, token_hash, token_prefix, scopes) VALUES (?, ?, ?, ?, ?)`,
			other.ID, "Other token", "cafebabecafebabecafebabecafebabe", "usr_cafebabe", "read",
		); err != nil {
			t.Fatalf("seed other user's token: %v", err)
		}

		tokens, err := q.UserTokens(userCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 {
			t.Fatalf("got %d tokens, want 1 (scoped to caller only)", len(tokens))
		}
		if tokens[0].Name == nil || *tokens[0].Name != "CI token" {
			t.Fatalf("Name = %v, want %q", tokens[0].Name, "CI token")
		}
		if tokens[0].TokenPrefix != "usr_deadbeef" {
			t.Fatalf("TokenPrefix = %q, want %q", tokens[0].TokenPrefix, "usr_deadbeef")
		}
		if tokens[0].Token != nil {
			t.Fatal("expected the raw Token to never be returned by a list query")
		}
	})
}

// --- SavedLocations / SavedLocation ---------------------------------------

func TestQueryResolver_SavedLocations(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "savedlocqryowner", "savedlocqryowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	if _, err := m.CreateSavedLocation(ownerCtx, "Home", 1, 1, nil, nil, nil); err != nil {
		t.Fatalf("seed location: %v", err)
	}

	other := seedGraphQLUser(t, ddb, "savedlocqryother", "savedlocqryother@example.com", "correctpass1")
	otherCtx := withGraphQLUserContext(context.Background(), other)
	if _, err := m.CreateSavedLocation(otherCtx, "Office", 2, 2, nil, nil, nil); err != nil {
		t.Fatalf("seed other user's location: %v", err)
	}

	locs, err := q.SavedLocations(ownerCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 1 || locs[0].Name != "Home" {
		t.Fatalf("locs = %+v, want exactly the caller's own %q location", locs, "Home")
	}
}

func TestQueryResolver_SavedLocation(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "savedlocsingleowner", "savedlocsingleowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	loc, err := m.CreateSavedLocation(ownerCtx, "Home", 1, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("seed location: %v", err)
	}
	id := formatGraphQLTestUserID(int64(loc.ID))

	t.Run("happy path returns the owner's location", func(t *testing.T) {
		got, err := q.SavedLocation(ownerCtx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Name != "Home" {
			t.Fatalf("got = %+v, want Name = %q", got, "Home")
		}
	})

	t.Run("cross-user isolation: cannot read another user's location", func(t *testing.T) {
		other := seedGraphQLUser(t, ddb, "savedlocsingleother", "savedlocsingleother@example.com", "correctpass1")
		otherCtx := withGraphQLUserContext(context.Background(), other)

		if _, err := q.SavedLocation(otherCtx, id); err == nil {
			t.Fatal("expected an error reading another user's saved location, got nil")
		}
	})

	t.Run("nonexistent id errors", func(t *testing.T) {
		if _, err := q.SavedLocation(ownerCtx, "999999"); err == nil {
			t.Fatal("expected an error for a nonexistent saved location id, got nil")
		}
	})
}

// --- Notifications / UnreadNotifications ----------------------------------

func TestQueryResolver_Notifications(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "notifqryowner", "notifqryowner@example.com", "correctpass1")
	other := seedGraphQLUser(t, ddb, "notifqryother", "notifqryother@example.com", "correctpass1")

	seedGraphQLNotification(t, ddb, owner.ID)
	seedGraphQLNotification(t, ddb, other.ID)

	ownerCtx := withGraphQLUserContext(context.Background(), owner)
	notifs, err := q.Notifications(ownerCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("got %d notifications, want exactly the caller's own 1", len(notifs))
	}
	if notifs[0].UserID == nil || int64(*notifs[0].UserID) != owner.ID {
		t.Fatalf("UserID = %v, want %d", notifs[0].UserID, owner.ID)
	}
}

func TestQueryResolver_UnreadNotifications(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "unreadqryowner", "unreadqryowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)

	t.Run("zero for a fresh user", func(t *testing.T) {
		got, err := q.UnreadNotifications(ownerCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Count != 0 {
			t.Fatalf("got = %+v, want Count = 0", got)
		}
	})

	t.Run("counts only the caller's own unread notifications", func(t *testing.T) {
		seedGraphQLNotification(t, ddb, owner.ID)
		seedGraphQLNotification(t, ddb, owner.ID)

		other := seedGraphQLUser(t, ddb, "unreadqryother", "unreadqryother@example.com", "correctpass1")
		seedGraphQLNotification(t, ddb, other.ID)

		got, err := q.UnreadNotifications(ownerCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Count != 2 {
			t.Fatalf("got = %+v, want Count = 2", got)
		}
	})
}

// --- ValidateUserInvite / ValidateServerInvite (non-empty token paths) ---

func TestQueryResolver_ValidateUserInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unknown token rejected", func(t *testing.T) {
		if _, err := q.ValidateUserInvite(context.Background(), "does-not-exist"); err == nil {
			t.Fatal("expected an error for an unknown invite token, got nil")
		}
	})

	t.Run("happy path returns masked email", func(t *testing.T) {
		invite, err := (&models.UserInviteModel{DB: ddb.Users}).CreateInvite("invitee", "invitee@example.com", "user", 7)
		if err != nil {
			t.Fatalf("seed invite: %v", err)
		}

		got, err := q.ValidateUserInvite(context.Background(), invite.Token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Username != "invitee" {
			t.Fatalf("got = %+v, want Username = %q", got, "invitee")
		}
		if got.Email == "invitee@example.com" {
			t.Fatal("expected the invite email to be masked, got the raw address")
		}
	})
}

func TestQueryResolver_ValidateServerInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	t.Run("unknown token rejected", func(t *testing.T) {
		if _, err := q.ValidateServerInvite(context.Background(), "does-not-exist"); err == nil {
			t.Fatal("expected an error for an unknown invite token, got nil")
		}
	})

	t.Run("happy path returns masked email", func(t *testing.T) {
		inviter, err := (&models.AdminModel{DB: ddb.Server}).Create("srvinviteradmin", "srvinviteradmin@example.com", "password123", true)
		if err != nil {
			t.Fatalf("seed inviting admin: %v", err)
		}

		invite, _, err := service.NewAdminInviteService(ddb.Server, "", nil).CreateInvite("invitedadmin@example.com", int(inviter.ID), "7d")
		if err != nil {
			t.Fatalf("seed server invite: %v", err)
		}

		got, err := q.ValidateServerInvite(context.Background(), invite.Token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("got nil validation result")
		}
		if got.Email == "invitedadmin@example.com" {
			t.Fatal("expected the invite email to be masked, got the raw address")
		}
	})
}

// --- AdminUsers / AdminStats / saved-location & notification counts ------

func TestQueryResolver_AdminUsers(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	seedGraphQLUser(t, ddb, "adminlistuser1", "adminlistuser1@example.com", "correctpass1")
	seedGraphQLUser(t, ddb, "adminlistuser2", "adminlistuser2@example.com", "correctpass1")

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	users, err := q.AdminUsers(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2", len(users))
	}
	for _, u := range users {
		if strings.Contains(u.Email, "@example.com") && u.Email != "" && !strings.Contains(u.Email, "*") {
			t.Fatalf("Email = %q, want it masked before being returned to an admin listing", u.Email)
		}
	}
}

func TestQueryResolver_AdminStats(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server, UsersDB: ddb.Users}}
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users}}

	owner := seedGraphQLUser(t, ddb, "statsqryowner", "statsqryowner@example.com", "correctpass1")
	ownerCtx := withGraphQLUserContext(context.Background(), owner)
	if _, err := m.CreateSavedLocation(ownerCtx, "Home", 1, 1, nil, nil, nil); err != nil {
		t.Fatalf("seed location: %v", err)
	}
	seedGraphQLNotification(t, ddb, owner.ID)

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	got, err := q.AdminStats(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Users == nil || got.Users.Total != 1 {
		t.Fatalf("got = %+v, want Users.Total = 1", got)
	}
	if got.Locations == nil || got.Locations.Total != 1 {
		t.Fatalf("Locations = %+v, want Total = 1", got.Locations)
	}
	if got.Notifications == nil || got.Notifications.Total != 1 {
		t.Fatalf("Notifications = %+v, want Total = 1", got.Notifications)
	}
}

// --- AdminServerAdmins / AdminServerAdmin ---------------------------------

func TestQueryResolver_AdminServerAdmins(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("overviewadmin", "overviewadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)
	got, err := q.AdminServerAdmins(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Count != 1 {
		t.Fatalf("got = %+v, want Count = 1", got)
	}
	if got.CurrentAdmin == nil || got.CurrentAdmin.ID != formatGraphQLTestUserID(admin.ID) {
		t.Fatalf("CurrentAdmin = %+v, want ID = %d", got.CurrentAdmin, admin.ID)
	}
}

func TestQueryResolver_AdminServerAdmin(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	self, err := (&models.AdminModel{DB: ddb.Server}).Create("selfadmin", "selfadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed self admin: %v", err)
	}
	other, err := (&models.AdminModel{DB: ddb.Server}).Create("otheradmin", "otheradmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed other admin: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), int(self.ID), self.Email)

	t.Run("happy path returns own account", func(t *testing.T) {
		got, err := q.AdminServerAdmin(adminCtx, formatGraphQLTestUserID(self.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Username != "selfadmin" {
			t.Fatalf("got = %+v, want Username = %q", got, "selfadmin")
		}
	})

	t.Run("cannot view another admin's account details", func(t *testing.T) {
		_, err := q.AdminServerAdmin(adminCtx, formatGraphQLTestUserID(other.ID))
		if err == nil || !strings.Contains(err.Error(), "private") {
			t.Fatalf("err = %v, want it to mention account details are private", err)
		}
	})

	t.Run("invalid id rejected", func(t *testing.T) {
		if _, err := q.AdminServerAdmin(adminCtx, "not-a-number"); err == nil {
			t.Fatal("expected an error for a non-numeric admin id, got nil")
		}
	})
}

// --- AdminUserInvites / AdminUserInvite -----------------------------------

func TestQueryResolver_AdminUserInvites(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	if _, err := (&models.UserInviteModel{DB: ddb.Users}).CreateInvite("invitedqryuser", "invitedqryuser@example.com", "user", 7); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	invites, err := q.AdminUserInvites(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invites) != 1 || invites[0].Username != "invitedqryuser" {
		t.Fatalf("invites = %+v, want exactly one for %q", invites, "invitedqryuser")
	}
}

func TestQueryResolver_AdminUserInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{UsersDB: ddb.Users}}

	invite, err := (&models.UserInviteModel{DB: ddb.Users}).CreateInvite("singleinviteqry", "singleinviteqry@example.com", "user", 7)
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("happy path", func(t *testing.T) {
		got, err := q.AdminUserInvite(adminCtx, formatGraphQLTestUserID(invite.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Username != "singleinviteqry" {
			t.Fatalf("got = %+v, want Username = %q", got, "singleinviteqry")
		}
	})

	t.Run("invalid id rejected", func(t *testing.T) {
		if _, err := q.AdminUserInvite(adminCtx, "not-a-number"); err == nil {
			t.Fatal("expected an error for a non-numeric invite id, got nil")
		}
	})

	t.Run("nonexistent id returns nil, nil", func(t *testing.T) {
		got, err := q.AdminUserInvite(adminCtx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("got = %+v, want nil for a nonexistent invite id", got)
		}
	})
}

// --- AdminSettings / AdminSetting -----------------------------------------

func TestQueryResolver_AdminSettings(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if err := (&models.SettingsModel{DB: ddb.Server}).SetString("qry_test_setting", "some-value"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	settings, err := q.AdminSettings(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range settings {
		if s.Key == "qry_test_setting" && s.Value == "some-value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("settings = %+v, want to find the seeded qry_test_setting", settings)
	}
}

func TestQueryResolver_AdminSetting(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if err := (&models.SettingsModel{DB: ddb.Server}).SetString("qry_single_setting", "abc"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("happy path", func(t *testing.T) {
		got, err := q.AdminSetting(adminCtx, "qry_single_setting")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Value != "abc" {
			t.Fatalf("got = %+v, want Value = %q", got, "abc")
		}
	})

	t.Run("unknown key errors", func(t *testing.T) {
		if _, err := q.AdminSetting(adminCtx, "does-not-exist"); err == nil {
			t.Fatal("expected an error for an unknown setting key, got nil")
		}
	})
}

// --- AdminTokens -----------------------------------------------------------

func TestQueryResolver_AdminTokens(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("tokenqryadmin", "tokenqryadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	t.Run("no session id in context errors", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKeyUserRole, "admin")
		if _, err := q.AdminTokens(ctx); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Fatalf("err = %v, want an unauthorized error", err)
		}
	})

	t.Run("has the token generated at admin creation", func(t *testing.T) {
		adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)
		tokens, err := q.AdminTokens(adminCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 || tokens[0].TokenPrefix == "" {
			t.Fatalf("tokens = %+v, want exactly one token with a non-empty prefix (AdminModel.Create always generates one)", tokens)
		}
	})

	t.Run("lists the admin's regenerated API token prefix", func(t *testing.T) {
		if _, err := (&models.AdminModel{DB: ddb.Server}).RegenerateAPIToken(admin.ID); err != nil {
			t.Fatalf("regenerate token: %v", err)
		}

		adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)
		tokens, err := q.AdminTokens(adminCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 || tokens[0].TokenPrefix == "" {
			t.Fatalf("tokens = %+v, want exactly one token with a non-empty prefix", tokens)
		}
		if tokens[0].Token != "" {
			t.Fatal("expected the raw admin token to never be returned by a list query")
		}
	})
}

// --- AdminAuditLogs --------------------------------------------------------

func TestQueryResolver_AdminAuditLogs(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if _, err := database.ExecContext(context.Background(), ddb.Server, database.TimeoutWrite,
		`INSERT INTO server_audit_log (ulid, actor_type, actor_id, action, resource_type, resource_id) VALUES (?, ?, ?, ?, ?, ?)`,
		ulid.Make().String(), "admin", "1", "test.action", "setting", "1",
	); err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	logs, err := q.AdminAuditLogs(adminCtx, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "test.action" {
		t.Fatalf("logs = %+v, want exactly one with Action = %q", logs, "test.action")
	}
}

// --- AdminTasks / AdminTaskHistory -----------------------------------------

func TestQueryResolver_AdminTasks(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if _, err := database.ExecContext(context.Background(), ddb.Server, database.TimeoutWrite,
		`INSERT INTO server_scheduler_state (task_name, schedule, enabled, run_count, fail_count) VALUES (?, ?, ?, ?, ?)`,
		"geoip_update", "0 3 * * 0", true, 0, 0,
	); err != nil {
		t.Fatalf("seed scheduled task: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	tasks, err := q.AdminTasks(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "geoip_update" {
		t.Fatalf("tasks = %+v, want exactly one for %q", tasks, "geoip_update")
	}
}

// AdminTaskHistory's happy path requires a *scheduler.Scheduler, a concrete
// struct with no seams for a fake, so only the resolver's own guard clauses
// (already-tested admin-role check aside) are covered here.
func TestQueryResolver_AdminTaskHistory_SchedulerUnavailable(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	_, err := q.AdminTaskHistory(adminCtx, "geoip_update", nil)
	if err == nil || err.Error() != "scheduler runtime unavailable" {
		t.Fatalf("err = %v, want %q", err, "scheduler runtime unavailable")
	}
}

// --- AdminChannels / AdminChannel / AdminChannelStats / AdminQueueStats --

func TestQueryResolver_AdminChannels(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if _, err := database.ExecContext(context.Background(), ddb.Server, database.TimeoutWrite,
		`INSERT INTO server_notification_channels (channel_type, channel_name, enabled, config) VALUES (?, ?, ?, ?)`,
		"email", "Email", true, "{}",
	); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	channels, err := q.AdminChannels(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(channels) != 1 || channels[0].Type != "email" {
		t.Fatalf("channels = %+v, want exactly one %q channel", channels, "email")
	}
}

func TestQueryResolver_AdminChannel(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	if _, err := database.ExecContext(context.Background(), ddb.Server, database.TimeoutWrite,
		`INSERT INTO server_notification_channels (channel_type, channel_name, enabled, config) VALUES (?, ?, ?, ?)`,
		"discord", "Discord", false, "{}",
	); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("happy path", func(t *testing.T) {
		got, err := q.AdminChannel(adminCtx, "discord")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Type != "discord" || got.Enabled {
			t.Fatalf("got = %+v, want Type = %q, Enabled = false", got, "discord")
		}
	})

	t.Run("unknown channel type errors", func(t *testing.T) {
		if _, err := q.AdminChannel(adminCtx, "does-not-exist"); err == nil {
			t.Fatal("expected an error for an unknown channel type, got nil")
		}
	})
}

func TestQueryResolver_AdminChannelStats(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	got, err := q.AdminChannelStats(adminCtx, "email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Sent != 0 || got.Failed != 0 {
		t.Fatalf("got = %+v, want a zeroed stats struct for a channel with no queue rows", got)
	}
}

func TestQueryResolver_AdminQueueStats(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")
	got, err := q.AdminQueueStats(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Pending != 0 || got.Processing != 0 || got.Completed != 0 || got.Failed != 0 {
		t.Fatalf("got = %+v, want a zeroed queue-stats struct with no queue rows", got)
	}
}

// --- AdminSMTPProviders ------------------------------------------------

func TestQueryResolver_AdminSMTPProviders(t *testing.T) {
	q := &queryResolver{&Resolver{}}
	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	providers, err := q.AdminSMTPProviders(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected at least one built-in SMTP provider preset")
	}
	for _, p := range providers {
		if p.Name == "" || p.Host == "" || p.Port == 0 {
			t.Fatalf("provider = %+v, want non-empty Name/Host and a nonzero Port", p)
		}
	}
}

// --- AdminPasskeys -----------------------------------------------------

func TestQueryResolver_AdminPasskeys(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("passkeyqryadmin", "passkeyqryadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)
	got, err := q.AdminPasskeys(adminCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d passkeys for a fresh admin, want 0", len(got))
	}
}

// AdminPasskeys now checks ctxKeyUserRole == "admin" the same way every
// other Admin* query resolver does (see schema.resolvers.go AdminUsers for
// the reference pattern). This guards against a context carrying a valid
// ctxKeyAdminID paired with a non-"admin" ctxKeyUserRole (e.g. from a bug
// elsewhere composing context values) reaching admin-only data.
func TestQueryResolver_AdminPasskeys_ChecksUserRole(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	q := &queryResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("roleasymmetryadmin", "roleasymmetryadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	ctx := context.WithValue(context.Background(), ctxKeyAdminID, int(admin.ID))
	ctx = context.WithValue(ctx, ctxKeyUserRole, "user")

	if _, err := q.AdminPasskeys(ctx); err == nil {
		t.Fatal("expected unauthorized error for a non-admin role, got nil")
	}
}
