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

func TestBuildStepRepository_SetLogPath(t *testing.T) {
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

	// Initially log_path should be nil
	got, _ := stepRepo.GetByID(ctx, step.ID)
	if got.LogPath != nil {
		t.Errorf("LogPath = %v, want nil", got.LogPath)
	}

	// Set log path
	if err := stepRepo.SetLogPath(ctx, step.ID, "/logs/42.log"); err != nil {
		t.Fatalf("SetLogPath() error = %v", err)
	}

	got, _ = stepRepo.GetByID(ctx, step.ID)
	if got.LogPath == nil || *got.LogPath != "/logs/42.log" {
		t.Errorf("LogPath = %v, want %q", got.LogPath, "/logs/42.log")
	}

	// SetLogPath on non-existent step should return ErrNotFound
	err := stepRepo.SetLogPath(ctx, 99999, "/logs/nope.log")
	if err != ErrNotFound {
		t.Errorf("SetLogPath on non-existent step: err = %v, want ErrNotFound", err)
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

func TestBuildStepRepository_SkipDependentSteps(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusPending}
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
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}

	// lint not failed yet — should skip nothing
	n, err := stepRepo.SkipDependentSteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("SkipDependentSteps() error = %v", err)
	}
	if n != 0 {
		t.Errorf("skipped = %d, want 0", n)
	}

	// Fail lint → test should be skipped
	if err := stepRepo.UpdateStatus(ctx, lint.ID, StepStatusFailure); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	n, err = stepRepo.SkipDependentSteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("SkipDependentSteps() error = %v", err)
	}
	if n != 1 {
		t.Errorf("skipped = %d, want 1", n)
	}

	got, _ := stepRepo.GetByID(ctx, test.ID)
	if got.Status != StepStatusSkipped {
		t.Errorf("test.Status = %q, want %q", got.Status, StepStatusSkipped)
	}
}

func TestBuildStepRepository_SkipReadySteps(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusPending}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	lint := &BuildStep{BuildID: build.ID, Name: "lint", Image: &image, Status: StepStatusFailure}
	// test is already "ready" (transitioned before lint failed)
	test := &BuildStep{BuildID: build.ID, Name: "test", Image: &image, Status: StepStatusReady}

	if err := stepRepo.Create(ctx, lint); err != nil {
		t.Fatalf("Create lint error = %v", err)
	}
	if err := stepRepo.Create(ctx, test); err != nil {
		t.Fatalf("Create test error = %v", err)
	}
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}

	// test is ready but lint failed → should be skipped
	n, err := stepRepo.SkipDependentSteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("SkipDependentSteps() error = %v", err)
	}
	if n != 1 {
		t.Errorf("skipped = %d, want 1", n)
	}

	got, _ := stepRepo.GetByID(ctx, test.ID)
	if got.Status != StepStatusSkipped {
		t.Errorf("test.Status = %q, want %q", got.Status, StepStatusSkipped)
	}
}

func TestBuildStepRepository_SkipCascade(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusPending}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	a := &BuildStep{BuildID: build.ID, Name: "a", Image: &image, Status: StepStatusReady}
	b := &BuildStep{BuildID: build.ID, Name: "b", Image: &image, Status: StepStatusWaiting}
	c := &BuildStep{BuildID: build.ID, Name: "c", Image: &image, Status: StepStatusWaiting}

	for _, s := range []*BuildStep{a, b, c} {
		if err := stepRepo.Create(ctx, s); err != nil {
			t.Fatalf("Create %s error = %v", s.Name, err)
		}
	}

	// b depends on a, c depends on b
	if err := stepRepo.AddDependency(ctx, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}
	if err := stepRepo.AddDependency(ctx, c.ID, b.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}

	// Fail a
	if err := stepRepo.UpdateStatus(ctx, a.ID, StepStatusFailure); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	// Loop until no more skips (cascade)
	total := int64(0)
	for {
		n, err := stepRepo.SkipDependentSteps(ctx, build.ID)
		if err != nil {
			t.Fatalf("SkipDependentSteps() error = %v", err)
		}
		total += n
		if n == 0 {
			break
		}
	}

	if total != 2 {
		t.Errorf("total skipped = %d, want 2", total)
	}

	gotB, _ := stepRepo.GetByID(ctx, b.ID)
	gotC, _ := stepRepo.GetByID(ctx, c.ID)
	if gotB.Status != StepStatusSkipped {
		t.Errorf("b.Status = %q, want skipped", gotB.Status)
	}
	if gotC.Status != StepStatusSkipped {
		t.Errorf("c.Status = %q, want skipped", gotC.Status)
	}
}

