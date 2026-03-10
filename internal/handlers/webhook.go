package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/webhook"
)

// WebhookHandler handles incoming webhook requests from Git providers.
type WebhookHandler struct {
	projects models.ProjectRepository
	github   webhook.Handler
	gitlab   webhook.Handler
	gitea    webhook.Handler
	logger   *slog.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(
	projects models.ProjectRepository,
	logger *slog.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		projects: projects,
		github:   webhook.NewGitHubHandler(),
		gitlab:   webhook.NewGitLabHandler(),
		gitea:    webhook.NewGiteaHandler(),
		logger:   logger,
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

	// TODO: Create build and queue for execution
	// This will be implemented when the Build model and worker system are ready
	// For now, we just acknowledge the webhook

	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"status":     "accepted",
		"message":    "build will be triggered",
		"project_id": project.ID,
		"event_type": event.EventType,
		"commit_sha": event.CommitSHA,
	}

	if event.Branch != "" {
		response["branch"] = event.Branch
	}
	if event.Tag != "" {
		response["tag"] = event.Tag
	}
	if event.PullRequest != nil {
		response["pull_request"] = event.PullRequest.Number
	}

	h.writeJSON(w, response)
}

// writeJSON writes a JSON response.
func (h *WebhookHandler) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to write JSON response", "error", err)
	}
}
