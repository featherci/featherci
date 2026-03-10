package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// StepStatus represents the current state of a build step.
type StepStatus string

const (
	// StepStatusPending indicates the step is queued but not yet evaluated.
	StepStatusPending StepStatus = "pending"
	// StepStatusWaiting indicates the step is waiting for dependencies to complete.
	StepStatusWaiting StepStatus = "waiting"
	// StepStatusReady indicates all dependencies are met and the step is ready to run.
	StepStatusReady StepStatus = "ready"
	// StepStatusRunning indicates the step is currently executing.
	StepStatusRunning StepStatus = "running"
	// StepStatusSuccess indicates the step completed successfully.
	StepStatusSuccess StepStatus = "success"
	// StepStatusFailure indicates the step failed.
	StepStatusFailure StepStatus = "failure"
	// StepStatusSkipped indicates the step was skipped (e.g., due to failed dependency).
	StepStatusSkipped StepStatus = "skipped"
	// StepStatusWaitingApproval indicates the step requires manual approval.
	StepStatusWaitingApproval StepStatus = "waiting_approval"
	// StepStatusCancelled indicates the step was cancelled.
	StepStatusCancelled StepStatus = "cancelled"
)

// BuildStep represents a single step within a build.
type BuildStep struct {
	ID               int64      `db:"id"`
	BuildID          int64      `db:"build_id"`
	Name             string     `db:"name"`
	Image            *string    `db:"image"`
	Status           StepStatus `db:"status"`
	ExitCode         *int       `db:"exit_code"`
	StartedAt        *time.Time `db:"started_at"`
	FinishedAt       *time.Time `db:"finished_at"`
	WorkerID         *string    `db:"worker_id"`
	LogPath          *string    `db:"log_path"`
	RequiresApproval bool       `db:"requires_approval"`
	ApprovedBy       *int64     `db:"approved_by"`
	ApprovedAt       *time.Time `db:"approved_at"`

	// JSON-serialized fields stored in the database
	CommandsJSON   string `db:"commands_json"`
	EnvJSON        string `db:"env_json"`
	DependsOnJSON  string `db:"depends_on_json"`
	WorkingDir     string `db:"working_dir"`
	TimeoutMinutes int    `db:"timeout_minutes"`

	// Deserialized fields (not stored directly)
	Commands  []string          `db:"-"`
	Env       map[string]string `db:"-"`
	DependsOn []string          `db:"-"`

	// Loaded via joins
	ApprovedByUser *User `db:"-"`
}

// IsTerminal returns true if the step is in a terminal state.
func (s *BuildStep) IsTerminal() bool {
	switch s.Status {
	case StepStatusSuccess, StepStatusFailure, StepStatusSkipped, StepStatusCancelled:
		return true
	default:
		return false
	}
}

// Duration returns the step duration. If still running, returns time since start.
func (s *BuildStep) Duration() time.Duration {
	if s.StartedAt == nil {
		return 0
	}
	if s.FinishedAt != nil {
		return s.FinishedAt.Sub(*s.StartedAt)
	}
	return time.Since(*s.StartedAt)
}

// GetTimeout returns the timeout in minutes, defaulting to 60 if not set.
func (s *BuildStep) GetTimeout() int {
	if s.TimeoutMinutes <= 0 {
		return 60
	}
	return s.TimeoutMinutes
}

// SerializeJSON converts Commands, Env, and DependsOn to JSON strings for storage.
func (s *BuildStep) SerializeJSON() error {
	if s.Commands != nil {
		data, err := json.Marshal(s.Commands)
		if err != nil {
			return err
		}
		s.CommandsJSON = string(data)
	} else {
		s.CommandsJSON = "[]"
	}

	if s.Env != nil {
		data, err := json.Marshal(s.Env)
		if err != nil {
			return err
		}
		s.EnvJSON = string(data)
	} else {
		s.EnvJSON = "{}"
	}

	if s.DependsOn != nil {
		data, err := json.Marshal(s.DependsOn)
		if err != nil {
			return err
		}
		s.DependsOnJSON = string(data)
	} else {
		s.DependsOnJSON = "[]"
	}

	return nil
}

// DeserializeJSON converts JSON strings from database to Go types.
func (s *BuildStep) DeserializeJSON() error {
	if s.CommandsJSON != "" {
		if err := json.Unmarshal([]byte(s.CommandsJSON), &s.Commands); err != nil {
			return err
		}
	}

	if s.EnvJSON != "" {
		if err := json.Unmarshal([]byte(s.EnvJSON), &s.Env); err != nil {
			return err
		}
	}

	if s.DependsOnJSON != "" {
		if err := json.Unmarshal([]byte(s.DependsOnJSON), &s.DependsOn); err != nil {
			return err
		}
	}

	return nil
}

