package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock provider
// ---------------------------------------------------------------------------

type mockProvider struct {
	createID  string
	createErr error
	deleteErr error
	lastToken string
	lastRepo  string
}

func (m *mockProvider) CreateWebhook(ctx context.Context, token, repoFullName, webhookURL, secret string) (string, error) {
	m.lastToken = token
	m.lastRepo = repoFullName
	return m.createID, m.createErr
}

func (m *mockProvider) DeleteWebhook(ctx context.Context, token, repoFullName, webhookID string) error {
	m.lastToken = token
	m.lastRepo = repoFullName
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// Manager tests
// ---------------------------------------------------------------------------

func TestShouldRegister(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"empty baseURL", "", false},
		{"localhost", "http://localhost:8080", false},
		{"127.0.0.1", "http://127.0.0.1:8080", false},
		{"::1", "http://[::1]:8080", false},
		{"public URL", "https://ci.example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				baseURL: tt.baseURL,
				logger:  slog.Default(),
			}
			if got := m.ShouldRegister(); got != tt.want {
				t.Errorf("ShouldRegister() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebhookURL(t *testing.T) {
	m := &Manager{
		baseURL: "https://ci.example.com",
		logger:  slog.Default(),
	}
	got := m.WebhookURL("github")
	want := "https://ci.example.com/webhooks/github"
	if got != want {
		t.Errorf("WebhookURL() = %q, want %q", got, want)
	}
}

func TestRegisterWebhook_UnknownProvider(t *testing.T) {
	m := &Manager{
		providers: map[string]WebhookProvider{},
		baseURL:   "https://ci.example.com",
		logger:    slog.Default(),
	}
	_, err := m.RegisterWebhook(context.Background(), "unknown", "tok", "owner/repo", "secret")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnregisterWebhook_UnknownProvider(t *testing.T) {
	m := &Manager{
		providers: map[string]WebhookProvider{},
		baseURL:   "https://ci.example.com",
		logger:    slog.Default(),
	}
	err := m.UnregisterWebhook(context.Background(), "unknown", "tok", "owner/repo", "123")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterWebhook_Success(t *testing.T) {
	mp := &mockProvider{createID: "wh-99"}
	m := &Manager{
		providers: map[string]WebhookProvider{"test": mp},
		baseURL:   "https://ci.example.com",
		logger:    slog.Default(),
	}
	id, err := m.RegisterWebhook(context.Background(), "test", "tok123", "owner/repo", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "wh-99" {
		t.Errorf("got id %q, want %q", id, "wh-99")
	}
	if mp.lastToken != "tok123" {
		t.Errorf("token not forwarded: got %q", mp.lastToken)
	}
	if mp.lastRepo != "owner/repo" {
		t.Errorf("repo not forwarded: got %q", mp.lastRepo)
	}
}

func TestUnregisterWebhook_Success(t *testing.T) {
	mp := &mockProvider{}
	m := &Manager{
		providers: map[string]WebhookProvider{"test": mp},
		baseURL:   "https://ci.example.com",
		logger:    slog.Default(),
	}
	err := m.UnregisterWebhook(context.Background(), "test", "tok123", "owner/repo", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.lastToken != "tok123" {
		t.Errorf("token not forwarded: got %q", mp.lastToken)
	}
}

func TestRegisterWebhook_ProviderError(t *testing.T) {
	mp := &mockProvider{createErr: fmt.Errorf("API down")}
	m := &Manager{
		providers: map[string]WebhookProvider{"test": mp},
		baseURL:   "https://ci.example.com",
		logger:    slog.Default(),
	}
	_, err := m.RegisterWebhook(context.Background(), "test", "tok", "owner/repo", "secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "API down") {
		t.Errorf("error should wrap provider error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GitHub webhook tests
// ---------------------------------------------------------------------------

func TestGitHubCreateWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 12345})
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	id, err := gh.CreateWebhook(context.Background(), "mytoken", "owner/repo", "https://ci.example.com/webhooks/github", "s3cret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "12345" {
		t.Errorf("got id %q, want %q", id, "12345")
	}
}

func TestGitHubCreateWebhook_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	_, err := gh.CreateWebhook(context.Background(), "tok", "owner/repo", "https://ci.example.com/webhooks/github", "s3cret")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestGitHubCreateWebhook_Headers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer mytoken" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer mytoken")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want %q", got, "application/vnd.github+json")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	gh.CreateWebhook(context.Background(), "mytoken", "owner/repo", "https://ci.example.com/webhooks/github", "secret")
}

func TestGitHubCreateWebhook_Body(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON body: %v", err)
		}
		if payload["name"] != "web" {
			t.Errorf("name = %v, want %q", payload["name"], "web")
		}
		if payload["active"] != true {
			t.Errorf("active = %v, want true", payload["active"])
		}
		events, ok := payload["events"].([]interface{})
		if !ok || len(events) != 2 {
			t.Errorf("events = %v, want [push, pull_request]", payload["events"])
		}
		cfg, ok := payload["config"].(map[string]interface{})
		if !ok {
			t.Fatal("config missing or wrong type")
		}
		if cfg["url"] != "https://ci.example.com/webhooks/github" {
			t.Errorf("config.url = %v", cfg["url"])
		}
		if cfg["content_type"] != "json" {
			t.Errorf("config.content_type = %v", cfg["content_type"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	gh.CreateWebhook(context.Background(), "tok", "owner/repo", "https://ci.example.com/webhooks/github", "secret")
}

func TestGitHubDeleteWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	err := gh.DeleteWebhook(context.Background(), "tok", "owner/repo", "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitHubDeleteWebhook_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	gh := &GitHubWebhook{BaseURL: server.URL}
	err := gh.DeleteWebhook(context.Background(), "tok", "owner/repo", "12345")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// ---------------------------------------------------------------------------
// GitLab webhook tests
// ---------------------------------------------------------------------------

func TestGitLabCreateWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 789})
	}))
	defer server.Close()

	gl := &GitLabWebhook{BaseURL: server.URL}
	id, err := gl.CreateWebhook(context.Background(), "glpat-token", "group/project", "https://ci.example.com/webhooks/gitlab", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "789" {
		t.Errorf("got id %q, want %q", id, "789")
	}
}

func TestGitLabCreateWebhook_PrivateTokenHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "glpat-token" {
			t.Errorf("PRIVATE-TOKEN = %q, want %q", got, "glpat-token")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gl := &GitLabWebhook{BaseURL: server.URL}
	gl.CreateWebhook(context.Background(), "glpat-token", "group/project", "https://ci.example.com/webhooks/gitlab", "secret")
}

func TestGitLabCreateWebhook_URLEncodedProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// url.PathEscape encodes "group/project" as "group%2Fproject".
		// Go's HTTP server decodes the path, so check RawPath or RequestURI instead.
		if !strings.Contains(r.RequestURI, "group%2Fproject") {
			t.Errorf("expected URL-encoded project in RequestURI, got %q", r.RequestURI)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gl := &GitLabWebhook{BaseURL: server.URL}
	gl.CreateWebhook(context.Background(), "tok", "group/project", "https://ci.example.com/webhooks/gitlab", "secret")
}

func TestGitLabDeleteWebhook_NoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	gl := &GitLabWebhook{BaseURL: server.URL}
	err := gl.DeleteWebhook(context.Background(), "tok", "group/project", "789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitLabDeleteWebhook_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	gl := &GitLabWebhook{BaseURL: server.URL}
	err := gl.DeleteWebhook(context.Background(), "tok", "group/project", "789")
	if err != nil {
		t.Fatalf("unexpected error: GitLab should accept 200, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Gitea webhook tests
// ---------------------------------------------------------------------------

func TestGiteaCreateWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 456})
	}))
	defer server.Close()

	gt := &GiteaWebhook{BaseURL: server.URL}
	id, err := gt.CreateWebhook(context.Background(), "giteatok", "owner/repo", "https://ci.example.com/webhooks/gitea", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "456" {
		t.Errorf("got id %q, want %q", id, "456")
	}
}

func TestGiteaCreateWebhook_TokenAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token giteatok" {
			t.Errorf("Authorization = %q, want %q", got, "token giteatok")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gt := &GiteaWebhook{BaseURL: server.URL}
	gt.CreateWebhook(context.Background(), "giteatok", "owner/repo", "https://ci.example.com/webhooks/gitea", "secret")
}

func TestGiteaCreateWebhook_BodyType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON body: %v", err)
		}
		if payload["type"] != "gitea" {
			t.Errorf("type = %v, want %q", payload["type"], "gitea")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": 1})
	}))
	defer server.Close()

	gt := &GiteaWebhook{BaseURL: server.URL}
	gt.CreateWebhook(context.Background(), "tok", "owner/repo", "https://ci.example.com/webhooks/gitea", "secret")
}

func TestGiteaDeleteWebhook_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	gt := &GiteaWebhook{BaseURL: server.URL}
	err := gt.DeleteWebhook(context.Background(), "tok", "owner/repo", "456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGiteaDeleteWebhook_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	gt := &GiteaWebhook{BaseURL: server.URL}
	err := gt.DeleteWebhook(context.Background(), "tok", "owner/repo", "456")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
