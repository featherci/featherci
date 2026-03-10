package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/featherci/featherci/internal/executor"
	"github.com/featherci/featherci/internal/middleware"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/templates"
)

const buildsPerPage = 20

// BuildListPageData holds data for the build list page.
type BuildListPageData struct {
	User       *models.User
	Project    *models.Project
	Builds     []*models.Build
	Page       int
	TotalPages int
}

// BuildShowPageData holds data for the build detail page.
type BuildShowPageData struct {
	User    *models.User
	Project *models.Project
	Build   *models.Build
}

// buildNotifier sends notifications when builds reach terminal state.
type buildNotifier interface {
	NotifyBuild(ctx context.Context, build *models.Build, project *models.Project) error
}

// BuildHandler handles build-related HTTP requests.
type BuildHandler struct {
	projects     models.ProjectRepository
	builds       models.BuildRepository
	steps        models.BuildStepRepository
	projectUsers models.ProjectUserRepository
	notifier     buildNotifier
	templates    *templates.Engine
	logger       *slog.Logger
}

// NewBuildHandler creates a new BuildHandler.
func NewBuildHandler(
	projects models.ProjectRepository,
	builds models.BuildRepository,
	steps models.BuildStepRepository,
	projectUsers models.ProjectUserRepository,
	notifier buildNotifier,
	tmpl *templates.Engine,
	logger *slog.Logger,
) *BuildHandler {
	return &BuildHandler{
		projects:     projects,
		builds:       builds,
		steps:        steps,
		projectUsers: projectUsers,
		notifier:     notifier,
		templates:    tmpl,
		logger:       logger,
	}
}

