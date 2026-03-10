---
model: sonnet
---

# Step 13: Git Operations

## Objective
Implement git operations for cloning repositories and checking out specific commits.

## Tasks

### 13.1 Create Git Service Interface
```go
type GitService interface {
    Clone(ctx context.Context, opts CloneOptions) error
    Checkout(ctx context.Context, repoPath, ref string) error
    GetWorkflowFile(ctx context.Context, repoPath string) ([]byte, error)
}

type CloneOptions struct {
    URL        string
    Path       string
    Branch     string
    Depth      int    // Shallow clone depth (0 for full)
    Token      string // For HTTPS auth
    SSHKey     string // For SSH auth
}
```

### 13.2 Implement Git Service Using CLI
```go
type CLIGitService struct {
    timeout time.Duration
}

func NewCLIGitService() *CLIGitService {
    return &CLIGitService{timeout: 5 * time.Minute}
}

func (s *CLIGitService) Clone(ctx context.Context, opts CloneOptions) error {
    args := []string{"clone"}
    
    if opts.Depth > 0 {
        args = append(args, "--depth", strconv.Itoa(opts.Depth))
    }
    
    if opts.Branch != "" {
        args = append(args, "--branch", opts.Branch)
    }
    
    // Handle authentication via URL modification for HTTPS
    cloneURL := opts.URL
    if opts.Token != "" && strings.HasPrefix(cloneURL, "https://") {
        // Insert token into URL: https://token@github.com/...
        u, _ := url.Parse(cloneURL)
        u.User = url.UserPassword("x-access-token", opts.Token)
        cloneURL = u.String()
    }
    
    args = append(args, cloneURL, opts.Path)
    
    cmd := exec.CommandContext(ctx, "git", args...)
    return cmd.Run()
}

func (s *CLIGitService) Checkout(ctx context.Context, repoPath, ref string) error {
    cmd := exec.CommandContext(ctx, "git", "checkout", ref)
    cmd.Dir = repoPath
    return cmd.Run()
}

func (s *CLIGitService) GetWorkflowFile(ctx context.Context, repoPath string) ([]byte, error) {
    path := filepath.Join(repoPath, ".featherci", "workflow.yml")
    return os.ReadFile(path)
}
```

### 13.3 Create Workspace Manager
```go
type WorkspaceManager struct {
    basePath string
}

func NewWorkspaceManager(basePath string) *WorkspaceManager {
    return &WorkspaceManager{basePath: basePath}
}

func (m *WorkspaceManager) CreateWorkspace(buildID int64) (string, error) {
    // Create unique directory for build
    // /workspaces/build-{id}-{timestamp}/
    path := filepath.Join(m.basePath, fmt.Sprintf("build-%d-%d", buildID, time.Now().Unix()))
    return path, os.MkdirAll(path, 0755)
}

func (m *WorkspaceManager) CleanupWorkspace(path string) error {
    return os.RemoveAll(path)
}

func (m *WorkspaceManager) GetWorkspacePath(buildID int64) string {
    // Find existing workspace for build
}
```

### 13.4 Clone With Token Refresh
```go
func (s *GitService) CloneWithAuth(ctx context.Context, project *Project, user *User, destPath string, commitSHA string) error {
    // 1. Get access token for the project's provider
    token, err := s.getAccessToken(ctx, project, user)
    if err != nil {
        return fmt.Errorf("getting access token: %w", err)
    }
    
    // 2. Clone repository
    opts := CloneOptions{
        URL:   project.CloneURL,
        Path:  destPath,
        Depth: 1, // Shallow clone for speed
        Token: token,
    }
    
    if err := s.Clone(ctx, opts); err != nil {
        return fmt.Errorf("cloning: %w", err)
    }
    
    // 3. Checkout specific commit
    if err := s.Checkout(ctx, destPath, commitSHA); err != nil {
        return fmt.Errorf("checkout: %w", err)
    }
    
    return nil
}
```

### 13.5 Token Provider
```go
type TokenProvider struct {
    users     UserRepository
    providers *auth.Registry
}

func (p *TokenProvider) GetTokenForProject(ctx context.Context, project *Project) (string, error) {
    // Find any user with access to this project
    // Use their access token
    // Refresh if needed
}
```

### 13.6 Workflow File Fetching via API
For initial build creation, fetch workflow file via provider API to avoid cloning:
```go
func (p *GitHubProvider) GetFileContent(ctx context.Context, token *oauth2.Token, repo, path, ref string) ([]byte, error) {
    // GET /repos/{owner}/{repo}/contents/{path}?ref={ref}
    // Returns base64 encoded content
}
```

Similar for GitLab and Gitea.

### 13.7 SSH Key Support (Optional)
```go
func (s *CLIGitService) CloneWithSSH(ctx context.Context, opts CloneOptions) error {
    // Write SSH key to temp file
    keyFile, err := writeTempSSHKey(opts.SSHKey)
    if err != nil {
        return err
    }
    defer os.Remove(keyFile)
    
    // Set GIT_SSH_COMMAND
    cmd := exec.CommandContext(ctx, "git", "clone", opts.URL, opts.Path)
    cmd.Env = append(os.Environ(), 
        fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no", keyFile))
    
    return cmd.Run()
}
```

### 13.8 Add Tests
- Test clone operation (with mocked git)
- Test checkout
- Test workspace creation/cleanup
- Test URL token injection

## Deliverables
- [ ] `internal/git/service.go` - Git service implementation
- [ ] `internal/git/workspace.go` - Workspace management
- [ ] `internal/git/tokens.go` - Token provider
- [ ] Tests for git operations

## Dependencies
- Step 04: OAuth providers (for API access)
- Step 05: User management (for tokens)

## Estimated Effort
Medium - Git integration
