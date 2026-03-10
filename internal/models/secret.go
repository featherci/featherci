package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// Secret represents an encrypted secret associated with a project.
type Secret struct {
	ID             int64     `db:"id"`
	ProjectID      int64     `db:"project_id"`
	Name           string    `db:"name"`
	EncryptedValue []byte    `db:"encrypted_value"`
	CreatedBy      int64     `db:"created_by"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// SecretRepository defines the interface for secret data access.
type SecretRepository interface {
	Create(ctx context.Context, secret *Secret) error
	GetByName(ctx context.Context, projectID int64, name string) (*Secret, error)
	ListByProject(ctx context.Context, projectID int64) ([]*Secret, error)
	Update(ctx context.Context, secret *Secret) error
	Delete(ctx context.Context, projectID int64, name string) error
}

// SQLiteSecretRepository implements SecretRepository using SQLite.
type SQLiteSecretRepository struct {
	db *sqlx.DB
}

// NewSecretRepository creates a new SQLite-backed secret repository.
func NewSecretRepository(db *sqlx.DB) *SQLiteSecretRepository {
	return &SQLiteSecretRepository{db: db}
}

// Create inserts a new secret.
func (r *SQLiteSecretRepository) Create(ctx context.Context, secret *Secret) error {
	query := `
		INSERT INTO secrets (project_id, name, encrypted_value, created_by)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		secret.ProjectID,
		secret.Name,
		secret.EncryptedValue,
		secret.CreatedBy,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	secret.ID = id
	return nil
}

// GetByName retrieves a secret by project ID and name.
func (r *SQLiteSecretRepository) GetByName(ctx context.Context, projectID int64, name string) (*Secret, error) {
	var s Secret
	query := `SELECT * FROM secrets WHERE project_id = ? AND name = ?`
	err := r.db.GetContext(ctx, &s, query, projectID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByProject retrieves all secrets for a project, ordered by name.
func (r *SQLiteSecretRepository) ListByProject(ctx context.Context, projectID int64) ([]*Secret, error) {
	var secrets []*Secret
	query := `SELECT * FROM secrets WHERE project_id = ? ORDER BY name ASC`
	err := r.db.SelectContext(ctx, &secrets, query, projectID)
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// Update updates an existing secret's encrypted value.
func (r *SQLiteSecretRepository) Update(ctx context.Context, secret *Secret) error {
	query := `UPDATE secrets SET encrypted_value = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, secret.EncryptedValue, secret.ID)
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

// Delete removes a secret by project ID and name.
func (r *SQLiteSecretRepository) Delete(ctx context.Context, projectID int64, name string) error {
	query := `DELETE FROM secrets WHERE project_id = ? AND name = ?`
	result, err := r.db.ExecContext(ctx, query, projectID, name)
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
