---
model: sonnet
---

# Step 10: Webhook Integration

## Objective
Implement webhook endpoints to receive events from GitHub, GitLab, and Gitea/Forgejo.

## Tasks

### 10.1 Create Webhook Handler Interface
```go
type WebhookEvent struct {
    Provider    string
    EventType   string    // "push", "pull_request", "merge_request"
    ProjectName string    // full_name
    Ref         string    // refs/heads/main, refs/tags/v1.0.0
    Branch      string    // extracted branch name
    CommitSHA   string
    CommitMsg   string
    CommitAuthor string
    PRNumber    int       // if PR event
    PRAction    string    // "opened", "synchronize", "closed"
    CloneURL    string
    Sender      string    // username who triggered
}

type WebhookHandler interface {
    ValidateSignature(r *http.Request, secret string) error
    ParseEvent(r *http.Request) (*WebhookEvent, error)
}
```

### 10.2 Implement GitHub Webhook Handler
```go
type GitHubWebhook struct{}

func (h *GitHubWebhook) ValidateSignature(r *http.Request, secret string) error {
    // X-Hub-Signature-256 header
    // HMAC-SHA256 of body with secret
}

func (h *GitHubWebhook) ParseEvent(r *http.Request) (*WebhookEvent, error) {
    eventType := r.Header.Get("X-GitHub-Event")
    // Parse JSON body based on event type
    // Handle: push, pull_request
}
```

GitHub events to handle:
- `push` - commits pushed to a branch
- `pull_request` - PR opened, updated, closed

### 10.3 Implement GitLab Webhook Handler
```go
type GitLabWebhook struct{}

func (h *GitLabWebhook) ValidateSignature(r *http.Request, secret string) error {
    // X-Gitlab-Token header (plain text comparison)
}

func (h *GitLabWebhook) ParseEvent(r *http.Request) (*WebhookEvent, error) {
    eventType := r.Header.Get("X-Gitlab-Event")
    // Parse JSON body
    // Handle: Push Hook, Merge Request Hook
}
```

GitLab events:
- `Push Hook` - commits pushed
- `Merge Request Hook` - MR opened, updated

### 10.4 Implement Gitea Webhook Handler
```go
type GiteaWebhook struct{}

func (h *GiteaWebhook) ValidateSignature(r *http.Request, secret string) error {
    // X-Gitea-Signature header (HMAC-SHA256)
}

func (h *GiteaWebhook) ParseEvent(r *http.Request) (*WebhookEvent, error) {
    eventType := r.Header.Get("X-Gitea-Event")
    // Parse JSON body
    // Handle: push, pull_request
}
```

### 10.5 Create HTTP Handlers
```go
func (h *WebhookHTTPHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
    event, err := h.processWebhook(r, "github", h.github)
    if err != nil {
        // Log error, return appropriate status
    }
    h.triggerBuild(r.Context(), event)
}

func (h *WebhookHTTPHandler) processWebhook(r *http.Request, provider string, handler WebhookHandler) (*WebhookEvent, error) {
    // 1. Find project by repo name
    // 2. Validate signature with project's webhook secret
    // 3. Parse event
    // 4. Return event
}
```

### 10.6 Create Build Trigger Logic
```go
func (h *WebhookHTTPHandler) triggerBuild(ctx context.Context, event *WebhookEvent) error {
    // 1. Find project
    project, err := h.projects.GetByFullName(ctx, event.Provider, event.ProjectName)
    
    // 2. Check if workflow file exists
    // This happens later during build execution
    
    // 3. Create build record
    build := &Build{
        ProjectID:         project.ID,
        CommitSHA:         event.CommitSHA,
        CommitMessage:     event.CommitMsg,
        CommitAuthor:      event.CommitAuthor,
        Branch:            event.Branch,
        PullRequestNumber: event.PRNumber,
        Status:            "pending",
    }
    
    // 4. Assign build number
    build.BuildNumber = h.builds.GetNextBuildNumber(ctx, project.ID)
    
    // 5. Save build
    h.builds.Create(ctx, build)
    
    // 6. Queue for execution (handled by worker system)
}
```

### 10.7 Webhook URL Generation
```go
func WebhookURL(baseURL, provider string) string {
    return fmt.Sprintf("%s/webhooks/%s", baseURL, provider)
}
```

Display this URL in project settings for manual webhook configuration.

### 10.8 Webhook Secret Generation
```go
func GenerateWebhookSecret() (string, error) {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return hex.EncodeToString(bytes), nil
}
```

### 10.9 Add Tests
- Test signature validation for each provider
- Test event parsing
- Test build creation from webhook
- Test unknown project handling
- Test invalid signature rejection

## Deliverables
- [ ] `internal/webhook/github.go` - GitHub webhook handler
- [ ] `internal/webhook/gitlab.go` - GitLab webhook handler
- [ ] `internal/webhook/gitea.go` - Gitea webhook handler
- [ ] `internal/handlers/webhook.go` - HTTP handlers
- [ ] Webhooks trigger builds correctly
- [ ] Invalid webhooks are rejected

## Dependencies
- Step 09: Project management (to look up projects)
- Step 03: Database (to create builds)

## Estimated Effort
Medium - Three webhook integrations
