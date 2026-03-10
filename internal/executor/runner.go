package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/featherci/featherci/internal/models"
)

// StepRunner orchestrates the execution of individual build steps.
type StepRunner struct {
	executor Executor
}

// NewStepRunner creates a runner backed by the given executor.
func NewStepRunner(executor Executor) *StepRunner {
	return &StepRunner{executor: executor}
}

// StepResult captures the outcome of running a build step,
// including the mapped status and log path for the caller to persist.
type StepResult struct {
	Status   models.StepStatus
	ExitCode *int
	LogPath  string
}

// RunStep executes a build step inside a container and returns the result.
// The caller is responsible for updating the database with the result.
func (r *StepRunner) RunStep(ctx context.Context, step *models.BuildStep, workspacePath string) *StepResult {
	// Set up log directory and writer.
	logDir := filepath.Join(filepath.Dir(workspacePath), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return failedResult(fmt.Sprintf("creating log dir: %v", err))
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%d.log", step.ID))

	// Determine image.
	image := "alpine:latest"
	if step.Image != nil && *step.Image != "" {
		image = *step.Image
	}

	// Determine working directory inside container.
	workDir := "/workspace"
	if step.WorkingDir != "" {
		workDir = filepath.Join("/workspace", step.WorkingDir)
	}

	// Build timeout from step config.
	timeout := time.Duration(step.GetTimeout()) * time.Minute

	opts := RunOptions{
		Image:    image,
		Commands: step.Commands,
		Env:      step.Env,
		WorkDir:  workDir,
		BindMounts: []BindMount{
			{Source: workspacePath, Target: "/workspace"},
		},
		Timeout: timeout,
	}

	result, err := r.executor.Run(ctx, opts)
	if err != nil {
		// Write error to log file for visibility.
		_ = os.WriteFile(logPath, []byte(fmt.Sprintf("executor error: %s\n", err)), 0644)
		return &StepResult{
			Status:  models.StepStatusFailure,
			LogPath: logPath,
		}
	}

	exitCode := result.ExitCode
	status := models.StepStatusSuccess
	if exitCode != 0 {
		status = models.StepStatusFailure
	}

	return &StepResult{
		Status:   status,
		ExitCode: &exitCode,
		LogPath:  logPath,
	}
}

func failedResult(msg string) *StepResult {
	return &StepResult{
		Status: models.StepStatusFailure,
	}
}
