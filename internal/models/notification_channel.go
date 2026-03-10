package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// NotificationChannel represents a configured notification destination for a project.
type NotificationChannel struct {
	ID              int64     `db:"id"`
	ProjectID       int64     `db:"project_id"`
	Name            string    `db:"name"`
	Type            string    `db:"type"`
	ConfigEncrypted []byte    `db:"config_encrypted"`
	OnSuccess       bool      `db:"on_success"`
	OnFailure       bool      `db:"on_failure"`
	OnCancelled     bool      `db:"on_cancelled"`
	Enabled         bool      `db:"enabled"`
	CreatedBy       int64     `db:"created_by"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// NotificationChannelRepository defines the interface for notification channel data access.
type NotificationChannelRepository interface {
	Create(ctx context.Context, channel *NotificationChannel) error
	GetByID(ctx context.Context, id int64) (*NotificationChannel, error)
	ListByProject(ctx context.Context, projectID int64) ([]*NotificationChannel, error)
	Update(ctx context.Context, channel *NotificationChannel) error
	Delete(ctx context.Context, id int64) error
	CountByProject(ctx context.Context, projectID int64) (int, error)
}

// SQLiteNotificationChannelRepository implements NotificationChannelRepository using SQLite.
type SQLiteNotificationChannelRepository struct {
	db *sqlx.DB
}

// NewNotificationChannelRepository creates a new SQLite-backed notification channel repository.
func NewNotificationChannelRepository(db *sqlx.DB) *SQLiteNotificationChannelRepository {
	return &SQLiteNotificationChannelRepository{db: db}
}

// Create inserts a new notification channel.
func (r *SQLiteNotificationChannelRepository) Create(ctx context.Context, channel *NotificationChannel) error {
	query := `
		INSERT INTO notification_channels (project_id, name, type, config_encrypted, on_success, on_failure, on_cancelled, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		channel.ProjectID,
		channel.Name,
		channel.Type,
		channel.ConfigEncrypted,
		channel.OnSuccess,
		channel.OnFailure,
		channel.OnCancelled,
		channel.Enabled,
		channel.CreatedBy,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	channel.ID = id
	return nil
}

// GetByID retrieves a notification channel by ID.
func (r *SQLiteNotificationChannelRepository) GetByID(ctx context.Context, id int64) (*NotificationChannel, error) {
	var ch NotificationChannel
	query := `SELECT * FROM notification_channels WHERE id = ?`
	err := r.db.GetContext(ctx, &ch, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// ListByProject retrieves all notification channels for a project, ordered by name.
func (r *SQLiteNotificationChannelRepository) ListByProject(ctx context.Context, projectID int64) ([]*NotificationChannel, error) {
	var channels []*NotificationChannel
	query := `SELECT * FROM notification_channels WHERE project_id = ? ORDER BY name ASC`
	err := r.db.SelectContext(ctx, &channels, query, projectID)
	if err != nil {
		return nil, err
	}
	return channels, nil
}

// Update updates a notification channel.
func (r *SQLiteNotificationChannelRepository) Update(ctx context.Context, channel *NotificationChannel) error {
	query := `
		UPDATE notification_channels
		SET name = ?, config_encrypted = ?, on_success = ?, on_failure = ?, on_cancelled = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query,
		channel.Name,
		channel.ConfigEncrypted,
		channel.OnSuccess,
		channel.OnFailure,
		channel.OnCancelled,
		channel.Enabled,
		channel.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a notification channel by ID.
func (r *SQLiteNotificationChannelRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM notification_channels WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByProject returns the number of notification channels for a project.
func (r *SQLiteNotificationChannelRepository) CountByProject(ctx context.Context, projectID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM notification_channels WHERE project_id = ?`
	err := r.db.GetContext(ctx, &count, query, projectID)
	return count, err
}
