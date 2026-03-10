// Package worker provides the build execution worker that polls for ready
// steps, clones repositories, runs steps in Docker, and updates statuses.
package worker

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/models"
)

// stepRepo is a subset of models.BuildStepRepository needed by the worker.
type stepRepo interface {
	ListReady(ctx context.Context) ([]*models.BuildStep, error)
	SetStarted(ctx context.Context, id int64, workerID string) error
	SetFinished(ctx context.Context, id int64, status models.StepStatus, exitCode *int, logPath string) error
	UpdateReadySteps(ctx context.Context, buildID int64) (int64, error)
	ListByBuild(ctx context.Context, buildID int64) ([]*models.BuildStep, error)
}

// buildRepo is a subset of models.BuildRepository needed by the worker.
type buildRepo interface {
	GetByID(ctx context.Context, id int64) (*models.Build, error)
	SetStarted(ctx context.Context, id int64) error
	SetFinished(ctx context.Context, id int64, status models.BuildStatus) error
	UpdateStatus(ctx context.Context, id int64, status models.BuildStatus) error
}

// projectRepo is a subset of models.ProjectRepository needed by the worker.
type projectRepo interface {
	GetByID(ctx context.Context, id int64) (*models.Project, error)
}

// workerRepo is a subset of models.WorkerRepository needed by the worker.
type workerRepo interface {
	Register(ctx context.Context, worker *models.Worker) error
	UpdateHeartbeat(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status models.WorkerStatus, currentStepID *int64) error
	SetOffline(ctx context.Context, id string) error
}

// tokenSource provides git access tokens for cloning.
type tokenSource interface {
	TokenForProject(ctx context.Context, projectID int64) (string, error)
}

// gitService abstracts git clone/checkout operations.
type gitService interface {
	Clone(ctx context.Context, cloneURL, token, provider, destDir string) error
	Checkout(ctx context.Context, repoDir, commitSHA string) error
}

// workspaceManager abstracts workspace path management.
type workspaceManager interface {
	GetPath(projectID, buildID int64) string
	Create(projectID, buildID int64) (string, error)
}

// Config holds worker configuration.
type Config struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	MaxConcurrent     int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:      2 * time.Second,
		HeartbeatInterval: 15 * time.Second,
		MaxConcurrent:     2,
	}
}

// Worker polls for ready build steps and executes them.
type Worker struct {
	id        string
	cfg       Config
	steps     stepRepo
	builds    buildRepo
	projects  projectRepo
	workers   workerRepo
	tokens    tokenSource
	git       gitService
	workspace workspaceManager
	runner    *executor.StepRunner
	sem       chan struct{}
	wg        sync.WaitGroup
	logger    *slog.Logger
}

// New creates a new Worker.
func New(
	cfg Config,
	steps stepRepo,
	builds buildRepo,
	projects projectRepo,
	workers workerRepo,
	tokens tokenSource,
	git gitService,
	workspace workspaceManager,
	runner *executor.StepRunner,
	logger *slog.Logger,
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		cfg:       cfg,
		steps:     steps,
		builds:    builds,
		projects:  projects,
		workers:   workers,
		tokens:    tokens,
		git:       git,
		workspace: workspace,
		runner:    runner,
		sem:       make(chan struct{}, cfg.MaxConcurrent),
		logger:    logger,
	}
}

// Start begins the worker poll loop and heartbeat. It blocks until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) error {
	id, err := w.generateID()
	if err != nil {
		return fmt.Errorf("generating worker ID: %w", err)
	}
	w.id = id

	// Register in DB
	worker := &models.Worker{
		ID:     w.id,
		Name:   w.id,
		Status: models.WorkerStatusIdle,
	}
	if err := w.workers.Register(ctx, worker); err != nil {
		return fmt.Errorf("registering worker: %w", err)
	}

	w.logger.Info("worker started", "id", w.id, "max_concurrent", w.cfg.MaxConcurrent)

	// Start heartbeat goroutine
	go w.heartbeatLoop(ctx)

	// Poll loop
	pollTicker := time.NewTicker(w.cfg.PollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker shutting down, waiting for in-flight steps...")
			w.wg.Wait()
			_ = w.workers.SetOffline(context.Background(), w.id)
			w.logger.Info("worker stopped", "id", w.id)
			return nil
		case <-pollTicker.C:
			w.poll(ctx)
		}
	}
}

