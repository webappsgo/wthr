package config

import "testing"

func TestParseBool(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal bool
		want       bool
		wantErr    bool
	}{
		{"true literal", "true", false, true, false},
		{"false literal", "false", true, false, false},
		{"1", "1", false, true, false},
		{"0", "0", true, false, false},
		{"yes", "yes", false, true, false},
		{"no", "no", true, false, false},
		{"on", "on", false, true, false},
		{"off", "off", true, false, false},
		{"enabled", "enabled", false, true, false},
		{"disabled", "disabled", true, false, false},
		{"mixed case TRUE", "TRUE", false, true, false},
		{"mixed case False", "False", true, false, false},
		{"whitespace padded", "  true  ", false, true, false},
		{"empty returns default true", "", true, true, false},
		{"empty returns default false", "", false, false, false},
		{"invalid value", "maybe", false, false, true},
		{"invalid gibberish", "banana", true, false, true},
		{"aye", "aye", false, true, false},
		{"nope", "nope", true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBool(tt.input, tt.defaultVal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseBool(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMustParseBool(t *testing.T) {
	t.Run("valid value does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustParseBool() unexpected panic: %v", r)
			}
		}()
		if got := MustParseBool("true", false); got != true {
			t.Errorf("MustParseBool() = %v, want true", got)
		}
	})

	t.Run("empty value returns default without panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustParseBool() unexpected panic: %v", r)
			}
		}()
		if got := MustParseBool("", true); got != true {
			t.Errorf("MustParseBool() = %v, want true", got)
		}
	})

	t.Run("invalid value panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MustParseBool() expected panic, got none")
			}
		}()
		MustParseBool("not-a-bool", false)
	})
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"YES", true},
		{"on", true},
		{" enabled ", true},
		{"false", false},
		{"no", false},
		{"", false},
		{"garbage", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsTruthy(tt.input); got != tt.want {
				t.Errorf("IsTruthy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsFalsy(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"false", true},
		{"NO", true},
		{"off", true},
		{" disabled ", true},
		{"true", false},
		{"yes", false},
		{"", false},
		{"garbage", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsFalsy(tt.input); got != tt.want {
				t.Errorf("IsFalsy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
