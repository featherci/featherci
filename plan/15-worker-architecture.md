---
model: opus
---

# Step 15: Worker Architecture

## Objective
Implement the master/worker architecture with HTTP polling for job distribution.

## Tasks

### 15.1 Create Worker Model
```go
type Worker struct {
    ID            string
    Name          string
    Status        WorkerStatus
    LastHeartbeat time.Time
    CurrentStepID *int64
    CreatedAt     time.Time
}

type WorkerStatus string

const (
    WorkerStatusOnline  WorkerStatus = "online"
    WorkerStatusOffline WorkerStatus = "offline"
    WorkerStatusBusy    WorkerStatus = "busy"
)

type WorkerRepository interface {
    Register(ctx context.Context, worker *Worker) error
    UpdateHeartbeat(ctx context.Context, id string) error
    UpdateStatus(ctx context.Context, id string, status WorkerStatus) error
    SetCurrentStep(ctx context.Context, id string, stepID *int64) error
    GetByID(ctx context.Context, id string) (*Worker, error)
    ListOnline(ctx context.Context) ([]*Worker, error)
    MarkOfflineStale(ctx context.Context, threshold time.Duration) error
}
```

### 15.2 Create Job Queue
```go
type JobQueue struct {
    steps    BuildStepRepository
    builds   BuildRepository
    projects ProjectRepository
    mu       sync.Mutex
}

func (q *JobQueue) GetNextJob(ctx context.Context, workerID string) (*Job, error) {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    // Find ready steps that aren't assigned
    step, err := q.steps.GetNextReady(ctx)
    if err != nil || step == nil {
        return nil, err
    }
    
    // Claim the step for this worker
    step.WorkerID = workerID
    step.Status = StepStatusRunning
    q.steps.Update(ctx, step)
    
    // Load related data
    build, _ := q.builds.GetByID(ctx, step.BuildID)
    project, _ := q.projects.GetByID(ctx, build.ProjectID)
    
    return &Job{
        Step:    step,
        Build:   build,
        Project: project,
    }, nil
}

type Job struct {
    Step    *BuildStep
    Build   *Build
    Project *Project
}
```

### 15.3 Create Worker API Handlers
```go
type WorkerAPIHandler struct {
    queue      *JobQueue
    workers    WorkerRepository
    steps      BuildStepRepository
    workerAuth string // Shared secret
}

// GET /api/worker/jobs - Poll for next job
func (h *WorkerAPIHandler) GetJob(w http.ResponseWriter, r *http.Request) {
    if !h.validateWorkerAuth(r) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    
    workerID := r.Header.Get("X-Worker-ID")
    
    job, err := h.queue.GetNextJob(r.Context(), workerID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    if job == nil {
        w.WriteHeader(http.StatusNoContent)
        return
    }
    
    json.NewEncoder(w).Encode(job)
}

// POST /api/worker/jobs/{id}/status - Update job status
func (h *WorkerAPIHandler) UpdateJobStatus(w http.ResponseWriter, r *http.Request) {
    // Parse status update
    // Update step status
    // If step completed, check if build status needs updating
    // If step completed, update ready steps
}

// POST /api/worker/jobs/{id}/logs - Append logs
func (h *WorkerAPIHandler) AppendLogs(w http.ResponseWriter, r *http.Request) {
    // Append log content to step's log file
}

// POST /api/worker/heartbeat - Worker heartbeat
func (h *WorkerAPIHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
    workerID := r.Header.Get("X-Worker-ID")
    h.workers.UpdateHeartbeat(r.Context(), workerID)
    w.WriteHeader(http.StatusOK)
}
```

### 15.4 Create Worker Client
```go
type WorkerClient struct {
    masterURL  string
    workerID   string
    authToken  string
    httpClient *http.Client
}

func NewWorkerClient(masterURL, workerID, authToken string) *WorkerClient {
    return &WorkerClient{
        masterURL:  masterURL,
        workerID:   workerID,
        authToken:  authToken,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

func (c *WorkerClient) PollJob(ctx context.Context) (*Job, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", c.masterURL+"/api/worker/jobs", nil)
    req.Header.Set("X-Worker-ID", c.workerID)
    req.Header.Set("Authorization", "Bearer "+c.authToken)
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == http.StatusNoContent {
        return nil, nil // No jobs available
    }
    
    var job Job
    json.NewDecoder(resp.Body).Decode(&job)
    return &job, nil
}

func (c *WorkerClient) UpdateStatus(ctx context.Context, stepID int64, status StepStatus, exitCode *int) error
func (c *WorkerClient) SendLogs(ctx context.Context, stepID int64, logs []byte) error
func (c *WorkerClient) Heartbeat(ctx context.Context) error
```

### 15.5 Create Worker Process
```go
type WorkerProcess struct {
    client   *WorkerClient
    executor Executor
    git      GitService
    config   *config.Config
    
    stopCh   chan struct{}
}

func (w *WorkerProcess) Start(ctx context.Context) error {
    // Register with master
    
    // Start heartbeat goroutine
    go w.heartbeatLoop(ctx)
    
    // Main work loop
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-w.stopCh:
            return nil
        default:
            job, err := w.client.PollJob(ctx)
            if err != nil {
                log.Printf("poll error: %v", err)
                time.Sleep(5 * time.Second)
                continue
            }
            
            if job == nil {
                time.Sleep(2 * time.Second)
                continue
            }
            
            w.executeJob(ctx, job)
        }
    }
}

func (w *WorkerProcess) executeJob(ctx context.Context, job *Job) {
    // 1. Clone repository if needed
    // 2. Run step
    // 3. Report status
    // 4. Send logs
}
```

### 15.6 Standalone Mode
```go
func (s *Server) startStandaloneWorker(ctx context.Context) {
    if s.config.Mode == "master" {
        return // Don't start worker
    }
    
    worker := &WorkerProcess{
        executor: s.executor,
        git:      s.git,
        // Use local queue instead of HTTP client
        localQueue: s.jobQueue,
    }
    
    go worker.StartLocal(ctx)
}
```

### 15.7 Worker Authentication
```go
func (h *WorkerAPIHandler) validateWorkerAuth(r *http.Request) bool {
    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "Bearer ") {
        return false
    }
    
    token := strings.TrimPrefix(auth, "Bearer ")
    return subtle.ConstantTimeCompare([]byte(token), []byte(h.workerAuth)) == 1
}
```

### 15.8 Add Tests
- Test job queue
- Test worker API
- Test worker client
- Test job execution flow

## Deliverables
- [ ] `internal/models/worker.go` - Worker model
- [ ] `internal/worker/queue.go` - Job queue
- [ ] `internal/worker/client.go` - Worker HTTP client
- [ ] `internal/worker/process.go` - Worker process
- [ ] `internal/handlers/worker_api.go` - API handlers
- [ ] Standalone mode works
- [ ] Distributed mode works

## Dependencies
- Step 14: Docker executor
- Step 12: Build model

## Estimated Effort
Large - Distributed system component
