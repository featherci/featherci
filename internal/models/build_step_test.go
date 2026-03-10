package models

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestBuildStepRepository_Create(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	step := &BuildStep{
		BuildID:        build.ID,
		Name:           "test",
		Image:          &image,
		Status:         StepStatusPending,
		Commands:       []string{"go test ./..."},
		Env:            map[string]string{"CGO_ENABLED": "0"},
		DependsOn:      []string{},
		WorkingDir:     "/app",
		TimeoutMinutes: 30,
	}

	err := stepRepo.Create(ctx, step)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if step.ID == 0 {
		t.Error("expected step.ID to be set")
	}
}

func TestBuildStepRepository_CreateBatch(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	steps := []*BuildStep{
		{
			BuildID:   build.ID,
			Name:      "lint",
			Image:     &image,
			Status:    StepStatusReady,
			Commands:  []string{"golangci-lint run"},
			DependsOn: []string{},
		},
		{
			BuildID:   build.ID,
			Name:      "test",
			Image:     &image,
			Status:    StepStatusWaiting,
			Commands:  []string{"go test ./..."},
			DependsOn: []string{"lint"},
		},
	}

	err := stepRepo.CreateBatch(ctx, steps)
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}

	for i, step := range steps {
		if step.ID == 0 {
			t.Errorf("steps[%d].ID should be set", i)
		}
	}
}

