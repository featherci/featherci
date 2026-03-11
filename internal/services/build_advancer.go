// Package services provides business logic services for FeatherCI.
package services

import (
	"context"
	"log/slog"

	"github.com/featherci/featherci/internal/models"
)

// BuildAdvancerStepRepo is the subset of BuildStepRepository needed by BuildAdvancer.
type BuildAdvancerStepRepo interface {
	ListByBuild(ctx context.Context, buildID int64) ([]*models.BuildStep, error)
	UpdateReadySteps(ctx context.Context, buildID int64) (int64, error)
	SkipDependentSteps(ctx context.Context, buildID int64) (int64, error)
}

// BuildAdvancerBuildRepo is the subset of BuildRepository needed by BuildAdvancer.
type BuildAdvancerBuildRepo interface {
	GetByID(ctx context.Context, id int64) (*models.Build, error)
	SetFinished(ctx context.Context, id int64, status models.BuildStatus) error
	UpdateStatus(ctx context.Context, id int64, status models.BuildStatus) error
}

// BuildAdvancerProjectRepo is the subset of ProjectRepository needed by BuildAdvancer.
type BuildAdvancerProjectRepo interface {
	GetByID(ctx context.Context, id int64) (*models.Project, error)
}

// BuildAdvancerStatusPoster posts commit statuses to git providers.
type BuildAdvancerStatusPoster interface {
	PostBuildStatus(ctx context.Context, project *models.Project, build *models.Build)
}

// BuildAdvancerNotifier sends build notifications.
type BuildAdvancerNotifier interface {
	NotifyBuild(ctx context.Context, build *models.Build, project *models.Project) error
}

// BuildAdvancer handles state transitions after a step completes:
// skipping dependents, unblocking ready steps, recalculating build status,
// sending notifications, and posting commit statuses.
type BuildAdvancer struct {
	steps    BuildAdvancerStepRepo
	builds   BuildAdvancerBuildRepo
	projects BuildAdvancerProjectRepo
	status   BuildAdvancerStatusPoster
	notifier BuildAdvancerNotifier
	logger   *slog.Logger
}

// NewBuildAdvancer creates a new BuildAdvancer.
func NewBuildAdvancer(
	steps BuildAdvancerStepRepo,
	builds BuildAdvancerBuildRepo,
	projects BuildAdvancerProjectRepo,
	status BuildAdvancerStatusPoster,
	notifier BuildAdvancerNotifier,
	logger *slog.Logger,
) *BuildAdvancer {
	if logger == nil {
		logger = slog.Default()
	}
	return &BuildAdvancer{
		steps:    steps,
		builds:   builds,
		projects: projects,
		status:   status,
		notifier: notifier,
		logger:   logger,
	}
}

// AdvanceBuild cascades skips, unblocks ready steps, and recalculates build status.
func (a *BuildAdvancer) AdvanceBuild(ctx context.Context, buildID int64) error {
	log := a.logger.With("build_id", buildID)

	// Cascade skip any steps whose dependencies have failed
	for {
		n, err := a.steps.SkipDependentSteps(ctx, buildID)
		if err != nil {
			log.Error("failed to skip dependent steps", "error", err)
			break
		}
		if n == 0 {
			break
		}
	}

	// Unblock steps whose dependencies all succeeded
	if _, err := a.steps.UpdateReadySteps(ctx, buildID); err != nil {
		log.Error("failed to update ready steps", "error", err)
	}

	return a.recalcBuildStatus(ctx, buildID, log)
}

func (a *BuildAdvancer) recalcBuildStatus(ctx context.Context, buildID int64, log *slog.Logger) error {
	allSteps, err := a.steps.ListByBuild(ctx, buildID)
	if err != nil {
		log.Error("failed to list build steps for status calc", "error", err)
		return err
	}

	build, err := a.builds.GetByID(ctx, buildID)
	if err != nil {
		log.Error("failed to reload build for status calc", "error", err)
		return err
	}

	build.Steps = allSteps
	newStatus := build.CalculateStatus()

	if newStatus == build.Status {
		return nil
	}

	if newStatus == models.BuildStatusSuccess || newStatus == models.BuildStatusFailure {
		if err := a.builds.SetFinished(ctx, buildID, newStatus); err != nil {
			log.Error("failed to finish build", "error", err)
			return err
		}

		// Post build-level commit status
		if a.status != nil {
			project, err := a.projects.GetByID(ctx, build.ProjectID)
			if err == nil {
				updatedBuild, err := a.builds.GetByID(ctx, buildID)
				if err == nil {
					go a.status.PostBuildStatus(context.Background(), project, updatedBuild)
				}
			}
		}

		// Send build notifications asynchronously
		if a.notifier != nil {
			updatedBuild, err := a.builds.GetByID(ctx, buildID)
			if err != nil {
				log.Error("failed to reload build for notification", "error", err)
			} else {
				project, err := a.projects.GetByID(ctx, updatedBuild.ProjectID)
				if err != nil {
					log.Error("failed to load project for notification", "error", err)
				} else {
					go func() {
						if err := a.notifier.NotifyBuild(context.Background(), updatedBuild, project); err != nil {
							log.Error("failed to send build notification", "error", err)
						}
					}()
				}
			}
		}
	} else {
		if err := a.builds.UpdateStatus(ctx, buildID, newStatus); err != nil {
			log.Error("failed to update build status", "error", err)
			return err
		}
	}

	return nil
}
