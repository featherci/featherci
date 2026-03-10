package status

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/featherci/featherci/internal/config"
	"github.com/featherci/featherci/internal/models"
)

// --- State mapping tests ---

func TestMapBuildStatus(t *testing.T) {
	tests := []struct {
		input models.BuildStatus
		want  CommitState
	}{
		{models.BuildStatusPending, StatePending},
		{models.BuildStatusRunning, StateRunning},
		{models.BuildStatusSuccess, StateSuccess},
		{models.BuildStatusFailure, StateFailure},
		{models.BuildStatusCancelled, StateCancelled},
	}
	for _, tt := range tests {
		got := mapBuildStatus(tt.input)
		if got != tt.want {
			t.Errorf("mapBuildStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMapGitHubState(t *testing.T) {
	tests := []struct {
		input CommitState
		want  string
	}{
		{StatePending, "pending"},
		{StateRunning, "pending"},
		{StateSuccess, "success"},
		{StateFailure, "failure"},
		{StateCancelled, "error"},
	}
	for _, tt := range tests {
		got := mapGitHubState(tt.input)
		if got != tt.want {
			t.Errorf("mapGitHubState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMapGitLabState(t *testing.T) {
	tests := []struct {
		input CommitState
		want  string
	}{
		{StatePending, "pending"},
		{StateRunning, "running"},
		{StateSuccess, "success"},
		{StateFailure, "failed"},
		{StateCancelled, "canceled"},
	}
	for _, tt := range tests {
		got := mapGitLabState(tt.input)
		if got != tt.want {
			t.Errorf("mapGitLabState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMapGiteaState(t *testing.T) {
	tests := []struct {
		input CommitState
		want  string
	}{
		{StatePending, "pending"},
		{StateRunning, "pending"},
		{StateSuccess, "success"},
		{StateFailure, "failure"},
		{StateCancelled, "error"},
	}
	for _, tt := range tests {
		got := mapGiteaState(tt.input)
		if got != tt.want {
			t.Errorf("mapGiteaState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- HTTP request formatting tests ---

func TestGitHubPosterPostStatus(t *testing.T) {
	var gotPath, gotAuth, gotState, gotContext string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		gotState = m["state"]
		gotContext = m["context"]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	poster := &GitHubPoster{BaseURL: srv.URL}
	err := poster.PostStatus(context.Background(), "gh-token", StatusOptions{
		Owner:     "octocat",
		Repo:      "hello",
		CommitSHA: "abc123",
		State:     StateSuccess,
		TargetURL: "https://ci.example.com/builds/1",
		Context:   "featherci",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/repos/octocat/hello/statuses/abc123" {
		t.Errorf("path = %q, want /repos/octocat/hello/statuses/abc123", gotPath)
	}
	if gotAuth != "Bearer gh-token" {
		t.Errorf("auth = %q, want Bearer gh-token", gotAuth)
	}
	if gotState != "success" {
		t.Errorf("state = %q, want success", gotState)
	}
	if gotContext != "featherci" {
		t.Errorf("context = %q, want featherci", gotContext)
	}
}

func TestGitLabPosterPostStatus(t *testing.T) {
	var gotRawPath, gotAuth, gotState, gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.RequestURI
		gotAuth = r.Header.Get("PRIVATE-TOKEN")
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		gotState = m["state"]
		gotName = m["name"]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	poster := &GitLabPoster{BaseURL: srv.URL}
	err := poster.PostStatus(context.Background(), "gl-token", StatusOptions{
		Owner:     "mygroup",
		Repo:      "myproject",
		CommitSHA: "def456",
		State:     StateFailure,
		TargetURL: "https://ci.example.com/builds/2",
		Context:   "featherci",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPath := "/api/v4/projects/mygroup%2Fmyproject/statuses/def456"
	if gotRawPath != wantPath {
		t.Errorf("path = %q, want %q", gotRawPath, wantPath)
	}
	if gotAuth != "gl-token" {
		t.Errorf("auth = %q, want gl-token", gotAuth)
	}
	if gotState != "failed" {
		t.Errorf("state = %q, want failed", gotState)
	}
	if gotName != "featherci" {
		t.Errorf("name = %q, want featherci", gotName)
	}
}

func TestGiteaPosterPostStatus(t *testing.T) {
	var gotPath, gotAuth, gotState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		gotState = m["state"]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	poster := &GiteaPoster{BaseURL: srv.URL}
	err := poster.PostStatus(context.Background(), "gt-token", StatusOptions{
		Owner:     "user",
		Repo:      "repo",
		CommitSHA: "789abc",
		State:     StateCancelled,
		TargetURL: "https://ci.example.com/builds/3",
		Context:   "featherci",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/repos/user/repo/statuses/789abc" {
		t.Errorf("path = %q, want /api/v1/repos/user/repo/statuses/789abc", gotPath)
	}
	if gotAuth != "token gt-token" {
		t.Errorf("auth = %q, want token gt-token", gotAuth)
	}
	if gotState != "error" {
		t.Errorf("state = %q, want error", gotState)
	}
}

// --- Error handling tests ---

func TestPosterNon200Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	posters := []struct {
		name   string
		poster StatusPoster
	}{
		{"github", &GitHubPoster{BaseURL: srv.URL}},
		{"gitlab", &GitLabPoster{BaseURL: srv.URL}},
		{"gitea", &GiteaPoster{BaseURL: srv.URL}},
	}

	opts := StatusOptions{
		Owner: "o", Repo: "r", CommitSHA: "abc",
		State: StateSuccess, Context: "featherci",
	}

	for _, p := range posters {
		err := p.poster.PostStatus(context.Background(), "token", opts)
		if err == nil {
			t.Errorf("%s: expected error for 403 response", p.name)
		}
	}
}

// --- Integration test: PostBuildStatus maps correctly ---

type mockTokenSource struct {
	token string
	err   error
}

func (m *mockTokenSource) TokenForProject(_ context.Context, _ int64) (string, error) {
	return m.token, m.err
}

func TestPostBuildStatusIntegration(t *testing.T) {
	var gotState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		gotState = m["state"]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := &config.Config{BaseURL: "https://ci.example.com"}
	tokens := &mockTokenSource{token: "test-token"}
	svc := NewStatusService(cfg, tokens, nil)

	// Override github poster to use test server
	svc.posters["github"] = &GitHubPoster{BaseURL: srv.URL}

	project := &models.Project{
		ID:       1,
		Provider: "github",
		FullName: "octocat/hello",
	}
	build := &models.Build{
		ID:          10,
		BuildNumber: 5,
		CommitSHA:   "abc123def456",
		Status:      models.BuildStatusRunning,
	}

	svc.PostBuildStatus(context.Background(), project, build)

	// GitHub maps "running" → "pending"
	if gotState != "pending" {
		t.Errorf("state = %q, want pending (running maps to pending on github)", gotState)
	}
}

func TestSplitFullName(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantRepo  string
	}{
		{"octocat/hello", "octocat", "hello"},
		{"group/subgroup/repo", "group", "subgroup/repo"},
		{"noslash", "noslash", ""},
	}
	for _, tt := range tests {
		owner, repo := splitFullName(tt.input)
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("splitFullName(%q) = (%q, %q), want (%q, %q)", tt.input, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}
