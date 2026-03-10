package models

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// Project represents a repository connected to FeatherCI.
type Project struct {
	ID            int64     `db:"id"`
	Provider      string    `db:"provider"`
	Namespace     string    `db:"namespace"`
	Name          string    `db:"name"`
	FullName      string    `db:"full_name"`
	CloneURL      string    `db:"clone_url"`
	WebhookSecret string    `db:"webhook_secret"`
	DefaultBranch string    `db:"default_branch"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// ProjectUser represents a user's access to a project.
type ProjectUser struct {
	ProjectID int64     `db:"project_id"`
	UserID    int64     `db:"user_id"`
	CanManage bool      `db:"can_manage"`
	CreatedAt time.Time `db:"created_at"`
}

// ProjectWithStatus extends Project with the latest build status.
type ProjectWithStatus struct {
	Project
	LastBuildStatus *string    `db:"last_build_status"`
	LastBuildAt     *time.Time `db:"last_build_at"`
}

// ProjectRepository defines the interface for project operations.
type ProjectRepository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id int64) (*Project, error)
	GetByFullName(ctx context.Context, provider, fullName string) (*Project, error)
	List(ctx context.Context) ([]*Project, error)
	ListWithStatus(ctx context.Context) ([]*ProjectWithStatus, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, id int64) error
}

// ProjectUserRepository defines the interface for project-user association operations.
type ProjectUserRepository interface {
	Add(ctx context.Context, projectID, userID int64, canManage bool) error
	Remove(ctx context.Context, projectID, userID int64) error
	GetUsersForProject(ctx context.Context, projectID int64) ([]*User, error)
	GetProjectsForUser(ctx context.Context, userID int64) ([]*Project, error)
	GetProjectsForUserWithStatus(ctx context.Context, userID int64) ([]*ProjectWithStatus, error)
	CanUserAccess(ctx context.Context, projectID, userID int64) (bool, error)
	CanUserManage(ctx context.Context, projectID, userID int64) (bool, error)
}

// SQLiteProjectRepository implements ProjectRepository using SQLite.
type SQLiteProjectRepository struct {
	db *sqlx.DB
}

// NewProjectRepository creates a new SQLite-backed project repository.
func NewProjectRepository(db *sqlx.DB) *SQLiteProjectRepository {
	return &SQLiteProjectRepository{db: db}
}

// Create creates a new project.
func (r *SQLiteProjectRepository) Create(ctx context.Context, project *Project) error {
	// Generate webhook secret if not set
	if project.WebhookSecret == "" {
		secret, err := generateWebhookSecret()
		if err != nil {
			return err
		}
		project.WebhookSecret = secret
	}

	query := `
		INSERT INTO projects (provider, namespace, name, full_name, clone_url, webhook_secret, default_branch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		project.Provider,
		project.Namespace,
		project.Name,
		project.FullName,
		project.CloneURL,
		project.WebhookSecret,
		project.DefaultBranch,
		now,
		now,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	project.ID = id
	project.CreatedAt = now
	project.UpdatedAt = now
	return nil
}

// GetByID retrieves a project by its ID.
func (r *SQLiteProjectRepository) GetByID(ctx context.Context, id int64) (*Project, error) {
	var project Project
	query := `SELECT * FROM projects WHERE id = ?`
	err := r.db.GetContext(ctx, &project, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// GetByFullName retrieves a project by its provider and full name.
func (r *SQLiteProjectRepository) GetByFullName(ctx context.Context, provider, fullName string) (*Project, error) {
	var project Project
	query := `SELECT * FROM projects WHERE provider = ? AND full_name = ?`
	err := r.db.GetContext(ctx, &project, query, provider, fullName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// List retrieves all projects.
func (r *SQLiteProjectRepository) List(ctx context.Context) ([]*Project, error) {
	var projects []*Project
	query := `SELECT * FROM projects ORDER BY full_name ASC`
	err := r.db.SelectContext(ctx, &projects, query)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// ListWithStatus retrieves all projects with their latest build status.
func (r *SQLiteProjectRepository) ListWithStatus(ctx context.Context) ([]*ProjectWithStatus, error) {
	var projects []*ProjectWithStatus
	query := `
		SELECT 
			p.*,
			b.status AS last_build_status,
			b.created_at AS last_build_at
		FROM projects p
		LEFT JOIN (
			SELECT project_id, status, created_at
			FROM builds b1
			WHERE build_number = (
				SELECT MAX(build_number) FROM builds b2 WHERE b2.project_id = b1.project_id
			)
		) b ON p.id = b.project_id
		ORDER BY p.full_name ASC
	`
	err := r.db.SelectContext(ctx, &projects, query)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// Update updates an existing project.
func (r *SQLiteProjectRepository) Update(ctx context.Context, project *Project) error {
	query := `
		UPDATE projects
		SET namespace = ?, name = ?, full_name = ?, clone_url = ?, 
		    webhook_secret = ?, default_branch = ?, updated_at = ?
		WHERE id = ?
	`
	project.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		project.Namespace,
		project.Name,
		project.FullName,
		project.CloneURL,
		project.WebhookSecret,
		project.DefaultBranch,
		project.UpdatedAt,
		project.ID,
	)
	return err
}

// Delete removes a project by its ID.
func (r *SQLiteProjectRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM projects WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// SQLiteProjectUserRepository implements ProjectUserRepository using SQLite.
type SQLiteProjectUserRepository struct {
	db *sqlx.DB
}

// NewProjectUserRepository creates a new SQLite-backed project-user repository.
func NewProjectUserRepository(db *sqlx.DB) *SQLiteProjectUserRepository {
	return &SQLiteProjectUserRepository{db: db}
}

// Add adds a user to a project.
func (r *SQLiteProjectUserRepository) Add(ctx context.Context, projectID, userID int64, canManage bool) error {
	query := `
		INSERT OR REPLACE INTO project_users (project_id, user_id, can_manage, created_at)
		VALUES (?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query, projectID, userID, canManage, time.Now())
	return err
}

// Remove removes a user from a project.
func (r *SQLiteProjectUserRepository) Remove(ctx context.Context, projectID, userID int64) error {
	query := `DELETE FROM project_users WHERE project_id = ? AND user_id = ?`
	_, err := r.db.ExecContext(ctx, query, projectID, userID)
	return err
}

// GetUsersForProject retrieves all users who have access to a project.
func (r *SQLiteProjectUserRepository) GetUsersForProject(ctx context.Context, projectID int64) ([]*User, error) {
	var users []*User
	query := `
		SELECT u.*
		FROM users u
		JOIN project_users pu ON u.id = pu.user_id
		WHERE pu.project_id = ?
		ORDER BY u.username ASC
	`
	err := r.db.SelectContext(ctx, &users, query, projectID)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// GetProjectsForUser retrieves all projects a user has access to.
func (r *SQLiteProjectUserRepository) GetProjectsForUser(ctx context.Context, userID int64) ([]*Project, error) {
	var projects []*Project
	query := `
		SELECT p.*
		FROM projects p
		JOIN project_users pu ON p.id = pu.project_id
		WHERE pu.user_id = ?
		ORDER BY p.full_name ASC
	`
	err := r.db.SelectContext(ctx, &projects, query, userID)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProjectsForUserWithStatus retrieves all projects a user has access to, with build status.
func (r *SQLiteProjectUserRepository) GetProjectsForUserWithStatus(ctx context.Context, userID int64) ([]*ProjectWithStatus, error) {
	var projects []*ProjectWithStatus
	query := `
		SELECT 
			p.*,
			b.status AS last_build_status,
			b.created_at AS last_build_at
		FROM projects p
		JOIN project_users pu ON p.id = pu.project_id
		LEFT JOIN (
			SELECT project_id, status, created_at
			FROM builds b1
			WHERE build_number = (
				SELECT MAX(build_number) FROM builds b2 WHERE b2.project_id = b1.project_id
			)
		) b ON p.id = b.project_id
		WHERE pu.user_id = ?
		ORDER BY p.full_name ASC
	`
	err := r.db.SelectContext(ctx, &projects, query, userID)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// CanUserAccess checks if a user can access a project.
func (r *SQLiteProjectUserRepository) CanUserAccess(ctx context.Context, projectID, userID int64) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM project_users WHERE project_id = ? AND user_id = ?`
	err := r.db.GetContext(ctx, &count, query, projectID, userID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CanUserManage checks if a user can manage a project (edit settings, delete).
func (r *SQLiteProjectUserRepository) CanUserManage(ctx context.Context, projectID, userID int64) (bool, error) {
	var canManage bool
	query := `SELECT can_manage FROM project_users WHERE project_id = ? AND user_id = ?`
	err := r.db.GetContext(ctx, &canManage, query, projectID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return canManage, nil
}

// generateWebhookSecret generates a secure random webhook secret.
func generateWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