// BuildStepRepository defines the interface for build step data access.
type BuildStepRepository interface {
	Create(ctx context.Context, step *BuildStep) error
	CreateBatch(ctx context.Context, steps []*BuildStep) error
	GetByID(ctx context.Context, id int64) (*BuildStep, error)
	ListByBuild(ctx context.Context, buildID int64) ([]*BuildStep, error)
	ListReady(ctx context.Context) ([]*BuildStep, error)
	ListWaitingApproval(ctx context.Context, projectID int64) ([]*BuildStep, error)
	Update(ctx context.Context, step *BuildStep) error
	UpdateStatus(ctx context.Context, id int64, status StepStatus) error
	SetStarted(ctx context.Context, id int64, workerID string) error
	SetFinished(ctx context.Context, id int64, status StepStatus, exitCode *int, logPath string) error
	SetApproval(ctx context.Context, id int64, userID int64) error
	AddDependency(ctx context.Context, stepID, dependsOnID int64) error
	GetDependencies(ctx context.Context, stepID int64) ([]*BuildStep, error)
	UpdateReadySteps(ctx context.Context, buildID int64) (int64, error)
}

// SQLiteBuildStepRepository implements BuildStepRepository using SQLite.
type SQLiteBuildStepRepository struct {
	db *sqlx.DB
}

// NewBuildStepRepository creates a new SQLite-backed build step repository.
func NewBuildStepRepository(db *sqlx.DB) *SQLiteBuildStepRepository {
	return &SQLiteBuildStepRepository{db: db}
}