func (w *Worker) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.workers.UpdateHeartbeat(ctx, w.id); err != nil {
				w.logger.Error("heartbeat failed", "error", err)
			}
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	readySteps, err := w.steps.ListReady(ctx)
	if err != nil {
		w.logger.Error("failed to list ready steps", "error", err)
		return
	}

	for _, step := range readySteps {
		// Non-blocking semaphore acquire
		select {
		case w.sem <- struct{}{}:
			// Got slot
		default:
			// At capacity
			return
		}

		// Claim step synchronously before spawning goroutine
		if err := w.steps.SetStarted(ctx, step.ID, w.id); err != nil {
			<-w.sem // release slot
			w.logger.Error("failed to claim step", "step_id", step.ID, "error", err)
			continue
		}

		w.wg.Add(1)
		go func(s *models.BuildStep) {
			defer w.wg.Done()
			defer func() { <-w.sem }()
			w.executeStep(ctx, s)
		}(step)
	}
}

func (w *Worker) executeStep(ctx context.Context, step *models.BuildStep) {
	log := w.logger.With("step_id", step.ID, "build_id", step.BuildID)

	// Load build
	build, err := w.builds.GetByID(ctx, step.BuildID)
	if err != nil {
		log.Error("failed to load build", "error", err)
		w.failStep(ctx, step, log)
		return
	}

	// Load project
	project, err := w.projects.GetByID(ctx, build.ProjectID)
	if err != nil {
		log.Error("failed to load project", "error", err)
		w.failStep(ctx, step, log)
		return
	}

	// If build is pending, mark it started
	if build.Status == models.BuildStatusPending {
		if err := w.builds.SetStarted(ctx, build.ID); err != nil {
			log.Error("failed to start build", "error", err)
		}
	}

	// Set up workspace: reuse if already exists
	wsPath := w.workspace.GetPath(project.ID, build.ID)
	if _, statErr := os.Stat(wsPath); os.IsNotExist(statErr) {
		if _, err := w.workspace.Create(project.ID, build.ID); err != nil {
			log.Error("failed to create workspace", "error", err)
			w.failStep(ctx, step, log)
			return
		}

		// Get token for cloning
		token, err := w.tokens.TokenForProject(ctx, project.ID)
		if err != nil {
			log.Error("failed to get token", "error", err)
			w.failStep(ctx, step, log)
			return
		}

		// Clone and checkout
		if err := w.git.Clone(ctx, project.CloneURL, token, project.Provider, wsPath); err != nil {
			log.Error("clone failed", "error", err)
			w.failStep(ctx, step, log)
			return
		}

		if err := w.git.Checkout(ctx, wsPath, build.CommitSHA); err != nil {
			log.Error("checkout failed", "error", err)
			w.failStep(ctx, step, log)
			return
		}
	}

	// Update worker status to busy
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusBusy, &step.ID)

	// Run step
	log.Info("running step", "name", step.Name)
	result := w.runner.RunStep(ctx, step, wsPath)

	// Persist result
	if err := w.steps.SetFinished(ctx, step.ID, result.Status, result.ExitCode, result.LogPath); err != nil {
		log.Error("failed to set step finished", "error", err)
	}

	// Unblock dependents
	if _, err := w.steps.UpdateReadySteps(ctx, step.BuildID); err != nil {
		log.Error("failed to update ready steps", "error", err)
	}

	// Recalculate build status
	w.recalcBuildStatus(ctx, build.ID, log)

	// Update worker status to idle
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusIdle, nil)

	log.Info("step completed", "name", step.Name, "status", result.Status)
}

func (w *Worker) failStep(ctx context.Context, step *models.BuildStep, log *slog.Logger) {
	if err := w.steps.SetFinished(ctx, step.ID, models.StepStatusFailure, nil, ""); err != nil {
		log.Error("failed to mark step as failed", "error", err)
	}
	if _, err := w.steps.UpdateReadySteps(ctx, step.BuildID); err != nil {
		log.Error("failed to update ready steps after failure", "error", err)
	}
	w.recalcBuildStatus(ctx, step.BuildID, log)
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusIdle, nil)
}

func (w *Worker) recalcBuildStatus(ctx context.Context, buildID int64, log *slog.Logger) {
	allSteps, err := w.steps.ListByBuild(ctx, buildID)
	if err != nil {
		log.Error("failed to list build steps for status calc", "error", err)
		return
	}

	build, err := w.builds.GetByID(ctx, buildID)
	if err != nil {
		log.Error("failed to reload build for status calc", "error", err)
		return
	}

	build.Steps = allSteps
	newStatus := build.CalculateStatus()

	if newStatus != build.Status {
		if newStatus == models.BuildStatusSuccess || newStatus == models.BuildStatusFailure {
			if err := w.builds.SetFinished(ctx, buildID, newStatus); err != nil {
				log.Error("failed to finish build", "error", err)
			}
		} else {
			if err := w.builds.UpdateStatus(ctx, buildID, newStatus); err != nil {
				log.Error("failed to update build status", "error", err)
			}
		}
	}
}

func (w *Worker) generateID() (string, error) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%x", hostname, suffix), nil
}
