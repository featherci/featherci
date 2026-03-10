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
	ListStale(ctx context.Context, threshold time.Duration) ([]*Worker, error)
	List(ctx context.Context) ([]*Worker, error)
	CountActive(ctx context.Context) (int, error)
	PurgeOffline(ctx context.Context, olderThan time.Duration) (int64, error)
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

// List returns all workers.
func (r *SQLiteWorkerRepository) List(ctx context.Context) ([]*Worker, error) {
	var workers []*Worker
	query := `SELECT * FROM workers ORDER BY name ASC`
	if err := r.db.SelectContext(ctx, &workers, query); err != nil {
		return nil, err
	}
	return workers, nil
}

// CountActive returns the number of non-offline workers.
func (r *SQLiteWorkerRepository) CountActive(ctx context.Context) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM workers WHERE status != 'offline'`
	err := r.db.GetContext(ctx, &count, query)
	return count, err
}

// PurgeOffline deletes offline workers whose last heartbeat is older than the given duration.
func (r *SQLiteWorkerRepository) PurgeOffline(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	query := `DELETE FROM workers WHERE status = 'offline' AND (last_heartbeat IS NULL OR last_heartbeat < ?)`
	result, err := r.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListStale returns workers whose last heartbeat is older than the threshold.
func (r *SQLiteWorkerRepository) ListStale(ctx context.Context, threshold time.Duration) ([]*Worker, error) {
	cutoff := time.Now().Add(-threshold)
	query := `SELECT * FROM workers WHERE status != 'offline' AND last_heartbeat IS NOT NULL AND last_heartbeat < ?`
	var workers []*Worker
	if err := r.db.SelectContext(ctx, &workers, query, cutoff); err != nil {
		return nil, err
	}
	return workers, nil
}
