// Package auth provides optional API-key authentication for dropcrate's
// mutating operations. It is disabled unless keys are configured, in which case
// uploads and deletes require a valid bearer token.
package auth

import (
	"crypto/subtle"
	"strings"
)

// Authenticator validates bearer API keys. The zero value (no keys) is
// disabled and accepts nothing; callers should skip auth when Enabled is false.
type Authenticator struct {
	keys [][]byte
}

// New builds an Authenticator from the given keys, ignoring blank entries.
func New(keys []string) *Authenticator {
	a := &Authenticator{}
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			a.keys = append(a.keys, []byte(k))
		}
	}
	return a
}

// Enabled reports whether any key is configured. When false, authentication
// should be bypassed entirely.
func (a *Authenticator) Enabled() bool { return len(a.keys) > 0 }

// Valid reports whether token matches a configured key. The comparison is
// constant-time to avoid leaking key material through timing. It always returns
// false when auth is disabled (no keys), so it must be gated behind Enabled.
func (a *Authenticator) Valid(token string) bool {
	if token == "" {
		return false
	}
	got := []byte(token)
	var ok bool
	// Compare against every key (no early exit) to keep timing independent of
	// which key matched.
	for _, k := range a.keys {
		if subtle.ConstantTimeCompare(got, k) == 1 {
			ok = true
		}
	}
	return ok
}

// BearerToken extracts the token from an "Authorization: Bearer <token>" value,
// returning "" if the header is missing or malformed.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
