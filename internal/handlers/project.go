package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/featherci/featherci/internal/auth"
	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/templates"
	"golang.org/x/oauth2"
)

// ProjectHandler handles project-related HTTP endpoints.
type ProjectHandler struct {
	projects     models.ProjectRepository
	projectUsers models.ProjectUserRepository
	users        models.UserRepository
	builds       models.BuildRepository
	providers    *auth.Registry
	templates    *templates.Engine
	logger       *slog.Logger
}

// NewProjectHandler creates a new project handler.
func NewProjectHandler(
	projects models.ProjectRepository,
	projectUsers models.ProjectUserRepository,
	users models.UserRepository,
	builds models.BuildRepository,
	providers *auth.Registry,
	templates *templates.Engine,
	logger *slog.Logger,
) *ProjectHandler {
	return &ProjectHandler{
		projects:     projects,
		projectUsers: projectUsers,
		users:        users,
		builds:       builds,
		providers:    providers,
		templates:    templates,
		logger:       logger,
	}
}

// ProjectListPageData holds data for the project list page.
type ProjectListPageData struct {
	User     *models.User
	Projects []*models.ProjectWithStatus
	DevMode  bool
}

// ProjectNewPageData holds data for the new project page.
type ProjectNewPageData struct {
	User         *models.User
	Repositories []RepositoryGroup
	DevMode      bool
	Error        string
}

// RepositoryGroup groups repositories by namespace/owner.
type RepositoryGroup struct {
	Namespace    string
	Repositories []RepositoryItem
}

// RepositoryItem represents a repository that can be added.
type RepositoryItem struct {
	auth.Repository
	AlreadyAdded    bool
	ProjectFullName string // For linking to already-added projects
}

// ProjectShowPageData holds data for the project detail page.
type ProjectShowPageData struct {
	User         *models.User
	Project      *models.Project
	RecentBuilds []*models.Build
	CanManage    bool
	DevMode      bool
	TotalBuilds  int
	SuccessRate  string
	AvgDuration  string
}

// ProjectSettingsPageData holds data for the project settings page.
type ProjectSettingsPageData struct {
	User       *models.User
	Project    *models.Project
	WebhookURL string
	DevMode    bool
	Error      string
	Success    string
}

