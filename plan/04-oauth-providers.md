---
model: sonnet
---

# Step 04: OAuth Authentication - Providers

## Objective
Implement OAuth 2.0 authentication for GitHub, GitLab, and Gitea/Forgejo.

## Tasks

### 4.1 Add OAuth Dependencies
```bash
go get golang.org/x/oauth2
go get golang.org/x/oauth2/github
go get golang.org/x/oauth2/gitlab
```

### 4.2 Create Provider Interface
```go
type Provider interface {
    Name() string
    AuthCodeURL(state string) string
    Exchange(ctx context.Context, code string) (*oauth2.Token, error)
    GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error)
    GetUserRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error)
    RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error)
}

type UserInfo struct {
    ID        string
    Username  string
    Email     string
    AvatarURL string
}

type Repository struct {
    ID          string
    FullName    string  // namespace/name
    Namespace   string
    Name        string
    CloneURL    string
    SSHURL      string
    Private     bool
    Permissions struct {
        Admin bool
        Push  bool
        Pull  bool
    }
}
```

### 4.3 Implement GitHub Provider
```go
type GitHubProvider struct {
    config *oauth2.Config
}

func NewGitHubProvider(clientID, clientSecret, callbackURL string) *GitHubProvider
func (p *GitHubProvider) Name() string
func (p *GitHubProvider) AuthCodeURL(state string) string
func (p *GitHubProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error)
func (p *GitHubProvider) GetUser(ctx context.Context, token *oauth2.Token) (*UserInfo, error)
func (p *GitHubProvider) GetUserRepositories(ctx context.Context, token *oauth2.Token) ([]Repository, error)
```

GitHub API endpoints:
- User: `GET /user`
- Repos: `GET /user/repos?per_page=100` (paginate)

### 4.4 Implement GitLab Provider
```go
type GitLabProvider struct {
    config  *oauth2.Config
    baseURL string // For self-hosted
}

func NewGitLabProvider(clientID, clientSecret, callbackURL, baseURL string) *GitLabProvider
```

GitLab API endpoints:
- User: `GET /api/v4/user`
- Repos: `GET /api/v4/projects?membership=true&per_page=100`

### 4.5 Implement Gitea Provider
```go
type GiteaProvider struct {
    config  *oauth2.Config
    baseURL string
}

func NewGiteaProvider(clientID, clientSecret, callbackURL, baseURL string) *GiteaProvider
```

Gitea API endpoints:
- User: `GET /api/v1/user`
- Repos: `GET /api/v1/user/repos?limit=50` (paginate)

### 4.6 Create Provider Registry
```go
type Registry struct {
    providers map[string]Provider
}

func NewRegistry(cfg *config.Config) *Registry
func (r *Registry) Get(name string) (Provider, bool)
func (r *Registry) Available() []string
```

### 4.7 Add Tests
- Mock HTTP responses for each provider
- Test OAuth flow
- Test user info parsing
- Test repository listing

## Deliverables
- [ ] `internal/auth/provider.go` - Interface definitions
- [ ] `internal/auth/github.go` - GitHub implementation
- [ ] `internal/auth/gitlab.go` - GitLab implementation
- [ ] `internal/auth/gitea.go` - Gitea implementation
- [ ] `internal/auth/registry.go` - Provider registry
- [ ] `internal/auth/*_test.go` - Tests for each provider

## Dependencies
- Step 02: Configuration (OAuth credentials)

## Estimated Effort
Medium - Three OAuth integrations
