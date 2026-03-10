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

	"github.com/featherci/featherci/internal/cache"
	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/models"
)

// stepRepo is a subset of models.BuildStepRepository needed by the worker.
type stepRepo interface {
	ListReady(ctx context.Context) ([]*models.BuildStep, error)
	SetStarted(ctx context.Context, id int64, workerID string) error
	SetFinished(ctx context.Context, id int64, status models.StepStatus, exitCode *int, logPath string) error
	UpdateReadySteps(ctx context.Context, buildID int64) (int64, error)
	SkipDependentSteps(ctx context.Context, buildID int64) (int64, error)
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

// secretSource provides decrypted secrets for builds.
type secretSource interface {
	GetDecryptedSecrets(ctx context.Context, projectID int64) (map[string]string, error)
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

// statusPoster posts commit statuses to git providers.
type statusPoster interface {
	PostBuildStatus(ctx context.Context, project *models.Project, build *models.Build)
	PostStepStatus(ctx context.Context, project *models.Project, build *models.Build, stepName string, stepStatus models.StepStatus)
}

// buildNotifier sends notifications when builds reach terminal state.
type buildNotifier interface {
	NotifyBuild(ctx context.Context, build *models.Build, project *models.Project) error
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
	secrets   secretSource
	git       gitService
	workspace workspaceManager
	runner    *executor.StepRunner
	status    statusPoster
	notifier  buildNotifier
	sem       chan struct{}
	wg        sync.WaitGroup
	logger    *slog.Logger

	// cloneMu guards per-build workspace setup so only one step clones.
	cloneMu   sync.Mutex
	cloneOnce map[int64]*cloneState
}

// cloneState tracks per-build workspace clone, ensuring only one goroutine clones.
type cloneState struct {
	once sync.Once
	err  error
}

// New creates a new Worker.
func New(
	cfg Config,
	steps stepRepo,
	builds buildRepo,
	projects projectRepo,
	workers workerRepo,
	tokens tokenSource,
	secrets secretSource,
	git gitService,
	workspace workspaceManager,
	runner *executor.StepRunner,
	status statusPoster,
	notifier buildNotifier,
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
		secrets:   secrets,
		git:       git,
		workspace: workspace,
		runner:    runner,
		status:    status,
		notifier:  notifier,
		sem:       make(chan struct{}, cfg.MaxConcurrent),
		cloneOnce: make(map[int64]*cloneState),
		logger:    logger,
	}
}

// SetID sets a fixed worker ID, overriding the default random generation.
// Must be called before Start.
func (w *Worker) SetID(id string) {
	w.id = id
}

// Start begins the worker poll loop and heartbeat. It blocks until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) error {
	if w.id == "" {
		id, err := w.generateID()
		if err != nil {
			return fmt.Errorf("generating worker ID: %w", err)
		}
		w.id = id
	}

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

	// If build is pending, mark it started and post initial statuses for all steps
	if build.Status == models.BuildStatusPending {
		if err := w.builds.SetStarted(ctx, build.ID); err != nil {
			log.Error("failed to start build", "error", err)
		}
		// Post pending status for all steps so they appear on the commit
		allSteps, err := w.steps.ListByBuild(ctx, build.ID)
		if err == nil {
			for _, s := range allSteps {
				go w.status.PostStepStatus(context.Background(), project, build, s.Name, models.StepStatusPending)
			}
		}
	}

	// Set up workspace: use sync.Once per build so only one step clones
	wsPath := w.workspace.GetPath(project.ID, build.ID)
	cs := w.getCloneState(build.ID)
	cs.once.Do(func() {
		if _, statErr := os.Stat(wsPath); !os.IsNotExist(statErr) {
			return // workspace already exists from a previous build attempt
		}
		if _, err := w.workspace.Create(project.ID, build.ID); err != nil {
			cs.err = fmt.Errorf("failed to create workspace: %w", err)
			return
		}

		token, err := w.tokens.TokenForProject(ctx, project.ID)
		if err != nil {
			cs.err = fmt.Errorf("failed to get token: %w", err)
			return
		}

		if err := w.git.Clone(ctx, project.CloneURL, token, project.Provider, wsPath); err != nil {
			cs.err = fmt.Errorf("clone failed: %w", err)
			return
		}

		if err := w.git.Checkout(ctx, wsPath, build.CommitSHA); err != nil {
			cs.err = fmt.Errorf("checkout failed: %w", err)
			return
		}
	})
	if cs.err != nil {
		log.Error("workspace setup failed", "error", cs.err)
		w.failStepWithContext(ctx, step, project, build, log)
		return
	}

	// Inject project secrets into step env (secrets as base, step env overrides)
	// Collect secret values for log masking.
	var secretValues []string
	if w.secrets != nil {
		projectSecrets, err := w.secrets.GetDecryptedSecrets(ctx, project.ID)
		if err != nil {
			log.Error("failed to get secrets", "error", err)
			// Non-fatal: continue without secrets
		} else if len(projectSecrets) > 0 {
			if step.Env == nil {
				step.Env = make(map[string]string, len(projectSecrets))
			}
			for k, v := range projectSecrets {
				secretValues = append(secretValues, v)
				if _, exists := step.Env[k]; !exists {
					step.Env[k] = v
				}
			}
		}
	}

	// Resolve cache key if step has cache config
	if step.Cache != nil {
		branch := ""
		if build.Branch != nil {
			branch = *build.Branch
		}
		step.CacheResolvedKey = cache.ResolveKey(step.Cache.Key, project.ID, branch, wsPath)
	}

	// Update worker status to busy
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusBusy, &step.ID)

	// Post step running status
	go w.status.PostStepStatus(context.Background(), project, build, step.Name, models.StepStatusRunning)

	// Run step
	log.Info("running step", "name", step.Name)
	result := w.runner.RunStep(ctx, step, wsPath, secretValues)

	// Persist result
	if err := w.steps.SetFinished(ctx, step.ID, result.Status, result.ExitCode, result.LogPath); err != nil {
		log.Error("failed to set step finished", "error", err)
	}

	// Post step finished status
	go w.status.PostStepStatus(context.Background(), project, build, step.Name, result.Status)

	// Skip failed dependents, unblock ready steps, recalculate build status
	w.advanceBuild(ctx, build.ID, log)

	// Update worker status to idle
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusIdle, nil)

	log.Info("step completed", "name", step.Name, "status", result.Status)
}