// lookupProject finds a project by namespace/name, trying all providers.
func (h *BuildHandler) lookupProject(r *http.Request) (*models.Project, error) {
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

// List shows all builds for a project.
// GET /projects/{namespace}/{name}/builds
func (h *BuildHandler) List(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// Parse page number
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	offset := (page - 1) * buildsPerPage

	// Get total count for pagination
	total, err := h.builds.CountByProject(ctx, project.ID)
	if err != nil {
		h.logger.Error("failed to count builds", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	totalPages := (total + buildsPerPage - 1) / buildsPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	builds, err := h.builds.ListByProject(ctx, project.ID, buildsPerPage, offset)
	if err != nil {
		h.logger.Error("failed to list builds", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := BuildListPageData{
		User:       user,
		Project:    project,
		Builds:     builds,
		Page:       page,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/builds/list.html", data); err != nil {
		h.logger.Error("failed to render build list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Show displays a build's details.
// GET /projects/{namespace}/{name}/builds/{number}
func (h *BuildHandler) Show(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	numberStr := r.PathValue("number")
	buildNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		http.Error(w, "Bad Request: invalid build number", http.StatusBadRequest)
		return
	}

	build, err := h.builds.GetByNumber(ctx, project.ID, buildNumber)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get build", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Load steps
	steps, err := h.steps.ListByBuild(ctx, build.ID)
	if err != nil {
		h.logger.Error("failed to list build steps", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	build.Steps = steps

	data := BuildShowPageData{
		User:    user,
		Project: project,
		Build:   build,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, "pages/builds/show.html", data); err != nil {
		h.logger.Error("failed to render build show", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// StepsFragment returns just the build steps partial for HTMX polling.
// GET /projects/{namespace}/{name}/builds/{number}/steps
func (h *BuildHandler) StepsFragment(w http.ResponseWriter, r *http.Request) {
	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	numberStr := r.PathValue("number")
	buildNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	build, err := h.builds.GetByNumber(ctx, project.ID, buildNumber)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get build", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	steps, err := h.steps.ListByBuild(ctx, build.ID)
	if err != nil {
		h.logger.Error("failed to list build steps", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	build.Steps = steps

	data := BuildShowPageData{
		Build:   build,
		Project: project,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Stop polling if build is terminal
	if build.IsTerminal() {
		w.Header().Set("HX-Trigger", "buildComplete")
	}
	if err := h.templates.RenderComponent(w, "build-steps", data); err != nil {
		h.logger.Error("failed to render build steps fragment", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// OOB swap to keep the build header status badge and cancel button in sync
	if err := h.templates.RenderComponent(w, "build-header-status-oob", data); err != nil {
		h.logger.Error("failed to render build header status OOB", "error", err)
	}
}

// Cancel cancels a build and all its non-terminal steps.
// POST /projects/{namespace}/{name}/builds/{number}/cancel
func (h *BuildHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found: project not found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	numberStr := r.PathValue("number")
	buildNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		http.Error(w, "Bad Request: invalid build number", http.StatusBadRequest)
		return
	}

	// Look up build
	build, err := h.builds.GetByNumber(ctx, project.ID, buildNumber)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found: build not found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get build", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	buildURL := fmt.Sprintf("/projects/%s/%s/builds/%d", project.Namespace, project.Name, build.BuildNumber)

	// Check if already terminal
	if build.IsTerminal() {
		http.Redirect(w, r, buildURL, http.StatusSeeOther)
		return
	}

	// Cancel all non-terminal steps
	if _, err := h.steps.CancelBuildSteps(ctx, build.ID); err != nil {
		h.logger.Error("failed to cancel build steps", "build_id", build.ID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Cancel the build itself
	if err := h.builds.CancelBuild(ctx, build.ID); err != nil && !errors.Is(err, models.ErrNotFound) {
		h.logger.Error("failed to cancel build", "build_id", build.ID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("build cancelled", "build_id", build.ID, "build_number", build.BuildNumber)

	// Send cancellation notifications asynchronously
	if h.notifier != nil {
		cancelledBuild, err := h.builds.GetByID(ctx, build.ID)
		if err == nil {
			go func() {
				if err := h.notifier.NotifyBuild(context.Background(), cancelledBuild, project); err != nil {
					h.logger.Error("failed to send cancellation notification", "error", err)
				}
			}()
		}
	}

	http.Redirect(w, r, buildURL, http.StatusSeeOther)
}

const defaultLogLimit = 500

// StepLogResponse is the JSON response for the step log endpoint.
type StepLogResponse struct {
	Lines  []string `json:"lines"`
	Offset int      `json:"offset"`
	Total  int      `json:"total"`
	Done   bool     `json:"done"`
}

// StepLog returns log lines for a build step as JSON.
// GET /projects/{namespace}/{name}/builds/{number}/steps/{stepID}/log
func (h *BuildHandler) StepLog(w http.ResponseWriter, r *http.Request) {
	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	buildNumber, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "Bad Request: invalid build number", http.StatusBadRequest)
		return
	}

	build, err := h.builds.GetByNumber(ctx, project.ID, buildNumber)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get build", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	stepID, err := strconv.ParseInt(r.PathValue("stepID"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request: invalid step ID", http.StatusBadRequest)
		return
	}

	step, err := h.steps.GetByID(ctx, stepID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get step", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Verify step belongs to this build.
	if step.BuildID != build.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Parse offset query param.
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	resp := StepLogResponse{
		Lines:  []string{},
		Offset: offset,
		Total:  0,
		Done:   step.IsTerminal(),
	}

	// Read log lines if path exists.
	if step.LogPath != nil && *step.LogPath != "" {
		if _, err := os.Stat(*step.LogPath); err == nil {
			total, err := executor.CountLines(*step.LogPath)
			if err == nil {
				resp.Total = total
			}

			if offset < total {
				lines, err := executor.ReadLines(*step.LogPath, offset, defaultLogLimit)
				if err == nil {
					resp.Lines = lines
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ApproveStep approves a step that is waiting for manual approval.
// POST /projects/{namespace}/{name}/builds/{number}/steps/{stepID}/approve
func (h *BuildHandler) ApproveStep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	project, err := h.lookupProject(r)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get project", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Verify user has access to the project
	canAccess, err := h.projectUsers.CanUserAccess(ctx, project.ID, user.ID)
	if err != nil {
		h.logger.Error("failed to check user access", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !canAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	numberStr := r.PathValue("number")
	buildNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		http.Error(w, "Bad Request: invalid build number", http.StatusBadRequest)
		return
	}

	build, err := h.builds.GetByNumber(ctx, project.ID, buildNumber)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get build", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	stepID, err := strconv.ParseInt(r.PathValue("stepID"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request: invalid step ID", http.StatusBadRequest)
		return
	}

	step, err := h.steps.GetByID(ctx, stepID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get step", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Verify step belongs to this build
	if step.BuildID != build.ID {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Verify step is waiting for approval
	if step.Status != models.StepStatusWaitingApproval {
		http.Error(w, "Bad Request: step is not waiting for approval", http.StatusBadRequest)
		return
	}

	// Approve the step (transitions to ready)
	if err := h.steps.SetApproval(ctx, stepID, user.ID); err != nil {
		h.logger.Error("failed to approve step", "step_id", stepID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Unblock any dependents
	if _, err := h.steps.UpdateReadySteps(ctx, build.ID); err != nil {
		h.logger.Error("failed to update ready steps after approval", "build_id", build.ID, "error", err)
	}

	h.logger.Info("step approved", "step_id", stepID, "approved_by", user.ID)

	// Redirect back to build page
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	http.Redirect(w, r, fmt.Sprintf("/projects/%s/%s/builds/%d", namespace, name, buildNumber), http.StatusSeeOther)
}
