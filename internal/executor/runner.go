package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/featherci/featherci/internal/models"
)

// StepCache abstracts cache save/restore for the step runner.
type StepCache interface {
	Save(key string, sourcePaths []string, workspacePath string) error
	Restore(key string, workspacePath string) error
}

// StepRunner orchestrates the execution of individual build steps.
type StepRunner struct {
	executor Executor
	cache    StepCache
}

// NewStepRunner creates a runner backed by the given executor.
// cache may be nil if caching is not configured.
func NewStepRunner(executor Executor, cache StepCache) *StepRunner {
	return &StepRunner{executor: executor, cache: cache}
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

	// Create log writer for capturing container output.
	logWriter, err := NewLogWriter(logPath)
	if err != nil {
		return failedResult(fmt.Sprintf("creating log writer: %v", err))
	}

	// Restore cache before execution
	if r.cache != nil && step.Cache != nil && step.CacheResolvedKey != "" {
		if err := r.cache.Restore(step.CacheResolvedKey, workspacePath); err != nil {
			// Log but don't fail the step
			f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if ferr == nil {
				fmt.Fprintf(f, "cache restore warning: %s\n", err)
				f.Close()
			}
		}
	}

	opts := RunOptions{
		Image:    image,
		Commands: step.Commands,
		Env:      step.Env,
		WorkDir:  workDir,
		BindMounts: []BindMount{
			{Source: workspacePath, Target: "/workspace"},
		},
		Timeout: timeout,
		Output:  logWriter,
	}

	result, runErr := r.executor.Run(ctx, opts)
	// Always flush and close the log writer.
	logWriter.Close()

	if runErr != nil {
		// Append error to log file for visibility.
		f, ferr := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
		if ferr == nil {
			fmt.Fprintf(f, "executor error: %s\n", runErr)
			f.Close()
		}
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

	// Save cache after successful execution
	if exitCode == 0 && r.cache != nil && step.Cache != nil && step.CacheResolvedKey != "" {
		if err := r.cache.Save(step.CacheResolvedKey, step.Cache.Paths, workspacePath); err != nil {
			f, ferr := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
			if ferr == nil {
				fmt.Fprintf(f, "cache save warning: %s\n", err)
				f.Close()
			}
		}
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
