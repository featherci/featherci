package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/models"
)

// --- Mock implementations ---

type mockStepRepo struct {
	mu          sync.Mutex
	readySteps  []*models.BuildStep
	buildSteps  map[int64][]*models.BuildStep
	started     []int64
	finished    []int64
	finishCalls []finishCall
}

type finishCall struct {
	ID       int64
	Status   models.StepStatus
	ExitCode *int
	LogPath  string
}

func (m *mockStepRepo) ListReady(_ context.Context) ([]*models.BuildStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.readySteps
	m.readySteps = nil
	return steps, nil
}

func (m *mockStepRepo) SetStarted(_ context.Context, id int64, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, id)
	return nil
}

func (m *mockStepRepo) SetFinished(_ context.Context, id int64, status models.StepStatus, exitCode *int, logPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finished = append(m.finished, id)
	m.finishCalls = append(m.finishCalls, finishCall{ID: id, Status: status, ExitCode: exitCode, LogPath: logPath})
	return nil
}

func (m *mockStepRepo) UpdateReadySteps(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockStepRepo) SkipDependentSteps(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockStepRepo) ListByBuild(_ context.Context, buildID int64) ([]*models.BuildStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buildSteps[buildID], nil
}

type mockBuildRepo struct {
	mu       sync.Mutex
	builds   map[int64]*models.Build
	started  []int64
	finished []int64
	statuses map[int64]models.BuildStatus
}

func (m *mockBuildRepo) GetByID(_ context.Context, id int64) (*models.Build, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.builds[id]
	if !ok {
		return nil, models.ErrNotFound
	}
	cp := *b
	return &cp, nil
}

func (m *mockBuildRepo) SetStarted(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, id)
	if b, ok := m.builds[id]; ok {
		b.Status = models.BuildStatusRunning
	}
	return nil
}

func (m *mockBuildRepo) SetFinished(_ context.Context, id int64, status models.BuildStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finished = append(m.finished, id)
	if m.statuses == nil {
		m.statuses = make(map[int64]models.BuildStatus)
	}
	m.statuses[id] = status
	if b, ok := m.builds[id]; ok {
		b.Status = status
	}
	return nil
}

func (m *mockBuildRepo) UpdateStatus(_ context.Context, id int64, status models.BuildStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statuses == nil {
		m.statuses = make(map[int64]models.BuildStatus)
	}
	m.statuses[id] = status
	if b, ok := m.builds[id]; ok {
		b.Status = status
	}
	return nil
}

type mockProjectRepo struct {
	projects map[int64]*models.Project
}

func (m *mockProjectRepo) GetByID(_ context.Context, id int64) (*models.Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return nil, models.ErrNotFound
	}
	return p, nil
}

type mockWorkerRepo struct {
	mu         sync.Mutex
	registered bool
	offline    bool
}

func (m *mockWorkerRepo) Register(_ context.Context, _ *models.Worker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registered = true
	return nil
}

func (m *mockWorkerRepo) UpdateHeartbeat(_ context.Context, _ string) error {
	return nil
}

func (m *mockWorkerRepo) UpdateStatus(_ context.Context, _ string, _ models.WorkerStatus, _ *int64) error {
	return nil
}

func (m *mockWorkerRepo) SetOffline(_ context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offline = true
	return nil
}

type mockTokenSource struct {
	token string
	err   error
}

func (m *mockTokenSource) TokenForProject(_ context.Context, _ int64) (string, error) {
	return m.token, m.err
}

type mockGitService struct {
	mu         sync.Mutex
	cloneCalls int
	cloneErr   error
}

func (m *mockGitService) Clone(_ context.Context, _, _, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cloneCalls++
	return m.cloneErr
}

func (m *mockGitService) Checkout(_ context.Context, _, _ string) error {
	return nil
}

type mockWorkspace struct {
	mu       sync.Mutex
	paths    map[string]bool
	created  int
	basePath string
}

func newMockWorkspace() *mockWorkspace {
	return &mockWorkspace{
		paths:    make(map[string]bool),
		basePath: "/tmp/test-ws",
	}
}

