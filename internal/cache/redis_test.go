package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/IgliHoxha/dropcrate/internal/files"
)

// newTestCache spins up an in-memory Redis and returns a cache pointed at it.
func newTestCache(t *testing.T) *MetadataCache {
	t.Helper()
	mr := miniredis.RunT(t)
	return &MetadataCache{
		rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		ttl: time.Hour,
	}
}

// TestRoundTripPreservesStorageKey guards the bug where files.File's `json:"-"`
// tag dropped StorageKey from the cached record, breaking downloads on a cache
// hit. Set then Get must return the storage key intact.
func TestRoundTripPreservesStorageKey(t *testing.T) {
	ctx := context.Background()
	c := newTestCache(t)

	exp := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	in := files.File{
		ID:          "abc",
		Filename:    "photo.png",
		ContentType: "image/png",
		Size:        2048,
		StorageKey:  "files/abc",
		CreatedAt:   time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
		ExpiresAt:   &exp,
	}

	c.Set(ctx, in)

	got, ok := c.Get(ctx, "abc")
	if !ok {
		t.Fatal("Get returned not-found after Set")
	}
	if got.StorageKey != in.StorageKey {
		t.Errorf("StorageKey = %q, want %q (regression: cache dropped the key)", got.StorageKey, in.StorageKey)
	}
	if got.ID != in.ID || got.Filename != in.Filename || got.Size != in.Size {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, exp)
	}
}

func TestGetMissAndDelete(t *testing.T) {
	ctx := context.Background()
	c := newTestCache(t)

	if _, ok := c.Get(ctx, "missing"); ok {
		t.Error("Get on unknown id returned ok=true")
	}

	c.Set(ctx, files.File{ID: "x", StorageKey: "files/x"})
	if _, ok := c.Get(ctx, "x"); !ok {
		t.Fatal("Get after Set returned not-found")
	}
	c.Delete(ctx, "x")
	if _, ok := c.Get(ctx, "x"); ok {
		t.Error("Get after Delete returned ok=true")
	}
}
