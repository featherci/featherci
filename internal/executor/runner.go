package executor

import (
	"context"
	"fmt"
	"io"
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
// secretValues contains the raw secret values to mask in log output.
func (r *StepRunner) RunStep(ctx context.Context, step *models.BuildStep, workspacePath string, logPath string, secretValues []string) *StepResult {
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

	// Wrap log writer with secret masking so secret values are never written to logs.
	var output io.Writer = logWriter
	output = NewMaskingWriter(output, secretValues)

	// Convert service configs to executor options.
	var services []ServiceOption
	for _, svc := range step.Services {
		services = append(services, ServiceOption{
			Image: svc.Image,
			Env:   svc.Env,
		})
	}

	bindMounts := []BindMount{
		{Source: workspacePath, Target: "/workspace"},
	}
	if step.Docker {
		bindMounts = append(bindMounts, BindMount{
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})
	}

	opts := RunOptions{
		Image:      image,
		Commands:   step.Commands,
		Env:        step.Env,
		WorkDir:    workDir,
		BindMounts: bindMounts,
		Timeout:  timeout,
		Output:   output,
		Services: services,
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
