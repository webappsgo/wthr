package handler

import "testing"

func TestPasswordHasSurroundingWhitespace(t *testing.T) {
	for _, test := range []struct {
		password string
		want     bool
	}{
		{password: "correct horse battery staple"},
		{password: "", want: false},
		{password: " leading", want: true},
		{password: "trailing ", want: true},
		{password: " both ", want: true},
		{password: "\tpassword", want: true},
		{password: "password\n", want: true},
	} {
		if got := passwordHasSurroundingWhitespace(test.password); got != test.want {
			t.Errorf("passwordHasSurroundingWhitespace(%q) = %t, want %t", test.password, got, test.want)
		}
	}
}
