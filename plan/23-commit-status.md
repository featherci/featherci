---
model: sonnet
---

# Step 23: Commit Status Integration

## Objective
Post build status updates to GitHub, GitLab, and Gitea/Forgejo using their commit status APIs.

## Tasks

### 23.1 Create Status Poster Interface
```go
type StatusPoster interface {
    PostStatus(ctx context.Context, opts StatusOptions) error
}

type StatusOptions struct {
    Provider    string
    Owner       string
    Repo        string
    CommitSHA   string
    State       CommitState
    TargetURL   string
    Description string
    Context     string // "featherci" or "featherci/step-name"
}

type CommitState string

const (
    CommitStatePending CommitState = "pending"
    CommitStateRunning CommitState = "running"  // GitLab only
    CommitStateSuccess CommitState = "success"
    CommitStateFailure CommitState = "failure"
    CommitStateError   CommitState = "error"
)
```

### 23.2 Implement GitHub Status Poster
```go
type GitHubStatusPoster struct {
    tokens TokenProvider
}

func (p *GitHubStatusPoster) PostStatus(ctx context.Context, opts StatusOptions) error {
    token, err := p.tokens.GetTokenForProject(ctx, opts.Owner, opts.Repo)
    if err != nil {
        return err
    }
    
    // GitHub uses different state names
    state := p.mapState(opts.State)
    
    // POST /repos/{owner}/{repo}/statuses/{sha}
    url := fmt.Sprintf("https://api.github.com/repos/%s/%s/statuses/%s",
        opts.Owner, opts.Repo, opts.CommitSHA)
    
    payload := map[string]string{
        "state":       state,
        "target_url":  opts.TargetURL,
        "description": opts.Description,
        "context":     opts.Context,
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/vnd.github+json")
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode >= 400 {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("GitHub API error: %s", body)
    }
    
    return nil
}

func (p *GitHubStatusPoster) mapState(state CommitState) string {
    switch state {
    case CommitStatePending, CommitStateRunning:
        return "pending"
    case CommitStateSuccess:
        return "success"
    case CommitStateFailure:
        return "failure"
    case CommitStateError:
        return "error"
    default:
        return "pending"
    }
}
```

### 23.3 Implement GitLab Status Poster
```go
type GitLabStatusPoster struct {
    tokens  TokenProvider
    baseURL string
}

func (p *GitLabStatusPoster) PostStatus(ctx context.Context, opts StatusOptions) error {
    token, err := p.tokens.GetTokenForProject(ctx, opts.Owner, opts.Repo)
    if err != nil {
        return err
    }
    
    // GitLab uses project ID or URL-encoded path
    projectPath := url.PathEscape(opts.Owner + "/" + opts.Repo)
    
    // POST /api/v4/projects/{id}/statuses/{sha}
    apiURL := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s",
        p.baseURL, projectPath, opts.CommitSHA)
    
    // GitLab state mapping
    state := p.mapState(opts.State)
    
    payload := map[string]string{
        "state":       state,
        "target_url":  opts.TargetURL,
        "description": opts.Description,
        "name":        opts.Context,
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
    req.Header.Set("PRIVATE-TOKEN", token)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    // ... error handling
    
    return nil
}

func (p *GitLabStatusPoster) mapState(state CommitState) string {
    switch state {
    case CommitStatePending:
        return "pending"
    case CommitStateRunning:
        return "running"
    case CommitStateSuccess:
        return "success"
    case CommitStateFailure:
        return "failed"
    case CommitStateError:
        return "failed"
    default:
        return "pending"
    }
}
```

### 23.4 Implement Gitea Status Poster
```go
type GiteaStatusPoster struct {
    tokens  TokenProvider
    baseURL string
}

func (p *GiteaStatusPoster) PostStatus(ctx context.Context, opts StatusOptions) error {
    token, err := p.tokens.GetTokenForProject(ctx, opts.Owner, opts.Repo)
    if err != nil {
        return err
    }
    
    // POST /api/v1/repos/{owner}/{repo}/statuses/{sha}
    apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/statuses/%s",
        p.baseURL, opts.Owner, opts.Repo, opts.CommitSHA)
    
    // Gitea uses same states as GitHub
    state := p.mapState(opts.State)
    
    payload := map[string]string{
        "state":       state,
        "target_url":  opts.TargetURL,
        "description": opts.Description,
        "context":     opts.Context,
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
    req.Header.Set("Authorization", "token "+token)
    req.Header.Set("Content-Type", "application/json")
    
    // ... rest similar to GitHub
    
    return nil
}
```

