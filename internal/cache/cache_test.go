package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndRestore(t *testing.T) {
	cacheDir := t.TempDir()
	workspace := t.TempDir()

	mgr := NewCacheManager(cacheDir)

	// Create files to cache
	os.MkdirAll(filepath.Join(workspace, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(workspace, "node_modules", "pkg", "index.js"), []byte("module.exports = {}"), 0644)
	os.WriteFile(filepath.Join(workspace, ".cache-file"), []byte("cached"), 0644)

	// Save
	err := mgr.Save("test-key", []string{"node_modules", ".cache-file"}, workspace)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Restore into a fresh workspace
	workspace2 := t.TempDir()
	err = mgr.Restore("test-key", workspace2)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify files exist
	data, err := os.ReadFile(filepath.Join(workspace2, "node_modules", "pkg", "index.js"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(data) != "module.exports = {}" {
		t.Errorf("unexpected content: %s", data)
	}

	data, err = os.ReadFile(filepath.Join(workspace2, ".cache-file"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(data) != "cached" {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestRestoreCacheMiss(t *testing.T) {
	cacheDir := t.TempDir()
	workspace := t.TempDir()

	mgr := NewCacheManager(cacheDir)

	err := mgr.Restore("nonexistent-key", workspace)
	if err != nil {
		t.Fatalf("expected nil on cache miss, got: %v", err)
	}
}

func TestCleanup(t *testing.T) {
	cacheDir := t.TempDir()
	workspace := t.TempDir()

	mgr := NewCacheManager(cacheDir)

	// Create a cached file
	os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("data"), 0644)
	if err := mgr.Save("old-key", []string{"file.txt"}, workspace); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Set modtime to 10 days ago
	archivePath := mgr.keyPath("old-key")
	metaPath := mgr.metaPath("old-key")
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(archivePath, oldTime, oldTime)
	os.Chtimes(metaPath, oldTime, oldTime)

	// Save a recent entry
	if err := mgr.Save("new-key", []string{"file.txt"}, workspace); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Cleanup with 7-day max age
	if err := mgr.Cleanup(7 * 24 * time.Hour); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Old entry should be gone
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Error("expected old archive to be removed")
	}

	// New entry should still exist
	newArchive := mgr.keyPath("new-key")
	if _, err := os.Stat(newArchive); err != nil {
		t.Error("expected new archive to still exist")
	}
}

func TestResolveKeyBranch(t *testing.T) {
	workspace := t.TempDir()

	key := ResolveKey("{{ .Branch }}-deps", 42, "main", workspace)
	if key != "project-42-main-deps" {
		t.Errorf("unexpected key: %s", key)
	}
}

func TestResolveKeyChecksum(t *testing.T) {
	workspace := t.TempDir()
	os.WriteFile(filepath.Join(workspace, "go.sum"), []byte("some content"), 0644)

	key := ResolveKey(`go-{{ checksum "go.sum" }}`, 1, "main", workspace)
	if !strings.HasPrefix(key, "project-1-go-") {
		t.Errorf("unexpected key prefix: %s", key)
	}
	// Should not contain "unknown"
	if strings.Contains(key, "unknown") {
		t.Errorf("expected resolved checksum, got: %s", key)
	}
}

func TestResolveKeyChecksumMissingFile(t *testing.T) {
	workspace := t.TempDir()

	key := ResolveKey(`go-{{ checksum "go.sum" }}`, 1, "main", workspace)
	if !strings.Contains(key, "unknown") {
		t.Errorf("expected 'unknown' for missing file, got: %s", key)
	}
}

func TestSaveSkipsMissingPaths(t *testing.T) {
	cacheDir := t.TempDir()
	workspace := t.TempDir()

	mgr := NewCacheManager(cacheDir)

	// Save with a path that doesn't exist - should not error
	err := mgr.Save("key", []string{"nonexistent-dir"}, workspace)
	if err != nil {
		t.Fatalf("Save should not error on missing paths: %v", err)
	}
}
