package models

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func setupWorkerTestDB(t *testing.T) *sqlx.DB {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	schema := `
		CREATE TABLE build_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			build_id INTEGER NOT NULL,
			name TEXT NOT NULL
		);

		CREATE TABLE workers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT DEFAULT 'offline',
			last_heartbeat DATETIME,
			current_step_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (current_step_id) REFERENCES build_steps(id)
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestWorkerRepository_Register(t *testing.T) {
	db := setupWorkerTestDB(t)
	defer db.Close()

	repo := NewWorkerRepository(db)
	ctx := context.Background()

	w := &Worker{
		ID:     "worker-abc123",
		Name:   "testhost-abc123",
		Status: WorkerStatusIdle,
	}

	if err := repo.Register(ctx, w); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify worker was inserted
	var got Worker
	if err := db.Get(&got, "SELECT * FROM workers WHERE id = ?", w.ID); err != nil {
		t.Fatalf("failed to query worker: %v", err)
	}
	if got.Name != w.Name {
		t.Errorf("got name %q, want %q", got.Name, w.Name)
	}
	if got.Status != WorkerStatusIdle {
		t.Errorf("got status %q, want %q", got.Status, WorkerStatusIdle)
	}

	// Re-register (idempotent)
	w.Name = "updated-name"
	if err := repo.Register(ctx, w); err != nil {
		t.Fatalf("re-Register failed: %v", err)
	}

	if err := db.Get(&got, "SELECT * FROM workers WHERE id = ?", w.ID); err != nil {
		t.Fatalf("failed to query after re-register: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("got name %q after re-register, want %q", got.Name, "updated-name")
	}
}

func TestWorkerRepository_UpdateHeartbeat(t *testing.T) {
	db := setupWorkerTestDB(t)
	defer db.Close()

	repo := NewWorkerRepository(db)
	ctx := context.Background()

	w := &Worker{ID: "worker-hb", Name: "host-hb", Status: WorkerStatusIdle}
	if err := repo.Register(ctx, w); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := repo.UpdateHeartbeat(ctx, w.ID); err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	var got Worker
	if err := db.Get(&got, "SELECT * FROM workers WHERE id = ?", w.ID); err != nil {
		t.Fatalf("failed to query worker: %v", err)
	}
	if got.LastHeartbeat == nil {
		t.Error("expected last_heartbeat to be set")
	}
}

func TestWorkerRepository_UpdateStatus(t *testing.T) {
	db := setupWorkerTestDB(t)
	defer db.Close()

	repo := NewWorkerRepository(db)
	ctx := context.Background()

	w := &Worker{ID: "worker-st", Name: "host-st", Status: WorkerStatusIdle}
	if err := repo.Register(ctx, w); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stepID := int64(42)
	if err := repo.UpdateStatus(ctx, w.ID, WorkerStatusBusy, &stepID); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	var got Worker
	if err := db.Get(&got, "SELECT * FROM workers WHERE id = ?", w.ID); err != nil {
		t.Fatalf("failed to query worker: %v", err)
	}
	if got.Status != WorkerStatusBusy {
		t.Errorf("got status %q, want %q", got.Status, WorkerStatusBusy)
	}
	if got.CurrentStepID == nil || *got.CurrentStepID != stepID {
		t.Errorf("got current_step_id %v, want %d", got.CurrentStepID, stepID)
	}
}

func TestWorkerRepository_SetOffline(t *testing.T) {
	db := setupWorkerTestDB(t)
	defer db.Close()

	repo := NewWorkerRepository(db)
	ctx := context.Background()

	w := &Worker{ID: "worker-off", Name: "host-off", Status: WorkerStatusBusy}
	if err := repo.Register(ctx, w); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := repo.SetOffline(ctx, w.ID); err != nil {
		t.Fatalf("SetOffline failed: %v", err)
	}

	var got Worker
	if err := db.Get(&got, "SELECT * FROM workers WHERE id = ?", w.ID); err != nil {
		t.Fatalf("failed to query worker: %v", err)
	}
	if got.Status != WorkerStatusOffline {
		t.Errorf("got status %q, want %q", got.Status, WorkerStatusOffline)
	}
	if got.CurrentStepID != nil {
		t.Errorf("expected current_step_id to be nil, got %v", got.CurrentStepID)
	}
}
