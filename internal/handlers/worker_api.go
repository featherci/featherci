package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/services"
)

// WorkerAPIHandler handles HTTP endpoints for remote workers.
type WorkerAPIHandler struct {
	steps    models.BuildStepRepository
	builds   models.BuildRepository
	projects models.ProjectRepository
	workers  models.WorkerRepository
	secrets  secretDecrypter
	tokens   tokenProvider
	status   stepStatusPoster
	advancer *services.BuildAdvancer
	logDir   string
	logger   *slog.Logger
}

// secretDecrypter provides decrypted secrets for builds.
type secretDecrypter interface {
	GetDecryptedSecrets(ctx context.Context, projectID int64) (map[string]string, error)
}

// tokenProvider provides git access tokens.
type tokenProvider interface {
	TokenForProject(ctx context.Context, projectID int64) (string, error)
}

// stepStatusPoster posts commit statuses for steps.
type stepStatusPoster interface {
	PostBuildStatus(ctx context.Context, project *models.Project, build *models.Build)
	PostStepStatus(ctx context.Context, project *models.Project, build *models.Build, stepName string, stepStatus models.StepStatus)
}

// NewWorkerAPIHandler creates a new WorkerAPIHandler.
func NewWorkerAPIHandler(
	steps models.BuildStepRepository,
	builds models.BuildRepository,
	projects models.ProjectRepository,
	workers models.WorkerRepository,
	secrets secretDecrypter,
	tokens tokenProvider,
	status stepStatusPoster,
	advancer *services.BuildAdvancer,
	logDir string,
	logger *slog.Logger,
) *WorkerAPIHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WorkerAPIHandler{
		steps:    steps,
		builds:   builds,
		projects: projects,
		workers:  workers,
		secrets:  secrets,
		tokens:   tokens,
		status:   status,
		advancer: advancer,
		logDir:   logDir,
		logger:   logger,
	}
}

// --- JSON request/response types ---

type stepReadyResponse struct {
	Steps []stepJSON `json:"steps"`
}

type stepJSON struct {
	ID               int64             `json:"id"`
	BuildID          int64             `json:"build_id"`
	Name             string            `json:"name"`
	Image            *string           `json:"image"`
	Status           string            `json:"status"`
	Commands         []string          `json:"commands"`
	Env              map[string]string `json:"env"`
	DependsOn        []string          `json:"depends_on"`
	Cache            *models.CacheConfig    `json:"cache,omitempty"`
	CacheResolvedKey string                 `json:"cache_resolved_key,omitempty"`
	Services         []models.ServiceConfig `json:"services,omitempty"`
	WorkingDir       string                 `json:"working_dir"`
	TimeoutMinutes   int                    `json:"timeout_minutes"`
	RequiresApproval bool                   `json:"requires_approval"`
	ConditionExpr    string                 `json:"condition_expr,omitempty"`
}

func stepToJSON(s *models.BuildStep) stepJSON {
	return stepJSON{
		ID:               s.ID,
		BuildID:          s.BuildID,
		Name:             s.Name,
		Image:            s.Image,
		Status:           string(s.Status),
		Commands:         s.Commands,
		Env:              s.Env,
		DependsOn:        s.DependsOn,
		Cache:            s.Cache,
		CacheResolvedKey: s.CacheResolvedKey,
		Services:         s.Services,
		WorkingDir:       s.WorkingDir,
		TimeoutMinutes:   s.TimeoutMinutes,
		RequiresApproval: s.RequiresApproval,
		ConditionExpr:    s.ConditionExpr,
	}
}

type buildJSON struct {
	ID                int64   `json:"id"`
	ProjectID         int64   `json:"project_id"`
	BuildNumber       int     `json:"build_number"`
	CommitSHA         string  `json:"commit_sha"`
	CommitMessage     *string `json:"commit_message"`
	CommitAuthor      *string `json:"commit_author"`
	Branch            *string `json:"branch"`
	PullRequestNumber *int    `json:"pull_request_number"`
	Status            string  `json:"status"`
}

func buildToJSON(b *models.Build) buildJSON {
	return buildJSON{
		ID:                b.ID,
		ProjectID:         b.ProjectID,
		BuildNumber:       b.BuildNumber,
		CommitSHA:         b.CommitSHA,
		CommitMessage:     b.CommitMessage,
		CommitAuthor:      b.CommitAuthor,
		Branch:            b.Branch,
		PullRequestNumber: b.PullRequestNumber,
		Status:            string(b.Status),
	}
}