// List shows all projects the user has access to.
// GET /projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	projects, err := h.projectUsers.GetProjectsForUserWithStatus(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("failed to get projects for user", "error", err, "user_id", user.ID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := ProjectListPageData{
		User:     user,
		Projects: projects,
		DevMode:  false, // Will be set by caller if needed
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/projects/list.html", data); err != nil {
		h.logger.Error("failed to render project list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// New shows the form to add a new project.
// GET /projects/new
func (h *ProjectHandler) New(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	repos, err := h.fetchUserRepositories(r, user)
	if err != nil {
		h.logger.Error("failed to fetch repositories", "error", err, "user_id", user.ID)
		data := ProjectNewPageData{
			User:    user,
			Error:   "Failed to fetch repositories from " + user.Provider + ". Please try again.",
			DevMode: false,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.templates.Render(w, "pages/projects/new.html", data); err != nil {
			h.logger.Error("failed to render new project page", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Group repositories by namespace
	groups := h.groupRepositories(r.Context(), repos, user.Provider)

	data := ProjectNewPageData{
		User:         user,
		Repositories: groups,
		DevMode:      false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/projects/new.html", data); err != nil {
		h.logger.Error("failed to render new project page", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create handles project creation.
// POST /projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	fullName := r.FormValue("full_name")
	cloneURL := r.FormValue("clone_url")

	if fullName == "" || cloneURL == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Parse namespace and name from full_name (e.g., "owner/repo")
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusBadRequest)
		return
	}
	namespace, name := parts[0], parts[1]

	ctx := r.Context()

	// Check if project already exists
	existingProject, err := h.projects.GetByFullName(ctx, user.Provider, fullName)
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		h.logger.Error("failed to check existing project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if existingProject != nil {
		// Project exists - add user to it if not already a member
		hasAccess, err := h.projectUsers.CanUserAccess(ctx, existingProject.ID, user.ID)
		if err != nil {
			h.logger.Error("failed to check user access", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !hasAccess {
			// Add user to project (not as manager since they didn't create it)
			if err := h.projectUsers.Add(ctx, existingProject.ID, user.ID, false); err != nil {
				h.logger.Error("failed to add user to project", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		}

		// Redirect to the existing project
		http.Redirect(w, r, fmt.Sprintf("/projects/%s/%s", existingProject.Namespace, existingProject.Name), http.StatusSeeOther)
		return
	}

	// Create new project
	project := &models.Project{
		Provider:      user.Provider,
		Namespace:     namespace,
		Name:          name,
		FullName:      fullName,
		CloneURL:      cloneURL,
		DefaultBranch: "main", // TODO: Fetch default branch from provider API
	}

	if err := h.projects.Create(ctx, project); err != nil {
		h.logger.Error("failed to create project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Add user as manager of the project
	if err := h.projectUsers.Add(ctx, project.ID, user.ID, true); err != nil {
		h.logger.Error("failed to add user to project", "error", err)
		// Try to clean up the project
		_ = h.projects.Delete(ctx, project.ID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("project created", "project_id", project.ID, "full_name", fullName, "user_id", user.ID)

	// Redirect to the new project
	http.Redirect(w, r, fmt.Sprintf("/projects/%s/%s", project.Namespace, project.Name), http.StatusSeeOther)
}

// Show displays a project's details.
// GET /projects/{namespace}/{name}
func (h *ProjectHandler) Show(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	namespace, name, err := h.getProjectFromPath(r)
	if err != nil {
		http.Error(w, "Invalid project path", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	fullName := namespace + "/" + name

	project, err := h.projects.GetByFullName(ctx, user.Provider, fullName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if user has access
	hasAccess, err := h.projectUsers.CanUserAccess(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check user access", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !hasAccess && !user.IsAdmin {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	canManage, err := h.projectUsers.CanUserManage(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		canManage = false
	}

	// Admin can always manage
	if user.IsAdmin {
		canManage = true
	}

	// Load recent builds
	recentBuilds, err := h.builds.ListByProject(ctx, project.ID, 10, 0)
	if err != nil {
		h.logger.Error("failed to load recent builds", "error", err)
		recentBuilds = nil
	}

	// Compute stats from recent builds
	totalBuilds, err := h.builds.CountByProject(ctx, project.ID)
	if err != nil {
		h.logger.Error("failed to count builds", "error", err)
	}

	var successCount int
	var totalDuration time.Duration
	var durationCount int
	for _, b := range recentBuilds {
		if b.Status == models.BuildStatusSuccess {
			successCount++
		}
		d := b.Duration()
		if d > 0 {
			totalDuration += d
			durationCount++
		}
	}

	successRate := "-"
	if len(recentBuilds) > 0 {
		successRate = fmt.Sprintf("%.0f%%", float64(successCount)/float64(len(recentBuilds))*100)
	}
	avgDuration := "-"
	if durationCount > 0 {
		avgDuration = formatDurationShort(totalDuration / time.Duration(durationCount))
	}

	data := ProjectShowPageData{
		User:         user,
		Project:      project,
		RecentBuilds: recentBuilds,
		CanManage:    canManage,
		DevMode:      false,
		TotalBuilds:  totalBuilds,
		SuccessRate:  successRate,
		AvgDuration:  avgDuration,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/projects/show.html", data); err != nil {
		h.logger.Error("failed to render project show", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Settings shows the project settings page.
// GET /projects/{namespace}/{name}/settings
func (h *ProjectHandler) Settings(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	namespace, name, err := h.getProjectFromPath(r)
	if err != nil {
		http.Error(w, "Invalid project path", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	fullName := namespace + "/" + name

	project, err := h.projects.GetByFullName(ctx, user.Provider, fullName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if user can manage project
	canManage, err := h.projectUsers.CanUserManage(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !canManage && !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// TODO: Build webhook URL from config
	webhookURL := fmt.Sprintf("/webhooks/%s", project.Provider)

	data := ProjectSettingsPageData{
		User:       user,
		Project:    project,
		WebhookURL: webhookURL,
		DevMode:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/projects/settings.html", data); err != nil {
		h.logger.Error("failed to render project settings", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Update handles project settings update.
// POST /projects/{namespace}/{name}/settings
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	namespace, name, err := h.getProjectFromPath(r)
	if err != nil {
		http.Error(w, "Invalid project path", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	fullName := namespace + "/" + name

	project, err := h.projects.GetByFullName(ctx, user.Provider, fullName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if user can manage project
	canManage, err := h.projectUsers.CanUserManage(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !canManage && !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Update default branch if provided
	if branch := r.FormValue("default_branch"); branch != "" {
		project.DefaultBranch = branch
	}

	if err := h.projects.Update(ctx, project); err != nil {
		h.logger.Error("failed to update project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("project updated", "project_id", project.ID, "user_id", user.ID)

	// Redirect back to settings with success message
	http.Redirect(w, r, fmt.Sprintf("/projects/%s/%s/settings?success=1", project.Namespace, project.Name), http.StatusSeeOther)
}

// Delete handles project deletion.
// POST /projects/{namespace}/{name}/delete
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	namespace, name, err := h.getProjectFromPath(r)
	if err != nil {
		http.Error(w, "Invalid project path", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	fullName := namespace + "/" + name

	project, err := h.projects.GetByFullName(ctx, user.Provider, fullName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if user can manage project
	canManage, err := h.projectUsers.CanUserManage(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !canManage && !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.projects.Delete(ctx, project.ID); err != nil {
		h.logger.Error("failed to delete project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("project deleted", "project_id", project.ID, "user_id", user.ID)

	// Redirect to projects list
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// fetchUserRepositories fetches repositories from the user's OAuth provider.
func (h *ProjectHandler) fetchUserRepositories(r *http.Request, user *models.User) ([]auth.Repository, error) {
	provider, ok := h.providers.Get(user.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", user.Provider)
	}

	token := &oauth2.Token{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
	}

	// Try to refresh token if we have a refresh token
	if token.RefreshToken != "" {
		newToken, err := provider.RefreshToken(r.Context(), token)
		if err == nil && newToken.AccessToken != token.AccessToken {
			// Token was refreshed, update stored tokens
			if err := h.users.UpdateTokens(r.Context(), user.ID, newToken.AccessToken, newToken.RefreshToken); err != nil {
				h.logger.Warn("failed to update user tokens", "error", err, "user_id", user.ID)
			}
			token = newToken
		}
	}

	return provider.GetRepositories(r.Context(), token)
}

// groupRepositories groups repositories by namespace and marks already-added ones.
func (h *ProjectHandler) groupRepositories(ctx context.Context, repos []auth.Repository, providerName string) []RepositoryGroup {
	// Get existing projects for this provider
	existingProjects := make(map[string]string) // fullName -> fullName (for URL)
	projects, err := h.projects.List(ctx)
	if err == nil {
		for _, p := range projects {
			if p.Provider == providerName {
				existingProjects[p.FullName] = p.FullName
			}
		}
	}

	// Group by namespace
	groupMap := make(map[string][]RepositoryItem)
	for _, repo := range repos {
		item := RepositoryItem{
			Repository:      repo,
			AlreadyAdded:    false,
			ProjectFullName: "",
		}
		if fullName, exists := existingProjects[repo.FullName]; exists {
			item.AlreadyAdded = true
			item.ProjectFullName = fullName
		}
		groupMap[repo.Namespace] = append(groupMap[repo.Namespace], item)
	}

	// Convert to slice and sort
	var groups []RepositoryGroup
	for ns, items := range groupMap {
		groups = append(groups, RepositoryGroup{
			Namespace:    ns,
			Repositories: items,
		})
	}

	return groups
}

// getProjectFromPath extracts the namespace and name from the URL path.
// Returns (namespace, name, error).
func (h *ProjectHandler) getProjectFromPath(r *http.Request) (string, string, error) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	if namespace == "" || name == "" {
		// Fallback to parsing from path for routes like /projects/{namespace}/{name}
		path := r.URL.Path
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "projects" {
			namespace = parts[1]
			name = parts[2]
		}
	}

	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("invalid project path")
	}

	return namespace, name, nil
}

func formatDurationShort(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
