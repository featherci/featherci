---
model: opus
---

# Step 12: Build Model and Management

## Objective
Implement the build and build step models with database operations.

## Tasks

### 12.1 Create Build Model
```go
type Build struct {
    ID                int64
    ProjectID         int64
    BuildNumber       int
    CommitSHA         string
    CommitMessage     string
    CommitAuthor      string
    Branch            string
    PullRequestNumber int
    Status            BuildStatus
    StartedAt         *time.Time
    FinishedAt        *time.Time
    CreatedAt         time.Time
    
    // Loaded via joins
    Project *Project
    Steps   []*BuildStep
}

type BuildStatus string

const (
    BuildStatusPending   BuildStatus = "pending"
    BuildStatusRunning   BuildStatus = "running"
    BuildStatusSuccess   BuildStatus = "success"
    BuildStatusFailure   BuildStatus = "failure"
    BuildStatusCancelled BuildStatus = "cancelled"
)

type BuildRepository interface {
    Create(ctx context.Context, build *Build) error
    GetByID(ctx context.Context, id int64) (*Build, error)
    GetByNumber(ctx context.Context, projectID int64, number int) (*Build, error)
    ListByProject(ctx context.Context, projectID int64, limit, offset int) ([]*Build, error)
    ListByUser(ctx context.Context, userID int64, limit, offset int) ([]*Build, error)
    ListPending(ctx context.Context) ([]*Build, error)
    Update(ctx context.Context, build *Build) error
    GetNextBuildNumber(ctx context.Context, projectID int64) (int, error)
    UpdateStatus(ctx context.Context, id int64, status BuildStatus) error
}
```

### 12.2 Create Build Step Model
```go
type BuildStep struct {
    ID               int64
    BuildID          int64
    Name             string
    Image            string
    Status           StepStatus
    ExitCode         *int
    StartedAt        *time.Time
    FinishedAt       *time.Time
    WorkerID         string
    LogPath          string
    RequiresApproval bool
    ApprovedBy       *int64
    ApprovedAt       *time.Time
    
    // From workflow
    Commands   []string
    Env        map[string]string
    DependsOn  []string
    WorkingDir string
    Cache      *CacheConfig
    
    // Loaded via joins
    Dependencies []*BuildStep
    ApprovedByUser *User
}

type StepStatus string

const (
    StepStatusPending         StepStatus = "pending"
    StepStatusWaiting         StepStatus = "waiting"      // Waiting for dependencies
    StepStatusReady           StepStatus = "ready"        // Dependencies met, ready to run
    StepStatusRunning         StepStatus = "running"
    StepStatusSuccess         StepStatus = "success"
    StepStatusFailure         StepStatus = "failure"
    StepStatusSkipped         StepStatus = "skipped"
    StepStatusWaitingApproval StepStatus = "waiting_approval"
    StepStatusCancelled       StepStatus = "cancelled"
)

type BuildStepRepository interface {
    Create(ctx context.Context, step *BuildStep) error
    CreateBatch(ctx context.Context, steps []*BuildStep) error
    GetByID(ctx context.Context, id int64) (*BuildStep, error)
    ListByBuild(ctx context.Context, buildID int64) ([]*BuildStep, error)
    ListReady(ctx context.Context) ([]*BuildStep, error)
    Update(ctx context.Context, step *BuildStep) error
    UpdateStatus(ctx context.Context, id int64, status StepStatus) error
    SetApproval(ctx context.Context, id int64, userID int64) error
    AddDependency(ctx context.Context, stepID, dependsOnID int64) error
}
```

### 12.3 Create Build From Workflow
```go
type BuildCreator struct {
    builds     BuildRepository
    steps      BuildStepRepository
    workflow   *workflow.Parser
}

func (c *BuildCreator) CreateBuild(ctx context.Context, project *Project, event *WebhookEvent, wf *Workflow) (*Build, error) {
    // 1. Create build record
    build := &Build{
        ProjectID:         project.ID,
        BuildNumber:       c.builds.GetNextBuildNumber(ctx, project.ID),
        CommitSHA:         event.CommitSHA,
        CommitMessage:     event.CommitMsg,
        CommitAuthor:      event.CommitAuthor,
        Branch:            event.Branch,
        PullRequestNumber: event.PRNumber,
        Status:            BuildStatusPending,
    }
    
    if err := c.builds.Create(ctx, build); err != nil {
        return nil, err
    }
    
    // 2. Create step records
    stepMap := make(map[string]*BuildStep)
    for _, s := range wf.Steps {
        step := &BuildStep{
            BuildID:          build.ID,
            Name:             s.Name,
            Image:            s.Image,
            Status:           StepStatusPending,
            Commands:         s.Commands,
            Env:              s.Env,
            DependsOn:        s.DependsOn,
            WorkingDir:       s.WorkingDir,
            RequiresApproval: s.Type == "approval",
        }
        
        if err := c.steps.Create(ctx, step); err != nil {
            return nil, err
        }
        stepMap[s.Name] = step
    }
    
    // 3. Create dependencies
    for _, s := range wf.Steps {
        step := stepMap[s.Name]
        for _, depName := range s.DependsOn {
            depStep := stepMap[depName]
            c.steps.AddDependency(ctx, step.ID, depStep.ID)
        }
    }
    
    return build, nil
}
```

### 12.4 Build Status Aggregation
```go
func (b *Build) CalculateStatus() BuildStatus {
    if len(b.Steps) == 0 {
        return BuildStatusPending
    }
    
    hasRunning := false
    hasPending := false
    hasFailure := false
    
    for _, step := range b.Steps {
        switch step.Status {
        case StepStatusRunning:
            hasRunning = true
        case StepStatusPending, StepStatusWaiting, StepStatusReady, StepStatusWaitingApproval:
            hasPending = true
        case StepStatusFailure:
            hasFailure = true
        case StepStatusCancelled:
            // Treat cancelled as failure for build status
            hasFailure = true
        }
    }
    
    if hasRunning {
        return BuildStatusRunning
    }
    if hasFailure && !hasPending {
        return BuildStatusFailure
    }
    if hasPending {
        return BuildStatusPending
    }
    return BuildStatusSuccess
}
```

### 12.5 Step Scheduling Logic
```go
func (s *BuildStepRepository) UpdateReadySteps(ctx context.Context, buildID int64) error {
    // Find steps where:
    // - Status is "waiting"
    // - All dependencies have status "success"
    // Update them to "ready"
    
    query := `
        UPDATE build_steps
        SET status = 'ready'
        WHERE build_id = ?
          AND status = 'waiting'
          AND NOT EXISTS (
              SELECT 1 FROM step_dependencies sd
              JOIN build_steps dep ON sd.depends_on_step_id = dep.id
              WHERE sd.step_id = build_steps.id
                AND dep.status NOT IN ('success')
          )
    `
}
```

### 12.6 Add Tests
- Test build creation
- Test step creation with dependencies
- Test status aggregation
- Test ready step detection
- Test build number incrementing

## Deliverables
- [ ] `internal/models/build.go` - Build model
- [ ] `internal/models/build_step.go` - BuildStep model
- [ ] `internal/services/build_creator.go` - Build creation service
- [ ] Tests for all components

## Dependencies
- Step 03: Database schema
- Step 11: Workflow parsing

## Estimated Effort
Medium - Core data models
