package services

import (
	"context"
	"testing"

	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/webhook"
	"github.com/featherci/featherci/internal/workflow"
)

// mockBuildRepo implements BuildRepository for testing.
type mockBuildRepo struct {
	builds      []*models.Build
	nextBuildNum int
}

func newMockBuildRepo() *mockBuildRepo {
	return &mockBuildRepo{nextBuildNum: 1}
}

func (r *mockBuildRepo) Create(_ context.Context, build *models.Build) error {
	build.ID = int64(len(r.builds) + 1)
	r.builds = append(r.builds, build)
	return nil
}

func (r *mockBuildRepo) GetNextBuildNumber(_ context.Context, _ int64) (int, error) {
	num := r.nextBuildNum
	r.nextBuildNum++
	return num, nil
}

// mockStepRepo implements BuildStepRepository for testing.
type mockStepRepo struct {
	steps        []*models.BuildStep
	dependencies map[int64][]int64
}

func newMockStepRepo() *mockStepRepo {
	return &mockStepRepo{
		dependencies: make(map[int64][]int64),
	}
}

func (r *mockStepRepo) Create(_ context.Context, step *models.BuildStep) error {
	step.ID = int64(len(r.steps) + 1)
	r.steps = append(r.steps, step)
	return nil
}

func (r *mockStepRepo) CreateBatch(_ context.Context, steps []*models.BuildStep) error {
	for _, step := range steps {
		step.ID = int64(len(r.steps) + 1)
		r.steps = append(r.steps, step)
	}
	return nil
}

func (r *mockStepRepo) AddDependency(_ context.Context, stepID, dependsOnID int64) error {
	r.dependencies[stepID] = append(r.dependencies[stepID], dependsOnID)
	return nil
}

func TestCreateBuildFromWebhook(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	project := &models.Project{ID: 1, FullName: "org/repo"}
	event := &webhook.Event{
		CommitSHA:     "abc123",
		CommitMessage: "fix: something",
		CommitAuthor:  "alice",
		Branch:        "main",
	}
	wf := &workflow.Workflow{
		Name: "CI",
		Steps: []workflow.Step{
			{Name: "lint", Image: "golang:1.22", Commands: []string{"golangci-lint run"}},
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}, DependsOn: []string{"lint"}},
		},
	}

	build, err := creator.CreateBuildFromWebhook(ctx, project, event, wf)
	if err != nil {
		t.Fatalf("CreateBuildFromWebhook() error = %v", err)
	}

	if build.ID == 0 {
		t.Error("expected build.ID to be set")
	}
	if build.BuildNumber != 1 {
		t.Errorf("BuildNumber = %d, want 1", build.BuildNumber)
	}
	if build.CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want %q", build.CommitSHA, "abc123")
	}
	if build.Status != models.BuildStatusPending {
		t.Errorf("Status = %q, want %q", build.Status, models.BuildStatusPending)
	}
	if *build.Branch != "main" {
		t.Errorf("Branch = %q, want %q", *build.Branch, "main")
	}

	// Check steps were created
	if len(stepRepo.steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(stepRepo.steps))
	}

	lint := stepRepo.steps[0]
	if lint.Name != "lint" {
		t.Errorf("steps[0].Name = %q, want %q", lint.Name, "lint")
	}
	if lint.Status != models.StepStatusReady {
		t.Errorf("lint.Status = %q, want %q", lint.Status, models.StepStatusReady)
	}

	test := stepRepo.steps[1]
	if test.Name != "test" {
		t.Errorf("steps[1].Name = %q, want %q", test.Name, "test")
	}
	if test.Status != models.StepStatusWaiting {
		t.Errorf("test.Status = %q, want %q", test.Status, models.StepStatusWaiting)
	}

	// Check dependencies
	deps := stepRepo.dependencies[test.ID]
	if len(deps) != 1 || deps[0] != lint.ID {
		t.Errorf("test dependencies = %v, want [%d]", deps, lint.ID)
	}

	// Check steps attached to build
	if len(build.Steps) != 2 {
		t.Errorf("len(build.Steps) = %d, want 2", len(build.Steps))
	}
}

