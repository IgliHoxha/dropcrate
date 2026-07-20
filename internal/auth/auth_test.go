package auth

import "testing"

func TestEnabled(t *testing.T) {
	if New(nil).Enabled() {
		t.Error("no keys should be disabled")
	}
	if New([]string{"  ", ""}).Enabled() {
		t.Error("blank-only keys should be disabled")
	}
	if !New([]string{"secret"}).Enabled() {
		t.Error("configured key should be enabled")
	}
}

func TestValid(t *testing.T) {
	a := New([]string{"k1", "k2"})
	cases := []struct {
		token string
		want  bool
	}{
		{"k1", true},
		{"k2", true},
		{"k3", false},
		{"", false},
		{"K1", false}, // case-sensitive
	}
	for _, tc := range cases {
		if got := a.Valid(tc.token); got != tc.want {
			t.Errorf("Valid(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"Bearer abc", "abc"},
		{"bearer abc", "abc"}, // scheme is case-insensitive
		{"Bearer  abc  ", "abc"},
		{"Basic abc", ""},
		{"abc", ""},
		{"", ""},
		{"Bearer ", ""},
	}
	for _, tc := range cases {
		if got := BearerToken(tc.header); got != tc.want {
			t.Errorf("BearerToken(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}
