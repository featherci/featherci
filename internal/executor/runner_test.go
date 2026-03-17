package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/featherci/featherci/internal/models"
)

// mockExecutor implements the Executor interface for testing.
type mockExecutor struct {
	runFn  func(ctx context.Context, opts RunOptions) (*RunResult, error)
	stopFn func(ctx context.Context, containerID string) error
}

func (m *mockExecutor) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if m.runFn != nil {
		return m.runFn(ctx, opts)
	}
	return &RunResult{ExitCode: 0, StartedAt: time.Now(), FinishedAt: time.Now()}, nil
}

func (m *mockExecutor) Stop(ctx context.Context, containerID string) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, containerID)
	}
	return nil
}

func strPtr(s string) *string { return &s }

// setupTestLogPath creates workspace and log dirs and returns (workspace, logPath).
func setupTestLogPath(t *testing.T, stepID int64) (string, string) {
	t.Helper()
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	os.MkdirAll(workspace, 0755)
	logDir := filepath.Join(dir, "logs")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, fmt.Sprintf("%d.log", stepID))
	return workspace, logPath
}

func TestStepRunner_RunStep_Success(t *testing.T) {
	var capturedOpts RunOptions
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{
				ContainerID: "c123",
				ExitCode:    0,
				StartedAt:   time.Now(),
				FinishedAt:  time.Now(),
			}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 42)

	step := &models.BuildStep{
		ID:             42,
		Image:          strPtr("golang:1.22"),
		Commands:       []string{"go test ./..."},
		Env:            map[string]string{"CI": "true"},
		WorkingDir:     "src",
		TimeoutMinutes: 30,
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if result.Status != models.StepStatusSuccess {
		t.Errorf("expected success, got %s", result.Status)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		t.Errorf("expected exit code 0")
	}

	// Verify options passed to executor.
	if capturedOpts.Image != "golang:1.22" {
		t.Errorf("expected image golang:1.22, got %s", capturedOpts.Image)
	}
	if capturedOpts.WorkDir != "/workspace/src" {
		t.Errorf("expected workdir /workspace/src, got %s", capturedOpts.WorkDir)
	}
	if capturedOpts.Timeout != 30*time.Minute {
		t.Errorf("expected 30m timeout, got %s", capturedOpts.Timeout)
	}
	if len(capturedOpts.BindMounts) != 1 || capturedOpts.BindMounts[0].Target != "/workspace" {
		t.Errorf("expected /workspace bind mount")
	}
	if capturedOpts.Env["CI"] != "true" {
		t.Error("expected CI=true in env")
	}
}

func TestStepRunner_RunStep_Failure(t *testing.T) {
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{ExitCode: 1, StartedAt: time.Now(), FinishedAt: time.Now()}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 1)

	step := &models.BuildStep{
		ID:       1,
		Image:    strPtr("alpine"),
		Commands: []string{"false"},
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if result.Status != models.StepStatusFailure {
		t.Errorf("expected failure, got %s", result.Status)
	}
	if result.ExitCode == nil || *result.ExitCode != 1 {
		t.Errorf("expected exit code 1")
	}
}

func TestStepRunner_RunStep_ExecutorError(t *testing.T) {
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return nil, errors.New("docker daemon not running")
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 1)

	step := &models.BuildStep{
		ID:       1,
		Image:    strPtr("alpine"),
		Commands: []string{"echo hi"},
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if result.Status != models.StepStatusFailure {
		t.Errorf("expected failure, got %s", result.Status)
	}
	if result.LogPath == "" {
		t.Error("expected log path to be set for error")
	}

	// Error should be written to log file.
	content, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(content), "docker daemon not running") {
		t.Errorf("expected error in log, got: %s", string(content))
	}
}

func TestStepRunner_RunStep_DefaultImage(t *testing.T) {
	var capturedOpts RunOptions
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{ExitCode: 0}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 1)

	step := &models.BuildStep{
		ID:       1,
		Commands: []string{"echo hi"},
	}

	runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if capturedOpts.Image != "alpine:latest" {
		t.Errorf("expected default image alpine:latest, got %s", capturedOpts.Image)
	}
}

