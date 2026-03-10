package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitService defines the interface for git operations.
type GitService interface {
	Clone(ctx context.Context, cloneURL, token, provider, destDir string) error
	Checkout(ctx context.Context, repoDir, commitSHA string) error
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecCommandRunner runs commands using os/exec.
type ExecCommandRunner struct{}

// Run executes a command and returns its combined output.
func (r *ExecCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// CLIGitService implements GitService using the git CLI.
type CLIGitService struct {
	runner CommandRunner
}

// NewCLIGitService creates a new CLIGitService.
func NewCLIGitService() *CLIGitService {
	return &CLIGitService{runner: &ExecCommandRunner{}}
}

// NewCLIGitServiceWithRunner creates a CLIGitService with a custom command runner (for testing).
func NewCLIGitServiceWithRunner(runner CommandRunner) *CLIGitService {
	return &CLIGitService{runner: runner}
}

// Clone performs a shallow clone of a repository.
func (s *CLIGitService) Clone(ctx context.Context, cloneURL, token, provider, destDir string) error {
	authURL, err := InjectToken(cloneURL, token, provider)
	if err != nil {
		return fmt.Errorf("failed to inject token: %w", err)
	}

	_, err = s.runner.Run(ctx, "git", "clone", "--depth=1", authURL, destDir)
	if err != nil {
		return fmt.Errorf("git clone failed: %w", sanitizeTokenFromError(err, token))
	}
	return nil
}

// Checkout checks out a specific commit. For shallow clones, it fetches the commit first.
func (s *CLIGitService) Checkout(ctx context.Context, repoDir, commitSHA string) error {
	// Try direct checkout first
	_, err := s.runner.Run(ctx, "git", "-C", repoDir, "checkout", commitSHA)
	if err == nil {
		return nil
	}

	// For shallow clones, fetch the specific commit then checkout
	_, err = s.runner.Run(ctx, "git", "-C", repoDir, "fetch", "--depth=1", "origin", commitSHA)
	if err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	_, err = s.runner.Run(ctx, "git", "-C", repoDir, "checkout", "FETCH_HEAD")
	if err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	return nil
}

// sanitizeTokenFromError removes tokens from error messages.
func sanitizeTokenFromError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}
