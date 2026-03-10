---
model: sonnet
---

# Step 21: Build Caching

## Objective
Implement build caching to speed up repeated builds by caching directories between runs.

## Tasks

### 21.1 Create Cache Storage
```go
type CacheStorage interface {
    Save(ctx context.Context, key string, sourcePath string) error
    Restore(ctx context.Context, key string, targetPath string) error
    Exists(ctx context.Context, key string) (bool, error)
    Delete(ctx context.Context, key string) error
    Cleanup(ctx context.Context, maxAge time.Duration) error
}

type FileCacheStorage struct {
    basePath string
}

func NewFileCacheStorage(basePath string) *FileCacheStorage {
    return &FileCacheStorage{basePath: basePath}
}

func (s *FileCacheStorage) keyPath(key string) string {
    // Hash the key to avoid filesystem issues
    hash := sha256.Sum256([]byte(key))
    return filepath.Join(s.basePath, hex.EncodeToString(hash[:]))
}

func (s *FileCacheStorage) Save(ctx context.Context, key string, sourcePath string) error {
    cachePath := s.keyPath(key)
    
    // Create tar.gz of source directory
    tarPath := cachePath + ".tar.gz"
    
    f, err := os.Create(tarPath)
    if err != nil {
        return err
    }
    defer f.Close()
    
    gw := gzip.NewWriter(f)
    defer gw.Close()
    
    tw := tar.NewWriter(gw)
    defer tw.Close()
    
    return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        
        header, err := tar.FileInfoHeader(info, "")
        if err != nil {
            return err
        }
        
        relPath, _ := filepath.Rel(sourcePath, path)
        header.Name = relPath
        
        if err := tw.WriteHeader(header); err != nil {
            return err
        }
        
        if !info.IsDir() {
            data, _ := os.Open(path)
            defer data.Close()
            io.Copy(tw, data)
        }
        
        return nil
    })
}

func (s *FileCacheStorage) Restore(ctx context.Context, key string, targetPath string) error {
    tarPath := s.keyPath(key) + ".tar.gz"
    
    f, err := os.Open(tarPath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil // Cache miss, not an error
        }
        return err
    }
    defer f.Close()
    
    gr, err := gzip.NewReader(f)
    if err != nil {
        return err
    }
    defer gr.Close()
    
    tr := tar.NewReader(gr)
    
    for {
        header, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        target := filepath.Join(targetPath, header.Name)
        
        switch header.Typeflag {
        case tar.TypeDir:
            os.MkdirAll(target, 0755)
        case tar.TypeReg:
            os.MkdirAll(filepath.Dir(target), 0755)
            out, _ := os.Create(target)
            io.Copy(out, tr)
            out.Close()
            os.Chmod(target, os.FileMode(header.Mode))
        }
    }
    
    return nil
}

func (s *FileCacheStorage) Exists(ctx context.Context, key string) (bool, error) {
    tarPath := s.keyPath(key) + ".tar.gz"
    _, err := os.Stat(tarPath)
    if os.IsNotExist(err) {
        return false, nil
    }
    return err == nil, err
}
```

