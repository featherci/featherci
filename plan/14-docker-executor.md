---
model: opus
---

# Step 14: Docker Executor

## Objective
Implement the Docker container execution engine for running pipeline steps.

## Tasks

### 14.1 Add Docker SDK Dependency
```bash
go get github.com/docker/docker/client
go get github.com/docker/docker/api/types
```

### 14.2 Create Executor Interface
```go
type Executor interface {
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
    Stop(ctx context.Context, containerID string) error
    Logs(ctx context.Context, containerID string) (io.ReadCloser, error)
}

type RunOptions struct {
    Image       string
    Commands    []string
    Env         map[string]string
    WorkDir     string
    BindMounts  []BindMount
    NetworkMode string
    Memory      int64  // Memory limit in bytes
    CPUs        float64
    Timeout     time.Duration
}

type BindMount struct {
    Source   string
    Target   string
    ReadOnly bool
}

type RunResult struct {
    ContainerID string
    ExitCode    int
    StartedAt   time.Time
    FinishedAt  time.Time
    Error       error
}
```

### 14.3 Implement Docker Executor
```go
type DockerExecutor struct {
    client *client.Client
}

func NewDockerExecutor() (*DockerExecutor, error) {
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return nil, err
    }
    return &DockerExecutor{client: cli}, nil
}

func (e *DockerExecutor) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    result := &RunResult{}
    
    // 1. Pull image if not present
    if err := e.pullImage(ctx, opts.Image); err != nil {
        return nil, fmt.Errorf("pulling image: %w", err)
    }
    
    // 2. Create container config
    config := &container.Config{
        Image:      opts.Image,
        Cmd:        []string{"/bin/sh", "-c", strings.Join(opts.Commands, " && ")},
        Env:        mapToEnvSlice(opts.Env),
        WorkingDir: opts.WorkDir,
    }
    
    hostConfig := &container.HostConfig{
        Binds:       formatBindMounts(opts.BindMounts),
        NetworkMode: container.NetworkMode(opts.NetworkMode),
        Resources: container.Resources{
            Memory:   opts.Memory,
            NanoCPUs: int64(opts.CPUs * 1e9),
        },
    }
    
    // 3. Create container
    resp, err := e.client.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
    if err != nil {
        return nil, fmt.Errorf("creating container: %w", err)
    }
    result.ContainerID = resp.ID
    
    // 4. Start container
    result.StartedAt = time.Now()
    if err := e.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        return nil, fmt.Errorf("starting container: %w", err)
    }
    
    // 5. Wait for completion with timeout
    waitCtx := ctx
    if opts.Timeout > 0 {
        var cancel context.CancelFunc
        waitCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
        defer cancel()
    }
    
    statusCh, errCh := e.client.ContainerWait(waitCtx, resp.ID, container.WaitConditionNotRunning)
    select {
    case err := <-errCh:
        if err != nil {
            // Timeout or other error - stop container
            e.Stop(ctx, resp.ID)
            result.Error = err
        }
    case status := <-statusCh:
        result.ExitCode = int(status.StatusCode)
    }
    
    result.FinishedAt = time.Now()
    
    // 6. Cleanup container
    e.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
    
    return result, nil
}
```

### 14.4 Image Pulling with Progress
```go
func (e *DockerExecutor) pullImage(ctx context.Context, image string) error {
    // Check if image exists locally
    _, _, err := e.client.ImageInspectWithRaw(ctx, image)
    if err == nil {
        return nil // Image exists
    }
    
    // Pull image
    reader, err := e.client.ImagePull(ctx, image, image.PullOptions{})
    if err != nil {
        return err
    }
    defer reader.Close()
    
    // Consume output (could log progress)
    io.Copy(io.Discard, reader)
    
    return nil
}
```

### 14.5 Log Streaming
```go
func (e *DockerExecutor) Logs(ctx context.Context, containerID string) (io.ReadCloser, error) {
    return e.client.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Follow:     true,
        Timestamps: true,
    })
}
```

### 14.6 Log Writer for Persistence
```go
type LogWriter struct {
    file     *os.File
    buffer   *bufio.Writer
    mu       sync.Mutex
    lastLine int
}

func NewLogWriter(path string) (*LogWriter, error) {
    f, err := os.Create(path)
    if err != nil {
        return nil, err
    }
    return &LogWriter{
        file:   f,
        buffer: bufio.NewWriter(f),
    }, nil
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.buffer.Write(p)
}

func (w *LogWriter) Close() error {
    w.buffer.Flush()
    return w.file.Close()
}

func (w *LogWriter) ReadLines(offset, limit int) ([]string, error) {
    // Read specific lines for streaming to UI
}
```

### 14.7 Step Runner
```go
type StepRunner struct {
    executor  Executor
    git       GitService
    workspace WorkspaceManager
    logs      LogStorage
    secrets   SecretService
}

func (r *StepRunner) RunStep(ctx context.Context, step *BuildStep, build *Build, project *Project) error {
    // 1. Get workspace path
    workspacePath := r.workspace.GetWorkspacePath(build.ID)
    
    // 2. Create log writer
    logPath := r.logs.PathForStep(step.ID)
    logWriter, _ := NewLogWriter(logPath)
    defer logWriter.Close()
    
    // 3. Get secrets as env vars
    secrets, _ := r.secrets.GetDecrypted(ctx, project.ID)
    env := mergeMaps(step.Env, secrets)
    
    // 4. Build run options
    opts := RunOptions{
        Image:    step.Image,
        Commands: step.Commands,
        Env:      env,
        WorkDir:  "/workspace",
        BindMounts: []BindMount{
            {Source: workspacePath, Target: "/workspace", ReadOnly: false},
        },
        Timeout: time.Duration(step.TimeoutMinutes) * time.Minute,
    }
    
    // 5. Run container
    result, err := r.executor.Run(ctx, opts)
    
    // 6. Update step status
    step.ExitCode = &result.ExitCode
    step.LogPath = logPath
    if result.ExitCode == 0 {
        step.Status = StepStatusSuccess
    } else {
        step.Status = StepStatusFailure
    }
    
    return err
}
```

### 14.8 Add Tests
- Test container creation
- Test command execution
- Test environment variables
- Test bind mounts
- Test timeout handling
- Test log capture

## Deliverables
- [ ] `internal/executor/executor.go` - Executor interface
- [ ] `internal/executor/docker.go` - Docker implementation
- [ ] `internal/executor/logs.go` - Log handling
- [ ] `internal/executor/runner.go` - Step runner
- [ ] Tests for executor

## Dependencies
- Step 13: Git operations (workspace)
- Step 12: Build model

## Estimated Effort
Large - Core execution engine
