package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/webhook"
	"github.com/featherci/featherci/internal/workflow"
)

// buildCreator creates builds from webhook events and parsed workflows.
type buildCreator interface {
	CreateBuildFromWebhook(ctx context.Context, project *models.Project, event *webhook.Event, wf *workflow.Workflow) (*models.Build, error)
}

// fileFetcher fetches file content from git provider APIs.
type fileFetcher interface {
	GetFileContent(ctx context.Context, provider, token, repoFullName, filePath, ref string) ([]byte, error)
}

// webhookTokenSource provides git access tokens for fetching workflow files.
type webhookTokenSource interface {
	TokenForProject(ctx context.Context, projectID int64) (string, error)
}

// workflowParser parses and validates workflow YAML.
type workflowParser interface {
	ParseAndValidate(content []byte) (*workflow.Workflow, error)
}

// statusPoster posts commit statuses to git providers.
type statusPoster interface {
	PostBuildStatus(ctx context.Context, project *models.Project, build *models.Build)
}

// WebhookHandler handles incoming webhook requests from Git providers.
type WebhookHandler struct {
	projects      models.ProjectRepository
	github        webhook.Handler
	gitlab        webhook.Handler
	gitea         webhook.Handler
	logger        *slog.Logger
	buildCreator  buildCreator
	fileFetcher   fileFetcher
	tokenSource   webhookTokenSource
	parser        workflowParser
	statusService statusPoster
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(
	projects models.ProjectRepository,
	logger *slog.Logger,
	bc buildCreator,
	ff fileFetcher,
	ts webhookTokenSource,
	p workflowParser,
	ss statusPoster,
) *WebhookHandler {
	return &WebhookHandler{
		projects:      projects,
		github:        webhook.NewGitHubHandler(),
		gitlab:        webhook.NewGitLabHandler(),
		gitea:         webhook.NewGiteaHandler(),
		logger:        logger,
		buildCreator:  bc,
		fileFetcher:   ff,
		tokenSource:   ts,
		parser:        p,
		statusService: ss,
	}
}

// HandleGitHub handles webhooks from GitHub.
// POST /webhooks/github
func (h *WebhookHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, "github", h.github)
}

// HandleGitLab handles webhooks from GitLab.
// POST /webhooks/gitlab
func (h *WebhookHandler) HandleGitLab(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, "gitlab", h.gitlab)
}

// HandleGitea handles webhooks from Gitea/Forgejo.
// POST /webhooks/gitea
func (h *WebhookHandler) HandleGitea(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, "gitea", h.gitea)
}

// handleWebhook processes a webhook from any provider.
func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request, provider string, handler webhook.Handler) {
	ctx := r.Context()

	// Extract repository full name from payload to look up project
	fullName, err := webhook.ExtractRepoFullName(r)
	if err != nil {
		h.logger.Warn("failed to extract repository name from webhook",
			"provider", provider,
			"error", err,
		)
		http.Error(w, "Bad Request: could not parse repository name", http.StatusBadRequest)
		return
	}

	if fullName == "" {
		h.logger.Warn("no repository name in webhook payload",
			"provider", provider,
		)
		http.Error(w, "Bad Request: no repository name found", http.StatusBadRequest)
		return
	}

	// Look up the project to get the webhook secret
	project, err := h.projects.GetByFullName(ctx, provider, fullName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			h.logger.Debug("webhook received for unknown project",
				"provider", provider,
				"full_name", fullName,
			)
			// Return 200 OK to prevent retries for unknown projects
			w.WriteHeader(http.StatusOK)
			h.writeJSON(w, map[string]string{
				"status":  "ignored",
				"message": "project not found",
			})
			return
		}
		h.logger.Error("failed to get project",
			"provider", provider,
			"full_name", fullName,
			"error", err,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Validate the signature with the project's webhook secret
	if err := handler.ValidateSignature(r, project.WebhookSecret); err != nil {
		h.logger.Warn("webhook signature validation failed",
			"provider", provider,
			"project_id", project.ID,
			"full_name", project.FullName,
			"error", err,
		)
		http.Error(w, "Unauthorized: invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse the full event
	event, err := handler.ParseEvent(r)
	if err != nil {
		h.logger.Warn("failed to parse webhook event",
			"provider", provider,
			"project_id", project.ID,
			"error", err,
		)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Handle ping events (webhook setup verification)
	if event.EventType == "ping" {
		h.logger.Info("webhook ping received",
			"provider", provider,
			"project_id", project.ID,
			"full_name", project.FullName,
		)
		w.WriteHeader(http.StatusOK)
		h.writeJSON(w, map[string]string{
			"status":  "ok",
			"message": "pong",
		})
		return
	}

	// Check if this event should trigger a build
	if !event.ShouldTriggerBuild() {
		h.logger.Debug("webhook event does not trigger build",
			"provider", provider,
			"project_id", project.ID,
			"event_type", event.EventType,
		)
		w.WriteHeader(http.StatusOK)
		h.writeJSON(w, map[string]string{
			"status":  "ignored",
			"message": "event does not trigger build",
		})
		return
	}

	// Log the event that would trigger a build
	h.logger.Info("webhook received - build trigger",
		"provider", provider,
		"project_id", project.ID,
		"full_name", project.FullName,
		"event_type", event.EventType,
		"branch", event.Branch,
		"tag", event.Tag,
		"commit_sha", event.CommitSHA,
		"sender", event.Sender,
	)

	// Get token for fetching workflow file
	token, err := h.tokenSource.TokenForProject(ctx, project.ID)
	if err != nil {
		h.logger.Error("failed to get token for project",
			"project_id", project.ID,
			"error", err,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Fetch workflow file from repository
	content, err := h.fileFetcher.GetFileContent(ctx, provider, token, project.FullName, workflow.DefaultWorkflowPath, event.CommitSHA)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.logger.Debug("no workflow file found",
				"project_id", project.ID,
				"commit_sha", event.CommitSHA,
			)
			w.WriteHeader(http.StatusOK)
			h.writeJSON(w, map[string]string{
				"status":  "ignored",
				"message": "no workflow file",
			})
			return
		}
		h.logger.Error("failed to fetch workflow file",
			"project_id", project.ID,
			"error", err,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Parse and validate workflow
	wf, err := h.parser.ParseAndValidate(content)
	if err != nil {
		h.logger.Warn("invalid workflow file",
			"project_id", project.ID,
			"error", err,
		)
		w.WriteHeader(http.StatusOK)
		h.writeJSON(w, map[string]string{
			"status":  "ignored",
			"message": "invalid workflow: " + err.Error(),
		})
		return
	}

	// Create build
	build, err := h.buildCreator.CreateBuildFromWebhook(ctx, project, event, wf)
	if err != nil {
		h.logger.Error("failed to create build",
			"project_id", project.ID,
			"error", err,
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("build created from webhook",
		"project_id", project.ID,
		"build_id", build.ID,
		"build_number", build.BuildNumber,
	)

	w.WriteHeader(http.StatusOK)
	h.writeJSON(w, map[string]interface{}{
		"status":       "accepted",
		"message":      "build created",
		"build_id":     build.ID,
		"build_number": build.BuildNumber,
	})
}

// writeJSON writes a JSON response.
func (h *WebhookHandler) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", "error", err)
	}
}
