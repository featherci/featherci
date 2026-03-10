package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubHandler_ValidateSignature(t *testing.T) {
	h := NewGitHubHandler()
	secret := "test-secret"

	tests := []struct {
		name      string
		body      string
		signature string
		wantErr   bool
	}{
		{
			name:      "valid signature",
			body:      `{"test": "data"}`,
			signature: computeGitHubSignature(`{"test": "data"}`, secret),
			wantErr:   false,
		},
		{
			name:      "invalid signature",
			body:      `{"test": "data"}`,
			signature: "sha256=invalid",
			wantErr:   true,
		},
		{
			name:      "missing signature header",
			body:      `{"test": "data"}`,
			signature: "",
			wantErr:   true,
		},
		{
			name:      "wrong signature format",
			body:      `{"test": "data"}`,
			signature: "invalid-format",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(tt.body))
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			err := h.ValidateSignature(req, secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitHubHandler_ParsePushEvent(t *testing.T) {
	h := NewGitHubHandler()

	payload := `{
		"ref": "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after": "abc123def456",
		"deleted": false,
		"repository": {
			"full_name": "owner/repo",
			"clone_url": "https://github.com/owner/repo.git"
		},
		"sender": {
			"login": "testuser"
		},
		"head_commit": {
			"id": "abc123def456",
			"message": "Test commit message",
			"author": {
				"name": "Test Author",
				"username": "testauthor"
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.Provider != "github" {
		t.Errorf("Provider = %q, want %q", event.Provider, "github")
	}
	if event.EventType != "push" {
		t.Errorf("EventType = %q, want %q", event.EventType, "push")
	}
	if event.FullName != "owner/repo" {
		t.Errorf("FullName = %q, want %q", event.FullName, "owner/repo")
	}
	if event.Branch != "main" {
		t.Errorf("Branch = %q, want %q", event.Branch, "main")
	}
	if event.CommitSHA != "abc123def456" {
		t.Errorf("CommitSHA = %q, want %q", event.CommitSHA, "abc123def456")
	}
	if event.CommitMessage != "Test commit message" {
		t.Errorf("CommitMessage = %q, want %q", event.CommitMessage, "Test commit message")
	}
	if event.CommitAuthor != "Test Author" {
		t.Errorf("CommitAuthor = %q, want %q", event.CommitAuthor, "Test Author")
	}
	if event.Sender != "testuser" {
		t.Errorf("Sender = %q, want %q", event.Sender, "testuser")
	}
}

func TestGitHubHandler_ParsePullRequestEvent(t *testing.T) {
	h := NewGitHubHandler()

	payload := `{
		"action": "opened",
		"number": 42,
		"pull_request": {
			"title": "Add new feature",
			"draft": false,
			"head": {
				"ref": "feature-branch",
				"sha": "abc123"
			},
			"base": {
				"ref": "main"
			}
		},
		"repository": {
			"full_name": "owner/repo",
			"clone_url": "https://github.com/owner/repo.git"
		},
		"sender": {
			"login": "testuser"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.EventType != "pull_request" {
		t.Errorf("EventType = %q, want %q", event.EventType, "pull_request")
	}
	if event.PullRequest == nil {
		t.Fatal("PullRequest is nil")
	}
	if event.PullRequest.Number != 42 {
		t.Errorf("PullRequest.Number = %d, want %d", event.PullRequest.Number, 42)
	}
	if event.PullRequest.Action != "opened" {
		t.Errorf("PullRequest.Action = %q, want %q", event.PullRequest.Action, "opened")
	}
	if event.PullRequest.SourceBranch != "feature-branch" {
		t.Errorf("PullRequest.SourceBranch = %q, want %q", event.PullRequest.SourceBranch, "feature-branch")
	}
	if event.PullRequest.TargetBranch != "main" {
		t.Errorf("PullRequest.TargetBranch = %q, want %q", event.PullRequest.TargetBranch, "main")
	}
}

func TestGitHubHandler_ParsePingEvent(t *testing.T) {
	h := NewGitHubHandler()

	payload := `{
		"zen": "Speak like a human.",
		"hook_id": 12345,
		"repository": {
			"full_name": "owner/repo"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "ping")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.EventType != "ping" {
		t.Errorf("EventType = %q, want %q", event.EventType, "ping")
	}
	if event.FullName != "owner/repo" {
		t.Errorf("FullName = %q, want %q", event.FullName, "owner/repo")
	}
}

func TestGitLabHandler_ValidateSignature(t *testing.T) {
	h := NewGitLabHandler()
	secret := "test-secret"

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   secret,
			wantErr: false,
		},
		{
			name:    "invalid token",
			token:   "wrong-secret",
			wantErr: true,
		},
		{
			name:    "missing token",
			token:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", nil)
			if tt.token != "" {
				req.Header.Set("X-Gitlab-Token", tt.token)
			}

			err := h.ValidateSignature(req, secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitLabHandler_ParsePushEvent(t *testing.T) {
	h := NewGitLabHandler()

	payload := `{
		"object_kind": "push",
		"ref": "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after": "abc123def456",
		"user_username": "testuser",
		"project": {
			"path_with_namespace": "owner/repo",
			"git_http_url": "https://gitlab.com/owner/repo.git"
		},
		"commits": [
			{
				"id": "abc123def456",
				"message": "Test commit",
				"author": {
					"name": "Test Author"
				}
			}
		],
		"total_commits_count": 1
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", strings.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Push Hook")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.Provider != "gitlab" {
		t.Errorf("Provider = %q, want %q", event.Provider, "gitlab")
	}
	if event.EventType != "push" {
		t.Errorf("EventType = %q, want %q", event.EventType, "push")
	}
	if event.FullName != "owner/repo" {
		t.Errorf("FullName = %q, want %q", event.FullName, "owner/repo")
	}
	if event.Branch != "main" {
		t.Errorf("Branch = %q, want %q", event.Branch, "main")
	}
}

func TestGitLabHandler_ParseMergeRequestEvent(t *testing.T) {
	h := NewGitLabHandler()

	payload := `{
		"object_kind": "merge_request",
		"user": {
			"username": "testuser"
		},
		"project": {
			"path_with_namespace": "owner/repo",
			"git_http_url": "https://gitlab.com/owner/repo.git"
		},
		"object_attributes": {
			"iid": 42,
			"title": "Add new feature",
			"source_branch": "feature-branch",
			"target_branch": "main",
			"state": "opened",
			"action": "open",
			"last_commit": {
				"id": "abc123",
				"message": "Test commit",
				"author": {
					"name": "Test Author"
				}
			},
			"draft": false
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", strings.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.EventType != "merge_request" {
		t.Errorf("EventType = %q, want %q", event.EventType, "merge_request")
	}
	if event.PullRequest == nil {
		t.Fatal("PullRequest is nil")
	}
	if event.PullRequest.Number != 42 {
		t.Errorf("PullRequest.Number = %d, want %d", event.PullRequest.Number, 42)
	}
	if event.PullRequest.Action != "opened" {
		t.Errorf("PullRequest.Action = %q, want %q", event.PullRequest.Action, "opened")
	}
}

func TestGiteaHandler_ValidateSignature(t *testing.T) {
	h := NewGiteaHandler()
	secret := "test-secret"

	tests := []struct {
		name      string
		body      string
		signature string
		wantErr   bool
	}{
		{
			name:      "valid signature",
			body:      `{"test": "data"}`,
			signature: computeGiteaSignature(`{"test": "data"}`, secret),
			wantErr:   false,
		},
		{
			name:      "invalid signature",
			body:      `{"test": "data"}`,
			signature: "invalid",
			wantErr:   true,
		},
		{
			name:      "missing signature",
			body:      `{"test": "data"}`,
			signature: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/gitea", strings.NewReader(tt.body))
			if tt.signature != "" {
				req.Header.Set("X-Gitea-Signature", tt.signature)
			}

			err := h.ValidateSignature(req, secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGiteaHandler_ParsePushEvent(t *testing.T) {
	h := NewGiteaHandler()

	payload := `{
		"ref": "refs/heads/main",
		"before": "0000000000000000000000000000000000000000",
		"after": "abc123def456",
		"repository": {
			"full_name": "owner/repo",
			"clone_url": "https://gitea.example.com/owner/repo.git"
		},
		"sender": {
			"login": "testuser"
		},
		"head_commit": {
			"id": "abc123def456",
			"message": "Test commit",
			"author": {
				"name": "Test Author"
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitea", strings.NewReader(payload))
	req.Header.Set("X-Gitea-Event", "push")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.Provider != "gitea" {
		t.Errorf("Provider = %q, want %q", event.Provider, "gitea")
	}
	if event.EventType != "push" {
		t.Errorf("EventType = %q, want %q", event.EventType, "push")
	}
	if event.FullName != "owner/repo" {
		t.Errorf("FullName = %q, want %q", event.FullName, "owner/repo")
	}
	if event.Branch != "main" {
		t.Errorf("Branch = %q, want %q", event.Branch, "main")
	}
}

func TestGiteaHandler_ParsePullRequestEvent(t *testing.T) {
	h := NewGiteaHandler()

	payload := `{
		"action": "opened",
		"number": 42,
		"pull_request": {
			"number": 42,
			"title": "Add new feature",
			"draft": false,
			"head": {
				"ref": "feature-branch",
				"sha": "abc123"
			},
			"base": {
				"ref": "main"
			}
		},
		"repository": {
			"full_name": "owner/repo",
			"clone_url": "https://gitea.example.com/owner/repo.git"
		},
		"sender": {
			"login": "testuser"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitea", strings.NewReader(payload))
	req.Header.Set("X-Gitea-Event", "pull_request")

	event, err := h.ParseEvent(req)
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.EventType != "pull_request" {
		t.Errorf("EventType = %q, want %q", event.EventType, "pull_request")
	}
	if event.PullRequest == nil {
		t.Fatal("PullRequest is nil")
	}
	if event.PullRequest.Number != 42 {
		t.Errorf("PullRequest.Number = %d, want %d", event.PullRequest.Number, 42)
	}
}

func TestEvent_ShouldTriggerBuild(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name:  "push event",
			event: Event{EventType: "push"},
			want:  true,
		},
		{
			name:  "tag push event",
			event: Event{EventType: "push", Tag: "v1.0.0"},
			want:  true,
		},
		{
			name: "PR opened",
			event: Event{
				EventType:   "pull_request",
				PullRequest: &PullRequestEvent{Action: "opened"},
			},
			want: true,
		},
		{
			name: "PR synchronize",
			event: Event{
				EventType:   "pull_request",
				PullRequest: &PullRequestEvent{Action: "synchronize"},
			},
			want: true,
		},
		{
			name: "PR reopened",
			event: Event{
				EventType:   "pull_request",
				PullRequest: &PullRequestEvent{Action: "reopened"},
			},
			want: true,
		},
		{
			name: "PR closed",
			event: Event{
				EventType:   "pull_request",
				PullRequest: &PullRequestEvent{Action: "closed"},
			},
			want: false,
		},
		{
			name: "MR opened (GitLab)",
			event: Event{
				EventType:   "merge_request",
				PullRequest: &PullRequestEvent{Action: "opened"},
			},
			want: true,
		},
		{
			name:  "ping event",
			event: Event{EventType: "ping"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.ShouldTriggerBuild(); got != tt.want {
				t.Errorf("ShouldTriggerBuild() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractRepoFullName(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name:    "GitHub format",
			body:    `{"repository": {"full_name": "owner/repo"}}`,
			want:    "owner/repo",
			wantErr: false,
		},
		{
			name:    "GitLab format",
			body:    `{"project": {"path_with_namespace": "group/project"}}`,
			want:    "group/project",
			wantErr: false,
		},
		{
			name:    "GitLab repository format",
			body:    `{"repository": {"path_with_namespace": "group/project"}}`,
			want:    "group/project",
			wantErr: false,
		},
		{
			name:    "empty body",
			body:    `{}`,
			want:    "",
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `{invalid`,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/test", strings.NewReader(tt.body))
			got, err := ExtractRepoFullName(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractRepoFullName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractRepoFullName() = %q, want %q", got, tt.want)
			}

			// Verify body was restored
			if !tt.wantErr {
				restoredBody, _ := io.ReadAll(req.Body)
				if string(restoredBody) != tt.body {
					t.Errorf("Body was not restored: got %q, want %q", string(restoredBody), tt.body)
				}
			}
		})
	}
}

func TestGitHubHandler_ParseDeletedBranch(t *testing.T) {
	h := NewGitHubHandler()

	payload := `{
		"ref": "refs/heads/feature-branch",
		"deleted": true,
		"repository": {
			"full_name": "owner/repo"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")

	_, err := h.ParseEvent(req)
	if err == nil {
		t.Error("ParseEvent() expected error for deleted branch, got nil")
	}
}

func TestGitLabHandler_ParseDeletedBranch(t *testing.T) {
	h := NewGitLabHandler()

	payload := `{
		"object_kind": "push",
		"ref": "refs/heads/feature-branch",
		"after": "0000000000000000000000000000000000000000",
		"project": {
			"path_with_namespace": "owner/repo"
		},
		"total_commits_count": 0
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", strings.NewReader(payload))
	req.Header.Set("X-Gitlab-Event", "Push Hook")

	_, err := h.ParseEvent(req)
	if err == nil {
		t.Error("ParseEvent() expected error for deleted branch, got nil")
	}
}

func TestGiteaHandler_ParseDeletedBranch(t *testing.T) {
	h := NewGiteaHandler()

	payload := `{
		"ref": "refs/heads/feature-branch",
		"after": "0000000000000000000000000000000000000000",
		"repository": {
			"full_name": "owner/repo"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitea", strings.NewReader(payload))
	req.Header.Set("X-Gitea-Event", "push")

	_, err := h.ParseEvent(req)
	if err == nil {
		t.Error("ParseEvent() expected error for deleted branch, got nil")
	}
}

// Helper functions to compute signatures
func computeGitHubSignature(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func computeGiteaSignature(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestEvent_IsPush(t *testing.T) {
	e := &Event{EventType: "push"}
	if !e.IsPush() {
		t.Error("IsPush() = false, want true")
	}

	e = &Event{EventType: "pull_request"}
	if e.IsPush() {
		t.Error("IsPush() = true, want false")
	}
}

func TestEvent_IsPullRequest(t *testing.T) {
	e := &Event{EventType: "pull_request"}
	if !e.IsPullRequest() {
		t.Error("IsPullRequest() = false, want true for pull_request")
	}

	e = &Event{EventType: "merge_request"}
	if !e.IsPullRequest() {
		t.Error("IsPullRequest() = false, want true for merge_request")
	}

	e = &Event{EventType: "push"}
	if e.IsPullRequest() {
		t.Error("IsPullRequest() = true, want false for push")
	}
}

func TestEvent_IsTag(t *testing.T) {
	e := &Event{Tag: "v1.0.0"}
	if !e.IsTag() {
		t.Error("IsTag() = false, want true")
	}

	e = &Event{Branch: "main"}
	if e.IsTag() {
		t.Error("IsTag() = true, want false")
	}
}

func TestGitHubHandler_ValidateSignature_RestoresBody(t *testing.T) {
	h := NewGitHubHandler()
	secret := "test-secret"
	body := `{"test": "data"}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", computeGitHubSignature(body, secret))

	err := h.ValidateSignature(req, secret)
	if err != nil {
		t.Fatalf("ValidateSignature() error = %v", err)
	}

	// Verify body was restored
	restoredBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read restored body: %v", err)
	}
	if string(restoredBody) != body {
		t.Errorf("Body was not restored: got %q, want %q", string(restoredBody), body)
	}
}

func TestExtractRepoFullName_RestoresBody(t *testing.T) {
	body := `{"repository": {"full_name": "owner/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/test", bytes.NewReader([]byte(body)))

	fullName, err := ExtractRepoFullName(req)
	if err != nil {
		t.Fatalf("ExtractRepoFullName() error = %v", err)
	}
	if fullName != "owner/repo" {
		t.Errorf("fullName = %q, want %q", fullName, "owner/repo")
	}

	// Read body again to verify it was restored
	restoredBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read restored body: %v", err)
	}
	if string(restoredBody) != body {
		t.Errorf("Body was not restored: got %q, want %q", string(restoredBody), body)
	}
}
