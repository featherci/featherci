---
model: opus
---

# Step 11: Workflow YAML Parsing

## Objective
Implement parsing of `.featherci/workflow.yml` files with support for steps, dependencies, Docker images, and approval gates.

## Tasks

### 11.1 Define Workflow Schema
```yaml
# .featherci/workflow.yml example
name: CI Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:

steps:
  - name: lint
    image: golangci/golangci-lint:latest
    commands:
      - golangci-lint run
    
  - name: test
    image: golang:1.22
    commands:
      - go test -v ./...
    env:
      CGO_ENABLED: "0"
    
  - name: build
    image: golang:1.22
    depends_on: [lint, test]
    commands:
      - go build -o app ./cmd/app
    
  - name: deploy-staging
    image: alpine:latest
    depends_on: [build]
    commands:
      - ./deploy.sh staging
    
  - name: approve-production
    type: approval
    depends_on: [deploy-staging]
    
  - name: deploy-production
    image: alpine:latest
    depends_on: [approve-production]
    commands:
      - ./deploy.sh production
```

### 11.2 Create Workflow Types
```go
type Workflow struct {
    Name  string         `yaml:"name"`
    On    TriggerConfig  `yaml:"on"`
    Steps []Step         `yaml:"steps"`
}

type TriggerConfig struct {
    Push        *PushTrigger        `yaml:"push"`
    PullRequest *PullRequestTrigger `yaml:"pull_request"`
}

type PushTrigger struct {
    Branches []string `yaml:"branches"`
    Tags     []string `yaml:"tags"`
}

type PullRequestTrigger struct {
    Branches []string `yaml:"branches"` // target branches
}

type Step struct {
    Name            string            `yaml:"name"`
    Type            string            `yaml:"type"`      // "" (default: command) or "approval"
    Image           string            `yaml:"image"`
    Commands        []string          `yaml:"commands"`
    DependsOn       []string          `yaml:"depends_on"`
    Env             map[string]string `yaml:"env"`
    WorkingDir      string            `yaml:"working_dir"`
    TimeoutMinutes  int               `yaml:"timeout_minutes"`
    ContinueOnError bool              `yaml:"continue_on_error"`
    Cache           *CacheConfig      `yaml:"cache"`
}

type CacheConfig struct {
    Paths []string `yaml:"paths"`
    Key   string   `yaml:"key"`
}
```

### 11.3 Create Workflow Parser
```go
type Parser struct{}

func (p *Parser) Parse(content []byte) (*Workflow, error)
func (p *Parser) ParseFile(path string) (*Workflow, error)
func (p *Parser) Validate(w *Workflow) error
```

### 11.4 Validation Rules
```go
func (p *Parser) Validate(w *Workflow) error {
    // 1. At least one step
    if len(w.Steps) == 0 {
        return errors.New("workflow must have at least one step")
    }
    
    // 2. Unique step names
    names := make(map[string]bool)
    for _, step := range w.Steps {
        if names[step.Name] {
            return fmt.Errorf("duplicate step name: %s", step.Name)
        }
        names[step.Name] = true
    }
    
    // 3. Dependencies reference existing steps
    for _, step := range w.Steps {
        for _, dep := range step.DependsOn {
            if !names[dep] {
                return fmt.Errorf("step %s depends on unknown step: %s", step.Name, dep)
            }
        }
    }
    
    // 4. No circular dependencies
    if err := p.detectCycles(w); err != nil {
        return err
    }
    
    // 5. Non-approval steps require image
    for _, step := range w.Steps {
        if step.Type != "approval" && step.Image == "" {
            return fmt.Errorf("step %s requires an image", step.Name)
        }
    }
    
    return nil
}
```

### 11.5 Cycle Detection
```go
func (p *Parser) detectCycles(w *Workflow) error {
    // Build adjacency list
    // DFS with three states: unvisited, visiting, visited
    // If we encounter "visiting" during DFS, there's a cycle
}
```

### 11.6 Trigger Matching
```go
func (w *Workflow) ShouldTrigger(event *WebhookEvent) bool {
    switch event.EventType {
    case "push":
        if w.On.Push == nil {
            return true // Default: trigger on all pushes
        }
        return w.On.Push.MatchesBranch(event.Branch) || w.On.Push.MatchesTag(event.Ref)
    case "pull_request":
        if w.On.PullRequest == nil {
            return false
        }
        return w.On.PullRequest.MatchesBranch(event.Branch)
    }
    return false
}

func (t *PushTrigger) MatchesBranch(branch string) bool {
    if len(t.Branches) == 0 {
        return true
    }
    for _, pattern := range t.Branches {
        if matchGlob(pattern, branch) {
            return true
        }
    }
    return false
}
```

### 11.7 Execution Order Calculation
```go
// Returns steps grouped by execution order
// Steps in the same group can run in parallel
func (w *Workflow) ExecutionGroups() [][]Step {
    // Topological sort respecting dependencies
    // Group steps that can run concurrently
}

// Returns steps that can be started given completed steps
func (w *Workflow) ReadySteps(completed []string) []Step {
    // Find steps where all dependencies are in completed list
}
```

### 11.8 Add Tests
- Test valid workflow parsing
- Test validation errors
- Test cycle detection
- Test trigger matching
- Test execution order calculation

## Deliverables
- [ ] `internal/workflow/types.go` - Workflow types
- [ ] `internal/workflow/parser.go` - YAML parser
- [ ] `internal/workflow/validator.go` - Validation logic
- [ ] `internal/workflow/scheduler.go` - Execution order
- [ ] `internal/workflow/workflow_test.go` - Tests
- [ ] Example workflow files for testing

## Dependencies
- None (can be developed independently)

## Estimated Effort
Medium - Complex parsing and validation logic