func (w *Worker) failStep(ctx context.Context, step *models.BuildStep, log *slog.Logger) {
	w.failStepWithContext(ctx, step, nil, nil, log)
}

func (w *Worker) failStepWithContext(ctx context.Context, step *models.BuildStep, project *models.Project, build *models.Build, log *slog.Logger) {
	if err := w.steps.SetFinished(ctx, step.ID, models.StepStatusFailure, nil, ""); err != nil {
		log.Error("failed to mark step as failed", "error", err)
	}
	if project != nil && build != nil {
		go w.status.PostStepStatus(context.Background(), project, build, step.Name, models.StepStatusFailure)
	}
	w.advanceBuild(ctx, step.BuildID, log)
	_ = w.workers.UpdateStatus(ctx, w.id, models.WorkerStatusIdle, nil)
}

func (w *Worker) advanceBuild(ctx context.Context, buildID int64, log *slog.Logger) {
	// Cascade skip any steps whose dependencies have failed
	for {
		n, err := w.steps.SkipDependentSteps(ctx, buildID)
		if err != nil {
			log.Error("failed to skip dependent steps", "error", err)
			break
		}
		if n == 0 {
			break
		}
	}
	// Unblock steps whose dependencies all succeeded
	if _, err := w.steps.UpdateReadySteps(ctx, buildID); err != nil {
		log.Error("failed to update ready steps", "error", err)
	}
	w.recalcBuildStatus(ctx, buildID, log)
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
			w.cleanupCloneOnce(buildID)
			if err := w.builds.SetFinished(ctx, buildID, newStatus); err != nil {
				log.Error("failed to finish build", "error", err)
			}
			// Send build notifications asynchronously
			if w.notifier != nil {
				// Reload build to get updated timestamps
				updatedBuild, err := w.builds.GetByID(ctx, buildID)
				if err != nil {
					log.Error("failed to reload build for notification", "error", err)
				} else {
					project, err := w.projects.GetByID(ctx, updatedBuild.ProjectID)
					if err != nil {
						log.Error("failed to load project for notification", "error", err)
					} else {
						go func() {
							if err := w.notifier.NotifyBuild(context.Background(), updatedBuild, project); err != nil {
								log.Error("failed to send build notification", "error", err)
							}
						}()
					}
				}
			}
		} else {
			if err := w.builds.UpdateStatus(ctx, buildID, newStatus); err != nil {
				log.Error("failed to update build status", "error", err)
			}
		}
	}
}

// getCloneState returns the clone state for the given build, creating one if needed.
func (w *Worker) getCloneState(buildID int64) *cloneState {
	w.cloneMu.Lock()
	defer w.cloneMu.Unlock()
	cs, ok := w.cloneOnce[buildID]
	if !ok {
		cs = &cloneState{}
		w.cloneOnce[buildID] = cs
	}
	return cs
}

// cleanupCloneOnce removes the sync.Once for a completed build to prevent leaks.
func (w *Worker) cleanupCloneOnce(buildID int64) {
	w.cloneMu.Lock()
	defer w.cloneMu.Unlock()
	delete(w.cloneOnce, buildID)
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
