package database

import (
	"fmt"
	"sort"
)

// Migration represents a database schema migration.
type Migration struct {
	Version     int
	Description string
	Up          string
	Down        string
}

// migrations holds all registered migrations in order.
var migrations []Migration

// RegisterMigration adds a migration to the list.
// This should be called from init() functions in migration files.
func RegisterMigration(m Migration) {
	migrations = append(migrations, m)
}

// Migrate applies all pending migrations to the database.
func (db *DB) Migrate() error {
	// Ensure migrations table exists
	if err := db.ensureMigrationsTable(); err != nil {
		return err
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	// Get current version
	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return err
	}

	// Apply pending migrations
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		fmt.Printf("Applying migration %d: %s\n", m.Version, m.Description)

		// Execute migration in a transaction
		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}

// Rollback reverts the last n migrations.
func (db *DB) Rollback(steps int) error {
	if steps <= 0 {
		return nil
	}

	// Sort migrations by version descending for rollback
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version > migrations[j].Version
	})

	currentVersion, err := db.getCurrentVersion()
	if err != nil {
		return err
	}

	rolled := 0
	for _, m := range migrations {
		if m.Version > currentVersion {
			continue
		}
		if rolled >= steps {
			break
		}

		fmt.Printf("Rolling back migration %d: %s\n", m.Version, m.Description)

		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for rollback %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.Down); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to rollback migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", m.Version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to remove migration record %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback %d: %w", m.Version, err)
		}

		rolled++
	}

	return nil
}

// CurrentVersion returns the current schema version.
func (db *DB) CurrentVersion() (int, error) {
	if err := db.ensureMigrationsTable(); err != nil {
		return 0, err
	}
	return db.getCurrentVersion()
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist.
func (db *DB) ensureMigrationsTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// getCurrentVersion returns the highest applied migration version.
func (db *DB) getCurrentVersion() (int, error) {
	var version int
	err := db.Get(&version, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	return version, err
}

// PendingMigrations returns the number of migrations waiting to be applied.
func (db *DB) PendingMigrations() (int, error) {
	currentVersion, err := db.CurrentVersion()
	if err != nil {
		return 0, err
	}

	pending := 0
	for _, m := range migrations {
		if m.Version > currentVersion {
			pending++
		}
	}
	return pending, nil
}