func TestBuildStepRepository_GetByID(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	step := &BuildStep{
		BuildID:        build.ID,
		Name:           "test",
		Image:          &image,
		Status:         StepStatusPending,
		Commands:       []string{"go test ./...", "go build ./..."},
		Env:            map[string]string{"CGO_ENABLED": "0", "GOOS": "linux"},
		DependsOn:      []string{"lint"},
		WorkingDir:     "/app",
		TimeoutMinutes: 45,
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("Create step error = %v", err)
	}

	got, err := stepRepo.GetByID(ctx, step.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
	if len(got.Commands) != 2 {
		t.Errorf("len(Commands) = %d, want 2", len(got.Commands))
	}
	if got.Env["CGO_ENABLED"] != "0" {
		t.Errorf("Env[CGO_ENABLED] = %q, want %q", got.Env["CGO_ENABLED"], "0")
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "lint" {
		t.Errorf("DependsOn = %v, want [lint]", got.DependsOn)
	}
	if got.WorkingDir != "/app" {
		t.Errorf("WorkingDir = %q, want %q", got.WorkingDir, "/app")
	}
	if got.TimeoutMinutes != 45 {
		t.Errorf("TimeoutMinutes = %d, want 45", got.TimeoutMinutes)
	}
}

func TestBuildStepRepository_ListByBuild(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	for _, name := range []string{"lint", "test", "build"} {
		step := &BuildStep{
			BuildID:  build.ID,
			Name:     name,
			Image:    &image,
			Status:   StepStatusPending,
			Commands: []string{"make " + name},
		}
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("Create step error = %v", err)
		}
	}

	steps, err := stepRepo.ListByBuild(ctx, build.ID)
	if err != nil {
		t.Fatalf("ListByBuild() error = %v", err)
	}

	if len(steps) != 3 {
		t.Errorf("len(steps) = %d, want 3", len(steps))
	}
}

func TestBuildStepRepository_Dependencies(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	lint := &BuildStep{BuildID: build.ID, Name: "lint", Image: &image, Status: StepStatusReady}
	test := &BuildStep{BuildID: build.ID, Name: "test", Image: &image, Status: StepStatusWaiting}
	buildStep := &BuildStep{BuildID: build.ID, Name: "build", Image: &image, Status: StepStatusWaiting}

	if err := stepRepo.Create(ctx, lint); err != nil {
		t.Fatalf("Create lint error = %v", err)
	}
	if err := stepRepo.Create(ctx, test); err != nil {
		t.Fatalf("Create test error = %v", err)
	}
	if err := stepRepo.Create(ctx, buildStep); err != nil {
		t.Fatalf("Create build step error = %v", err)
	}

	// test depends on lint
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency() error = %v", err)
	}
	// build depends on both
	if err := stepRepo.AddDependency(ctx, buildStep.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency() error = %v", err)
	}
	if err := stepRepo.AddDependency(ctx, buildStep.ID, test.ID); err != nil {
		t.Fatalf("AddDependency() error = %v", err)
	}

	// Get dependencies for test step
	deps, err := stepRepo.GetDependencies(ctx, test.ID)
	if err != nil {
		t.Fatalf("GetDependencies() error = %v", err)
	}
	if len(deps) != 1 {
		t.Errorf("len(deps) = %d, want 1", len(deps))
	}
	if deps[0].Name != "lint" {
		t.Errorf("deps[0].Name = %q, want %q", deps[0].Name, "lint")
	}

	// Get dependencies for build step
	deps, err = stepRepo.GetDependencies(ctx, buildStep.ID)
	if err != nil {
		t.Fatalf("GetDependencies() error = %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("len(deps) = %d, want 2", len(deps))
	}
}

func TestBuildStepRepository_UpdateReadySteps(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	lint := &BuildStep{BuildID: build.ID, Name: "lint", Image: &image, Status: StepStatusReady}
	test := &BuildStep{BuildID: build.ID, Name: "test", Image: &image, Status: StepStatusWaiting}

	if err := stepRepo.Create(ctx, lint); err != nil {
		t.Fatalf("Create lint error = %v", err)
	}
	if err := stepRepo.Create(ctx, test); err != nil {
		t.Fatalf("Create test error = %v", err)
	}

	// test depends on lint
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency() error = %v", err)
	}

	// Test is waiting, lint not done - should not update
	updated, err := stepRepo.UpdateReadySteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("UpdateReadySteps() error = %v", err)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0", updated)
	}

	// Mark lint as success
	if err := stepRepo.UpdateStatus(ctx, lint.ID, StepStatusSuccess); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Now test should become ready
	updated, err = stepRepo.UpdateReadySteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("UpdateReadySteps() error = %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}

	// Verify test is now ready
	gotTest, err := stepRepo.GetByID(ctx, test.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if gotTest.Status != StepStatusReady {
		t.Errorf("test.Status = %q, want %q", gotTest.Status, StepStatusReady)
	}
}

func TestBuildStepRepository_SetStartedAndFinished(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	step := &BuildStep{
		BuildID:  build.ID,
		Name:     "test",
		Image:    &image,
		Status:   StepStatusReady,
		Commands: []string{"go test ./..."},
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("Create step error = %v", err)
	}

	// Set started
	if err := stepRepo.SetStarted(ctx, step.ID, "worker-1"); err != nil {
		t.Fatalf("SetStarted() error = %v", err)
	}

	got, _ := stepRepo.GetByID(ctx, step.ID)
	if got.Status != StepStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, StepStatusRunning)
	}
	if got.WorkerID == nil || *got.WorkerID != "worker-1" {
		t.Errorf("WorkerID = %v, want %q", got.WorkerID, "worker-1")
	}
	if got.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}

	// Set finished with success
	exitCode := 0
	if err := stepRepo.SetFinished(ctx, step.ID, StepStatusSuccess, &exitCode, "/logs/test.log"); err != nil {
		t.Fatalf("SetFinished() error = %v", err)
	}

	got, _ = stepRepo.GetByID(ctx, step.ID)
	if got.Status != StepStatusSuccess {
		t.Errorf("Status = %q, want %q", got.Status, StepStatusSuccess)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", got.ExitCode)
	}
	if got.LogPath == nil || *got.LogPath != "/logs/test.log" {
		t.Errorf("LogPath = %v, want %q", got.LogPath, "/logs/test.log")
	}
	if got.FinishedAt == nil {
		t.Error("FinishedAt should not be nil")
	}
}

