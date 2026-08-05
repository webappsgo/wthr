package graphql

import (
	"context"
	"strconv"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	models "github.com/webappsgo/wthr/src/server/model"
)

// formatGraphQLTestUserID renders an int64 user id as the string form the
// GraphQL layer expects for ID-typed arguments.
func formatGraphQLTestUserID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// --- AdminUpdateUser -----------------------------------------------------

func TestMutationResolver_AdminUpdateUser(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users, ServerDB: ddb.Server}}

	target := seedGraphQLUser(t, ddb, "targetuser", "targetuser@example.com", "correctpass1")

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminUpdateUser(context.Background(), "1", nil, nil, nil)
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	t.Run("non-admin role rejected", func(t *testing.T) {
		userCtx := withGraphQLUserContext(context.Background(), target)
		_, err := m.AdminUpdateUser(userCtx, "1", nil, nil, nil)
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("invalid user id", func(t *testing.T) {
		_, err := m.AdminUpdateUser(adminCtx, "not-a-number", nil, nil, nil)
		if err == nil || err.Error() != "invalid user id" {
			t.Fatalf("err = %v, want %q", err, "invalid user id")
		}
	})

	t.Run("unknown user id", func(t *testing.T) {
		_, err := m.AdminUpdateUser(adminCtx, "999999", nil, nil, nil)
		if err == nil {
			t.Fatal("expected an error loading a nonexistent user")
		}
	})

	t.Run("invalid new username rejected", func(t *testing.T) {
		bad := "!!"
		_, err := m.AdminUpdateUser(adminCtx, formatGraphQLTestUserID(target.ID), &bad, nil, nil)
		if err == nil {
			t.Fatal("expected a validation error for an invalid username")
		}
	})

	t.Run("invalid new email rejected", func(t *testing.T) {
		bad := "not-an-email"
		_, err := m.AdminUpdateUser(adminCtx, formatGraphQLTestUserID(target.ID), nil, &bad, nil)
		if err == nil {
			t.Fatal("expected a validation error for an invalid email")
		}
	})

	t.Run("happy path updates username, email, role and masks email in response", func(t *testing.T) {
		newUsername := "renameduser"
		newEmail := "renamed@example.com"
		newRole := "admin"

		updated, err := m.AdminUpdateUser(adminCtx, formatGraphQLTestUserID(target.ID), &newUsername, &newEmail, &newRole)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated == nil || updated.Username != "renameduser" {
			t.Fatalf("Username = %+v, want %q", updated, "renameduser")
		}
		if updated.Role != "admin" {
			t.Fatalf("Role = %q, want %q", updated.Role, "admin")
		}
		if updated.Email == newEmail {
			t.Fatal("expected the returned email to be masked, not the raw new email")
		}

		fresh, err := (&models.UserModel{DB: ddb.Users}).GetByID(target.ID)
		if err != nil {
			t.Fatalf("reload user: %v", err)
		}
		if fresh.Email != newEmail {
			t.Fatalf("stored Email = %q, want %q (masking must only apply to the response)", fresh.Email, newEmail)
		}
	})

	t.Run("blank role leaves role unchanged", func(t *testing.T) {
		blank := "  "
		before, err := (&models.UserModel{DB: ddb.Users}).GetByID(target.ID)
		if err != nil {
			t.Fatalf("reload user: %v", err)
		}
		updated, err := m.AdminUpdateUser(adminCtx, formatGraphQLTestUserID(target.ID), nil, nil, &blank)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Role != before.Role {
			t.Fatalf("Role = %q, want unchanged %q", updated.Role, before.Role)
		}
	})
}

// --- AdminDeleteUser -------------------------------------------------------

func TestMutationResolver_AdminDeleteUser(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users, ServerDB: ddb.Server}}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminDeleteUser(context.Background(), "1")
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("invalid user id", func(t *testing.T) {
		_, err := m.AdminDeleteUser(adminCtx, "nope")
		if err == nil || err.Error() != "invalid user id" {
			t.Fatalf("err = %v, want %q", err, "invalid user id")
		}
	})

	t.Run("unknown user id returns not-found response, not an error", func(t *testing.T) {
		result, err := m.AdminDeleteUser(adminCtx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false for an unknown user id")
		}
		if result.Message != "User not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "User not found")
		}
	})

	t.Run("happy path deletes the user", func(t *testing.T) {
		target := seedGraphQLUser(t, ddb, "deleteme", "deleteme@example.com", "correctpass1")
		result, err := m.AdminDeleteUser(adminCtx, formatGraphQLTestUserID(target.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		if _, err := (&models.UserModel{DB: ddb.Users}).GetByID(target.ID); err == nil {
			t.Fatal("expected the user to be gone after deletion")
		}
	})
}

// --- AdminCreateUserInvite -------------------------------------------------

func TestMutationResolver_AdminCreateUserInvite(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{UsersDB: ddb.Users, ServerDB: ddb.Server}}
	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminCreateUserInvite(context.Background(), "someone", "someone@example.com", nil, nil)
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	t.Run("invalid username rejected", func(t *testing.T) {
		_, err := m.AdminCreateUserInvite(adminCtx, "!!", "valid@example.com", nil, nil)
		if err == nil {
			t.Fatal("expected a validation error for an invalid username")
		}
	})

	t.Run("invalid email rejected", func(t *testing.T) {
		_, err := m.AdminCreateUserInvite(adminCtx, "validusername", "not-an-email", nil, nil)
		if err == nil {
			t.Fatal("expected a validation error for an invalid email")
		}
	})

	t.Run("username already in use rejected", func(t *testing.T) {
		seedGraphQLUser(t, ddb, "existinguser", "existinguser@example.com", "correctpass1")
		_, err := m.AdminCreateUserInvite(adminCtx, "existinguser", "different@example.com", nil, nil)
		if err == nil || err.Error() != "username is already in use" {
			t.Fatalf("err = %v, want %q", err, "username is already in use")
		}
	})

	t.Run("email already in use rejected", func(t *testing.T) {
		seedGraphQLUser(t, ddb, "otheruser2", "taken@example.com", "correctpass1")
		_, err := m.AdminCreateUserInvite(adminCtx, "brandnewname", "taken@example.com", nil, nil)
		if err == nil || err.Error() != "email is already in use" {
			t.Fatalf("err = %v, want %q", err, "email is already in use")
		}
	})

	t.Run("happy path defaults role to user and uses config expiration", func(t *testing.T) {
		invite, err := m.AdminCreateUserInvite(adminCtx, "inviteduser", "invited@example.com", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invite == nil || invite.Username != "inviteduser" {
			t.Fatalf("invite = %+v, want Username = %q", invite, "inviteduser")
		}
		if invite.Role != "user" {
			t.Fatalf("Role = %q, want %q", invite.Role, "user")
		}
		if invite.Token == "" {
			t.Fatal("expected a non-empty invite token")
		}
	})

	t.Run("explicit role and expiresInDays are honored", func(t *testing.T) {
		role := "admin"
		days := 3
		invite, err := m.AdminCreateUserInvite(adminCtx, "inviteduser2", "invited2@example.com", &role, &days)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invite.Role != "admin" {
			t.Fatalf("Role = %q, want %q", invite.Role, "admin")
		}
	})
}

// --- AdminInviteServerAdmin -------------------------------------------------

func TestMutationResolver_AdminInviteServerAdmin(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	inviter, err := (&models.AdminModel{DB: ddb.Server}).Create("inviteradmin2", "inviteradmin2@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed inviter admin: %v", err)
	}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminInviteServerAdmin(context.Background(), "new-admin@example.com", nil)
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), int(inviter.ID), inviter.Email)

	t.Run("invalid email rejected", func(t *testing.T) {
		_, err := m.AdminInviteServerAdmin(adminCtx, "not-an-email", nil)
		if err == nil {
			t.Fatal("expected a validation error for an invalid email")
		}
	})

	t.Run("invalid expiration rejected", func(t *testing.T) {
		bogus := "99d"
		_, err := m.AdminInviteServerAdmin(adminCtx, "candidate@example.com", &bogus)
		if err == nil {
			t.Fatal("expected an error for an unsupported expiration value")
		}
	})

	t.Run("email already registered as admin rejected", func(t *testing.T) {
		_, err := m.AdminInviteServerAdmin(adminCtx, "inviteradmin2@example.com", nil)
		if err == nil {
			t.Fatal("expected an error inviting an email that is already an admin")
		}
	})

	t.Run("happy path defaults to 24h expiration", func(t *testing.T) {
		invite, err := m.AdminInviteServerAdmin(adminCtx, "newserveradmin@example.com", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invite == nil || invite.Email != "newserveradmin@example.com" {
			t.Fatalf("invite = %+v, want Email = %q", invite, "newserveradmin@example.com")
		}
		if invite.ExpiresIn != "24h" {
			t.Fatalf("ExpiresIn = %q, want %q", invite.ExpiresIn, "24h")
		}
		if invite.Token == "" {
			t.Fatal("expected a non-empty invite token")
		}
	})

	t.Run("explicit expiration is honored", func(t *testing.T) {
		expiration := "1h"
		invite, err := m.AdminInviteServerAdmin(adminCtx, "anothernewadmin@example.com", &expiration)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invite.ExpiresIn != "1h" {
			t.Fatalf("ExpiresIn = %q, want %q", invite.ExpiresIn, "1h")
		}
	})
}

