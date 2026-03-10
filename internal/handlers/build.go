package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/featherci/featherci/internal/models"
)

// BuildHandler handles build-related HTTP requests.
type BuildHandler struct {
	projects models.ProjectRepository
	builds   models.BuildRepository
	steps    models.BuildStepRepository
	logger   *slog.Logger
}

// NewBuildHandler creates a new BuildHandler.
func NewBuildHandler(
	projects models.ProjectRepository,
	builds models.BuildRepository,
	steps models.BuildStepRepository,
	logger *slog.Logger,
) *BuildHandler {
	return &BuildHandler{
		projects: projects,
		builds:   builds,
		steps:    steps,
		logger:   logger,
	}
}

// Cancel cancels a build and all its non-terminal steps.
// POST /projects/{namespace}/{name}/builds/{number}/cancel
func (h *BuildHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	numberStr := r.PathValue("number")

	buildNumber, err := strconv.Atoi(numberStr)
	if err != nil {
		http.Error(w, "Bad Request: invalid build number", http.StatusBadRequest)
		return
	}

	// Look up project by namespace/name — try all providers
	fullName := namespace + "/" + name
	var project *models.Project
	for _, provider := range []string{"github", "gitlab", "gitea"} {
		project, err = h.projects.GetByFullName(ctx, provider, fullName)
		if err == nil {
			break
		}
		if !errors.Is(err, models.ErrNotFound) {
			h.logger.Error("failed to get project", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	if project == nil {
		http.Error(w, "Not Found: project not found", http.StatusNotFound)
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

	// Check if already terminal
	if build.IsTerminal() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ignored",
			"message": "build already in terminal state",
		})
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "cancelled",
		"build_id":     build.ID,
		"build_number": build.BuildNumber,
	})
}
