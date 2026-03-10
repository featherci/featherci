package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// BuildStatus represents the current state of a build.
type BuildStatus string

const (
	// BuildStatusPending indicates the build is queued but not started.
	BuildStatusPending BuildStatus = "pending"
	// BuildStatusRunning indicates the build is currently executing.
	BuildStatusRunning BuildStatus = "running"
	// BuildStatusSuccess indicates the build completed successfully.
	BuildStatusSuccess BuildStatus = "success"
	// BuildStatusFailure indicates the build failed.
	BuildStatusFailure BuildStatus = "failure"
	// BuildStatusCancelled indicates the build was cancelled.
	BuildStatusCancelled BuildStatus = "cancelled"
)

// Build represents a CI build execution.
type Build struct {
	ID                int64       `db:"id"`
	ProjectID         int64       `db:"project_id"`
	BuildNumber       int         `db:"build_number"`
	CommitSHA         string      `db:"commit_sha"`
	CommitMessage     *string     `db:"commit_message"`
	CommitAuthor      *string     `db:"commit_author"`
	Branch            *string     `db:"branch"`
	PullRequestNumber *int        `db:"pull_request_number"`
	Status            BuildStatus `db:"status"`
	StartedAt         *time.Time  `db:"started_at"`
	FinishedAt        *time.Time  `db:"finished_at"`
	CreatedAt         time.Time   `db:"created_at"`

	// Loaded via joins (not stored in builds table)
	Project *Project     `db:"-"`
	Steps   []*BuildStep `db:"-"`
}

// IsTerminal returns true if the build is in a terminal state (no more changes expected).
func (b *Build) IsTerminal() bool {
	switch b.Status {
	case BuildStatusSuccess, BuildStatusFailure, BuildStatusCancelled:
		return true
	default:
		return false
	}
}

// Duration returns the build duration. If the build is still running, returns time since start.
func (b *Build) Duration() time.Duration {
	if b.StartedAt == nil {
		return 0
	}
	if b.FinishedAt != nil {
		return b.FinishedAt.Sub(*b.StartedAt)
	}
	return time.Since(*b.StartedAt)
}

// CalculateStatus determines the overall build status based on step statuses.
func (b *Build) CalculateStatus() BuildStatus {
	if len(b.Steps) == 0 {
		return BuildStatusPending
	}

	hasRunning := false
	hasPending := false
	hasFailure := false

	for _, step := range b.Steps {
		switch step.Status {
		case StepStatusRunning:
			hasRunning = true
		case StepStatusPending, StepStatusWaiting, StepStatusReady, StepStatusWaitingApproval:
			hasPending = true
		case StepStatusFailure, StepStatusCancelled:
			hasFailure = true
		}
	}

	if hasRunning {
		return BuildStatusRunning
	}
	if hasFailure && !hasPending {
		return BuildStatusFailure
	}
	if hasPending {
		if hasFailure {
			// Some steps failed but others are still pending - keep running
			return BuildStatusRunning
		}
		return BuildStatusPending
	}
	return BuildStatusSuccess
}

// BuildRepository defines the interface for build data access.
type BuildRepository interface {
	Create(ctx context.Context, build *Build) error
	GetByID(ctx context.Context, id int64) (*Build, error)
	GetByNumber(ctx context.Context, projectID int64, number int) (*Build, error)
	ListByProject(ctx context.Context, projectID int64, limit, offset int) ([]*Build, error)
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]*Build, error)
	ListPending(ctx context.Context) ([]*Build, error)
	Update(ctx context.Context, build *Build) error
	UpdateStatus(ctx context.Context, id int64, status BuildStatus) error
	GetNextBuildNumber(ctx context.Context, projectID int64) (int, error)
	SetStarted(ctx context.Context, id int64) error
	SetFinished(ctx context.Context, id int64, status BuildStatus) error
	CancelBuild(ctx context.Context, id int64) error
}

// SQLiteBuildRepository implements BuildRepository using SQLite.
type SQLiteBuildRepository struct {
	db *sqlx.DB
}

// NewBuildRepository creates a new SQLite-backed build repository.
func NewBuildRepository(db *sqlx.DB) *SQLiteBuildRepository {
	return &SQLiteBuildRepository{db: db}
}

