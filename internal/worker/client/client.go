// Package client provides HTTP client implementations of worker interfaces
// for distributed mode, where a remote worker communicates with the master over HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/featherci/featherci/internal/models"
)

// Client communicates with the master's worker API over HTTP.
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

// New creates a new worker API client.
func New(baseURL, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StepClient returns a step repository backed by HTTP.
func (c *Client) StepClient() *StepClient { return &StepClient{c: c} }

// BuildClient returns a build repository backed by HTTP.
func (c *Client) BuildClient() *BuildClient { return &BuildClient{c: c} }

// ProjectClient returns a project repository backed by HTTP.
func (c *Client) ProjectClient() *ProjectClient { return &ProjectClient{c: c} }

// WorkerClient returns a worker repository backed by HTTP.
func (c *Client) WorkerClient() *WorkerClient { return &WorkerClient{c: c} }

// --- StepClient implements worker.stepRepo ---

// StepClient wraps the base Client for step operations.
type StepClient struct{ c *Client }

// ListReady fetches ready steps from the master.
func (s *StepClient) ListReady(ctx context.Context) ([]*models.BuildStep, error) {
	var resp struct {
		Steps []stepJSON `json:"steps"`
	}
	if err := s.c.get(ctx, "/api/worker/steps/ready", &resp); err != nil {
		return nil, err
	}
	steps := make([]*models.BuildStep, len(resp.Steps))
	for i, sj := range resp.Steps {
		steps[i] = sj.toModel()
	}
	return steps, nil
}

// SetStarted claims a step on the master.
func (s *StepClient) SetStarted(ctx context.Context, id int64, workerID string) error {
	return s.c.post(ctx, fmt.Sprintf("/api/worker/steps/%d/claim", id),
		map[string]string{"worker_id": workerID}, nil)
}

// SetLogPath saves the log path on the master so the UI can stream logs.
func (s *StepClient) SetLogPath(ctx context.Context, id int64, logPath string) error {
	return s.c.post(ctx, fmt.Sprintf("/api/worker/steps/%d/logpath", id),
		map[string]string{"log_path": logPath}, nil)
}

// SetFinished reports step completion to the master.
func (s *StepClient) SetFinished(ctx context.Context, id int64, status models.StepStatus, exitCode *int, logPath string) error {
	return s.c.post(ctx, fmt.Sprintf("/api/worker/steps/%d/complete", id),
		map[string]any{"status": string(status), "exit_code": exitCode, "log_path": logPath}, nil)
}

// UpdateReadySteps is a no-op — the master handles this via AdvanceBuild.
func (s *StepClient) UpdateReadySteps(context.Context, int64) (int64, error) { return 0, nil }

// SkipDependentSteps is a no-op — the master handles this via AdvanceBuild.
func (s *StepClient) SkipDependentSteps(context.Context, int64) (int64, error) { return 0, nil }

// ListByBuild fetches all steps for a build from the master.
func (s *StepClient) ListByBuild(ctx context.Context, buildID int64) ([]*models.BuildStep, error) {
	var resp struct {
		Steps []stepJSON `json:"steps"`
	}
	if err := s.c.get(ctx, fmt.Sprintf("/api/worker/builds/%d/steps", buildID), &resp); err != nil {
		return nil, err
	}
	steps := make([]*models.BuildStep, len(resp.Steps))
	for i, sj := range resp.Steps {
		steps[i] = sj.toModel()
	}
	return steps, nil
}

// --- BuildClient implements worker.buildRepo ---

// BuildClient wraps the base Client for build operations.
type BuildClient struct{ c *Client }

// GetByID fetches a build from the master.
func (b *BuildClient) GetByID(ctx context.Context, id int64) (*models.Build, error) {
	var resp buildJSON
	if err := b.c.get(ctx, fmt.Sprintf("/api/worker/builds/%d", id), &resp); err != nil {
		return nil, err
	}
	return resp.toModel(), nil
}

// SetStarted marks a build as started on the master.
func (b *BuildClient) SetStarted(ctx context.Context, id int64) error {
	return b.c.post(ctx, fmt.Sprintf("/api/worker/builds/%d/started", id), struct{}{}, nil)
}

// SetFinished is a no-op — the master handles this via AdvanceBuild.
func (b *BuildClient) SetFinished(context.Context, int64, models.BuildStatus) error { return nil }

// UpdateStatus is a no-op — the master handles this via AdvanceBuild.
func (b *BuildClient) UpdateStatus(context.Context, int64, models.BuildStatus) error { return nil }

// --- ProjectClient implements worker.projectRepo ---

// ProjectClient wraps the base Client for project operations.
type ProjectClient struct{ c *Client }

// GetByID fetches project metadata from the master.
func (p *ProjectClient) GetByID(ctx context.Context, id int64) (*models.Project, error) {
	var resp projectJSON
	if err := p.c.get(ctx, fmt.Sprintf("/api/worker/projects/%d", id), &resp); err != nil {
		return nil, err
	}
	return resp.toModel(), nil
}

// --- WorkerClient implements worker.workerRepo ---

// WorkerClient wraps the base Client for worker registration operations.
type WorkerClient struct{ c *Client }

// Register registers this worker with the master.
func (w *WorkerClient) Register(ctx context.Context, worker *models.Worker) error {
	return w.c.post(ctx, "/api/worker/register",
		map[string]string{"id": worker.ID, "name": worker.Name}, nil)
}

// UpdateHeartbeat sends a heartbeat to the master.
func (w *WorkerClient) UpdateHeartbeat(ctx context.Context, id string) error {
	return w.c.post(ctx, "/api/worker/heartbeat",
		map[string]string{"id": id}, nil)
}

// UpdateStatus updates worker status on the master.
func (w *WorkerClient) UpdateStatus(ctx context.Context, id string, status models.WorkerStatus, currentStepID *int64) error {
	return w.c.post(ctx, "/api/worker/status",
		map[string]any{"id": id, "status": string(status), "current_step_id": currentStepID}, nil)
}

// SetOffline marks this worker as offline on the master.
func (w *WorkerClient) SetOffline(ctx context.Context, id string) error {
	return w.c.post(ctx, "/api/worker/offline",
		map[string]string{"id": id}, nil)
}

// --- SecretClient (on base Client) ---

// GetDecryptedSecrets fetches decrypted secrets from the master.
func (c *Client) GetDecryptedSecrets(ctx context.Context, projectID int64) (map[string]string, error) {
	var resp struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/worker/projects/%d/secrets", projectID), &resp); err != nil {
		return nil, err
	}
	return resp.Secrets, nil
}

