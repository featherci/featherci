package models

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func setupBuildTestDB(t *testing.T) *sqlx.DB {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create tables
	schema := `
		CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			namespace TEXT NOT NULL,
			name TEXT NOT NULL,
			full_name TEXT NOT NULL,
			clone_url TEXT NOT NULL,
			webhook_secret TEXT,
			webhook_id TEXT DEFAULT '',
			default_branch TEXT DEFAULT 'main',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, full_name)
		);

		CREATE TABLE project_users (
			project_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			can_manage BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_id, user_id)
		);

		CREATE TABLE builds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			build_number INTEGER NOT NULL,
			commit_sha TEXT NOT NULL,
			commit_message TEXT,
			commit_author TEXT,
			branch TEXT,
			pull_request_number INTEGER,
			status TEXT DEFAULT 'pending',
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(project_id, build_number)
		);

		CREATE TABLE build_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			build_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			image TEXT,
			status TEXT DEFAULT 'pending',
			exit_code INTEGER,
			started_at DATETIME,
			finished_at DATETIME,
			worker_id TEXT,
			log_path TEXT,
			requires_approval BOOLEAN DEFAULT FALSE,
			approved_by INTEGER,
			approved_at DATETIME,
			commands_json TEXT DEFAULT '[]',
			env_json TEXT DEFAULT '{}',
			depends_on_json TEXT DEFAULT '[]',
			cache_json TEXT DEFAULT '',
			services_json TEXT DEFAULT '',
			working_dir TEXT DEFAULT '',
			timeout_minutes INTEGER DEFAULT 60,
			condition_expr TEXT DEFAULT ''
		);

		CREATE TABLE step_dependencies (
			step_id INTEGER NOT NULL,
			depends_on_step_id INTEGER NOT NULL,
			PRIMARY KEY (step_id, depends_on_step_id)
		);

		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			username TEXT NOT NULL,
			email TEXT,
			avatar_url TEXT,
			access_token TEXT,
			refresh_token TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func createTestProject(t *testing.T, db *sqlx.DB) *Project {
	repo := NewProjectRepository(db)
	project := &Project{
		Provider:      "github",
		Namespace:     "testorg",
		Name:          "testrepo",
		FullName:      "testorg/testrepo",
		CloneURL:      "https://github.com/testorg/testrepo.git",
		DefaultBranch: "main",
	}
	if err := repo.Create(context.Background(), project); err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}
	return project
}

func TestBuildRepository_Create(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	branch := "main"
	msg := "Initial commit"
	author := "testuser"

	build := &Build{
		ProjectID:     project.ID,
		CommitSHA:     "abc123",
		CommitMessage: &msg,
		CommitAuthor:  &author,
		Branch:        &branch,
		Status:        BuildStatusPending,
	}

	err := repo.Create(ctx, build)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if build.ID == 0 {
		t.Error("expected build.ID to be set")
	}
	if build.BuildNumber != 1 {
		t.Errorf("BuildNumber = %d, want 1", build.BuildNumber)
	}
}

func TestBuildRepository_GetNextBuildNumber(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	// First build number should be 1
	num, err := repo.GetNextBuildNumber(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetNextBuildNumber() error = %v", err)
	}
	if num != 1 {
		t.Errorf("num = %d, want 1", num)
	}

	// Create a build
	build := &Build{
		ProjectID:   project.ID,
		BuildNumber: num,
		CommitSHA:   "abc123",
		Status:      BuildStatusPending,
	}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Next build number should be 2
	num, err = repo.GetNextBuildNumber(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetNextBuildNumber() error = %v", err)
	}
	if num != 2 {
		t.Errorf("num = %d, want 2", num)
	}
}

func TestBuildRepository_GetByID(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	branch := "main"
	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Branch:    &branch,
		Status:    BuildStatusPending,
	}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(ctx, build.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want %q", got.CommitSHA, "abc123")
	}
	if *got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", *got.Branch, "main")
	}
}

func TestBuildRepository_ListByProject(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	// Create 3 builds
	for i := 0; i < 3; i++ {
		build := &Build{
			ProjectID: project.ID,
			CommitSHA: "sha" + string(rune('0'+i)),
			Status:    BuildStatusPending,
		}
		if err := repo.Create(ctx, build); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List with pagination
	builds, err := repo.ListByProject(ctx, project.ID, 2, 0)
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(builds) != 2 {
		t.Errorf("len(builds) = %d, want 2", len(builds))
	}

	// Should be in descending order by build number
	if builds[0].BuildNumber != 3 {
		t.Errorf("builds[0].BuildNumber = %d, want 3", builds[0].BuildNumber)
	}
}

func TestBuildRepository_UpdateStatus(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := repo.UpdateStatus(ctx, build.ID, BuildStatusRunning)
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	got, err := repo.GetByID(ctx, build.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != BuildStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, BuildStatusRunning)
	}
}

func TestBuildRepository_SetStartedAndFinished(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Set started
	if err := repo.SetStarted(ctx, build.ID); err != nil {
		t.Fatalf("SetStarted() error = %v", err)
	}

	got, _ := repo.GetByID(ctx, build.ID)
	if got.Status != BuildStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, BuildStatusRunning)
	}
	if got.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}

	// Set finished
	if err := repo.SetFinished(ctx, build.ID, BuildStatusSuccess); err != nil {
		t.Fatalf("SetFinished() error = %v", err)
	}

	got, _ = repo.GetByID(ctx, build.ID)
	if got.Status != BuildStatusSuccess {
		t.Errorf("Status = %q, want %q", got.Status, BuildStatusSuccess)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should not be nil")
	}
}

func TestBuildRepository_CancelBuild(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	repo := NewBuildRepository(db)
	ctx := context.Background()

	// Cancel a pending build
	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusPending}
	if err := repo.Create(ctx, build); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.CancelBuild(ctx, build.ID); err != nil {
		t.Fatalf("CancelBuild() error = %v", err)
	}

	got, _ := repo.GetByID(ctx, build.ID)
	if got.Status != BuildStatusCancelled {
		t.Errorf("Status = %q, want %q", got.Status, BuildStatusCancelled)
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}

	// Cancel an already-finished build should return ErrNotFound
	build2 := &Build{ProjectID: project.ID, CommitSHA: "def456", Status: BuildStatusSuccess}
	if err := repo.Create(ctx, build2); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	// Manually set status to success (Create sets it to pending, need to update)
	if err := repo.UpdateStatus(ctx, build2.ID, BuildStatusSuccess); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	err := repo.CancelBuild(ctx, build2.ID)
	if err != ErrNotFound {
		t.Errorf("CancelBuild on finished build: err = %v, want ErrNotFound", err)
	}
}

func TestBuild_CalculateStatus(t *testing.T) {
	tests := []struct {
		name     string
		steps    []*BuildStep
		expected BuildStatus
	}{
		{
			name:     "no steps",
			steps:    nil,
			expected: BuildStatusPending,
		},
		{
			name: "all pending",
			steps: []*BuildStep{
				{Status: StepStatusPending},
				{Status: StepStatusWaiting},
			},
			expected: BuildStatusPending,
		},
		{
			name: "some running",
			steps: []*BuildStep{
				{Status: StepStatusSuccess},
				{Status: StepStatusRunning},
			},
			expected: BuildStatusRunning,
		},
		{
			name: "all success",
			steps: []*BuildStep{
				{Status: StepStatusSuccess},
				{Status: StepStatusSuccess},
			},
			expected: BuildStatusSuccess,
		},
		{
			name: "failure with no pending",
			steps: []*BuildStep{
				{Status: StepStatusSuccess},
				{Status: StepStatusFailure},
			},
			expected: BuildStatusFailure,
		},
		{
			name: "failure with pending - still running",
			steps: []*BuildStep{
				{Status: StepStatusFailure},
				{Status: StepStatusPending},
			},
			expected: BuildStatusRunning,
		},
		{
			name: "only waiting approval steps remain",
			steps: []*BuildStep{
				{Status: StepStatusSuccess},
				{Status: StepStatusWaitingApproval},
			},
			expected: BuildStatusWaitingApproval,
		},
		{
			name: "waiting approval with downstream waiting steps",
			steps: []*BuildStep{
				{Status: StepStatusSuccess},
				{Status: StepStatusWaitingApproval},
				{Status: StepStatusWaiting},
			},
			expected: BuildStatusWaitingApproval,
		},
		{
			name: "failure with waiting approval - still running",
			steps: []*BuildStep{
				{Status: StepStatusFailure},
				{Status: StepStatusWaitingApproval},
			},
			expected: BuildStatusRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := &Build{Steps: tt.steps}
			got := build.CalculateStatus()
			if got != tt.expected {
				t.Errorf("CalculateStatus() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuild_Duration(t *testing.T) {
	now := time.Now()

	// Not started - zero duration
	build := &Build{}
	if build.Duration() != 0 {
		t.Errorf("Duration() = %v, want 0", build.Duration())
	}

	// Started but not finished - time since start
	started := now.Add(-5 * time.Minute)
	build.StartedAt = &started
	duration := build.Duration()
	if duration < 5*time.Minute || duration > 6*time.Minute {
		t.Errorf("Duration() = %v, want ~5m", duration)
	}

	// Finished - exact duration
	finished := now.Add(-2 * time.Minute)
	build.FinishedAt = &finished
	if build.Duration() != 3*time.Minute {
		t.Errorf("Duration() = %v, want 3m", build.Duration())
	}
}

func TestBuild_IsTerminal(t *testing.T) {
	tests := []struct {
		status   BuildStatus
		terminal bool
	}{
		{BuildStatusPending, false},
		{BuildStatusRunning, false},
		{BuildStatusWaitingApproval, false},
		{BuildStatusSuccess, true},
		{BuildStatusFailure, true},
		{BuildStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			build := &Build{Status: tt.status}
			if build.IsTerminal() != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", build.IsTerminal(), tt.terminal)
			}
		})
	}
}
