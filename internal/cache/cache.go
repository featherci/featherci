// Package cache provides build artifact caching between runs.
package cache

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CacheManager handles saving and restoring cached build artifacts.
type CacheManager struct {
	basePath string
}

// NewCacheManager creates a new CacheManager that stores cache entries under basePath.
func NewCacheManager(basePath string) *CacheManager {
	return &CacheManager{basePath: basePath}
}

// cacheMeta stores metadata alongside a cache archive.
type cacheMeta struct {
	Paths []string `json:"paths"`
}

func (m *CacheManager) keyPath(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(m.basePath, fmt.Sprintf("%x.tar.gz", h))
}

func (m *CacheManager) metaPath(key string) string {
	return m.keyPath(key) + ".meta"
}

// Save compresses the given paths (relative to workspacePath) into a cache archive.
func (m *CacheManager) Save(key string, sourcePaths []string, workspacePath string) error {
	if err := os.MkdirAll(m.basePath, 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	archivePath := m.keyPath(key)

	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("creating cache archive: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, p := range sourcePaths {
		absPath := filepath.Join(workspacePath, p)
		if err := addToTar(tw, absPath, p); err != nil {
			// Skip paths that don't exist
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("adding %s to cache: %w", p, err)
		}
	}

	// Close writers explicitly to flush before writing meta
	tw.Close()
	gw.Close()
	f.Close()

	// Write metadata
	meta := cacheMeta{Paths: sourcePaths}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling cache meta: %w", err)
	}
	if err := os.WriteFile(m.metaPath(key), metaData, 0644); err != nil {
		return fmt.Errorf("writing cache meta: %w", err)
	}

	return nil
}

// Restore extracts a cached archive back into workspacePath.
// Returns nil on cache miss (no error).
func (m *CacheManager) Restore(key string, workspacePath string) error {
	archivePath := m.keyPath(key)
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return nil // cache miss
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening cache archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		target := filepath.Join(workspacePath, header.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(workspacePath)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(workspacePath) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent dir for %s: %w", target, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("extracting %s: %w", target, err)
			}
			out.Close()
		}
	}

	return nil
}

// Cleanup removes cache entries older than maxAge.
func (m *CacheManager) Cleanup(maxAge time.Duration) error {
	entries, err := os.ReadDir(m.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading cache dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(m.basePath, entry.Name()))
		}
	}

	return nil
}

// addToTar recursively adds a file or directory to a tar writer.
func addToTar(tw *tar.Writer, absPath, relPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.Walk(absPath, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			// Compute path relative to the parent of the original relPath's base
			rel, err := filepath.Rel(filepath.Dir(absPath), path)
			if err != nil {
				return err
			}
			// Reconstruct using the original relPath prefix
			name := filepath.Join(filepath.Dir(relPath), rel)

			header, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				return err
			}
			header.Name = name

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if !fi.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = io.Copy(tw, f)
				return err
			}
			return nil
		})
	}

	// Single file
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = relPath

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}
