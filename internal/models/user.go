// Package models provides data models and repositories for FeatherCI.
package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// User represents an authenticated user in the system.
type User struct {
	ID           int64     `db:"id"`
	Provider     string    `db:"provider"`
	ProviderID   string    `db:"provider_id"`
	Username     string    `db:"username"`
	Email        string    `db:"email"`
	AvatarURL    string    `db:"avatar_url"`
	AccessToken  string    `db:"access_token"`
	RefreshToken string    `db:"refresh_token"`
	IsAdmin      bool      `db:"is_admin"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// UserRepository defines the interface for user data access.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByProviderID(ctx context.Context, provider, providerID string) (*User, error)
	Update(ctx context.Context, user *User) error
	UpdateTokens(ctx context.Context, id int64, accessToken, refreshToken string) error
	List(ctx context.Context) ([]*User, error)
	Delete(ctx context.Context, id int64) error
}

// SQLiteUserRepository implements UserRepository using SQLite.
type SQLiteUserRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new SQLite-backed user repository.
func NewUserRepository(db *sqlx.DB) *SQLiteUserRepository {
	return &SQLiteUserRepository{db: db}
}

// Create inserts a new user into the database.
func (r *SQLiteUserRepository) Create(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (provider, provider_id, username, email, avatar_url, access_token, refresh_token, is_admin, created_at, updated_at)
		VALUES (:provider, :provider_id, :username, :email, :avatar_url, :access_token, :refresh_token, :is_admin, :created_at, :updated_at)
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	result, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id

	return nil
}

// GetByID retrieves a user by their ID.
func (r *SQLiteUserRepository) GetByID(ctx context.Context, id int64) (*User, error) {
	var user User
	query := `SELECT * FROM users WHERE id = ?`

	err := r.db.GetContext(ctx, &user, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetByProviderID retrieves a user by their provider and provider-specific ID.
func (r *SQLiteUserRepository) GetByProviderID(ctx context.Context, provider, providerID string) (*User, error) {
	var user User
	query := `SELECT * FROM users WHERE provider = ? AND provider_id = ?`

	err := r.db.GetContext(ctx, &user, query, provider, providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// Update updates an existing user's information.
func (r *SQLiteUserRepository) Update(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET username = :username,
		    email = :email,
		    avatar_url = :avatar_url,
		    access_token = :access_token,
		    refresh_token = :refresh_token,
		    is_admin = :is_admin,
		    updated_at = :updated_at
		WHERE id = :id
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.NamedExecContext(ctx, query, user)
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

// UpdateTokens updates only the access and refresh tokens for a user.
func (r *SQLiteUserRepository) UpdateTokens(ctx context.Context, id int64, accessToken, refreshToken string) error {
	query := `UPDATE users SET access_token = ?, refresh_token = ?, updated_at = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, accessToken, refreshToken, time.Now(), id)
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

// List retrieves all users from the database.
func (r *SQLiteUserRepository) List(ctx context.Context) ([]*User, error) {
	var users []*User
	query := `SELECT * FROM users ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, err
	}

	return users, nil
}

// Delete removes a user from the database.
func (r *SQLiteUserRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = ?`

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