### 21.2 Create Cache Key Generator
```go
type CacheKeyGenerator struct{}

func (g *CacheKeyGenerator) Generate(projectID int64, template string, files map[string][]byte) string {
    // Template can include:
    // - {{ .Branch }}
    // - {{ checksum "package-lock.json" }}
    // - {{ .OS }}
    // - {{ .Arch }}
    
    // Example: "npm-{{ checksum "package-lock.json" }}"
    
    // Parse template and substitute values
    // For checksum, hash the file contents
    
    var result strings.Builder
    result.WriteString(fmt.Sprintf("project-%d-", projectID))
    
    // Simple template processing
    re := regexp.MustCompile(`\{\{\s*checksum\s+"([^"]+)"\s*\}\}`)
    processed := re.ReplaceAllStringFunc(template, func(match string) string {
        matches := re.FindStringSubmatch(match)
        if len(matches) > 1 {
            filename := matches[1]
            if content, ok := files[filename]; ok {
                hash := sha256.Sum256(content)
                return hex.EncodeToString(hash[:8])
            }
        }
        return "unknown"
    })
    
    result.WriteString(processed)
    return result.String()
}
```

### 21.3 Integrate with Step Runner
```go
func (r *StepRunner) RunStep(ctx context.Context, step *BuildStep, build *Build, project *Project) error {
    workspacePath := r.workspace.GetWorkspacePath(build.ID)
    
    // Restore cache before execution
    if step.Cache != nil && step.Cache.Key != "" {
        cacheKey := r.cacheKeyGen.Generate(project.ID, step.Cache.Key, r.getChecksumFiles(workspacePath))
        
        for _, cachePath := range step.Cache.Paths {
            targetPath := filepath.Join(workspacePath, cachePath)
            if err := r.cache.Restore(ctx, cacheKey+"-"+cachePath, targetPath); err != nil {
                log.Printf("cache restore warning: %v", err)
            }
        }
    }
    
    // Run the step
    result, err := r.executor.Run(ctx, opts)
    
    // Save cache after successful execution
    if step.Cache != nil && result.ExitCode == 0 {
        cacheKey := r.cacheKeyGen.Generate(project.ID, step.Cache.Key, r.getChecksumFiles(workspacePath))
        
        for _, cachePath := range step.Cache.Paths {
            sourcePath := filepath.Join(workspacePath, cachePath)
            if _, err := os.Stat(sourcePath); err == nil {
                if err := r.cache.Save(ctx, cacheKey+"-"+cachePath, sourcePath); err != nil {
                    log.Printf("cache save warning: %v", err)
                }
            }
        }
    }
    
    return err
}
```

### 21.4 Cache Configuration in Workflow
```yaml
steps:
  - name: install
    image: node:20
    commands:
      - npm ci
    cache:
      key: npm-{{ checksum "package-lock.json" }}
      paths:
        - node_modules
        
  - name: build
    image: golang:1.22
    commands:
      - go build ./...
    cache:
      key: go-{{ checksum "go.sum" }}
      paths:
        - /go/pkg/mod
```

### 21.5 Cache Cleanup Job
```go
func (s *CacheStorage) Cleanup(ctx context.Context, maxAge time.Duration) error {
    entries, err := os.ReadDir(s.basePath)
    if err != nil {
        return err
    }
    
    cutoff := time.Now().Add(-maxAge)
    
    for _, entry := range entries {
        info, err := entry.Info()
        if err != nil {
            continue
        }
        
        if info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(s.basePath, entry.Name()))
        }
    }
    
    return nil
}

// Run periodically
func (o *BuildOrchestrator) startCacheCleanup(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            o.cache.Cleanup(ctx, 7*24*time.Hour) // 7 days
        }
    }
}
```

### 21.6 Cache Storage Limits
```go
type CacheStorageWithLimits struct {
    *FileCacheStorage
    maxSizeBytes int64
}

func (s *CacheStorageWithLimits) Save(ctx context.Context, key string, sourcePath string) error {
    // Check current cache size
    currentSize := s.calculateTotalSize()
    
    // If over limit, clean oldest entries
    if currentSize > s.maxSizeBytes {
        s.cleanOldest(currentSize - s.maxSizeBytes)
    }
    
    return s.FileCacheStorage.Save(ctx, key, sourcePath)
}
```

### 21.7 Add Tests
- Test cache save/restore
- Test key generation with checksums
- Test cleanup by age
- Test size limits

## Deliverables
- [ ] `internal/cache/storage.go` - Cache storage implementation
- [ ] `internal/cache/keys.go` - Cache key generation
- [ ] Workflow cache configuration support
- [ ] Cache restore/save in step runner
- [ ] Automatic cleanup
- [ ] Tests

## Dependencies
- Step 11: Workflow parsing (cache config)
- Step 14: Docker executor (step runner)

## Estimated Effort
Medium - Performance optimization feature
