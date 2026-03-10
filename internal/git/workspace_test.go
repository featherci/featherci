package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceManager_Create(t *testing.T) {
	base := t.TempDir()
	wm := NewWorkspaceManager(base)

	path, err := wm.Create(42, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(base, "42", "100")
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("workspace dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("workspace path is not a directory")
	}
}

func TestWorkspaceManager_GetPath(t *testing.T) {
	wm := NewWorkspaceManager("/tmp/workspaces")
	got := wm.GetPath(1, 2)
	want := filepath.Join("/tmp/workspaces", "1", "2")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWorkspaceManager_Cleanup(t *testing.T) {
	base := t.TempDir()
	wm := NewWorkspaceManager(base)

	_, err := wm.Create(10, 20)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Write a file inside to verify full removal
	testFile := filepath.Join(wm.GetPath(10, 20), "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if err := wm.Cleanup(10, 20); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(wm.GetPath(10, 20)); !os.IsNotExist(err) {
		t.Fatal("workspace directory still exists after cleanup")
	}
}

func TestWorkspaceManager_CleanupNonexistent(t *testing.T) {
	base := t.TempDir()
	wm := NewWorkspaceManager(base)

	// Cleaning up a non-existent workspace should not error
	if err := wm.Cleanup(999, 999); err != nil {
		t.Fatalf("cleanup of non-existent workspace failed: %v", err)
	}
}
