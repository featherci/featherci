---
model: opus
---

# Step 16: Build Orchestration

## Objective
Implement the build orchestration logic that manages build lifecycle and step scheduling.

## Tasks

### 16.1 Create Build Orchestrator
```go
type BuildOrchestrator struct {
    builds     BuildRepository
    steps      BuildStepRepository
    projects   ProjectRepository
    queue      *JobQueue
    git        GitService
    workflow   *workflow.Parser
    status     StatusPoster
    
    ticker     *time.Ticker
    stopCh     chan struct{}
}

func NewBuildOrchestrator(deps Dependencies) *BuildOrchestrator {
    return &BuildOrchestrator{
        builds:   deps.Builds,
        steps:    deps.Steps,
        projects: deps.Projects,
        queue:    deps.Queue,
        git:      deps.Git,
        workflow: deps.Workflow,
        status:   deps.Status,
        ticker:   time.NewTicker(5 * time.Second),
        stopCh:   make(chan struct{}),
    }
}
```

### 16.2 Orchestrator Main Loop
```go
func (o *BuildOrchestrator) Start(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-o.stopCh:
            return
        case <-o.ticker.C:
            o.processBuilds(ctx)
        }
    }
}

func (o *BuildOrchestrator) processBuilds(ctx context.Context) {
    // 1. Get pending builds
    builds, _ := o.builds.ListPending(ctx)
    
    for _, build := range builds {
        o.processBuild(ctx, build)
    }
    
    // 2. Update ready steps
    o.updateReadySteps(ctx)
    
    // 3. Check for completed builds
    o.checkCompletedBuilds(ctx)
    
    // 4. Mark stale workers offline
    o.workers.MarkOfflineStale(ctx, 30*time.Second)
}
```

### 16.3 Build Processing
```go
func (o *BuildOrchestrator) processBuild(ctx context.Context, build *Build) error {
    // If build has no steps, initialize it
    if len(build.Steps) == 0 {
        return o.initializeBuild(ctx, build)
    }
    
    return nil
}

func (o *BuildOrchestrator) initializeBuild(ctx context.Context, build *Build) error {
    project, _ := o.projects.GetByID(ctx, build.ProjectID)
    
    // 1. Fetch workflow file from repo
    content, err := o.git.FetchWorkflowFile(ctx, project, build.CommitSHA)
    if err != nil {
        build.Status = BuildStatusFailure
        o.builds.Update(ctx, build)
        return err
    }
    
    // 2. Parse workflow
    wf, err := o.workflow.Parse(content)
    if err != nil {
        build.Status = BuildStatusFailure
        o.builds.Update(ctx, build)
        return err
    }
    
    // 3. Create steps
    for _, s := range wf.Steps {
        step := &BuildStep{
            BuildID:          build.ID,
            Name:             s.Name,
            Image:            s.Image,
            Commands:         s.Commands,
            Env:              s.Env,
            DependsOn:        s.DependsOn,
            RequiresApproval: s.Type == "approval",
            Status:           StepStatusWaiting,
        }
        
        // Steps with no dependencies start as ready
        if len(s.DependsOn) == 0 {
            step.Status = StepStatusReady
        }
        
        o.steps.Create(ctx, step)
    }
    
    // 4. Create dependencies
    // ...
    
    // 5. Update build status
    build.Status = BuildStatusRunning
    build.StartedAt = timePtr(time.Now())
    o.builds.Update(ctx, build)
    
    // 6. Post status to provider
    o.status.PostPending(ctx, project, build)
    
    return nil
}
```

### 16.4 Step Status Transitions
```go
func (o *BuildOrchestrator) updateReadySteps(ctx context.Context) {
    // Find steps where:
    // - Status is "waiting"
    // - All dependencies have status "success"
    // Update them to "ready"
    
    query := `
        UPDATE build_steps
        SET status = 'ready', updated_at = CURRENT_TIMESTAMP
        WHERE status = 'waiting'
          AND id NOT IN (
              SELECT sd.step_id
              FROM step_dependencies sd
              JOIN build_steps dep ON sd.depends_on_step_id = dep.id
              WHERE dep.status != 'success'
          )
    `
    o.db.Exec(query)
    
    // Handle approval steps
    query = `
        UPDATE build_steps
        SET status = 'waiting_approval', updated_at = CURRENT_TIMESTAMP
        WHERE status = 'ready'
          AND requires_approval = true
    `
    o.db.Exec(query)
}

func (o *BuildOrchestrator) onStepCompleted(ctx context.Context, step *BuildStep) {
    // If step failed, skip dependent steps
    if step.Status == StepStatusFailure {
        o.skipDependentSteps(ctx, step.ID)
    }
    
    // Update ready steps
    o.updateReadySteps(ctx)
    
    // Check if build is complete
    o.checkBuildCompletion(ctx, step.BuildID)
}

func (o *BuildOrchestrator) skipDependentSteps(ctx context.Context, stepID int64) {
    query := `
        WITH RECURSIVE deps AS (
            SELECT step_id FROM step_dependencies WHERE depends_on_step_id = ?
            UNION ALL
            SELECT sd.step_id FROM step_dependencies sd
            JOIN deps d ON sd.depends_on_step_id = d.step_id
        )
        UPDATE build_steps SET status = 'skipped'
        WHERE id IN (SELECT step_id FROM deps)
    `
    o.db.Exec(query, stepID)
}
```

### 16.5 Build Completion Detection
```go
func (o *BuildOrchestrator) checkBuildCompletion(ctx context.Context, buildID int64) {
    steps, _ := o.steps.ListByBuild(ctx, buildID)
    
    allDone := true
    hasFailure := false
    
    for _, step := range steps {
        switch step.Status {
        case StepStatusPending, StepStatusWaiting, StepStatusReady, StepStatusRunning, StepStatusWaitingApproval:
            allDone = false
        case StepStatusFailure:
            hasFailure = true
        }
    }
    
    if !allDone {
        return
    }
    
    build, _ := o.builds.GetByID(ctx, buildID)
    
    if hasFailure {
        build.Status = BuildStatusFailure
    } else {
        build.Status = BuildStatusSuccess
    }
    build.FinishedAt = timePtr(time.Now())
    
    o.builds.Update(ctx, build)
    
    // Post final status to provider
    project, _ := o.projects.GetByID(ctx, build.ProjectID)
    o.status.PostFinal(ctx, project, build)
}
```

### 16.6 Build Cancellation
```go
func (o *BuildOrchestrator) CancelBuild(ctx context.Context, buildID int64) error {
    build, _ := o.builds.GetByID(ctx, buildID)
    
    if build.Status != BuildStatusPending && build.Status != BuildStatusRunning {
        return errors.New("build cannot be cancelled")
    }
    
    // Cancel all pending/running steps
    o.steps.CancelAll(ctx, buildID)
    
    // Update build status
    build.Status = BuildStatusCancelled
    build.FinishedAt = timePtr(time.Now())
    o.builds.Update(ctx, build)
    
    // TODO: Signal workers to stop running containers
    
    return nil
}
```

### 16.7 Add Tests
- Test build initialization
- Test step status transitions
- Test dependency resolution
- Test build completion detection
- Test cancellation

## Deliverables
- [ ] `internal/orchestrator/orchestrator.go` - Main orchestrator
- [ ] `internal/orchestrator/scheduler.go` - Step scheduling
- [ ] `internal/orchestrator/completion.go` - Build completion
- [ ] Tests for orchestration logic

## Dependencies
- Step 12: Build model
- Step 15: Worker architecture

## Estimated Effort
Large - Core orchestration logic
