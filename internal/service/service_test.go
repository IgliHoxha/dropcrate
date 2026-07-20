package service

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/IgliHoxha/dropcrate/internal/events"
	"github.com/IgliHoxha/dropcrate/internal/files"
	"github.com/IgliHoxha/dropcrate/internal/storage"
)

// --- fakes ---------------------------------------------------------------

type fakeRepo struct {
	items      map[string]files.File
	insertErr  error
	expired    []files.File
	deleteCall int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string]files.File{}} }

func (r *fakeRepo) Insert(_ context.Context, f files.File) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.items[f.ID] = f
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (files.File, error) {
	f, ok := r.items[id]
	if !ok {
		return files.File{}, files.ErrNotFound
	}
	return f, nil
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	r.deleteCall++
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) StorageKeysExpiredBefore(_ context.Context, limit int) ([]files.File, error) {
	if len(r.expired) > limit {
		return r.expired[:limit], nil
	}
	return r.expired, nil
}

type fakeStore struct {
	blobs      map[string][]byte
	putErr     error
	deleteKeys []string
}

func newFakeStore() *fakeStore { return &fakeStore{blobs: map[string][]byte{}} }

func (s *fakeStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (int64, error) {
	if s.putErr != nil {
		return 0, s.putErr
	}
	b, _ := io.ReadAll(r)
	s.blobs[key] = b
	return int64(len(b)), nil
}

func (s *fakeStore) Get(_ context.Context, key string) (*storage.Object, error) {
	b, ok := s.blobs[key]
	if !ok {
		return nil, files.ErrNotFound
	}
	return &storage.Object{Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b))}, nil
}

func (s *fakeStore) Delete(_ context.Context, key string) error {
	s.deleteKeys = append(s.deleteKeys, key)
	delete(s.blobs, key)
	return nil
}

type fakeCache struct{ items map[string]files.File }

func newFakeCache() *fakeCache { return &fakeCache{items: map[string]files.File{}} }

func (c *fakeCache) Get(_ context.Context, id string) (files.File, bool) {
	f, ok := c.items[id]
	return f, ok
}
func (c *fakeCache) Set(_ context.Context, f files.File) { c.items[f.ID] = f }
func (c *fakeCache) Delete(_ context.Context, id string) { delete(c.items, id) }

type fakePublisher struct{ types []string }

func (p *fakePublisher) Publish(_ context.Context, e events.Event) { p.types = append(p.types, e.Type) }
func (p *fakePublisher) Close() error                              { return nil }

type deps struct {
	repo  *fakeRepo
	store *fakeStore
	cache *fakeCache
	pub   *fakePublisher
	svc   *Service
}

func newSvc(t *testing.T, baseTTL time.Duration, maxUpload int64) deps {
	t.Helper()
	repo, store, cache, pub := newFakeRepo(), newFakeStore(), newFakeCache(), &fakePublisher{}
	return deps{repo, store, cache, pub, New(repo, store, cache, pub, baseTTL, maxUpload)}
}

func upload(t *testing.T, s *Service, body string, ttl time.Duration) (files.File, error) {
	t.Helper()
	return s.Upload(context.Background(), UploadInput{
		Filename: "f.txt", ContentType: "text/plain",
		Size: int64(len(body)), Body: strings.NewReader(body), TTL: ttl,
	})
}

// --- tests ---------------------------------------------------------------

func TestUploadTTL(t *testing.T) {
	cases := []struct {
		name       string
		baseTTL    time.Duration
		ttl        time.Duration
		wantExpiry bool
	}{
		{"default applied", time.Hour, 0, true},
		{"explicit ttl", time.Hour, 2 * time.Hour, true},
		{"never expire", time.Hour, -1, false},
		{"no default, no ttl", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newSvc(t, tc.baseTTL, 0)
			f, err := upload(t, d.svc, "hello", tc.ttl)
			if err != nil {
				t.Fatalf("Upload: %v", err)
			}
			if (f.ExpiresAt != nil) != tc.wantExpiry {
				t.Errorf("ExpiresAt present=%v, want %v", f.ExpiresAt != nil, tc.wantExpiry)
			}
			if tc.ttl > 0 && f.ExpiresAt != nil {
				want := f.CreatedAt.Add(tc.ttl)
				if !f.ExpiresAt.Equal(want) {
					t.Errorf("ExpiresAt = %v, want %v", f.ExpiresAt, want)
				}
			}
			if _, ok := d.repo.items[f.ID]; !ok {
				t.Error("record not persisted")
			}
			if string(d.store.blobs[f.StorageKey]) != "hello" {
				t.Error("bytes not stored")
			}
			if len(d.pub.types) != 1 || d.pub.types[0] != events.TypeFileUploaded {
				t.Errorf("events = %v, want [file.uploaded]", d.pub.types)
			}
		})
	}
}