// Create inserts a new build into the database.
func (r *SQLiteBuildRepository) Create(ctx context.Context, build *Build) error {
	// Get next build number if not set
	if build.BuildNumber == 0 {
		num, err := r.GetNextBuildNumber(ctx, build.ProjectID)
		if err != nil {
			return err
		}
		build.BuildNumber = num
	}

	query := `
		INSERT INTO builds (project_id, build_number, commit_sha, commit_message, commit_author, branch, pull_request_number, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		build.ProjectID,
		build.BuildNumber,
		build.CommitSHA,
		build.CommitMessage,
		build.CommitAuthor,
		build.Branch,
		build.PullRequestNumber,
		build.Status,
		now,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	build.ID = id
	build.CreatedAt = now
	return nil
}

// GetByID retrieves a build by its ID.
func (r *SQLiteBuildRepository) GetByID(ctx context.Context, id int64) (*Build, error) {
	var build Build
	query := `SELECT * FROM builds WHERE id = ?`
	err := r.db.GetContext(ctx, &build, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &build, nil
}

// GetByNumber retrieves a build by project ID and build number.
func (r *SQLiteBuildRepository) GetByNumber(ctx context.Context, projectID int64, number int) (*Build, error) {
	var build Build
	query := `SELECT * FROM builds WHERE project_id = ? AND build_number = ?`
	err := r.db.GetContext(ctx, &build, query, projectID, number)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &build, nil
}

// ListByProject retrieves builds for a project with pagination.
func (r *SQLiteBuildRepository) ListByProject(ctx context.Context, projectID int64, limit, offset int) ([]*Build, error) {
	var builds []*Build
	query := `
		SELECT * FROM builds 
		WHERE project_id = ? 
		ORDER BY build_number DESC 
		LIMIT ? OFFSET ?
	`
	err := r.db.SelectContext(ctx, &builds, query, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	return builds, nil
}

// ListByUser retrieves builds for projects the user has access to.
func (r *SQLiteBuildRepository) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]*Build, error) {
	var builds []*Build
	query := `
		SELECT b.* FROM builds b
		JOIN project_users pu ON b.project_id = pu.project_id
		WHERE pu.user_id = ?
		ORDER BY b.created_at DESC
		LIMIT ? OFFSET ?
	`
	err := r.db.SelectContext(ctx, &builds, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	return builds, nil
}

// ListPending retrieves all builds with pending status.
func (r *SQLiteBuildRepository) ListPending(ctx context.Context) ([]*Build, error) {
	var builds []*Build
	query := `SELECT * FROM builds WHERE status = 'pending' ORDER BY created_at ASC`
	err := r.db.SelectContext(ctx, &builds, query)
	if err != nil {
		return nil, err
	}
	return builds, nil
}

// Update updates a build record.
func (r *SQLiteBuildRepository) Update(ctx context.Context, build *Build) error {
	query := `
		UPDATE builds
		SET status = ?, started_at = ?, finished_at = ?
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query, build.Status, build.StartedAt, build.FinishedAt, build.ID)
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

// UpdateStatus updates only the status field of a build.
func (r *SQLiteBuildRepository) UpdateStatus(ctx context.Context, id int64, status BuildStatus) error {
	query := `UPDATE builds SET status = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, id)
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

// GetNextBuildNumber returns the next build number for a project.
func (r *SQLiteBuildRepository) GetNextBuildNumber(ctx context.Context, projectID int64) (int, error) {
	var maxNum sql.NullInt64
	query := `SELECT MAX(build_number) FROM builds WHERE project_id = ?`
	err := r.db.GetContext(ctx, &maxNum, query, projectID)
	if err != nil {
		return 0, err
	}
	if !maxNum.Valid {
		return 1, nil
	}
	return int(maxNum.Int64) + 1, nil
}

// SetStarted marks a build as started.
func (r *SQLiteBuildRepository) SetStarted(ctx context.Context, id int64) error {
	now := time.Now()
	query := `UPDATE builds SET status = 'running', started_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, now, id)
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

// SetFinished marks a build as finished with the given status.
func (r *SQLiteBuildRepository) SetFinished(ctx context.Context, id int64, status BuildStatus) error {
	now := time.Now()
	query := `UPDATE builds SET status = ?, finished_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, now, id)
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

// CancelBuild cancels a build if it is still in a non-terminal state.
func (r *SQLiteBuildRepository) CancelBuild(ctx context.Context, id int64) error {
	now := time.Now()
	query := `UPDATE builds SET status = 'cancelled', finished_at = ? WHERE id = ? AND status IN ('pending', 'running')`
	result, err := r.db.ExecContext(ctx, query, now, id)
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
