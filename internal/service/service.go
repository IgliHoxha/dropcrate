// Package service wires the object store, metadata repository, and cache into
// the upload/download/delete use cases the API exposes.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/IgliHoxha/dropcrate/internal/events"
	"github.com/IgliHoxha/dropcrate/internal/files"
	"github.com/IgliHoxha/dropcrate/internal/storage"
)

// ErrTooLarge is returned when an upload exceeds the configured size limit.
var ErrTooLarge = errors.New("file too large")

// Repository is the metadata persistence the service depends on.
// *files.Repository satisfies it.
type Repository interface {
	Insert(ctx context.Context, f files.File) error
	GetByID(ctx context.Context, id string) (files.File, error)
	Delete(ctx context.Context, id string) error
	StorageKeysExpiredBefore(ctx context.Context, limit int) ([]files.File, error)
}

// Cache is the metadata cache the service depends on.
// *cache.MetadataCache satisfies it.
type Cache interface {
	Get(ctx context.Context, id string) (files.File, bool)
	Set(ctx context.Context, f files.File)
	Delete(ctx context.Context, id string)
}

// Service coordinates the persistence layers for a single file.
type Service struct {
	repo      Repository
	store     storage.Storage
	cache     Cache
	events    events.Publisher
	baseTTL   time.Duration
	maxUpload int64
}

// New constructs a Service. defaultTTL is applied when an upload does not
// request an explicit lifetime; pass 0 to make files permanent by default.
// publisher receives best-effort domain events; pass events.Nop{} to disable.
// maxUpload rejects uploads whose declared size exceeds it; 0 means unlimited.
func New(repo Repository, store storage.Storage, cache Cache, publisher events.Publisher, defaultTTL time.Duration, maxUpload int64) *Service {
	return &Service{repo: repo, store: store, cache: cache, events: publisher, baseTTL: defaultTTL, maxUpload: maxUpload}
}

// MaxUpload returns the configured upload size limit in bytes (0 = unlimited),
// so streaming transports can enforce it before buffering an entire upload.
func (s *Service) MaxUpload() int64 { return s.maxUpload }

// UploadInput carries everything needed to store one file.
type UploadInput struct {
	Filename    string
	ContentType string
	Size        int64
	Body        io.Reader
	// TTL overrides the service default. A negative TTL means "never expire".
	TTL time.Duration
}

// Upload streams the body into object storage and records its metadata. If the
// metadata write fails, the orphaned blob is cleaned up.
func (s *Service) Upload(ctx context.Context, in UploadInput) (files.File, error) {
	if s.maxUpload > 0 && in.Size > s.maxUpload {
		return files.File{}, ErrTooLarge
	}

	id := uuid.NewString()
	f := files.File{
		ID:          id,
		Filename:    in.Filename,
		ContentType: defaultString(in.ContentType, "application/octet-stream"),
		Size:        in.Size,
		StorageKey:  "files/" + id,
		CreatedAt:   time.Now().UTC(),
	}

	switch {
	case in.TTL < 0:
		// explicit "never expire": leave ExpiresAt nil
	case in.TTL > 0:
		exp := f.CreatedAt.Add(in.TTL)
		f.ExpiresAt = &exp
	case s.baseTTL > 0:
		exp := f.CreatedAt.Add(s.baseTTL)
		f.ExpiresAt = &exp
	}

	written, err := s.store.Put(ctx, f.StorageKey, in.Body, in.Size, f.ContentType)
	if err != nil {
		return files.File{}, fmt.Errorf("store bytes: %w", err)
	}
	// The declared size may be unknown (-1) for streamed uploads; record what
	// was actually written, and enforce the limit now that it is known.
	f.Size = written
	if s.maxUpload > 0 && written > s.maxUpload {
		_ = s.store.Delete(ctx, f.StorageKey)
		return files.File{}, ErrTooLarge
	}

	if err := s.repo.Insert(ctx, f); err != nil {
		// Best-effort rollback of the blob we just wrote.
		_ = s.store.Delete(ctx, f.StorageKey)
		return files.File{}, err
	}

	s.cache.Set(ctx, f)
	s.events.Publish(ctx, events.FromFile(events.TypeFileUploaded, f, time.Now().UTC()))
	return f, nil
}

// Metadata returns a file's record, consulting the cache first. Expired files
// are treated as not found and reaped opportunistically.
func (s *Service) Metadata(ctx context.Context, id string) (files.File, error) {
	f, ok := s.cache.Get(ctx, id)
	if !ok {
		var err error
		f, err = s.repo.GetByID(ctx, id)
		if err != nil {
			return files.File{}, err
		}
		s.cache.Set(ctx, f)
	}

	if f.Expired(time.Now().UTC()) {
		_ = s.Delete(ctx, id)
		return files.File{}, files.ErrNotFound
	}
	return f, nil
}

// Download returns a file's metadata alongside an open reader for its bytes.
// The caller must close Object.Body.
func (s *Service) Download(ctx context.Context, id string) (files.File, *storage.Object, error) {
	f, err := s.Metadata(ctx, id)
	if err != nil {
		return files.File{}, nil, err
	}

	obj, err := s.store.Get(ctx, f.StorageKey)
	if err != nil {
		return files.File{}, nil, err
	}
	return f, obj, nil
}

// Delete removes a file's bytes, metadata, and cache entry.
func (s *Service) Delete(ctx context.Context, id string) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == files.ErrNotFound {
			return nil
		}
		return err
	}

	_ = s.store.Delete(ctx, f.StorageKey)
	s.cache.Delete(ctx, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.events.Publish(ctx, events.FromFile(events.TypeFileDeleted, f, time.Now().UTC()))
	return nil
}

// PurgeExpired reclaims up to batch files whose expiry has passed, removing
// their bytes, cache entries, and metadata. It returns the number purged so a
// caller can loop until a batch comes back short (fully drained).
func (s *Service) PurgeExpired(ctx context.Context, batch int) (int, error) {
	expired, err := s.repo.StorageKeysExpiredBefore(ctx, batch)
	if err != nil {
		return 0, err
	}

	purged := 0
	for _, f := range expired {
		_ = s.store.Delete(ctx, f.StorageKey)
		s.cache.Delete(ctx, f.ID)
		if err := s.repo.Delete(ctx, f.ID); err != nil {
			return purged, fmt.Errorf("purge %s: %w", f.ID, err)
		}
		// Expired records carry only id/storage key, so the event is id-centric.
		s.events.Publish(ctx, events.FromFile(events.TypeFileExpired, f, time.Now().UTC()))
		purged++
	}
	return purged, nil
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