func TestBuildStepRepository_SkipNoFalsePositive(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusPending}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	lint := &BuildStep{BuildID: build.ID, Name: "lint", Image: &image, Status: StepStatusSuccess}
	test := &BuildStep{BuildID: build.ID, Name: "test", Image: &image, Status: StepStatusWaiting}

	if err := stepRepo.Create(ctx, lint); err != nil {
		t.Fatalf("Create lint error = %v", err)
	}
	if err := stepRepo.Create(ctx, test); err != nil {
		t.Fatalf("Create test error = %v", err)
	}
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}

	// All deps succeeded — should NOT skip
	n, err := stepRepo.SkipDependentSteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("SkipDependentSteps() error = %v", err)
	}
	if n != 0 {
		t.Errorf("skipped = %d, want 0", n)
	}

	got, _ := stepRepo.GetByID(ctx, test.ID)
	if got.Status != StepStatusWaiting {
		t.Errorf("test.Status = %q, want %q", got.Status, StepStatusWaiting)
	}
}

func TestBuildStepRepository_CancelBuildSteps(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusRunning}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	// Create steps in various states
	steps := []*BuildStep{
		{BuildID: build.ID, Name: "pending", Image: &image, Status: StepStatusPending},
		{BuildID: build.ID, Name: "waiting", Image: &image, Status: StepStatusWaiting},
		{BuildID: build.ID, Name: "ready", Image: &image, Status: StepStatusReady},
		{BuildID: build.ID, Name: "running", Image: &image, Status: StepStatusRunning},
		{BuildID: build.ID, Name: "success", Image: &image, Status: StepStatusSuccess},
	}
	for _, s := range steps {
		if err := stepRepo.Create(ctx, s); err != nil {
			t.Fatalf("Create %s error = %v", s.Name, err)
		}
	}

	n, err := stepRepo.CancelBuildSteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("CancelBuildSteps() error = %v", err)
	}
	// pending, waiting, ready should be cancelled (3); running and success untouched
	if n != 3 {
		t.Errorf("cancelled = %d, want 3", n)
	}

	// Verify running is still running
	gotRunning, _ := stepRepo.GetByID(ctx, steps[3].ID)
	if gotRunning.Status != StepStatusRunning {
		t.Errorf("running.Status = %q, want %q", gotRunning.Status, StepStatusRunning)
	}

	// Verify success is still success
	gotSuccess, _ := stepRepo.GetByID(ctx, steps[4].ID)
	if gotSuccess.Status != StepStatusSuccess {
		t.Errorf("success.Status = %q, want %q", gotSuccess.Status, StepStatusSuccess)
	}
}

