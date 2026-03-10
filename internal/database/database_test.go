package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestOpenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestMigrate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Check current version
	version, err := db.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion() error = %v", err)
	}
	if version != 2 {
		t.Errorf("CurrentVersion() = %d, want 2", version)
	}

	// Verify tables exist
	tables := []string{"users", "projects", "project_users", "secrets", "builds", "build_steps", "step_dependencies", "workers"}
	for _, table := range tables {
		var count int
		err := db.Get(&count, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
		if err != nil {
			t.Errorf("Failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Table %s does not exist", table)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Run migrations twice - should be idempotent
	if err := db.Migrate(); err != nil {
		t.Fatalf("First Migrate() error = %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Second Migrate() error = %v", err)
	}

	version, err := db.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion() error = %v", err)
	}
	if version != 2 {
		t.Errorf("CurrentVersion() = %d, want 2", version)
	}
}

func TestRollback(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Rollback one migration
	if err := db.Rollback(1); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	version, err := db.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion() error = %v", err)
	}
	if version != 1 {
		t.Errorf("CurrentVersion() after rollback = %d, want 1", version)
	}
}

func TestForeignKeys(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Insert a project
	result, err := db.Exec(`
		INSERT INTO projects (provider, namespace, name, full_name, clone_url)
		VALUES ('github', 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo.git')
	`)
	if err != nil {
		t.Fatalf("Insert project error = %v", err)
	}
	projectID, _ := result.LastInsertId()

	// Insert a build for the project
	result, err = db.Exec(`
		INSERT INTO builds (project_id, build_number, commit_sha)
		VALUES (?, 1, 'abc123')
	`, projectID)
	if err != nil {
		t.Fatalf("Insert build error = %v", err)
	}

	// Delete the project - should cascade delete the build
	_, err = db.Exec("DELETE FROM projects WHERE id = ?", projectID)
	if err != nil {
		t.Fatalf("Delete project error = %v", err)
	}

	// Verify build was deleted
	var count int
	db.Get(&count, "SELECT COUNT(*) FROM builds")
	if count != 0 {
		t.Error("Build was not cascade deleted when project was deleted")
	}
}

func TestIndexesExist(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	indexes := []string{
		"idx_builds_project_id",
		"idx_builds_status",
		"idx_builds_created_at",
		"idx_build_steps_build_id",
		"idx_build_steps_status",
		"idx_project_users_user_id",
		"idx_secrets_project_id",
		"idx_users_username",
	}

	for _, idx := range indexes {
		var count int
		err := db.Get(&count, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx)
		if err != nil {
			t.Errorf("Failed to check index %s: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("Index %s does not exist", idx)
		}
	}
}

func TestPendingMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Before migrations
	pending, err := db.PendingMigrations()
	if err != nil {
		t.Fatalf("PendingMigrations() error = %v", err)
	}
	if pending != 2 {
		t.Errorf("PendingMigrations() before migrate = %d, want 2", pending)
	}

	// After migrations
	db.Migrate()
	pending, err = db.PendingMigrations()
	if err != nil {
		t.Fatalf("PendingMigrations() error = %v", err)
	}
	if pending != 0 {
		t.Errorf("PendingMigrations() after migrate = %d, want 0", pending)
	}
}
