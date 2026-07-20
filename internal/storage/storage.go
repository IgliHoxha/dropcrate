// Package storage abstracts the object store that holds file bytes.
package storage

import (
	"context"
	"io"
)

// Object is a stored blob returned from Get. Callers must close Body.
type Object struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

// Storage is the minimal object-store contract dropcrate depends on. Any
// S3-compatible backend (MinIO, AWS S3, Ceph, ...) can satisfy it.
type Storage interface {
	// Put streams r into the store under key and returns the number of bytes
	// written. A size of -1 means the length is unknown (streamed), in which
	// case the backend determines it while reading.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error)
	Get(ctx context.Context, key string) (*Object, error)
	Delete(ctx context.Context, key string) error
}
