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

// GitHubHandler handles GitHub webhooks.
type GitHubHandler struct{}

// NewGitHubHandler creates a new GitHub webhook handler.
func NewGitHubHandler() *GitHubHandler {
	return &GitHubHandler{}
}

// ValidateSignature validates the GitHub webhook signature.
// GitHub uses X-Hub-Signature-256 header with HMAC-SHA256.
func (h *GitHubHandler) ValidateSignature(r *http.Request, secret string) error {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return errors.New("missing X-Hub-Signature-256 header")
	}

	// Signature format: sha256=<hex>
	if !strings.HasPrefix(signature, "sha256=") {
		return errors.New("invalid signature format")
	}
	signature = strings.TrimPrefix(signature, "sha256=")

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

// ParseEvent parses a GitHub webhook payload.
func (h *GitHubHandler) ParseEvent(r *http.Request) (*Event, error) {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		return nil, errors.New("missing X-GitHub-Event header")
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
	case "ping":
		return h.parsePingEvent(body)
	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// githubPushPayload represents a GitHub push event payload.
type githubPushPayload struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Created    bool   `json:"created"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
	Pusher struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"pusher"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
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
}

func (h *GitHubHandler) parsePushEvent(body []byte) (*Event, error) {
	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse push payload: %w", err)
	}

	// Skip deleted branch events
	if payload.Deleted {
		return nil, errors.New("branch deleted, skipping")
	}

	event := &Event{
		Provider:  "github",
		EventType: "push",
		FullName:  payload.Repository.FullName,
		Ref:       payload.Ref,
		CommitSHA: payload.After,
		CloneURL:  payload.Repository.CloneURL,
		Sender:    payload.Sender.Login,
	}

	// Extract branch or tag name from ref
	if strings.HasPrefix(payload.Ref, "refs/heads/") {
		event.Branch = strings.TrimPrefix(payload.Ref, "refs/heads/")
	} else if strings.HasPrefix(payload.Ref, "refs/tags/") {
		event.Tag = strings.TrimPrefix(payload.Ref, "refs/tags/")
	}

	// Get commit details from head_commit
	if payload.HeadCommit != nil {
		event.CommitMessage = payload.HeadCommit.Message
		event.CommitAuthor = payload.HeadCommit.Author.Name
		if event.CommitAuthor == "" {
			event.CommitAuthor = payload.HeadCommit.Author.Username
		}
	}

	return event, nil
}

// githubPullRequestPayload represents a GitHub pull_request event payload.
type githubPullRequestPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Title string `json:"title"`
		Draft bool   `json:"draft"`
		Head  struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (h *GitHubHandler) parsePullRequestEvent(body []byte) (*Event, error) {
	var payload githubPullRequestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse pull_request payload: %w", err)
	}

	event := &Event{
		Provider:      "github",
		EventType:     "pull_request",
		FullName:      payload.Repository.FullName,
		Ref:           fmt.Sprintf("refs/pull/%d/head", payload.Number),
		Branch:        payload.PullRequest.Head.Ref,
		CommitSHA:     payload.PullRequest.Head.SHA,
		CloneURL:      payload.Repository.CloneURL,
		Sender:        payload.Sender.Login,
		CommitMessage: payload.PullRequest.Title,
		CommitAuthor:  payload.PullRequest.User.Login,
		PullRequest: &PullRequestEvent{
			Number:       payload.Number,
			Action:       payload.Action,
			Title:        payload.PullRequest.Title,
			SourceBranch: payload.PullRequest.Head.Ref,
			TargetBranch: payload.PullRequest.Base.Ref,
			Draft:        payload.PullRequest.Draft,
		},
	}

	return event, nil
}

// githubPingPayload represents a GitHub ping event payload (sent when webhook is created).
type githubPingPayload struct {
	Zen    string `json:"zen"`
	HookID int    `json:"hook_id"`
	Hook   struct {
		Type   string   `json:"type"`
		Events []string `json:"events"`
	} `json:"hook"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (h *GitHubHandler) parsePingEvent(body []byte) (*Event, error) {
	var payload githubPingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse ping payload: %w", err)
	}

	// Return a special ping event that won't trigger a build
	return &Event{
		Provider:  "github",
		EventType: "ping",
		FullName:  payload.Repository.FullName,
	}, nil
}