func TestUploadTooLarge(t *testing.T) {
	d := newSvc(t, 0, 4) // 4-byte limit
	_, err := upload(t, d.svc, "hello", 0)
	if err != ErrTooLarge {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if len(d.store.blobs) != 0 || len(d.repo.items) != 0 {
		t.Error("oversized upload should store nothing")
	}
}

func TestUploadRepoFailureRollsBackBlob(t *testing.T) {
	d := newSvc(t, 0, 0)
	d.repo.insertErr = context.DeadlineExceeded
	_, err := upload(t, d.svc, "hello", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(d.store.deleteKeys) != 1 {
		t.Errorf("orphaned blob not cleaned up: deletes=%v", d.store.deleteKeys)
	}
	if len(d.pub.types) != 0 {
		t.Error("no event should be published on failure")
	}
}

func TestMetadataCacheMissPopulates(t *testing.T) {
	d := newSvc(t, 0, 0)
	f, _ := upload(t, d.svc, "hello", 0)
	delete(d.cache.items, f.ID) // force a miss; repo still has it

	got, err := d.svc.Metadata(context.Background(), f.ID)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.StorageKey != f.StorageKey {
		t.Errorf("StorageKey = %q, want %q", got.StorageKey, f.StorageKey)
	}
	if _, ok := d.cache.items[f.ID]; !ok {
		t.Error("cache not repopulated on miss")
	}
}

func TestMetadataExpiredIsNotFoundAndReaped(t *testing.T) {
	d := newSvc(t, 0, 0)
	f, _ := upload(t, d.svc, "hello", 0)
	// Backdate expiry into the past in both repo and cache.
	past := time.Now().UTC().Add(-time.Hour)
	f.ExpiresAt = &past
	d.repo.items[f.ID] = f
	d.cache.items[f.ID] = f

	_, err := d.svc.Metadata(context.Background(), f.ID)
	if err != files.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, ok := d.repo.items[f.ID]; ok {
		t.Error("expired file should be reaped from repo")
	}
}

func TestDeletePublishesAndUnknownIsNoop(t *testing.T) {
	d := newSvc(t, 0, 0)
	f, _ := upload(t, d.svc, "hello", 0)
	d.pub.types = nil // ignore the upload event

	if err := d.svc.Delete(context.Background(), f.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(d.pub.types) != 1 || d.pub.types[0] != events.TypeFileDeleted {
		t.Errorf("events = %v, want [file.deleted]", d.pub.types)
	}

	d.pub.types = nil
	if err := d.svc.Delete(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("Delete unknown: %v", err)
	}
	if len(d.pub.types) != 0 {
		t.Error("deleting an unknown id should publish nothing")
	}
}

func TestPurgeExpired(t *testing.T) {
	d := newSvc(t, 0, 0)
	d.repo.expired = []files.File{
		{ID: "a", StorageKey: "files/a"},
		{ID: "b", StorageKey: "files/b"},
	}
	n, err := d.svc.PurgeExpired(context.Background(), 10)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if n != 2 {
		t.Errorf("purged = %d, want 2", n)
	}
	if len(d.pub.types) != 2 || d.pub.types[0] != events.TypeFileExpired {
		t.Errorf("events = %v, want two file.expired", d.pub.types)
	}
	if len(d.store.deleteKeys) != 2 {
		t.Errorf("blob deletes = %v, want 2", d.store.deleteKeys)
	}
}
