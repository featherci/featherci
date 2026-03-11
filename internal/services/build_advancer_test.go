package services

import (
	"context"
	"testing"

	"github.com/featherci/featherci/internal/models"
)

// --- Mock implementations ---

type mockAdvancerStepRepo struct {
	steps          []*models.BuildStep
	listErr        error
	skipCallCount  int
	skipResults    []int64 // return values for successive SkipDependentSteps calls
	readyUpdated   int64
	readyErr       error
}

func (m *mockAdvancerStepRepo) ListByBuild(_ context.Context, _ int64) ([]*models.BuildStep, error) {
	return m.steps, m.listErr
}

func (m *mockAdvancerStepRepo) UpdateReadySteps(_ context.Context, _ int64) (int64, error) {
	return m.readyUpdated, m.readyErr
}

func (m *mockAdvancerStepRepo) SkipDependentSteps(_ context.Context, _ int64) (int64, error) {
	idx := m.skipCallCount
	m.skipCallCount++
	if idx < len(m.skipResults) {
		return m.skipResults[idx], nil
	}
	return 0, nil
}

type mockAdvancerBuildRepo struct {
	build         *models.Build
	getErr        error
	finishedID    int64
	finishedStat  models.BuildStatus
	updatedID     int64
	updatedStat   models.BuildStatus
	setFinishErr  error
	updateStatErr error
}

func (m *mockAdvancerBuildRepo) GetByID(_ context.Context, id int64) (*models.Build, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	// Return a copy to avoid mutation issues
	b := *m.build
	return &b, nil
}

func (m *mockAdvancerBuildRepo) SetFinished(_ context.Context, id int64, status models.BuildStatus) error {
	m.finishedID = id
	m.finishedStat = status
	return m.setFinishErr
}

func (m *mockAdvancerBuildRepo) UpdateStatus(_ context.Context, id int64, status models.BuildStatus) error {
	m.updatedID = id
	m.updatedStat = status
	return m.updateStatErr
}

type mockAdvancerProjectRepo struct {
	project *models.Project
	getErr  error
}

func (m *mockAdvancerProjectRepo) GetByID(_ context.Context, _ int64) (*models.Project, error) {
	return m.project, m.getErr
}

type mockStatusPoster struct {
	called bool
}

func (m *mockStatusPoster) PostBuildStatus(_ context.Context, _ *models.Project, _ *models.Build) {
	m.called = true
}

type mockNotifier struct {
	called bool
}

func (m *mockNotifier) NotifyBuild(_ context.Context, _ *models.Build, _ *models.Project) error {
	m.called = true
	return nil
}

// --- Tests ---

func TestAdvanceBuild_AllSucceeded_BuildCompletes(t *testing.T) {
	stepRepo := &mockAdvancerStepRepo{
		steps: []*models.BuildStep{
			{ID: 1, BuildID: 10, Status: models.StepStatusSuccess},
			{ID: 2, BuildID: 10, Status: models.StepStatusSuccess},
		},
		skipResults: []int64{0},
	}
	buildRepo := &mockAdvancerBuildRepo{
		build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning},
	}
	projectRepo := &mockAdvancerProjectRepo{
		project: &models.Project{ID: 1, FullName: "org/repo"},
	}

	advancer := NewBuildAdvancer(stepRepo, buildRepo, projectRepo, nil, nil, nil)

	err := advancer.AdvanceBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("AdvanceBuild() error = %v", err)
	}

	if buildRepo.finishedID != 10 {
		t.Errorf("expected SetFinished called with build ID 10, got %d", buildRepo.finishedID)
	}
	if buildRepo.finishedStat != models.BuildStatusSuccess {
		t.Errorf("expected build status success, got %s", buildRepo.finishedStat)
	}
}

