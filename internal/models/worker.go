package models

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// WorkerStatus represents the current state of a worker.
type WorkerStatus string

const (
	WorkerStatusOffline WorkerStatus = "offline"
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
)

// Worker represents a build execution agent.
type Worker struct {
	ID            string       `db:"id"`
	Name          string       `db:"name"`
	Status        WorkerStatus `db:"status"`
	LastHeartbeat *time.Time   `db:"last_heartbeat"`
	CurrentStepID *int64       `db:"current_step_id"`
	CreatedAt     time.Time    `db:"created_at"`
}

// WorkerRepository defines the interface for worker data access.
type WorkerRepository interface {
	Register(ctx context.Context, worker *Worker) error
	UpdateHeartbeat(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status WorkerStatus, currentStepID *int64) error
	SetOffline(ctx context.Context, id string) error
}

// SQLiteWorkerRepository implements WorkerRepository using SQLite.
type SQLiteWorkerRepository struct {
	db *sqlx.DB
}

// NewWorkerRepository creates a new SQLite-backed worker repository.
func NewWorkerRepository(db *sqlx.DB) *SQLiteWorkerRepository {
	return &SQLiteWorkerRepository{db: db}
}

// Register inserts or replaces a worker record for idempotent restarts.
func (r *SQLiteWorkerRepository) Register(ctx context.Context, worker *Worker) error {
	now := time.Now()
	query := `INSERT OR REPLACE INTO workers (id, name, status, last_heartbeat, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, worker.ID, worker.Name, worker.Status, now, now)
	if err != nil {
		return err
	}
	worker.CreatedAt = now
	return nil
}

// UpdateHeartbeat updates the last_heartbeat timestamp for a worker.
func (r *SQLiteWorkerRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	now := time.Now()
	query := `UPDATE workers SET last_heartbeat = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, now, id)
	return err
}

// UpdateStatus updates a worker's status and current step ID.
func (r *SQLiteWorkerRepository) UpdateStatus(ctx context.Context, id string, status WorkerStatus, currentStepID *int64) error {
	query := `UPDATE workers SET status = ?, current_step_id = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, status, currentStepID, id)
	return err
}

// SetOffline sets a worker's status to offline and clears the current step.
func (r *SQLiteWorkerRepository) SetOffline(ctx context.Context, id string) error {
	query := `UPDATE workers SET status = 'offline', current_step_id = NULL WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
