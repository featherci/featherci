// Package services contains business logic services for FeatherCI.
package services

import (
	"context"
	"fmt"

	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/webhook"
	"github.com/featherci/featherci/internal/workflow"
)

// BuildCreator creates builds from webhook events and parsed workflows.
type BuildCreator struct {
	builds BuildRepository
	steps  BuildStepRepository
}

// BuildRepository is a subset of models.BuildRepository needed by BuildCreator.
type BuildRepository interface {
	Create(ctx context.Context, build *models.Build) error
	GetNextBuildNumber(ctx context.Context, projectID int64) (int, error)
}

// BuildStepRepository is a subset of models.BuildStepRepository needed by BuildCreator.
type BuildStepRepository interface {
	Create(ctx context.Context, step *models.BuildStep) error
	CreateBatch(ctx context.Context, steps []*models.BuildStep) error
	AddDependency(ctx context.Context, stepID, dependsOnID int64) error
}

// NewBuildCreator creates a new BuildCreator.
func NewBuildCreator(builds BuildRepository, steps BuildStepRepository) *BuildCreator {
	return &BuildCreator{
		builds: builds,
		steps:  steps,
	}
}

// CreateBuildFromWebhook creates a new build from a webhook event and workflow.
// It creates the build record and all step records with their dependencies.
func (c *BuildCreator) CreateBuildFromWebhook(
	ctx context.Context,
	project *models.Project,
	event *webhook.Event,
	wf *workflow.Workflow,
) (*models.Build, error) {
	// Get next build number
	buildNum, err := c.builds.GetNextBuildNumber(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next build number: %w", err)
	}

	// Determine branch - for PRs, use the source branch
	var branch *string
	if event.Branch != "" {
		branch = &event.Branch
	} else if event.PullRequest != nil {
		branch = &event.PullRequest.SourceBranch
	}

	// Determine PR number
	var prNumber *int
	if event.PullRequest != nil {
		prNumber = &event.PullRequest.Number
	}

	// Determine commit message
	var commitMsg *string
	if event.CommitMessage != "" {
		commitMsg = &event.CommitMessage
	}

	// Determine commit author
	var commitAuthor *string
	if event.CommitAuthor != "" {
		commitAuthor = &event.CommitAuthor
	}

	// Create build record
	build := &models.Build{
		ProjectID:         project.ID,
		BuildNumber:       buildNum,
		CommitSHA:         event.CommitSHA,
		CommitMessage:     commitMsg,
		CommitAuthor:      commitAuthor,
		Branch:            branch,
		PullRequestNumber: prNumber,
		Status:            models.BuildStatusPending,
	}

	if err := c.builds.Create(ctx, build); err != nil {
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	// Create steps
	if err := c.createSteps(ctx, build, wf); err != nil {
		return nil, fmt.Errorf("failed to create build steps: %w", err)
	}

	return build, nil
}

// createSteps creates all build steps from the workflow definition.
func (c *BuildCreator) createSteps(ctx context.Context, build *models.Build, wf *workflow.Workflow) error {
	if len(wf.Steps) == 0 {
		return nil
	}

	// Create step records
	stepMap := make(map[string]*models.BuildStep)
	steps := make([]*models.BuildStep, 0, len(wf.Steps))

	for _, s := range wf.Steps {
		// Determine initial status
		status := models.StepStatusPending
		if s.HasDependencies() {
			status = models.StepStatusWaiting
		} else if s.IsApproval() {
			status = models.StepStatusWaitingApproval
		} else {
			status = models.StepStatusReady
		}

		var image *string
		if s.Image != "" {
			image = &s.Image
		}

		var cache *models.CacheConfig
		if s.Cache != nil {
			cache = &models.CacheConfig{
				Paths: s.Cache.Paths,
				Key:   s.Cache.Key,
			}
		}

		// Evaluate condition: if condition is false, skip this step
		if s.If != "" && build.Branch != nil {
			vars := map[string]string{"branch": *build.Branch}
			condMet, err := workflow.EvaluateCondition(s.If, vars)
			if err == nil && !condMet {
				status = models.StepStatusSkipped
			}
		}

		step := &models.BuildStep{
			BuildID:          build.ID,
			Name:             s.Name,
			Image:            image,
			Status:           status,
			RequiresApproval: s.IsApproval(),
			Commands:         s.Commands,
			Env:              s.Env,
			DependsOn:        s.DependsOn,
			WorkingDir:       s.WorkingDir,
			TimeoutMinutes:   s.GetTimeout(),
			Cache:            cache,
			ConditionExpr:    s.If,
		}

		steps = append(steps, step)
		stepMap[s.Name] = step
	}

	// Create all steps in batch
	if err := c.steps.CreateBatch(ctx, steps); err != nil {
		return err
	}

	// Create dependencies - now all steps have IDs
	for _, s := range wf.Steps {
		step := stepMap[s.Name]
		for _, depName := range s.DependsOn {
			depStep, ok := stepMap[depName]
			if !ok {
				// This shouldn't happen if workflow validation passed
				return fmt.Errorf("dependency %q not found for step %q", depName, s.Name)
			}
			if err := c.steps.AddDependency(ctx, step.ID, depStep.ID); err != nil {
				return fmt.Errorf("failed to add dependency %q -> %q: %w", s.Name, depName, err)
			}
		}
	}

	// Attach steps to build
	build.Steps = steps

	return nil
}

// CreateBuild creates a new build with explicit parameters (useful for manual triggers).
func (c *BuildCreator) CreateBuild(
	ctx context.Context,
	projectID int64,
	commitSHA string,
	commitMessage string,
	commitAuthor string,
	branch string,
	wf *workflow.Workflow,
) (*models.Build, error) {
	// Get next build number
	buildNum, err := c.builds.GetNextBuildNumber(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next build number: %w", err)
	}

	// Create build record
	build := &models.Build{
		ProjectID:     projectID,
		BuildNumber:   buildNum,
		CommitSHA:     commitSHA,
		CommitMessage: &commitMessage,
		CommitAuthor:  &commitAuthor,
		Branch:        &branch,
		Status:        models.BuildStatusPending,
	}

	if err := c.builds.Create(ctx, build); err != nil {
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	// Create steps
	if err := c.createSteps(ctx, build, wf); err != nil {
		return nil, fmt.Errorf("failed to create build steps: %w", err)
	}

	return build, nil
}