func TestBuildStepRepository_SetApproval(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Status:    BuildStatusPending,
	}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	step := &BuildStep{
		BuildID:          build.ID,
		Name:             "deploy",
		Status:           StepStatusWaitingApproval,
		RequiresApproval: true,
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("Create step error = %v", err)
	}

	// Create a user for approval
	userRepo := NewUserRepository(db)
	user := &User{
		Provider:   "github",
		ProviderID: "123",
		Username:   "approver",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user error = %v", err)
	}

	// Approve the step
	if err := stepRepo.SetApproval(ctx, step.ID, user.ID); err != nil {
		t.Fatalf("SetApproval() error = %v", err)
	}

	got, _ := stepRepo.GetByID(ctx, step.ID)
	if got.Status != StepStatusReady {
		t.Errorf("Status = %q, want %q", got.Status, StepStatusReady)
	}
	if got.ApprovedBy == nil || *got.ApprovedBy != user.ID {
		t.Errorf("ApprovedBy = %v, want %d", got.ApprovedBy, user.ID)
	}
	if got.ApprovedAt == nil {
		t.Error("ApprovedAt should not be nil")
	}
}

func TestBuildStep_JSONSerialization(t *testing.T) {
	step := &BuildStep{
		Commands:  []string{"go build", "go test"},
		Env:       map[string]string{"GOOS": "linux", "GOARCH": "amd64"},
		DependsOn: []string{"lint", "format"},
	}

	if err := step.SerializeJSON(); err != nil {
		t.Fatalf("SerializeJSON() error = %v", err)
	}

	if step.CommandsJSON == "" || step.CommandsJSON == "[]" {
		t.Error("CommandsJSON should be populated")
	}

	// Create new step and deserialize
	newStep := &BuildStep{
		CommandsJSON:  step.CommandsJSON,
		EnvJSON:       step.EnvJSON,
		DependsOnJSON: step.DependsOnJSON,
	}

	if err := newStep.DeserializeJSON(); err != nil {
		t.Fatalf("DeserializeJSON() error = %v", err)
	}

	if len(newStep.Commands) != 2 {
		t.Errorf("len(Commands) = %d, want 2", len(newStep.Commands))
	}
	if newStep.Commands[0] != "go build" {
		t.Errorf("Commands[0] = %q, want %q", newStep.Commands[0], "go build")
	}
	if len(newStep.Env) != 2 {
		t.Errorf("len(Env) = %d, want 2", len(newStep.Env))
	}
	if newStep.Env["GOOS"] != "linux" {
		t.Errorf("Env[GOOS] = %q, want %q", newStep.Env["GOOS"], "linux")
	}
	if len(newStep.DependsOn) != 2 {
		t.Errorf("len(DependsOn) = %d, want 2", len(newStep.DependsOn))
	}
}

func TestBuildStep_Duration(t *testing.T) {
	now := time.Now()

	// Not started
	step := &BuildStep{}
	if step.Duration() != 0 {
		t.Errorf("Duration() = %v, want 0", step.Duration())
	}

	// Started but not finished
	started := now.Add(-10 * time.Minute)
	step.StartedAt = &started
	duration := step.Duration()
	if duration < 10*time.Minute || duration > 11*time.Minute {
		t.Errorf("Duration() = %v, want ~10m", duration)
	}

	// Finished
	finished := now.Add(-5 * time.Minute)
	step.FinishedAt = &finished
	if step.Duration() != 5*time.Minute {
		t.Errorf("Duration() = %v, want 5m", step.Duration())
	}
}

func TestBuildStep_IsTerminal(t *testing.T) {
	tests := []struct {
		status   StepStatus
		terminal bool
	}{
		{StepStatusPending, false},
		{StepStatusWaiting, false},
		{StepStatusReady, false},
		{StepStatusRunning, false},
		{StepStatusWaitingApproval, false},
		{StepStatusSuccess, true},
		{StepStatusFailure, true},
		{StepStatusSkipped, true},
		{StepStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			step := &BuildStep{Status: tt.status}
			if step.IsTerminal() != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", step.IsTerminal(), tt.terminal)
			}
		})
	}
}

func TestBuildStep_GetTimeout(t *testing.T) {
	// Default timeout
	step := &BuildStep{}
	if step.GetTimeout() != 60 {
		t.Errorf("GetTimeout() = %d, want 60", step.GetTimeout())
	}

	// Custom timeout
	step.TimeoutMinutes = 30
	if step.GetTimeout() != 30 {
		t.Errorf("GetTimeout() = %d, want 30", step.GetTimeout())
	}

	// Zero should return default
	step.TimeoutMinutes = 0
	if step.GetTimeout() != 60 {
		t.Errorf("GetTimeout() = %d, want 60", step.GetTimeout())
	}
}
