package urlsign

import (
	"net/url"
	"testing"
	"time"
)

func TestDisabled(t *testing.T) {
	s := New("", time.Hour)
	if s.Enabled() {
		t.Fatal("empty key should be disabled")
	}
	if q := s.Query("abc", time.Now()); q != "" {
		t.Errorf("Query on disabled = %q, want empty", q)
	}
	if err := s.Verify("abc", "1", "x", time.Now()); err != ErrDisabled {
		t.Errorf("Verify on disabled = %v, want ErrDisabled", err)
	}
}

// parse pulls exp and sig out of a signed query string.
func parse(t *testing.T, q string) (exp, sig string) {
	t.Helper()
	v, err := url.ParseQuery(q)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", q, err)
	}
	return v.Get(ParamExpires), v.Get(ParamSignature)
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	s := New("secret", time.Hour)

	exp, sig := parse(t, s.Query("file-1", now))
	if exp == "" || sig == "" {
		t.Fatal("Query did not produce exp and sig")
	}
	if err := s.Verify("file-1", exp, sig, now); err != nil {
		t.Errorf("valid link rejected: %v", err)
	}
}

func TestVerifyRejections(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	s := New("secret", time.Hour)
	exp, sig := parse(t, s.Query("file-1", now))

	t.Run("wrong id", func(t *testing.T) {
		if err := s.Verify("file-2", exp, sig, now); err != ErrBadSignature {
			t.Errorf("err = %v, want ErrBadSignature", err)
		}
	})
	t.Run("tampered signature", func(t *testing.T) {
		if err := s.Verify("file-1", exp, sig+"00", now); err != ErrBadSignature {
			t.Errorf("err = %v, want ErrBadSignature", err)
		}
	})
	t.Run("tampered expiry", func(t *testing.T) {
		if err := s.Verify("file-1", "9999999999", sig, now); err != ErrBadSignature {
			t.Errorf("err = %v, want ErrBadSignature", err)
		}
	})
	t.Run("expired link", func(t *testing.T) {
		later := now.Add(2 * time.Hour)
		if err := s.Verify("file-1", exp, sig, later); err != ErrExpired {
			t.Errorf("err = %v, want ErrExpired", err)
		}
	})
	t.Run("different key", func(t *testing.T) {
		other := New("other-secret", time.Hour)
		if err := other.Verify("file-1", exp, sig, now); err != ErrBadSignature {
			t.Errorf("err = %v, want ErrBadSignature", err)
		}
	})
}