// --- AdminDeleteServerAdmin --------------------------------------------------

func TestMutationResolver_AdminDeleteServerAdmin(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	primary, err := (&models.AdminModel{DB: ddb.Server}).Create("primaryadmin", "primaryadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed primary admin: %v", err)
	}
	if primary.ID != 1 {
		t.Fatalf("expected the first-created admin to get id 1, got %d", primary.ID)
	}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminDeleteServerAdmin(context.Background(), "2")
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), int(primary.ID), primary.Email)

	t.Run("invalid admin id", func(t *testing.T) {
		_, err := m.AdminDeleteServerAdmin(adminCtx, "nope")
		if err == nil || err.Error() != "invalid admin id" {
			t.Fatalf("err = %v, want %q", err, "invalid admin id")
		}
	})

	t.Run("cannot delete your own admin account", func(t *testing.T) {
		result, err := m.AdminDeleteServerAdmin(adminCtx, formatGraphQLTestUserID(primary.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false deleting your own admin account")
		}
	})

	t.Run("primary admin account cannot be deleted", func(t *testing.T) {
		second, err := (&models.AdminModel{DB: ddb.Server}).Create("seconddeleter", "seconddeleter@example.com", "password123", true)
		if err != nil {
			t.Fatalf("seed second admin: %v", err)
		}
		secondCtx := withGraphQLAdminValues(context.Background(), int(second.ID), second.Email)

		result, err := m.AdminDeleteServerAdmin(secondCtx, "1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false deleting the primary admin account")
		}
	})

	t.Run("unknown admin id returns not-found response, not an error", func(t *testing.T) {
		result, err := m.AdminDeleteServerAdmin(adminCtx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false for an unknown admin id")
		}
		if result.Message != "Admin not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "Admin not found")
		}
	})

	t.Run("cannot delete the last active super admin", func(t *testing.T) {
		soleSuperAdmin, err := (&models.AdminModel{DB: ddb.Server}).Create("solesuper", "solesuper@example.com", "password123", true)
		if err != nil {
			t.Fatalf("seed sole super admin: %v", err)
		}
		nonSuperCaller, err := (&models.AdminModel{DB: ddb.Server}).Create("nonsupercaller", "nonsupercaller@example.com", "password123", false)
		if err != nil {
			t.Fatalf("seed non-super caller admin: %v", err)
		}
		callerCtx := withGraphQLAdminValues(context.Background(), int(nonSuperCaller.ID), nonSuperCaller.Email)

		// Deactivate every other active super admin (seeded by earlier subtests
		// in this test, e.g. "primary" and "second") so soleSuperAdmin is the
		// only active super admin left.
		allAdmins, err := (&models.AdminModel{DB: ddb.Server}).GetAll()
		if err != nil {
			t.Fatalf("list admins: %v", err)
		}
		for _, other := range allAdmins {
			if other.ID == soleSuperAdmin.ID || !other.IsSuperAdmin || !other.IsActive {
				continue
			}
			if err := (&models.AdminModel{DB: ddb.Server}).Update(other.ID, other.Username, other.Email, other.IsSuperAdmin, false); err != nil {
				t.Fatalf("deactivate other super admin %d: %v", other.ID, err)
			}
		}

		result, err := m.AdminDeleteServerAdmin(callerCtx, formatGraphQLTestUserID(soleSuperAdmin.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false deleting the last active super admin")
		}
		if result.Message != "Cannot delete the last active super admin" {
			t.Fatalf("Message = %q, want %q", result.Message, "Cannot delete the last active super admin")
		}
	})

	t.Run("happy path deletes a non-super, non-primary, non-self admin", func(t *testing.T) {
		target, err := (&models.AdminModel{DB: ddb.Server}).Create("deletablesub", "deletablesub@example.com", "password123", false)
		if err != nil {
			t.Fatalf("seed deletable admin: %v", err)
		}

		result, err := m.AdminDeleteServerAdmin(adminCtx, formatGraphQLTestUserID(target.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}

		if _, err := (&models.AdminModel{DB: ddb.Server}).GetByID(target.ID); err == nil {
			t.Fatal("expected the admin to be gone after deletion")
		}
	})
}

// --- AdminUpdateSetting -------------------------------------------------------

func TestMutationResolver_AdminUpdateSetting(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminUpdateSetting(context.Background(), "server.title", "New Title")
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), 1, "admin@example.com")

	t.Run("unknown key errors", func(t *testing.T) {
		_, err := m.AdminUpdateSetting(adminCtx, "does.not.exist", "value")
		if err == nil {
			t.Fatal("expected an error updating a setting that does not exist")
		}
	})

	t.Run("happy path updates an existing setting", func(t *testing.T) {
		if err := (&models.SettingsModel{DB: ddb.Server}).SetString("server.title", "Original Title"); err != nil {
			t.Fatalf("seed setting: %v", err)
		}

		updated, err := m.AdminUpdateSetting(adminCtx, "server.title", "Updated Title")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated == nil || updated.Value != "Updated Title" {
			t.Fatalf("setting = %+v, want Value = %q", updated, "Updated Title")
		}
		if updated.Key != "server.title" {
			t.Fatalf("Key = %q, want %q", updated.Key, "server.title")
		}
	})
}

