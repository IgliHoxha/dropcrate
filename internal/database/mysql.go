// Package database provides the MySQL connection and schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Registers the "mysql" driver with database/sql.
	_ "github.com/go-sql-driver/mysql"
)

// Open dials MySQL, verifies connectivity, and applies connection-pool
// defaults suitable for a small service.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}
