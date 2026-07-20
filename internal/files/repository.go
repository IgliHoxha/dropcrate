package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository persists file metadata in MySQL.
type Repository struct {
	db *sql.DB
}

// NewRepository wraps an open *sql.DB.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Insert stores a new file record.
func (r *Repository) Insert(ctx context.Context, f File) error {
	const q = `
		INSERT INTO files (id, filename, content_type, size, storage_key, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, q,
		f.ID, f.Filename, f.ContentType, f.Size, f.StorageKey, f.CreatedAt, f.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	return nil
}

// GetByID returns the file with the given id, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id string) (File, error) {
	const q = `
		SELECT id, filename, content_type, size, storage_key, created_at, expires_at
		FROM files WHERE id = ?`

	var f File
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.StorageKey, &f.CreatedAt, &f.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("get file: %w", err)
	}
	return f, nil
}

// Delete removes a file record. Deleting a missing id is not an error.
func (r *Repository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM files WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// StorageKeysExpiredBefore returns the storage keys and ids of every file
// whose expiry has already passed, so a sweeper can reclaim their bytes.
func (r *Repository) StorageKeysExpiredBefore(ctx context.Context, limit int) ([]File, error) {
	const q = `
		SELECT id, storage_key FROM files
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
		LIMIT ?`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired files: %w", err)
	}
	defer rows.Close()

	var out []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
