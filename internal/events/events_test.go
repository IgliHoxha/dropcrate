package events

import (
	"context"
	"testing"
	"time"

	"github.com/IgliHoxha/dropcrate/internal/files"
)

func TestFromFile(t *testing.T) {
	exp := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	f := files.File{
		ID:          "abc",
		Filename:    "photo.png",
		ContentType: "image/png",
		Size:        1234,
		StorageKey:  "files/abc",
		CreatedAt:   now,
		ExpiresAt:   &exp,
	}

	e := FromFile(TypeFileUploaded, f, now)

	if e.Type != TypeFileUploaded {
		t.Errorf("Type = %q, want %q", e.Type, TypeFileUploaded)
	}
	if e.FileID != "abc" || e.Filename != "photo.png" || e.Size != 1234 {
		t.Errorf("unexpected projection: %+v", e)
	}
	if e.ExpiresAt == nil || !e.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %v, want %v", e.ExpiresAt, exp)
	}
	if !e.OccurredAt.Equal(now) {
		t.Errorf("OccurredAt = %v, want %v", e.OccurredAt, now)
	}
}

// Nop must accept events and close without doing anything or panicking, since
// it is the default publisher when no broker is configured.
func TestNopPublisher(t *testing.T) {
	var p Publisher = Nop{}
	p.Publish(context.Background(), Event{Type: TypeFileDeleted, FileID: "x"})
	if err := p.Close(); err != nil {
		t.Errorf("Nop.Close() = %v, want nil", err)
	}
}