type projectJSON struct {
	ID            int64  `json:"id"`
	Provider      string `json:"provider"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
}

func projectToJSON(p *models.Project) projectJSON {
	return projectJSON{
		ID:            p.ID,
		Provider:      p.Provider,
		Namespace:     p.Namespace,
		Name:          p.Name,
		FullName:      p.FullName,
		CloneURL:      p.CloneURL,
		DefaultBranch: p.DefaultBranch,
	}
}

type claimRequest struct {
	WorkerID string `json:"worker_id"`
}

type completeRequest struct {
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code"`
	LogPath  string `json:"log_path"`
}

type registerRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type heartbeatRequest struct {
	ID string `json:"id"`
}

type statusRequest struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	CurrentStepID *int64 `json:"current_step_id"`
}

type offlineRequest struct {
	ID string `json:"id"`
}

type buildStartedRequest struct {
	// empty — just POST to signal
}

// --- Handlers ---

// ListReadySteps returns steps that are ready to be executed.
// GET /api/worker/steps/ready
func (h *WorkerAPIHandler) ListReadySteps(w http.ResponseWriter, r *http.Request) {
	steps, err := h.steps.ListReady(r.Context())
	if err != nil {
		h.jsonError(w, "failed to list ready steps", http.StatusInternalServerError)
		return
	}
	resp := stepReadyResponse{Steps: make([]stepJSON, 0, len(steps))}
	for _, s := range steps {
		resp.Steps = append(resp.Steps, stepToJSON(s))
	}
	h.jsonOK(w, resp)
}

// ClaimStep marks a step as started by a worker.
// POST /api/worker/steps/{id}/claim
func (h *WorkerAPIHandler) ClaimStep(w http.ResponseWriter, r *http.Request) {
	stepID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid step ID", http.StatusBadRequest)
		return
	}
	var req claimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		h.jsonError(w, "worker_id is required", http.StatusBadRequest)
		return
	}
	if err := h.steps.SetStarted(r.Context(), stepID, req.WorkerID); err != nil {
		if err == models.ErrNotFound {
			h.jsonError(w, "step not found or already claimed", http.StatusConflict)
			return
		}
		h.jsonError(w, "failed to claim step", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"status": "claimed"})
}

// CompleteStep records step completion and triggers build advancement.
// POST /api/worker/steps/{id}/complete
func (h *WorkerAPIHandler) CompleteStep(w http.ResponseWriter, r *http.Request) {
	stepID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid step ID", http.StatusBadRequest)
		return
	}
	var req completeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	stepStatus := models.StepStatus(req.Status)
	if err := h.steps.SetFinished(r.Context(), stepID, stepStatus, req.ExitCode, req.LogPath); err != nil {
		h.jsonError(w, "failed to complete step", http.StatusInternalServerError)
		return
	}

	// Load step to get build ID and post step status
	step, err := h.steps.GetByID(r.Context(), stepID)
	if err == nil {
		// Post step commit status
		build, berr := h.builds.GetByID(r.Context(), step.BuildID)
		if berr == nil {
			project, perr := h.projects.GetByID(r.Context(), build.ProjectID)
			if perr == nil {
				go h.status.PostStepStatus(context.Background(), project, build, step.Name, stepStatus)
			}
		}

		// Advance build state (skip dependents, unblock ready, recalc status, notify)
		if err := h.advancer.AdvanceBuild(r.Context(), step.BuildID); err != nil {
			h.logger.Error("build advancement failed", "build_id", step.BuildID, "error", err)
		}
	}

	h.jsonOK(w, map[string]string{"status": "completed"})
}

// UploadLog receives a log file upload for a step.
// POST /api/worker/steps/{id}/log
func (h *WorkerAPIHandler) UploadLog(w http.ResponseWriter, r *http.Request) {
	stepID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid step ID", http.StatusBadRequest)
		return
	}

	// Create log directory
	logPath := filepath.Join(h.logDir, strconv.FormatInt(stepID, 10)+".log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		h.jsonError(w, "failed to create log directory", http.StatusInternalServerError)
		return
	}

	f, err := os.Create(logPath)
	if err != nil {
		h.jsonError(w, "failed to create log file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, r.Body); err != nil {
		h.jsonError(w, "failed to write log", http.StatusInternalServerError)
		return
	}

	// Update the step's log path
	step, err := h.steps.GetByID(r.Context(), stepID)
	if err == nil {
		step.LogPath = &logPath
		_ = h.steps.Update(r.Context(), step)
	}

	h.jsonOK(w, map[string]string{"status": "uploaded", "path": logPath})
}

// GetBuild returns build metadata.
// GET /api/worker/builds/{id}
func (h *WorkerAPIHandler) GetBuild(w http.ResponseWriter, r *http.Request) {
	buildID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid build ID", http.StatusBadRequest)
		return
	}
	build, err := h.builds.GetByID(r.Context(), buildID)
	if err != nil {
		h.jsonError(w, "build not found", http.StatusNotFound)
		return
	}
	h.jsonOK(w, buildToJSON(build))
}

