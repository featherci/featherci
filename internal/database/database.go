// Package database provides SQLite database connection and management.
package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// DB wraps sqlx.DB with FeatherCI-specific functionality.
type DB struct {
	*sqlx.DB
}

// Open creates a new database connection.
// The path should be a file path for persistent storage, or ":memory:" for testing.
func Open(path string) (*DB, error) {
	// SQLite connection string with recommended settings
	dsn := fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)

	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings (SQLite works best with single connection for writes)
	db.SetMaxOpenConns(1)

	// Verify connection works
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.DB.Ping()
}
