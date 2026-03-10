package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/templates"
)

// AdminPageData holds data for the admin page.
type AdminPageData struct {
	User     *models.User
	Tab      string // "overview", "users", "workers", "projects"
	Users    []*models.User
	Workers  []*models.Worker
	Projects []*models.ProjectWithStatus
	Stats    AdminStats
	Error    string
	Success  string
}

// AdminStats holds system-wide statistics.
type AdminStats struct {
	UserCount    int
	ProjectCount int
	BuildCount   int
	WorkerCount  int
}

// AdminHandler handles admin panel requests.
type AdminHandler struct {
	users     models.UserRepository
	projects  models.ProjectRepository
	builds    models.BuildRepository
	workers   models.WorkerRepository
	templates *templates.Engine
	logger    *slog.Logger
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(
	users models.UserRepository,
	projects models.ProjectRepository,
	builds models.BuildRepository,
	workers models.WorkerRepository,
	tmpl *templates.Engine,
	logger *slog.Logger,
) *AdminHandler {
	return &AdminHandler{
		users:     users,
		projects:  projects,
		builds:    builds,
		workers:   workers,
		templates: tmpl,
		logger:    logger,
	}
}

// Dashboard renders the admin panel.
// GET /admin
func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	ctx := r.Context()

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "overview"
	}

	data := AdminPageData{
		User:    user,
		Tab:     tab,
		Success: r.URL.Query().Get("success"),
		Error:   r.URL.Query().Get("error"),
	}

	// Always load stats for the overview section
	userCount := 0
	if users, err := h.users.List(ctx); err == nil {
		userCount = len(users)
	}
	projectCount, _ := h.projects.CountAll(ctx)
	buildCount, _ := h.builds.Count(ctx)
	workerCount, _ := h.workers.CountActive(ctx)
	data.Stats = AdminStats{
		UserCount:    userCount,
		ProjectCount: projectCount,
		BuildCount:   buildCount,
		WorkerCount:  workerCount,
	}

	// Load tab-specific data
	switch tab {
	case "users":
		users, err := h.users.List(ctx)
		if err != nil {
			h.logger.Error("failed to list users", "error", err)
			data.Error = "Failed to load users"
		}
		data.Users = users
	case "workers":
		workers, err := h.workers.List(ctx)
		if err != nil {
			h.logger.Error("failed to list workers", "error", err)
			data.Error = "Failed to load workers"
		}
		data.Workers = workers
	case "projects":
		projects, err := h.projects.ListWithStatus(ctx)
		if err != nil {
			h.logger.Error("failed to list projects", "error", err)
			data.Error = "Failed to load projects"
		}
		data.Projects = projects
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/admin.html", data); err != nil {
		h.logger.Error("failed to render admin page", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// AddUser creates a placeholder user record.
// POST /admin/users
func (h *AdminHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin?tab=users&error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	provider := strings.TrimSpace(r.FormValue("provider"))
	username := strings.TrimSpace(r.FormValue("username"))

	if provider == "" || username == "" {
		http.Redirect(w, r, "/admin?tab=users&error=Provider+and+username+are+required", http.StatusSeeOther)
		return
	}

	// Validate provider
	switch provider {
	case "github", "gitlab", "gitea":
	default:
		http.Redirect(w, r, "/admin?tab=users&error=Invalid+provider", http.StatusSeeOther)
		return
	}

	// Check if user already exists
	_, err := h.users.GetByUsername(r.Context(), provider, username)
	if err == nil {
		http.Redirect(w, r, "/admin?tab=users&error=User+already+exists", http.StatusSeeOther)
		return
	}

	newUser := &models.User{
		Provider: provider,
		Username: username,
	}
	if err := h.users.Create(r.Context(), newUser); err != nil {
		h.logger.Error("failed to create user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&error=Failed+to+create+user", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?tab=users&success=User+added", http.StatusSeeOther)
}

// ToggleAdmin toggles admin status for a user.
// POST /admin/users/{id}/toggle-admin
func (h *AdminHandler) ToggleAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin?tab=users&error=Invalid+user+ID", http.StatusSeeOther)
		return
	}

	currentUser := middleware.UserFromContext(r.Context())
	if currentUser.ID == id {
		http.Redirect(w, r, "/admin?tab=users&error=Cannot+change+your+own+admin+status", http.StatusSeeOther)
		return
	}

	target, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&error=User+not+found", http.StatusSeeOther)
		return
	}

	target.IsAdmin = !target.IsAdmin
	if err := h.users.Update(r.Context(), target); err != nil {
		h.logger.Error("failed to update user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&error=Failed+to+update+user", http.StatusSeeOther)
		return
	}

	msg := fmt.Sprintf("Admin+%s+for+%s", map[bool]string{true: "enabled", false: "disabled"}[target.IsAdmin], target.Username)
	http.Redirect(w, r, "/admin?tab=users&success="+msg, http.StatusSeeOther)
}

// RemoveUser removes a user.
// POST /admin/users/{id}/delete
func (h *AdminHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin?tab=users&error=Invalid+user+ID", http.StatusSeeOther)
		return
	}

	currentUser := middleware.UserFromContext(r.Context())
	if currentUser.ID == id {
		http.Redirect(w, r, "/admin?tab=users&error=Cannot+delete+yourself", http.StatusSeeOther)
		return
	}

	if err := h.users.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete user", "error", err)
		http.Redirect(w, r, "/admin?tab=users&error=Failed+to+delete+user", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?tab=users&success=User+removed", http.StatusSeeOther)
}
