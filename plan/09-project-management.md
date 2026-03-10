---
model: opus
---

# Step 09: Project Management

## Objective
Implement project creation, listing, and management functionality.

## Tasks

### 9.1 Create Project Model
```go
type Project struct {
    ID            int64
    Provider      string    // 'github', 'gitlab', 'gitea'
    Namespace     string    // org/user name
    Name          string    // repo name
    FullName      string    // namespace/name
    CloneURL      string
    WebhookSecret string
    DefaultBranch string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type ProjectRepository interface {
    Create(ctx context.Context, project *Project) error
    GetByID(ctx context.Context, id int64) (*Project, error)
    GetByFullName(ctx context.Context, provider, fullName string) (*Project, error)
    List(ctx context.Context) ([]*Project, error)
    ListForUser(ctx context.Context, userID int64) ([]*Project, error)
    Update(ctx context.Context, project *Project) error
    Delete(ctx context.Context, id int64) error
}
```

### 9.2 Create Project User Association Model
```go
type ProjectUser struct {
    ProjectID int64
    UserID    int64
    CanManage bool
    CreatedAt time.Time
}

type ProjectUserRepository interface {
    Add(ctx context.Context, projectID, userID int64, canManage bool) error
    Remove(ctx context.Context, projectID, userID int64) error
    GetUsersForProject(ctx context.Context, projectID int64) ([]*User, error)
    GetProjectsForUser(ctx context.Context, userID int64) ([]*Project, error)
    CanUserAccess(ctx context.Context, projectID, userID int64) (bool, error)
    CanUserManage(ctx context.Context, projectID, userID int64) (bool, error)
}
```

### 9.3 Create Project Handlers
```go
type ProjectHandler struct {
    projects     ProjectRepository
    projectUsers ProjectUserRepository
    users        UserRepository
    providers    *auth.Registry
}

// List all projects user has access to
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request)

// Show form to add new project
func (h *ProjectHandler) New(w http.ResponseWriter, r *http.Request)

// Handle project creation
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request)

// Show project details
func (h *ProjectHandler) Show(w http.ResponseWriter, r *http.Request)

// Show project settings
func (h *ProjectHandler) Settings(w http.ResponseWriter, r *http.Request)

// Update project settings
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request)

// Delete project
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request)
```

### 9.4 Create "New Project" Flow
When user clicks "New Project":
1. Fetch list of repos from OAuth provider using stored access token
2. Display repos grouped by namespace
3. Filter out repos already added as projects
4. User selects a repo
5. System checks if project already exists (another user added it)
6. If exists: add user to project_users with appropriate permissions
7. If new: create project, generate webhook secret, add user as manager

### 9.5 Create Project List Template
`web/templates/pages/projects/list.html`:
- Grid of project cards
- Show recent build status for each
- Link to project details
- "Add Project" button

### 9.6 Create New Project Template
`web/templates/pages/projects/new.html`:
- Tabs or sections for each OAuth provider
- List of available repos
- Search/filter functionality
- Indicate which repos are already added

### 9.7 Create Project Detail Template
`web/templates/pages/projects/show.html`:
- Project name and provider
- Default branch
- Recent builds list
- Link to settings, secrets
- Trigger build button

### 9.8 Create Project Settings Template
`web/templates/pages/projects/settings.html`:
- Webhook URL display
- Default branch selection
- Delete project option (with confirmation)

### 9.9 Repository Fetching with Pagination
```go
func (h *ProjectHandler) fetchUserRepos(ctx context.Context, user *User) ([]auth.Repository, error) {
    provider, ok := h.providers.Get(user.Provider)
    if !ok {
        return nil, fmt.Errorf("unknown provider: %s", user.Provider)
    }
    
    token := &oauth2.Token{
        AccessToken:  user.AccessToken,
        RefreshToken: user.RefreshToken,
    }
    
    // Refresh token if needed
    if token.RefreshToken != "" {
        newToken, err := provider.RefreshToken(ctx, token)
        if err == nil {
            token = newToken
            // Update stored tokens
        }
    }
    
    return provider.GetUserRepositories(ctx, token)
}
```

### 9.10 Add Tests
- Test project CRUD
- Test user-project associations
- Test "project already exists" flow
- Test permission checks

## Deliverables
- [ ] `internal/models/project.go` - Project model
- [ ] `internal/handlers/project.go` - Project handlers
- [ ] `web/templates/pages/projects/*.html` - Project templates
- [ ] Project listing works
- [ ] Project creation works
- [ ] Existing project detection works

## Dependencies
- Step 04: OAuth providers (for repo listing)
- Step 05: User management
- Step 08: Templates

## Estimated Effort
Large - Core feature
