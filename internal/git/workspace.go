package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// WorkspaceManager manages build workspace directories.
type WorkspaceManager struct {
	basePath string
}

// NewWorkspaceManager creates a new WorkspaceManager with the given base path.
func NewWorkspaceManager(basePath string) *WorkspaceManager {
	return &WorkspaceManager{basePath: basePath}
}

// Create creates a workspace directory for a build and returns its path.
func (w *WorkspaceManager) Create(projectID, buildID int64) (string, error) {
	path := w.GetPath(projectID, buildID)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("failed to create workspace: %w", err)
	}
	return path, nil
}

// GetPath returns the workspace path for a build without creating it.
func (w *WorkspaceManager) GetPath(projectID, buildID int64) string {
	return filepath.Join(w.basePath, fmt.Sprintf("%d", projectID), fmt.Sprintf("%d", buildID))
}

// Cleanup removes a build's workspace directory.
func (w *WorkspaceManager) Cleanup(projectID, buildID int64) error {
	path := w.GetPath(projectID, buildID)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to cleanup workspace: %w", err)
	}
	return nil
}
