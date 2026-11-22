package service

import "testing"

func TestValidSAMLProviderName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "okta", want: true},
		{name: "corp-idp_2", want: true},
		{name: "", want: false},
		{name: "Upper", want: false},
		{name: "2provider", want: false},
		{name: "provider.name", want: false},
		{name: "this-provider-name-is-far-too-long-for-the-provider-name-limit", want: false},
	}
	for _, test := range tests {
		if got := validSAMLProviderName(test.name); got != test.want {
			t.Errorf("validSAMLProviderName(%q) = %t, want %t", test.name, got, test.want)
		}
	}
}
