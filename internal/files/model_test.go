package files

import (
	"testing"
	"time"
)

func TestFileExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		file File
		want bool
	}{
		{"no expiry", File{}, false},
		{"expires in the future", File{ExpiresAt: &future}, false},
		{"expired in the past", File{ExpiresAt: &past}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.file.Expired(now); got != tc.want {
				t.Fatalf("Expired(%v) = %v, want %v", now, got, tc.want)
			}
		})
	}
}
