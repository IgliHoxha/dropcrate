// Package cache stores file metadata in Redis to avoid a MySQL round-trip on
// hot downloads. Every method is best-effort: a cache error never breaks a
// request, it just falls through to the source of truth.
package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/IgliHoxha/dropcrate/internal/config"
	"github.com/IgliHoxha/dropcrate/internal/files"
)

// MetadataCache caches files.File records keyed by id.
type MetadataCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// New builds a MetadataCache from Redis configuration.
func New(cfg config.RedisConfig, ttl time.Duration) *MetadataCache {
	return &MetadataCache{
		rdb: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
		ttl: ttl,
	}
}

// Ping verifies connectivity to Redis.
func (c *MetadataCache) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close releases the underlying connection pool.
func (c *MetadataCache) Close() error {
	return c.rdb.Close()
}

func key(id string) string { return "file:" + id }

// entry is the cache's own serialization of a file. It is deliberately separate
// from files.File's JSON tags: the API type hides StorageKey with `json:"-"`,
// but the cache must persist it so a cache hit can still locate the blob.
type entry struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	StorageKey  string     `json:"storage_key"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Get returns a cached file and whether it was present.
func (c *MetadataCache) Get(ctx context.Context, id string) (files.File, bool) {
	raw, err := c.rdb.Get(ctx, key(id)).Bytes()
	if err != nil {
		return files.File{}, false
	}
	var e entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return files.File{}, false
	}
	return files.File{
		ID:          e.ID,
		Filename:    e.Filename,
		ContentType: e.ContentType,
		Size:        e.Size,
		StorageKey:  e.StorageKey,
		CreatedAt:   e.CreatedAt,
		ExpiresAt:   e.ExpiresAt,
	}, true
}

// Set caches a file record for the configured TTL.
func (c *MetadataCache) Set(ctx context.Context, f files.File) {
	raw, err := json.Marshal(entry{
		ID:          f.ID,
		Filename:    f.Filename,
		ContentType: f.ContentType,
		Size:        f.Size,
		StorageKey:  f.StorageKey,
		CreatedAt:   f.CreatedAt,
		ExpiresAt:   f.ExpiresAt,
	})
	if err != nil {
		return
	}
	_ = c.rdb.Set(ctx, key(f.ID), raw, c.ttl).Err()
}

// Delete evicts a cached record.
func (c *MetadataCache) Delete(ctx context.Context, id string) {
	_ = c.rdb.Del(ctx, key(id)).Err()
}
