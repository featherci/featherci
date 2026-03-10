package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/services"
	"github.com/featherci/featherci/internal/templates"
)

// SecretListPageData holds data for the secrets list page.
type SecretListPageData struct {
	User    *models.User
	Project *models.Project
	Secrets []*models.Secret
	Success string
	Error   string
}

// SecretHandler handles secret-related HTTP requests.
type SecretHandler struct {
	secrets      *services.SecretService
	projects     models.ProjectRepository
	projectUsers models.ProjectUserRepository
	templates    *templates.Engine
	logger       *slog.Logger
}

// NewSecretHandler creates a new SecretHandler.
func NewSecretHandler(
	secrets *services.SecretService,
	projects models.ProjectRepository,
	projectUsers models.ProjectUserRepository,
	tmpl *templates.Engine,
	logger *slog.Logger,
) *SecretHandler {
	return &SecretHandler{
		secrets:      secrets,
		projects:     projects,
		projectUsers: projectUsers,
		templates:    tmpl,
		logger:       logger,
	}
}

// lookupProject finds a project by namespace/name, trying all providers.
func (h *SecretHandler) lookupProject(r *http.Request) (*models.Project, error) {
	ctx := r.Context()
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	fullName := namespace + "/" + name

	var project *models.Project
	var lastErr error
	for _, provider := range []string{"github", "gitlab", "gitea"} {
		p, err := h.projects.GetByFullName(ctx, provider, fullName)
		if err == nil {
			project = p
			break
		}
		lastErr = err
		if !errors.Is(err, models.ErrNotFound) {
			return nil, err
		}
	}
	if project == nil {
		return nil, lastErr
	}
	return project, nil
}

// requireManage checks auth and project manage permission. Returns user, project, or writes error.
func (h *SecretHandler) requireManage(w http.ResponseWriter, r *http.Request) (*models.User, *models.Project, bool) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return nil, nil, false
	}

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return nil, nil, false
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, nil, false
	}

	canManage, err := h.projectUsers.CanUserManage(r.Context(), project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check manage permission", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, nil, false
	}
	if !canManage && !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return nil, nil, false
	}

	return user, project, true
}

// List shows all secrets for a project.
// GET /projects/{namespace}/{name}/secrets
func (h *SecretHandler) List(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	secrets, err := h.secrets.ListSecrets(r.Context(), project.ID)
	if err != nil {
		h.logger.Error("failed to list secrets", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := SecretListPageData{
		User:    user,
		Project: project,
		Secrets: secrets,
		Success: r.URL.Query().Get("success"),
		Error:   r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/secrets/list.html", data); err != nil {
		h.logger.Error("failed to render secrets list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create adds a new secret.
// POST /projects/{namespace}/{name}/secrets
func (h *SecretHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	value := r.FormValue("value")

	redirectURL := "/projects/" + project.Namespace + "/" + project.Name + "/secrets"

	if err := h.secrets.CreateSecret(r.Context(), project.ID, name, value, user.ID); err != nil {
		http.Redirect(w, r, redirectURL+"?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectURL+"?success=Secret+created", http.StatusSeeOther)
}

// Delete removes a secret.
// POST /projects/{namespace}/{name}/secrets/{secretName}/delete
func (h *SecretHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, project, ok := h.requireManage(w, r)
	if !ok {
		return
	}

	secretName := r.PathValue("secretName")
	redirectURL := "/projects/" + project.Namespace + "/" + project.Name + "/secrets"

	if err := h.secrets.DeleteSecret(r.Context(), project.ID, secretName); err != nil {
		h.logger.Error("failed to delete secret", "error", err)
		http.Redirect(w, r, redirectURL+"?error=Failed+to+delete+secret", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectURL+"?success=Secret+deleted", http.StatusSeeOther)
}