func TestBuildStepRepository_ResetStepsForWorker(t *testing.T) {
	db := setupBuildTestDB(t)
	defer db.Close()

	project := createTestProject(t, db)
	buildRepo := NewBuildRepository(db)
	stepRepo := NewBuildStepRepository(db)
	ctx := context.Background()

	build := &Build{ProjectID: project.ID, CommitSHA: "abc123", Status: BuildStatusRunning}
	if err := buildRepo.Create(ctx, build); err != nil {
		t.Fatalf("Create build error = %v", err)
	}

	image := "golang:1.22"
	// Two running steps for "stale-worker", one for "other-worker"
	s1 := &BuildStep{BuildID: build.ID, Name: "s1", Image: &image, Status: StepStatusReady}
	s2 := &BuildStep{BuildID: build.ID, Name: "s2", Image: &image, Status: StepStatusReady}
	s3 := &BuildStep{BuildID: build.ID, Name: "s3", Image: &image, Status: StepStatusReady}
	for _, s := range []*BuildStep{s1, s2, s3} {
		if err := stepRepo.Create(ctx, s); err != nil {
			t.Fatalf("Create error = %v", err)
		}
	}

	// Mark as started by different workers
	if err := stepRepo.SetStarted(ctx, s1.ID, "stale-worker"); err != nil {
		t.Fatalf("SetStarted error = %v", err)
	}
	if err := stepRepo.SetStarted(ctx, s2.ID, "stale-worker"); err != nil {
		t.Fatalf("SetStarted error = %v", err)
	}
	if err := stepRepo.SetStarted(ctx, s3.ID, "other-worker"); err != nil {
		t.Fatalf("SetStarted error = %v", err)
	}

	if err := stepRepo.ResetStepsForWorker(ctx, "stale-worker"); err != nil {
		t.Fatalf("ResetStepsForWorker() error = %v", err)
	}

	// s1, s2 should be reset to ready
	got1, _ := stepRepo.GetByID(ctx, s1.ID)
	got2, _ := stepRepo.GetByID(ctx, s2.ID)
	got3, _ := stepRepo.GetByID(ctx, s3.ID)

	if got1.Status != StepStatusReady {
		t.Errorf("s1.Status = %q, want ready", got1.Status)
	}
	if got1.WorkerID != nil {
		t.Errorf("s1.WorkerID = %v, want nil", got1.WorkerID)
	}
	if got1.StartedAt != nil {
		t.Errorf("s1.StartedAt = %v, want nil", got1.StartedAt)
	}
	if got2.Status != StepStatusReady {
		t.Errorf("s2.Status = %q, want ready", got2.Status)
	}
	// s3 should be untouched
	if got3.Status != StepStatusRunning {
		t.Errorf("s3.Status = %q, want running", got3.Status)
	}
}

func TestBuildStepRepository_UpdateReadySteps_ApprovalSteps(t *testing.T) {
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
	deploy := &BuildStep{BuildID: build.ID, Name: "deploy", Image: &image, Status: StepStatusWaiting, RequiresApproval: true}

	if err := stepRepo.Create(ctx, lint); err != nil {
		t.Fatalf("Create lint error = %v", err)
	}
	if err := stepRepo.Create(ctx, test); err != nil {
		t.Fatalf("Create test error = %v", err)
	}
	if err := stepRepo.Create(ctx, deploy); err != nil {
		t.Fatalf("Create deploy error = %v", err)
	}

	// test depends on lint, deploy depends on lint
	if err := stepRepo.AddDependency(ctx, test.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}
	if err := stepRepo.AddDependency(ctx, deploy.ID, lint.ID); err != nil {
		t.Fatalf("AddDependency error = %v", err)
	}

	// Mark lint as success
	if err := stepRepo.UpdateStatus(ctx, lint.ID, StepStatusSuccess); err != nil {
		t.Fatalf("UpdateStatus error = %v", err)
	}

	// UpdateReadySteps should transition test → ready, deploy → waiting_approval
	updated, err := stepRepo.UpdateReadySteps(ctx, build.ID)
	if err != nil {
		t.Fatalf("UpdateReadySteps() error = %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2", updated)
	}

	gotTest, err := stepRepo.GetByID(ctx, test.ID)
	if err != nil {
		t.Fatalf("GetByID(test) error = %v", err)
	}
	if gotTest.Status != StepStatusReady {
		t.Errorf("test.Status = %q, want %q", gotTest.Status, StepStatusReady)
	}

	gotDeploy, err := stepRepo.GetByID(ctx, deploy.ID)
	if err != nil {
		t.Fatalf("GetByID(deploy) error = %v", err)
	}
	if gotDeploy.Status != StepStatusWaitingApproval {
		t.Errorf("deploy.Status = %q, want %q", gotDeploy.Status, StepStatusWaitingApproval)
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
