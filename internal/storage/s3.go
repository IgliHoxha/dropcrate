package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/IgliHoxha/dropcrate/internal/config"
)

// S3Storage is an S3-compatible implementation of Storage backed by minio-go.
type S3Storage struct {
	client *minio.Client
	bucket string
}

// NewS3 constructs an S3Storage and ensures the target bucket exists.
func NewS3(ctx context.Context, cfg config.S3Config) (*S3Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	s := &S3Storage{client: client, bucket: cfg.Bucket}
	if err := s.ensureBucket(ctx, cfg.Region); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *S3Storage) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		return fmt.Errorf("create bucket %q: %w", s.bucket, err)
	}
	return nil
}

// Ping verifies the object store is reachable and the bucket is accessible.
// It is used by readiness checks, not by the core Storage interface.
func (s *S3Storage) Ping(ctx context.Context) error {
	if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("s3 ping: %w", err)
	}
	return nil
}

// Put streams r into the object store under key and returns the bytes written.
// size may be -1 for an unknown length, in which case minio-go uploads in
// multiple parts.
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return 0, fmt.Errorf("put object %q: %w", key, err)
	}
	return info.Size, nil
}

// Get opens the object at key. The caller owns and must close Object.Body.
func (s *S3Storage) Get(ctx context.Context, key string) (*Object, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}

	// GetObject is lazy; Stat forces the request and surfaces a missing key.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("stat object %q: %w", key, err)
	}

	return &Object{
		Body:          obj,
		ContentType:   info.ContentType,
		ContentLength: info.Size,
	}, nil
}

// Delete removes the object at key. Deleting a missing key is not an error.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