// ListBuildSteps returns all steps for a build.
// GET /api/worker/builds/{id}/steps
func (h *WorkerAPIHandler) ListBuildSteps(w http.ResponseWriter, r *http.Request) {
	buildID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid build ID", http.StatusBadRequest)
		return
	}
	steps, err := h.steps.ListByBuild(r.Context(), buildID)
	if err != nil {
		h.jsonError(w, "failed to list steps", http.StatusInternalServerError)
		return
	}
	result := make([]stepJSON, 0, len(steps))
	for _, s := range steps {
		result = append(result, stepToJSON(s))
	}
	h.jsonOK(w, map[string]any{"steps": result})
}

// BuildStarted marks a build as started and posts initial pending statuses.
// POST /api/worker/builds/{id}/started
func (h *WorkerAPIHandler) BuildStarted(w http.ResponseWriter, r *http.Request) {
	buildID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid build ID", http.StatusBadRequest)
		return
	}
	if err := h.builds.SetStarted(r.Context(), buildID); err != nil {
		h.jsonError(w, "failed to start build", http.StatusInternalServerError)
		return
	}

	// Post pending status for all steps
	build, err := h.builds.GetByID(r.Context(), buildID)
	if err == nil {
		project, perr := h.projects.GetByID(r.Context(), build.ProjectID)
		if perr == nil {
			allSteps, serr := h.steps.ListByBuild(r.Context(), buildID)
			if serr == nil {
				for _, s := range allSteps {
					go h.status.PostStepStatus(context.Background(), project, build, s.Name, models.StepStatusPending)
				}
			}
		}
	}

	h.jsonOK(w, map[string]string{"status": "started"})
}

// GetProject returns project metadata.
// GET /api/worker/projects/{id}
func (h *WorkerAPIHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid project ID", http.StatusBadRequest)
		return
	}
	project, err := h.projects.GetByID(r.Context(), projectID)
	if err != nil {
		h.jsonError(w, "project not found", http.StatusNotFound)
		return
	}
	h.jsonOK(w, projectToJSON(project))
}

// GetProjectSecrets returns decrypted secrets for a project.
// GET /api/worker/projects/{id}/secrets
func (h *WorkerAPIHandler) GetProjectSecrets(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid project ID", http.StatusBadRequest)
		return
	}
	secrets, err := h.secrets.GetDecryptedSecrets(r.Context(), projectID)
	if err != nil {
		h.jsonError(w, "failed to get secrets", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]any{"secrets": secrets})
}

// GetProjectToken returns a clone token for a project.
// GET /api/worker/projects/{id}/token
func (h *WorkerAPIHandler) GetProjectToken(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.pathInt64(r, "id")
	if err != nil {
		h.jsonError(w, "invalid project ID", http.StatusBadRequest)
		return
	}
	token, err := h.tokens.TokenForProject(r.Context(), projectID)
	if err != nil {
		h.jsonError(w, "failed to get token", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"token": token})
}

// Register registers a new worker.
// POST /api/worker/register
func (h *WorkerAPIHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		h.jsonError(w, "id is required", http.StatusBadRequest)
		return
	}
	worker := &models.Worker{
		ID:     req.ID,
		Name:   req.Name,
		Status: models.WorkerStatusIdle,
	}
	if worker.Name == "" {
		worker.Name = worker.ID
	}
	if err := h.workers.Register(r.Context(), worker); err != nil {
		h.jsonError(w, "failed to register worker", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"status": "registered"})
}

// Heartbeat updates a worker's heartbeat timestamp.
// POST /api/worker/heartbeat
func (h *WorkerAPIHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		h.jsonError(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.workers.UpdateHeartbeat(r.Context(), req.ID); err != nil {
		h.jsonError(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"status": "ok"})
}

// UpdateStatus updates a worker's status.
// POST /api/worker/status
func (h *WorkerAPIHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		h.jsonError(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.workers.UpdateStatus(r.Context(), req.ID, models.WorkerStatus(req.Status), req.CurrentStepID); err != nil {
		h.jsonError(w, "failed to update status", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"status": "ok"})
}

// SetOffline marks a worker as offline.
// POST /api/worker/offline
func (h *WorkerAPIHandler) SetOffline(w http.ResponseWriter, r *http.Request) {
	var req offlineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		h.jsonError(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.workers.SetOffline(r.Context(), req.ID); err != nil {
		h.jsonError(w, "failed to set offline", http.StatusInternalServerError)
		return
	}
	h.jsonOK(w, map[string]string{"status": "ok"})
}

// --- Helpers ---

func (h *WorkerAPIHandler) pathInt64(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

func (h *WorkerAPIHandler) jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *WorkerAPIHandler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