### 23.5 Create Status Service
```go
type StatusService struct {
    posters map[string]StatusPoster
    baseURL string
}

func NewStatusService(cfg *config.Config, tokens TokenProvider) *StatusService {
    s := &StatusService{
        posters: make(map[string]StatusPoster),
        baseURL: cfg.BaseURL,
    }
    
    s.posters["github"] = &GitHubStatusPoster{tokens: tokens}
    s.posters["gitlab"] = &GitLabStatusPoster{tokens: tokens, baseURL: cfg.GitLabURL}
    s.posters["gitea"] = &GiteaStatusPoster{tokens: tokens, baseURL: cfg.GiteaURL}
    
    return s
}

func (s *StatusService) PostBuildStatus(ctx context.Context, project *Project, build *Build) error {
    poster, ok := s.posters[project.Provider]
    if !ok {
        return fmt.Errorf("unknown provider: %s", project.Provider)
    }
    
    state := s.buildStatusToCommitState(build.Status)
    
    return poster.PostStatus(ctx, StatusOptions{
        Provider:    project.Provider,
        Owner:       project.Namespace,
        Repo:        project.Name,
        CommitSHA:   build.CommitSHA,
        State:       state,
        TargetURL:   fmt.Sprintf("%s/projects/%s/builds/%d", s.baseURL, project.FullName, build.BuildNumber),
        Description: s.buildDescription(build),
        Context:     "featherci",
    })
}

func (s *StatusService) buildStatusToCommitState(status BuildStatus) CommitState {
    switch status {
    case BuildStatusPending:
        return CommitStatePending
    case BuildStatusRunning:
        return CommitStateRunning
    case BuildStatusSuccess:
        return CommitStateSuccess
    case BuildStatusFailure:
        return CommitStateFailure
    case BuildStatusCancelled:
        return CommitStateError
    default:
        return CommitStatePending
    }
}

func (s *StatusService) buildDescription(build *Build) string {
    switch build.Status {
    case BuildStatusPending:
        return "Build is queued"
    case BuildStatusRunning:
        return "Build is running"
    case BuildStatusSuccess:
        return "Build succeeded"
    case BuildStatusFailure:
        return "Build failed"
    case BuildStatusCancelled:
        return "Build was cancelled"
    default:
        return "Build status unknown"
    }
}
```

### 23.6 Integrate with Build Orchestrator
```go
func (o *BuildOrchestrator) initializeBuild(ctx context.Context, build *Build) error {
    // ... create steps ...
    
    // Post pending status
    project, _ := o.projects.GetByID(ctx, build.ProjectID)
    o.status.PostBuildStatus(ctx, project, build)
    
    return nil
}

func (o *BuildOrchestrator) checkBuildCompletion(ctx context.Context, buildID int64) {
    // ... determine final status ...
    
    // Post final status
    project, _ := o.projects.GetByID(ctx, build.ProjectID)
    o.status.PostBuildStatus(ctx, project, build)
}
```

### 23.7 Add Tests
- Test status mapping for each provider
- Test API request formatting
- Test error handling
- Mock HTTP responses

## Deliverables
- [ ] `internal/status/poster.go` - Status poster interface
- [ ] `internal/status/github.go` - GitHub implementation
- [ ] `internal/status/gitlab.go` - GitLab implementation
- [ ] `internal/status/gitea.go` - Gitea implementation
- [ ] `internal/status/service.go` - Status service
- [ ] Integration with build orchestrator
- [ ] Tests

## Dependencies
- Step 04: OAuth providers (token access)
- Step 16: Build orchestration

## Estimated Effort
Medium - Three API integrations