func TestAdvanceBuild_StepFailed_DependentsSkipped_BuildFails(t *testing.T) {
	stepRepo := &mockAdvancerStepRepo{
		steps: []*models.BuildStep{
			{ID: 1, BuildID: 10, Status: models.StepStatusSuccess},
			{ID: 2, BuildID: 10, Status: models.StepStatusFailure},
			{ID: 3, BuildID: 10, Status: models.StepStatusSkipped},
		},
		// First call returns 1 (skipped something), second returns 0 (done)
		skipResults: []int64{1, 0},
	}
	buildRepo := &mockAdvancerBuildRepo{
		build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning},
	}
	projectRepo := &mockAdvancerProjectRepo{
		project: &models.Project{ID: 1, FullName: "org/repo"},
	}

	advancer := NewBuildAdvancer(stepRepo, buildRepo, projectRepo, nil, nil, nil)

	err := advancer.AdvanceBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("AdvanceBuild() error = %v", err)
	}

	if buildRepo.finishedStat != models.BuildStatusFailure {
		t.Errorf("expected build status failure, got %s", buildRepo.finishedStat)
	}
	if stepRepo.skipCallCount < 2 {
		t.Errorf("expected at least 2 SkipDependentSteps calls, got %d", stepRepo.skipCallCount)
	}
}

func TestAdvanceBuild_MixedSteps_BuildStaysRunning(t *testing.T) {
	stepRepo := &mockAdvancerStepRepo{
		steps: []*models.BuildStep{
			{ID: 1, BuildID: 10, Status: models.StepStatusSuccess},
			{ID: 2, BuildID: 10, Status: models.StepStatusRunning},
			{ID: 3, BuildID: 10, Status: models.StepStatusWaiting},
		},
		skipResults: []int64{0},
	}
	buildRepo := &mockAdvancerBuildRepo{
		build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning},
	}
	projectRepo := &mockAdvancerProjectRepo{
		project: &models.Project{ID: 1, FullName: "org/repo"},
	}

	advancer := NewBuildAdvancer(stepRepo, buildRepo, projectRepo, nil, nil, nil)

	err := advancer.AdvanceBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("AdvanceBuild() error = %v", err)
	}

	// Build status unchanged (still running), so neither SetFinished nor UpdateStatus should be called
	if buildRepo.finishedID != 0 {
		t.Errorf("expected SetFinished not called, but was called with ID %d", buildRepo.finishedID)
	}
	if buildRepo.updatedID != 0 {
		t.Errorf("expected UpdateStatus not called, but was called with ID %d", buildRepo.updatedID)
	}
}

func TestAdvanceBuild_PendingToRunning_UpdatesStatus(t *testing.T) {
	stepRepo := &mockAdvancerStepRepo{
		steps: []*models.BuildStep{
			{ID: 1, BuildID: 10, Status: models.StepStatusRunning},
			{ID: 2, BuildID: 10, Status: models.StepStatusWaiting},
		},
		skipResults: []int64{0},
	}
	buildRepo := &mockAdvancerBuildRepo{
		build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusPending},
	}
	projectRepo := &mockAdvancerProjectRepo{
		project: &models.Project{ID: 1, FullName: "org/repo"},
	}

	advancer := NewBuildAdvancer(stepRepo, buildRepo, projectRepo, nil, nil, nil)

	err := advancer.AdvanceBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("AdvanceBuild() error = %v", err)
	}

	if buildRepo.updatedID != 10 {
		t.Errorf("expected UpdateStatus called with build ID 10, got %d", buildRepo.updatedID)
	}
	if buildRepo.updatedStat != models.BuildStatusRunning {
		t.Errorf("expected status running, got %s", buildRepo.updatedStat)
	}
}

func TestAdvanceBuild_WithStatusPosterAndNotifier(t *testing.T) {
	stepRepo := &mockAdvancerStepRepo{
		steps: []*models.BuildStep{
			{ID: 1, BuildID: 10, Status: models.StepStatusSuccess},
		},
		skipResults: []int64{0},
	}
	buildRepo := &mockAdvancerBuildRepo{
		build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning},
	}
	projectRepo := &mockAdvancerProjectRepo{
		project: &models.Project{ID: 1, FullName: "org/repo"},
	}
	poster := &mockStatusPoster{}
	notifier := &mockNotifier{}

	advancer := NewBuildAdvancer(stepRepo, buildRepo, projectRepo, poster, notifier, nil)

	err := advancer.AdvanceBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("AdvanceBuild() error = %v", err)
	}

	if buildRepo.finishedStat != models.BuildStatusSuccess {
		t.Errorf("expected build status success, got %s", buildRepo.finishedStat)
	}
	// Status poster and notifier are called in goroutines, so we cannot reliably
	// check them without synchronization. The important thing is no panic.
}