// Create inserts a new build step into the database.
func (r *SQLiteBuildStepRepository) Create(ctx context.Context, step *BuildStep) error {
	if err := step.SerializeJSON(); err != nil {
		return err
	}

	query := `
		INSERT INTO build_steps (build_id, name, image, status, requires_approval, commands_json, env_json, depends_on_json, working_dir, timeout_minutes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		step.BuildID,
		step.Name,
		step.Image,
		step.Status,
		step.RequiresApproval,
		step.CommandsJSON,
		step.EnvJSON,
		step.DependsOnJSON,
		step.WorkingDir,
		step.TimeoutMinutes,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	step.ID = id
	return nil
}

// CreateBatch inserts multiple build steps in a transaction.
func (r *SQLiteBuildStepRepository) CreateBatch(ctx context.Context, steps []*BuildStep) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO build_steps (build_id, name, image, status, requires_approval, commands_json, env_json, depends_on_json, working_dir, timeout_minutes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	stmt, err := tx.PreparexContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, step := range steps {
		if err := step.SerializeJSON(); err != nil {
			return err
		}

		result, err := stmt.ExecContext(ctx,
			step.BuildID,
			step.Name,
			step.Image,
			step.Status,
			step.RequiresApproval,
			step.CommandsJSON,
			step.EnvJSON,
			step.DependsOnJSON,
			step.WorkingDir,
			step.TimeoutMinutes,
		)
		if err != nil {
			return err
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		step.ID = id
	}

	return tx.Commit()
}

// GetByID retrieves a build step by its ID.
func (r *SQLiteBuildStepRepository) GetByID(ctx context.Context, id int64) (*BuildStep, error) {
	var step BuildStep
	query := `SELECT * FROM build_steps WHERE id = ?`
	err := r.db.GetContext(ctx, &step, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := step.DeserializeJSON(); err != nil {
		return nil, err
	}
	return &step, nil
}

// ListByBuild retrieves all steps for a build, ordered by name.
func (r *SQLiteBuildStepRepository) ListByBuild(ctx context.Context, buildID int64) ([]*BuildStep, error) {
	var steps []*BuildStep
	query := `SELECT * FROM build_steps WHERE build_id = ? ORDER BY id ASC`
	err := r.db.SelectContext(ctx, &steps, query, buildID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if err := step.DeserializeJSON(); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// ListReady retrieves all steps that are ready to run.
func (r *SQLiteBuildStepRepository) ListReady(ctx context.Context) ([]*BuildStep, error) {
	var steps []*BuildStep
	query := `SELECT * FROM build_steps WHERE status = 'ready' ORDER BY id ASC`
	err := r.db.SelectContext(ctx, &steps, query)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if err := step.DeserializeJSON(); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// ListWaitingApproval retrieves all steps waiting for approval for a project.
func (r *SQLiteBuildStepRepository) ListWaitingApproval(ctx context.Context, projectID int64) ([]*BuildStep, error) {
	var steps []*BuildStep
	query := `
		SELECT bs.* FROM build_steps bs
		JOIN builds b ON bs.build_id = b.id
		WHERE b.project_id = ? AND bs.status = 'waiting_approval'
		ORDER BY bs.id ASC
	`
	err := r.db.SelectContext(ctx, &steps, query, projectID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if err := step.DeserializeJSON(); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// Update updates a build step record.
func (r *SQLiteBuildStepRepository) Update(ctx context.Context, step *BuildStep) error {
	if err := step.SerializeJSON(); err != nil {
		return err
	}

	query := `
		UPDATE build_steps
		SET status = ?, exit_code = ?, started_at = ?, finished_at = ?, worker_id = ?, log_path = ?,
		    approved_by = ?, approved_at = ?, commands_json = ?, env_json = ?, depends_on_json = ?,
		    working_dir = ?, timeout_minutes = ?
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query,
		step.Status,
		step.ExitCode,
		step.StartedAt,
		step.FinishedAt,
		step.WorkerID,
		step.LogPath,
		step.ApprovedBy,
		step.ApprovedAt,
		step.CommandsJSON,
		step.EnvJSON,
		step.DependsOnJSON,
		step.WorkingDir,
		step.TimeoutMinutes,
		step.ID,
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

// UpdateStatus updates only the status field of a build step.
func (r *SQLiteBuildStepRepository) UpdateStatus(ctx context.Context, id int64, status StepStatus) error {
	query := `UPDATE build_steps SET status = ? WHERE id = ?`
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

// SetStarted marks a step as started with the given worker ID.
func (r *SQLiteBuildStepRepository) SetStarted(ctx context.Context, id int64, workerID string) error {
	now := time.Now()
	query := `UPDATE build_steps SET status = 'running', started_at = ?, worker_id = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, now, workerID, id)
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

// SetFinished marks a step as finished with the given status, exit code, and log path.
func (r *SQLiteBuildStepRepository) SetFinished(ctx context.Context, id int64, status StepStatus, exitCode *int, logPath string) error {
	now := time.Now()
	query := `UPDATE build_steps SET status = ?, finished_at = ?, exit_code = ?, log_path = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, now, exitCode, logPath, id)
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

// SetApproval records approval for a step.
func (r *SQLiteBuildStepRepository) SetApproval(ctx context.Context, id int64, userID int64) error {
	now := time.Now()
	query := `UPDATE build_steps SET status = 'ready', approved_by = ?, approved_at = ? WHERE id = ? AND status = 'waiting_approval'`
	result, err := r.db.ExecContext(ctx, query, userID, now, id)
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

// AddDependency creates a dependency relationship between two steps.
func (r *SQLiteBuildStepRepository) AddDependency(ctx context.Context, stepID, dependsOnID int64) error {
	query := `INSERT INTO step_dependencies (step_id, depends_on_step_id) VALUES (?, ?)`
	_, err := r.db.ExecContext(ctx, query, stepID, dependsOnID)
	return err
}

// GetDependencies retrieves all steps that a step depends on.
func (r *SQLiteBuildStepRepository) GetDependencies(ctx context.Context, stepID int64) ([]*BuildStep, error) {
	var steps []*BuildStep
	query := `
		SELECT bs.* FROM build_steps bs
		JOIN step_dependencies sd ON bs.id = sd.depends_on_step_id
		WHERE sd.step_id = ?
	`
	err := r.db.SelectContext(ctx, &steps, query, stepID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if err := step.DeserializeJSON(); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// UpdateReadySteps transitions steps from 'waiting' to 'ready' when all their dependencies are successful.
// Returns the number of steps updated.
func (r *SQLiteBuildStepRepository) UpdateReadySteps(ctx context.Context, buildID int64) (int64, error) {
	query := `
		UPDATE build_steps
		SET status = 'ready'
		WHERE build_id = ?
		  AND status = 'waiting'
		  AND NOT EXISTS (
			  SELECT 1 FROM step_dependencies sd
			  JOIN build_steps dep ON sd.depends_on_step_id = dep.id
			  WHERE sd.step_id = build_steps.id
			    AND dep.status NOT IN ('success')
		  )
	`
	result, err := r.db.ExecContext(ctx, query, buildID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
