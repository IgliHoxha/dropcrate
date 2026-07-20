// Package events defines dropcrate's domain events and the publisher that
// emits them. Publishing is an optional, best-effort side channel: it is a
// no-op until a broker is configured, and a broker problem never blocks or
// fails the file operation that produced the event.
package events

import (
	"time"

	"github.com/IgliHoxha/dropcrate/internal/files"
)

// Event types. These are the topic suffixes appended to the configured prefix.
const (
	TypeFileUploaded = "file.uploaded"
	TypeFileDeleted  = "file.deleted"
	TypeFileExpired  = "file.expired"
)

// Event is the payload published for a file lifecycle change. It is a
// projection of files.File that deliberately omits the internal StorageKey.
type Event struct {
	Type        string     `json:"type"`
	FileID      string     `json:"file_id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// FromFile builds an Event of the given type from a file record.
func FromFile(eventType string, f files.File, now time.Time) Event {
	return Event{
		Type:        eventType,
		FileID:      f.ID,
		Filename:    f.Filename,
		ContentType: f.ContentType,
		Size:        f.Size,
		CreatedAt:   f.CreatedAt,
		ExpiresAt:   f.ExpiresAt,
		OccurredAt:  now,
	}
}
