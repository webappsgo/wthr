package handler

import "testing"

func TestNormalizeSAMLUsername(t *testing.T) {
	tests := []struct{ candidate, email, want string }{
		{candidate: "@Alice.Example", want: "aliceexample"},
		{email: "river@example.org", want: "river"},
		{candidate: "", email: "", want: "user"},
	}
	for _, test := range tests {
		if got := normalizeSAMLUsername(test.candidate, test.email); got != test.want {
			t.Errorf("normalizeSAMLUsername(%q, %q) = %q, want %q", test.candidate, test.email, got, test.want)
		}
	}
}

func TestMappedSAMLRole(t *testing.T) {
	mapping := map[string][]string{"admin": {"server-admins"}, "moderator": {"support"}, "user": {"members"}}
	for _, test := range []struct {
		groups []string
		want   string
	}{
		{groups: []string{"members"}, want: "user"},
		{groups: []string{"support", "members"}, want: "moderator"},
		{groups: []string{"server-admins", "members"}, want: "admin"},
		{groups: []string{"unknown"}, want: "user"},
	} {
		if got := mappedSAMLRole(test.groups, mapping); got != test.want {
			t.Errorf("mappedSAMLRole(%v) = %q, want %q", test.groups, got, test.want)
		}
	}
}
