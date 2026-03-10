package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GitLabHandler handles GitLab webhooks.
type GitLabHandler struct{}

// NewGitLabHandler creates a new GitLab webhook handler.
func NewGitLabHandler() *GitLabHandler {
	return &GitLabHandler{}
}

// ValidateSignature validates the GitLab webhook signature.
// GitLab uses X-Gitlab-Token header with plain text secret comparison.
func (h *GitLabHandler) ValidateSignature(r *http.Request, secret string) error {
	token := r.Header.Get("X-Gitlab-Token")
	if token == "" {
		return errors.New("missing X-Gitlab-Token header")
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return errors.New("token mismatch")
	}

	return nil
}

// ParseEvent parses a GitLab webhook payload.
func (h *GitLabHandler) ParseEvent(r *http.Request) (*Event, error) {
	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType == "" {
		return nil, errors.New("missing X-Gitlab-Event header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	switch eventType {
	case "Push Hook":
		return h.parsePushEvent(body)
	case "Merge Request Hook":
		return h.parseMergeRequestEvent(body)
	case "Tag Push Hook":
		return h.parseTagPushEvent(body)
	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// gitlabPushPayload represents a GitLab push event payload.
type gitlabPushPayload struct {
	ObjectKind   string `json:"object_kind"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Ref          string `json:"ref"`
	CheckoutSHA  string `json:"checkout_sha"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`
	Project      struct {
		PathWithNamespace string `json:"path_with_namespace"`
		GitHTTPURL        string `json:"git_http_url"`
		GitSSHURL         string `json:"git_ssh_url"`
	} `json:"project"`
	Commits []struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
		Author    struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
	TotalCommitsCount int `json:"total_commits_count"`
}

func (h *GitLabHandler) parsePushEvent(body []byte) (*Event, error) {
	var payload gitlabPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse push payload: %w", err)
	}

	// Skip if no commits (branch deleted)
	if payload.TotalCommitsCount == 0 && payload.After == "0000000000000000000000000000000000000000" {
		return nil, errors.New("branch deleted, skipping")
	}

	event := &Event{
		Provider:  "gitlab",
		EventType: "push",
		FullName:  payload.Project.PathWithNamespace,
		Ref:       payload.Ref,
		CommitSHA: payload.After,
		CloneURL:  payload.Project.GitHTTPURL,
		Sender:    payload.UserUsername,
	}

	// Extract branch name from ref
	if strings.HasPrefix(payload.Ref, "refs/heads/") {
		event.Branch = strings.TrimPrefix(payload.Ref, "refs/heads/")
	}

	// Get commit details from last commit
	if len(payload.Commits) > 0 {
		lastCommit := payload.Commits[len(payload.Commits)-1]
		event.CommitMessage = lastCommit.Message
		event.CommitAuthor = lastCommit.Author.Name
	}

	return event, nil
}

// gitlabTagPushPayload represents a GitLab tag push event payload.
type gitlabTagPushPayload struct {
	ObjectKind   string `json:"object_kind"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Ref          string `json:"ref"`
	CheckoutSHA  string `json:"checkout_sha"`
	UserName     string `json:"user_name"`
	UserUsername string `json:"user_username"`
	Project      struct {
		PathWithNamespace string `json:"path_with_namespace"`
		GitHTTPURL        string `json:"git_http_url"`
	} `json:"project"`
	Message string `json:"message"`
}

func (h *GitLabHandler) parseTagPushEvent(body []byte) (*Event, error) {
	var payload gitlabTagPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse tag push payload: %w", err)
	}

	// Skip tag deletion
	if payload.After == "0000000000000000000000000000000000000000" {
		return nil, errors.New("tag deleted, skipping")
	}

	event := &Event{
		Provider:      "gitlab",
		EventType:     "push",
		FullName:      payload.Project.PathWithNamespace,
		Ref:           payload.Ref,
		CommitSHA:     payload.CheckoutSHA,
		CloneURL:      payload.Project.GitHTTPURL,
		Sender:        payload.UserUsername,
		CommitMessage: payload.Message,
		CommitAuthor:  payload.UserName,
	}

	// Extract tag name from ref
	if strings.HasPrefix(payload.Ref, "refs/tags/") {
		event.Tag = strings.TrimPrefix(payload.Ref, "refs/tags/")
	}

	return event, nil
}

// gitlabMergeRequestPayload represents a GitLab merge request event payload.
type gitlabMergeRequestPayload struct {
	ObjectKind string `json:"object_kind"`
	User       struct {
		Username string `json:"username"`
	} `json:"user"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
		GitHTTPURL        string `json:"git_http_url"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		State        string `json:"state"`
		Action       string `json:"action"`
		LastCommit   struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
		} `json:"last_commit"`
		WorkInProgress bool `json:"work_in_progress"`
		Draft          bool `json:"draft"`
	} `json:"object_attributes"`
}

func (h *GitLabHandler) parseMergeRequestEvent(body []byte) (*Event, error) {
	var payload gitlabMergeRequestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse merge_request payload: %w", err)
	}

	// Map GitLab actions to our standard actions
	action := payload.ObjectAttributes.Action
	switch action {
	case "open":
		action = "opened"
	case "update":
		action = "synchronize"
	case "close":
		action = "closed"
	case "reopen":
		action = "reopened"
	case "merge":
		action = "merged"
	}

	event := &Event{
		Provider:      "gitlab",
		EventType:     "merge_request",
		FullName:      payload.Project.PathWithNamespace,
		Ref:           fmt.Sprintf("refs/merge-requests/%d/head", payload.ObjectAttributes.IID),
		Branch:        payload.ObjectAttributes.SourceBranch,
		CommitSHA:     payload.ObjectAttributes.LastCommit.ID,
		CommitMessage: payload.ObjectAttributes.LastCommit.Message,
		CommitAuthor:  payload.ObjectAttributes.LastCommit.Author.Name,
		CloneURL:      payload.Project.GitHTTPURL,
		Sender:        payload.User.Username,
		PullRequest: &PullRequestEvent{
			Number:       payload.ObjectAttributes.IID,
			Action:       action,
			Title:        payload.ObjectAttributes.Title,
			SourceBranch: payload.ObjectAttributes.SourceBranch,
			TargetBranch: payload.ObjectAttributes.TargetBranch,
			Draft:        payload.ObjectAttributes.Draft || payload.ObjectAttributes.WorkInProgress,
		},
	}

	return event, nil
}
