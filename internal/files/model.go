// Package files contains the domain model, persistence, and business logic
// for stored files.
package files

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a file id has no live record (it never
// existed, was deleted, or has expired).
var ErrNotFound = errors.New("file not found")

// File is the metadata dropcrate tracks for each stored blob. The bytes
// themselves live in the object store under StorageKey.
type File struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	StorageKey  string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Expired reports whether the file's expiry has passed as of now.
func (f File) Expired(now time.Time) bool {
	return f.ExpiresAt != nil && now.After(*f.ExpiresAt)
}
