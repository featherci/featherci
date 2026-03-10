// Package webhook handles incoming webhooks from Git providers.
package webhook

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// Event represents a parsed webhook event from a Git provider.
type Event struct {
	// Provider is the source provider (github, gitlab, gitea).
	Provider string

	// EventType is the type of event (push, pull_request, merge_request).
	EventType string

	// FullName is the full repository name (owner/repo).
	FullName string

	// Ref is the full git ref (refs/heads/main, refs/tags/v1.0.0).
	Ref string

	// Branch is the extracted branch name (main, feature/foo).
	Branch string

	// Tag is the extracted tag name if this is a tag event.
	Tag string

	// CommitSHA is the commit hash.
	CommitSHA string

	// CommitMessage is the commit message.
	CommitMessage string

	// CommitAuthor is the commit author's name or username.
	CommitAuthor string

	// CloneURL is the URL to clone the repository.
	CloneURL string

	// Sender is the username who triggered the event.
	Sender string

	// PullRequest contains PR/MR details if this is a pull request event.
	PullRequest *PullRequestEvent
}

// PullRequestEvent contains pull request or merge request details.
type PullRequestEvent struct {
	// Number is the PR/MR number.
	Number int

	// Action is the PR action (opened, synchronize, closed, reopened, merged).
	Action string

	// Title is the PR title.
	Title string

	// SourceBranch is the source branch of the PR.
	SourceBranch string

	// TargetBranch is the target branch of the PR.
	TargetBranch string

	// Draft indicates if the PR is a draft.
	Draft bool
}

// Handler defines the interface for webhook handlers.
type Handler interface {
	// ValidateSignature validates the webhook signature.
	// Returns nil if valid, error otherwise.
	ValidateSignature(r *http.Request, secret string) error

	// ParseEvent parses the webhook payload into an Event.
	ParseEvent(r *http.Request) (*Event, error)
}

// IsPush returns true if this is a push event.
func (e *Event) IsPush() bool {
	return e.EventType == "push"
}

// IsPullRequest returns true if this is a pull request event.
func (e *Event) IsPullRequest() bool {
	return e.EventType == "pull_request" || e.EventType == "merge_request"
}

// IsTag returns true if this is a tag push event.
func (e *Event) IsTag() bool {
	return e.Tag != ""
}

// ShouldTriggerBuild returns true if this event should trigger a build.
func (e *Event) ShouldTriggerBuild() bool {
	if e.IsPush() {
		// Always build pushes (branch or tag)
		return true
	}

	if e.IsPullRequest() && e.PullRequest != nil {
		// Build on PR open, sync (new commits), or reopen
		switch e.PullRequest.Action {
		case "opened", "synchronize", "reopened":
			return true
		}
	}

	return false
}

// ExtractRepoFullName extracts the repository full name from a webhook payload
// without fully parsing it. This allows looking up the project before signature validation.
// It returns the full name and restores the request body for later use.
func ExtractRepoFullName(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	// Restore body for later use
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Parse just enough to get the repository name
	var payload struct {
		Repository struct {
			FullName          string `json:"full_name"`           // GitHub, Gitea
			PathWithNamespace string `json:"path_with_namespace"` // GitLab
		} `json:"repository"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"` // GitLab (merge request)
		} `json:"project"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	// Try different fields based on provider payload format
	if payload.Repository.FullName != "" {
		return payload.Repository.FullName, nil
	}
	if payload.Repository.PathWithNamespace != "" {
		return payload.Repository.PathWithNamespace, nil
	}
	if payload.Project.PathWithNamespace != "" {
		return payload.Project.PathWithNamespace, nil
	}

	return "", nil
}
