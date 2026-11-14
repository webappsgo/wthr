package util

import "testing"

func TestIsAdminSectionName(t *testing.T) {
	for _, name := range []string{"settings", "users", "backup", "security", "network", "setup", "notifications", "web"} {
		if !IsAdminSectionName(name) {
			t.Errorf("IsAdminSectionName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "config", "alice", "administrator", "misc", "dashboard", "logout"} {
		if IsAdminSectionName(name) {
			t.Errorf("IsAdminSectionName(%q) = true, want false", name)
		}
	}
}

func TestIsValidAdminRootPath(t *testing.T) {
	if !IsValidAdminRootPath("") {
		t.Error("the dashboard (empty segment) must be a valid admin root path")
	}
	if !IsValidAdminRootPath("settings") {
		t.Error("settings must be a valid admin root path")
	}
	if IsValidAdminRootPath("config") {
		t.Error("config must not be a valid admin root path")
	}
}

func TestAdminSectionNamesExcludeDashboard(t *testing.T) {
	names := AdminSectionNames()
	if len(names) == 0 {
		t.Fatal("AdminSectionNames() is empty")
	}
	for _, name := range names {
		if name == "" {
			t.Fatal("AdminSectionNames() must not include the empty dashboard name")
		}
	}
}

func TestValidateAdminUsername(t *testing.T) {
	valid := []string{"administrator", "admin", "alice", "bob-2", "a1"}
	for _, name := range valid {
		if err := ValidateAdminUsername(name); err != nil {
			t.Errorf("ValidateAdminUsername(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"settings", "backup", "users", "Alice", " alice", "a", "-bob", "bob-", "bo--b", "a_b", ""}
	for _, name := range invalid {
		if err := ValidateAdminUsername(name); err == nil {
			t.Errorf("ValidateAdminUsername(%q) = nil, want an error", name)
		}
	}
}

func TestValidateUsernameRejectsReservedNames(t *testing.T) {
	for _, name := range []string{"feed", "graphql", "admin1", "orgs", "invite"} {
		if err := ValidateUsername(name); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want a reserved-name error", name)
		}
	}
	if err := ValidateUsername("alice-wonder"); err != nil {
		t.Errorf("ValidateUsername(alice-wonder) = %v, want nil", err)
	}
}