func TestCreateBuildFromWebhook_PullRequest(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	project := &models.Project{ID: 1}
	prNum := 42
	event := &webhook.Event{
		CommitSHA: "def456",
		PullRequest: &webhook.PullRequestEvent{
			Number:       prNum,
			SourceBranch: "feature/foo",
			TargetBranch: "main",
		},
	}
	wf := &workflow.Workflow{
		Name:  "CI",
		Steps: []workflow.Step{{Name: "test", Image: "golang:1.22", Commands: []string{"go test"}}},
	}

	build, err := creator.CreateBuildFromWebhook(ctx, project, event, wf)
	if err != nil {
		t.Fatalf("CreateBuildFromWebhook() error = %v", err)
	}

	if build.PullRequestNumber == nil || *build.PullRequestNumber != 42 {
		t.Errorf("PullRequestNumber = %v, want 42", build.PullRequestNumber)
	}
	if build.Branch == nil || *build.Branch != "feature/foo" {
		t.Errorf("Branch = %v, want %q", build.Branch, "feature/foo")
	}
}

func TestCreateBuildFromWebhook_NoSteps(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	project := &models.Project{ID: 1}
	event := &webhook.Event{CommitSHA: "abc123", Branch: "main"}
	wf := &workflow.Workflow{Name: "CI"}

	build, err := creator.CreateBuildFromWebhook(ctx, project, event, wf)
	if err != nil {
		t.Fatalf("CreateBuildFromWebhook() error = %v", err)
	}

	if len(stepRepo.steps) != 0 {
		t.Errorf("len(steps) = %d, want 0", len(stepRepo.steps))
	}
	if build.Steps != nil {
		t.Errorf("build.Steps = %v, want nil", build.Steps)
	}
}

func TestCreateBuildFromWebhook_ApprovalStep(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	project := &models.Project{ID: 1}
	event := &webhook.Event{CommitSHA: "abc123", Branch: "main"}
	wf := &workflow.Workflow{
		Name: "Deploy",
		Steps: []workflow.Step{
			{Name: "build", Image: "golang:1.22", Commands: []string{"go build"}},
			{Name: "approve", Type: workflow.StepTypeApproval, DependsOn: []string{"build"}},
			{Name: "deploy", Image: "alpine:3", Commands: []string{"deploy.sh"}, DependsOn: []string{"approve"}},
		},
	}

	_, err := creator.CreateBuildFromWebhook(ctx, project, event, wf)
	if err != nil {
		t.Fatalf("CreateBuildFromWebhook() error = %v", err)
	}

	// Build step should be ready (no deps)
	if stepRepo.steps[0].Status != models.StepStatusReady {
		t.Errorf("build.Status = %q, want %q", stepRepo.steps[0].Status, models.StepStatusReady)
	}

	// Approve step has deps, so it should be waiting (not waiting_approval yet)
	if stepRepo.steps[1].Status != models.StepStatusWaiting {
		t.Errorf("approve.Status = %q, want %q", stepRepo.steps[1].Status, models.StepStatusWaiting)
	}
	if !stepRepo.steps[1].RequiresApproval {
		t.Error("approve.RequiresApproval should be true")
	}

	// Deploy step should be waiting (has deps)
	if stepRepo.steps[2].Status != models.StepStatusWaiting {
		t.Errorf("deploy.Status = %q, want %q", stepRepo.steps[2].Status, models.StepStatusWaiting)
	}
}

func TestCreateBuild_Manual(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	wf := &workflow.Workflow{
		Name: "CI",
		Steps: []workflow.Step{
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}},
		},
	}

	build, err := creator.CreateBuild(ctx, 1, "sha123", "manual build", "admin", "main", wf)
	if err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	if build.CommitSHA != "sha123" {
		t.Errorf("CommitSHA = %q, want %q", build.CommitSHA, "sha123")
	}
	if *build.CommitMessage != "manual build" {
		t.Errorf("CommitMessage = %q, want %q", *build.CommitMessage, "manual build")
	}
	if len(stepRepo.steps) != 1 {
		t.Errorf("len(steps) = %d, want 1", len(stepRepo.steps))
	}
}

func TestCreateBuild_IncrementsBuildNumber(t *testing.T) {
	buildRepo := newMockBuildRepo()
	stepRepo := newMockStepRepo()
	creator := NewBuildCreator(buildRepo, stepRepo)
	ctx := context.Background()

	project := &models.Project{ID: 1}
	event := &webhook.Event{CommitSHA: "abc", Branch: "main"}
	wf := &workflow.Workflow{Name: "CI"}

	build1, _ := creator.CreateBuildFromWebhook(ctx, project, event, wf)
	build2, _ := creator.CreateBuildFromWebhook(ctx, project, event, wf)

	if build1.BuildNumber != 1 {
		t.Errorf("build1.BuildNumber = %d, want 1", build1.BuildNumber)
	}
	if build2.BuildNumber != 2 {
		t.Errorf("build2.BuildNumber = %d, want 2", build2.BuildNumber)
	}
}