func TestStepRunner_RunStep_DefaultWorkDir(t *testing.T) {
	var capturedOpts RunOptions
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{ExitCode: 0}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 1)

	step := &models.BuildStep{
		ID:       1,
		Image:    strPtr("alpine"),
		Commands: []string{"pwd"},
	}

	runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if capturedOpts.WorkDir != "/workspace" {
		t.Errorf("expected default workdir /workspace, got %s", capturedOpts.WorkDir)
	}
}

func TestStepRunner_RunStep_LogPath(t *testing.T) {
	exec := &mockExecutor{}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 99)

	step := &models.BuildStep{
		ID:       99,
		Image:    strPtr("alpine"),
		Commands: []string{"echo hi"},
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if result.LogPath != logPath {
		t.Errorf("expected log path %s, got %s", logPath, result.LogPath)
	}
}

func TestStepRunner_RunStep_OutputCapture(t *testing.T) {
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			// Simulate container writing output to the provided writer.
			if opts.Output != nil {
				opts.Output.Write([]byte("step output line 1\nstep output line 2\n"))
			}
			return &RunResult{ExitCode: 0, StartedAt: time.Now(), FinishedAt: time.Now()}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 50)

	step := &models.BuildStep{
		ID:       50,
		Image:    strPtr("alpine"),
		Commands: []string{"echo hello"},
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if result.Status != models.StepStatusSuccess {
		t.Errorf("expected success, got %s", result.Status)
	}

	// Verify log file has the captured output.
	content, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	if !strings.Contains(string(content), "step output line 1") {
		t.Errorf("expected log to contain output, got: %s", string(content))
	}
	if !strings.Contains(string(content), "step output line 2") {
		t.Errorf("expected log to contain second line, got: %s", string(content))
	}
}

func TestStepRunner_RunStep_DefaultTimeout(t *testing.T) {
	var capturedOpts RunOptions
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{ExitCode: 0}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 1)

	step := &models.BuildStep{
		ID:       1,
		Image:    strPtr("alpine"),
		Commands: []string{"echo hi"},
		// TimeoutMinutes defaults to 0, GetTimeout() returns 60
	}

	runner.RunStep(context.Background(), step, workspace, logPath, nil)

	if capturedOpts.Timeout != 60*time.Minute {
		t.Errorf("expected 60m default timeout, got %s", capturedOpts.Timeout)
	}
}

func TestStepRunner_RunStep_SecretMasking(t *testing.T) {
	exec := &mockExecutor{
		runFn: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			// Simulate container printing secret values in output.
			if opts.Output != nil {
				opts.Output.Write([]byte("token is supersecret123\npassword is hunter2\nno secrets here\n"))
			}
			return &RunResult{ExitCode: 0, StartedAt: time.Now(), FinishedAt: time.Now()}, nil
		},
	}
	runner := NewStepRunner(exec, nil)

	workspace, logPath := setupTestLogPath(t, 77)

	step := &models.BuildStep{
		ID:       77,
		Image:    strPtr("alpine"),
		Commands: []string{"echo secrets"},
	}

	result := runner.RunStep(context.Background(), step, workspace, logPath, []string{"supersecret123", "hunter2"})

	if result.Status != models.StepStatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}

	content, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	logStr := string(content)
	if strings.Contains(logStr, "supersecret123") {
		t.Error("secret 'supersecret123' was not masked in log output")
	}
	if strings.Contains(logStr, "hunter2") {
		t.Error("secret 'hunter2' was not masked in log output")
	}
	if !strings.Contains(logStr, "token is ***") {
		t.Errorf("expected masked output, got: %s", logStr)
	}
	if !strings.Contains(logStr, "no secrets here") {
		t.Errorf("expected non-secret content to be preserved, got: %s", logStr)
	}
}