// --- TokenClient (on base Client) ---

// TokenForProject fetches a clone token from the master.
func (c *Client) TokenForProject(ctx context.Context, projectID int64) (string, error) {
	var resp struct {
		Token string `json:"token"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/worker/projects/%d/token", projectID), &resp); err != nil {
		return "", err
	}
	return resp.Token, nil
}

// --- LogUploader (on base Client) ---

// UploadLog sends a step log file to the master.
func (c *Client) UploadLog(ctx context.Context, stepID int64, logPath string) error {
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	url := fmt.Sprintf("%s/api/worker/steps/%d/log", c.baseURL, stepID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, f)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload log failed: %s %s", resp.Status, body)
	}
	return nil
}

// --- No-op implementations for distributed mode ---

// NoopStatusPoster is a no-op status poster for distributed workers.
type NoopStatusPoster struct{}

// PostBuildStatus is a no-op.
func (NoopStatusPoster) PostBuildStatus(context.Context, *models.Project, *models.Build) {}

// PostStepStatus is a no-op.
func (NoopStatusPoster) PostStepStatus(context.Context, *models.Project, *models.Build, string, models.StepStatus) {
}

// NoopNotifier is a no-op notifier for distributed workers.
type NoopNotifier struct{}

// NotifyBuild is a no-op.
func (NoopNotifier) NotifyBuild(context.Context, *models.Build, *models.Project) error { return nil }

// NoopAdvancer is a no-op advancer for distributed workers.
type NoopAdvancer struct{}

// AdvanceBuild is a no-op — the master advances after CompleteStep.
func (NoopAdvancer) AdvanceBuild(context.Context, int64) error { return nil }

// --- HTTP helpers ---

func (c *Client) get(ctx context.Context, path string, out any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	return c.doJSON(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	return c.doJSON(req, out)
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, body)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// --- JSON types for deserialization ---

type stepJSON struct {
	ID               int64               `json:"id"`
	BuildID          int64               `json:"build_id"`
	Name             string              `json:"name"`
	Image            *string             `json:"image"`
	Status           string              `json:"status"`
	Commands         []string            `json:"commands"`
	Env              map[string]string   `json:"env"`
	DependsOn        []string            `json:"depends_on"`
	Cache            *models.CacheConfig    `json:"cache,omitempty"`
	CacheResolvedKey string                 `json:"cache_resolved_key,omitempty"`
	Services         []models.ServiceConfig `json:"services,omitempty"`
	WorkingDir       string                 `json:"working_dir"`
	TimeoutMinutes   int                    `json:"timeout_minutes"`
	RequiresApproval bool                   `json:"requires_approval"`
	ConditionExpr    string                 `json:"condition_expr,omitempty"`
}

func (s stepJSON) toModel() *models.BuildStep {
	return &models.BuildStep{
		ID:               s.ID,
		BuildID:          s.BuildID,
		Name:             s.Name,
		Image:            s.Image,
		Status:           models.StepStatus(s.Status),
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

func (b buildJSON) toModel() *models.Build {
	return &models.Build{
		ID:                b.ID,
		ProjectID:         b.ProjectID,
		BuildNumber:       b.BuildNumber,
		CommitSHA:         b.CommitSHA,
		CommitMessage:     b.CommitMessage,
		CommitAuthor:      b.CommitAuthor,
		Branch:            b.Branch,
		PullRequestNumber: b.PullRequestNumber,
		Status:            models.BuildStatus(b.Status),
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

func (p projectJSON) toModel() *models.Project {
	return &models.Project{
		ID:            p.ID,
		Provider:      p.Provider,
		Namespace:     p.Namespace,
		Name:          p.Name,
		FullName:      p.FullName,
		CloneURL:      p.CloneURL,
		DefaultBranch: p.DefaultBranch,
	}
}