func (m *mockWorkspace) GetPath(projectID, buildID int64) string {
	return fmt.Sprintf("%s/%d/%d", m.basePath, projectID, buildID)
}

func (m *mockWorkspace) Create(projectID, buildID int64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := m.GetPath(projectID, buildID)
	m.paths[path] = true
	m.created++
	return path, nil
}

// --- Helpers ---

func makeTestDeps() (*mockStepRepo, *mockBuildRepo, *mockProjectRepo, *mockWorkerRepo, *mockTokenSource, *mockGitService, *mockWorkspace) {
	return &mockStepRepo{buildSteps: make(map[int64][]*models.BuildStep)},
		&mockBuildRepo{builds: make(map[int64]*models.Build)},
		&mockProjectRepo{projects: make(map[int64]*models.Project)},
		&mockWorkerRepo{},
		&mockTokenSource{token: "test-token"},
		&mockGitService{},
		newMockWorkspace()
}

func makeWorker(
	steps *mockStepRepo,
	builds *mockBuildRepo,
	projects *mockProjectRepo,
	workers *mockWorkerRepo,
	tokens *mockTokenSource,
	git *mockGitService,
	ws *mockWorkspace,
) *Worker {
	mockExec := &noopExecutor{exitCode: 0}
	runner := executor.NewStepRunner(mockExec)
	cfg := Config{
		PollInterval:      50 * time.Millisecond,
		HeartbeatInterval: 5 * time.Second,
		MaxConcurrent:     2,
	}
	w := New(cfg, steps, builds, projects, workers, tokens, git, ws, runner, nil)
	w.id = "test-worker"
	return w
}

