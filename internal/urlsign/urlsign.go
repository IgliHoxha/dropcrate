// Package urlsign produces and verifies HMAC-signed, expiring download links.
// When enabled, a download URL carries an expiry and a signature over the file
// id, so links cannot be forged or reused past their lifetime even if the id
// leaks. It is disabled when constructed without a key.
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"time"
)

// Query parameter names on a signed URL.
const (
	ParamExpires   = "exp"
	ParamSignature = "sig"
)

var (
	// ErrDisabled is returned by Verify when signing is not configured.
	ErrDisabled = errors.New("url signing disabled")
	// ErrExpired is returned when the link's expiry has passed.
	ErrExpired = errors.New("download link expired")
	// ErrBadSignature is returned when the signature is missing or invalid.
	ErrBadSignature = errors.New("invalid download link signature")
)

// Signer signs and verifies download links. The zero value is not usable; use
// New. A Signer built with an empty key is disabled.
type Signer struct {
	key []byte
	ttl time.Duration
}

// New builds a Signer. An empty key disables signing (Enabled reports false).
// ttl is how long a freshly signed link stays valid.
func New(key string, ttl time.Duration) *Signer {
	if key == "" {
		return &Signer{}
	}
	return &Signer{key: []byte(key), ttl: ttl}
}

// Enabled reports whether signing is configured.
func (s *Signer) Enabled() bool { return len(s.key) > 0 }

// Query returns the signed query parameters ("exp=…&sig=…") for id, valid for
// the configured TTL from now. It returns "" when signing is disabled.
func (s *Signer) Query(id string, now time.Time) string {
	if !s.Enabled() {
		return ""
	}
	exp := now.Add(s.ttl).Unix()
	v := url.Values{}
	v.Set(ParamExpires, strconv.FormatInt(exp, 10))
	v.Set(ParamSignature, s.sign(id, exp))
	return v.Encode()
}

// Verify checks that a request for id carries a valid, unexpired signature. It
// returns nil on success. When signing is disabled it returns ErrDisabled, so
// callers should gate on Enabled first.
func (s *Signer) Verify(id, expRaw, sig string, now time.Time) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	want := s.sign(id, exp)
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return ErrBadSignature
	}
	// Check expiry only after the signature is validated so a tampered exp is
	// reported as a bad signature rather than as expired.
	if now.Unix() > exp {
		return ErrExpired
	}
	return nil
}

// sign returns the hex HMAC-SHA256 of the id and expiry.
func (s *Signer) sign(id string, exp int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(id))
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