// --- AdminGenerateToken / AdminRevokeToken ------------------------------------

func TestMutationResolver_AdminGenerateToken(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("tokenadmin", "tokenadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminGenerateToken(context.Background())
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	t.Run("admin role without admin id in context is rejected", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxKeyUserRole, "admin")
		_, err := m.AdminGenerateToken(ctx)
		if err == nil || err.Error() != "unauthorized: admin session required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin session required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)

	t.Run("happy path generates a new token", func(t *testing.T) {
		token, err := m.AdminGenerateToken(adminCtx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == nil || token.Token == "" {
			t.Fatalf("token = %+v, want a non-empty Token", token)
		}
		if token.TokenPrefix == "" {
			t.Fatal("expected a non-empty TokenPrefix")
		}
		if token.ID != int(admin.ID) {
			t.Fatalf("ID = %d, want %d", token.ID, admin.ID)
		}
	})
}

func TestMutationResolver_AdminRevokeToken(t *testing.T) {
	ddb := newAuthMutationTestDB(t)
	m := &mutationResolver{&Resolver{ServerDB: ddb.Server}}

	admin, err := (&models.AdminModel{DB: ddb.Server}).Create("revoketokenadmin", "revoketokenadmin@example.com", "password123", true)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	t.Run("unauthorized without admin role", func(t *testing.T) {
		_, err := m.AdminRevokeToken(context.Background(), formatGraphQLTestUserID(admin.ID))
		if err == nil || err.Error() != "unauthorized: admin access required" {
			t.Fatalf("err = %v, want %q", err, "unauthorized: admin access required")
		}
	})

	adminCtx := withGraphQLAdminValues(context.Background(), int(admin.ID), admin.Email)

	t.Run("id not matching the caller's own admin id is not found", func(t *testing.T) {
		result, err := m.AdminRevokeToken(adminCtx, "999999")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatal("expected Success = false revoking a token id that isn't the caller's own")
		}
		if result.Message != "Token not found" {
			t.Fatalf("Message = %q, want %q", result.Message, "Token not found")
		}
	})

	t.Run("happy path revokes the caller's own token", func(t *testing.T) {
		if _, err := (&models.AdminModel{DB: ddb.Server}).RegenerateAPIToken(admin.ID); err != nil {
			t.Fatalf("seed api token: %v", err)
		}

		result, err := m.AdminRevokeToken(adminCtx, formatGraphQLTestUserID(admin.ID))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected Success = true, message: %q", result.Message)
		}
	})
}

// Keeps the config import in use: AdminCreateUserInvite falls back to this
// default expiration (7 days) when no explicit expiresInDays is provided,
// exercised implicitly by the "happy path" subtest above.
var _ = config.GetUserInviteExpirationDays