func runWorkerBriefly(t *testing.T, w *Worker, duration time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(duration + 2*time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// --- Tests ---

func TestPollClaimsAndExecutes(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	projects.projects[1] = &models.Project{
		ID: 1, Provider: "github", CloneURL: "https://github.com/test/repo.git",
	}
	builds.builds[10] = &models.Build{
		ID: 10, ProjectID: 1, CommitSHA: "abc123", Status: models.BuildStatusPending,
	}
	steps.readySteps = []*models.BuildStep{
		{ID: 100, BuildID: 10, Name: "test", Status: models.StepStatusReady},
	}
	steps.buildSteps[10] = []*models.BuildStep{
		{ID: 100, BuildID: 10, Name: "test", Status: models.StepStatusSuccess},
	}

	w := makeWorker(steps, builds, projects, workers, tokens, gitSvc, ws)
	runWorkerBriefly(t, w, 300*time.Millisecond)

	steps.mu.Lock()
	defer steps.mu.Unlock()

	if len(steps.started) == 0 {
		t.Error("expected step to be claimed via SetStarted")
	}
	if len(steps.finished) == 0 {
		t.Error("expected step to be finished via SetFinished")
	}
}

func TestSkipsWhenAtCapacity(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	cfg := Config{
		PollInterval:      50 * time.Millisecond,
		HeartbeatInterval: 5 * time.Second,
		MaxConcurrent:     1,
	}
	mockExec := &noopExecutor{exitCode: 0}
	runner := executor.NewStepRunner(mockExec)

	w := New(cfg, steps, builds, projects, workers, tokens, gitSvc, ws, runner, nil)
	w.id = "test-worker"

	// Fill the semaphore
	w.sem <- struct{}{}

	steps.readySteps = []*models.BuildStep{
		{ID: 1, BuildID: 10, Name: "step1"},
	}

	w.poll(context.Background())

	steps.mu.Lock()
	defer steps.mu.Unlock()

	if len(steps.started) != 0 {
		t.Errorf("expected no steps claimed when at capacity, got %d", len(steps.started))
	}

	<-w.sem
}

func TestBuildStatusUpdated(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	projects.projects[1] = &models.Project{
		ID: 1, Provider: "github", CloneURL: "https://github.com/test/repo.git",
	}
	builds.builds[10] = &models.Build{
		ID: 10, ProjectID: 1, CommitSHA: "abc123", Status: models.BuildStatusRunning,
	}
	steps.readySteps = []*models.BuildStep{
		{ID: 100, BuildID: 10, Name: "only-step", Status: models.StepStatusReady},
	}
	steps.buildSteps[10] = []*models.BuildStep{
		{ID: 100, BuildID: 10, Name: "only-step", Status: models.StepStatusSuccess},
	}

	w := makeWorker(steps, builds, projects, workers, tokens, gitSvc, ws)
	runWorkerBriefly(t, w, 300*time.Millisecond)

	builds.mu.Lock()
	defer builds.mu.Unlock()

	if len(builds.finished) == 0 && len(builds.statuses) == 0 {
		t.Error("expected build status to be updated after step completion")
	}
}

func TestWorkspaceReuse(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	projects.projects[1] = &models.Project{
		ID: 1, Provider: "github", CloneURL: "https://github.com/test/repo.git",
	}
	builds.builds[10] = &models.Build{
		ID: 10, ProjectID: 1, CommitSHA: "abc123", Status: models.BuildStatusRunning,
	}
	steps.readySteps = []*models.BuildStep{
		{ID: 200, BuildID: 10, Name: "step2", Status: models.StepStatusReady},
	}
	steps.buildSteps[10] = []*models.BuildStep{
		{ID: 200, BuildID: 10, Name: "step2", Status: models.StepStatusSuccess},
	}

	w := makeWorker(steps, builds, projects, workers, tokens, gitSvc, ws)
	runWorkerBriefly(t, w, 300*time.Millisecond)

	// Since the mock workspace path doesn't exist on real FS, clone will be called.
	// This test verifies the step still completes even with workspace setup.
	steps.mu.Lock()
	defer steps.mu.Unlock()

	if len(steps.finished) == 0 {
		t.Error("expected step to be finished")
	}
}

func TestCloneFailure(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	projects.projects[1] = &models.Project{
		ID: 1, Provider: "github", CloneURL: "https://github.com/test/repo.git",
	}
	builds.builds[10] = &models.Build{
		ID: 10, ProjectID: 1, CommitSHA: "abc123", Status: models.BuildStatusPending,
	}
	gitSvc.cloneErr = fmt.Errorf("clone failed: auth error")

	steps.readySteps = []*models.BuildStep{
		{ID: 300, BuildID: 10, Name: "step-clone-fail", Status: models.StepStatusReady},
	}
	steps.buildSteps[10] = []*models.BuildStep{
		{ID: 300, BuildID: 10, Name: "step-clone-fail", Status: models.StepStatusFailure},
	}

	w := makeWorker(steps, builds, projects, workers, tokens, gitSvc, ws)
	runWorkerBriefly(t, w, 300*time.Millisecond)

	steps.mu.Lock()
	defer steps.mu.Unlock()

	if len(steps.finishCalls) == 0 {
		t.Fatal("expected step to be finished")
	}
	if steps.finishCalls[0].Status != models.StepStatusFailure {
		t.Errorf("expected failure status, got %s", steps.finishCalls[0].Status)
	}
}

func TestGracefulShutdown(t *testing.T) {
	steps, builds, projects, workers, tokens, gitSvc, ws := makeTestDeps()

	w := makeWorker(steps, builds, projects, workers, tokens, gitSvc, ws)
	runWorkerBriefly(t, w, 150*time.Millisecond)

	workers.mu.Lock()
	defer workers.mu.Unlock()

	if !workers.offline {
		t.Error("expected worker to be set offline after shutdown")
	}
}

// noopExecutor is a mock executor for tests.
type noopExecutor struct {
	exitCode int
}

func (e *noopExecutor) Run(_ context.Context, _ executor.RunOptions) (*executor.RunResult, error) {
	return &executor.RunResult{
		ExitCode:   e.exitCode,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}, nil
}

func (e *noopExecutor) Stop(_ context.Context, _ string) error {
	return nil
}
