package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GiteaHandler handles Gitea/Forgejo webhooks.
type GiteaHandler struct{}

// NewGiteaHandler creates a new Gitea webhook handler.
func NewGiteaHandler() *GiteaHandler {
	return &GiteaHandler{}
}

// ValidateSignature validates the Gitea webhook signature.
// Gitea uses X-Gitea-Signature header with HMAC-SHA256.
func (h *GiteaHandler) ValidateSignature(r *http.Request, secret string) error {
	signature := r.Header.Get("X-Gitea-Signature")
	if signature == "" {
		return errors.New("missing X-Gitea-Signature header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New("signature mismatch")
	}

	// Restore body for parsing
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	return nil
}

// ParseEvent parses a Gitea webhook payload.
func (h *GiteaHandler) ParseEvent(r *http.Request) (*Event, error) {
	eventType := r.Header.Get("X-Gitea-Event")
	if eventType == "" {
		return nil, errors.New("missing X-Gitea-Event header")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	switch eventType {
	case "push":
		return h.parsePushEvent(body)
	case "pull_request":
		return h.parsePullRequestEvent(body)
	case "create":
		return h.parseCreateEvent(body)
	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// giteaPushPayload represents a Gitea push event payload.
type giteaPushPayload struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	CompareURL string `json:"compare_url"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Pusher struct {
		Login    string `json:"login"`
		Username string `json:"username"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	} `json:"pusher"`
	Sender struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"sender"`
	Commits []struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
		Author    struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
	} `json:"commits"`
	HeadCommit *struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
		Author    struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Username string `json:"username"`
		} `json:"author"`
	} `json:"head_commit"`
	TotalCommits int `json:"total_commits"`
}

func (h *GiteaHandler) parsePushEvent(body []byte) (*Event, error) {
	var payload giteaPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse push payload: %w", err)
	}

	// Skip deleted branch events (after is all zeros)
	if payload.After == "0000000000000000000000000000000000000000" {
		return nil, errors.New("branch deleted, skipping")
	}

	event := &Event{
		Provider:  "gitea",
		EventType: "push",
		FullName:  payload.Repository.FullName,
		Ref:       payload.Ref,
		CommitSHA: payload.After,
		CloneURL:  payload.Repository.CloneURL,
		Sender:    payload.Sender.Login,
	}

	// Fallback for sender
	if event.Sender == "" {
		event.Sender = payload.Sender.Username
	}
	if event.Sender == "" {
		event.Sender = payload.Pusher.Login
	}

	// Extract branch or tag name from ref
	if strings.HasPrefix(payload.Ref, "refs/heads/") {
		event.Branch = strings.TrimPrefix(payload.Ref, "refs/heads/")
	} else if strings.HasPrefix(payload.Ref, "refs/tags/") {
		event.Tag = strings.TrimPrefix(payload.Ref, "refs/tags/")
	}

	// Get commit details from head_commit or last commit
	if payload.HeadCommit != nil {
		event.CommitMessage = payload.HeadCommit.Message
		event.CommitAuthor = payload.HeadCommit.Author.Name
		if event.CommitAuthor == "" {
			event.CommitAuthor = payload.HeadCommit.Author.Username
		}
	} else if len(payload.Commits) > 0 {
		lastCommit := payload.Commits[len(payload.Commits)-1]
		event.CommitMessage = lastCommit.Message
		event.CommitAuthor = lastCommit.Author.Name
		if event.CommitAuthor == "" {
			event.CommitAuthor = lastCommit.Author.Username
		}
	}

	return event, nil
}

// giteaPullRequestPayload represents a Gitea pull_request event payload.
type giteaPullRequestPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		ID      int    `json:"id"`
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
		Head    struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Sender struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"sender"`
}

func (h *GiteaHandler) parsePullRequestEvent(body []byte) (*Event, error) {
	var payload giteaPullRequestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse pull_request payload: %w", err)
	}

	// Map Gitea actions to our standard actions
	action := payload.Action
	switch action {
	case "opened":
		// Already correct
	case "synchronized", "synced":
		action = "synchronize"
	case "closed":
		// Already correct
	case "reopened":
		// Already correct
	}

	sender := payload.Sender.Login
	if sender == "" {
		sender = payload.Sender.Username
	}

	event := &Event{
		Provider:  "gitea",
		EventType: "pull_request",
		FullName:  payload.Repository.FullName,
		Ref:       fmt.Sprintf("refs/pull/%d/head", payload.PullRequest.Number),
		Branch:    payload.PullRequest.Head.Ref,
		CommitSHA: payload.PullRequest.Head.SHA,
		CloneURL:  payload.Repository.CloneURL,
		Sender:    sender,
		PullRequest: &PullRequestEvent{
			Number:       payload.PullRequest.Number,
			Action:       action,
			Title:        payload.PullRequest.Title,
			SourceBranch: payload.PullRequest.Head.Ref,
			TargetBranch: payload.PullRequest.Base.Ref,
			Draft:        payload.PullRequest.Draft,
		},
	}

	return event, nil
}

// giteaCreatePayload represents a Gitea create event payload (tag/branch creation).
type giteaCreatePayload struct {
	Ref           string `json:"ref"`
	RefType       string `json:"ref_type"` // "tag" or "branch"
	SHA           string `json:"sha"`
	DefaultBranch string `json:"default_branch"`
	Repository    struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Sender struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"sender"`
}

func (h *GiteaHandler) parseCreateEvent(body []byte) (*Event, error) {
	var payload giteaCreatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse create payload: %w", err)
	}

	// We only care about tag creation for builds
	// Branch creation is already handled by push events
	if payload.RefType != "tag" {
		return nil, fmt.Errorf("create event for %s, not tag", payload.RefType)
	}

	sender := payload.Sender.Login
	if sender == "" {
		sender = payload.Sender.Username
	}

	event := &Event{
		Provider:  "gitea",
		EventType: "push",
		FullName:  payload.Repository.FullName,
		Ref:       fmt.Sprintf("refs/tags/%s", payload.Ref),
		Tag:       payload.Ref,
		CommitSHA: payload.SHA,
		CloneURL:  payload.Repository.CloneURL,
		Sender:    sender,
	}

	return event, nil
}
