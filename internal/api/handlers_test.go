package api

import (
	"testing"
	"time"
)

func TestParseTTL(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},       // use server default
		{"0", -1, false},     // never expire
		{"never", -1, false}, // never expire
		{"24h", 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"nonsense", 0, true},
	}

	for _, tc := range cases {
		got, err := parseTTL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTTL(%q) expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTTL(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseTTL(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
