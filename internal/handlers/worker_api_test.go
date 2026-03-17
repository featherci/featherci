package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/featherci/featherci/internal/models"
	"github.com/featherci/featherci/internal/services"
)

// --- Mock implementations ---

type mockStepRepo struct {
	readySteps []*models.BuildStep
	listErr    error
	step       *models.BuildStep
	getErr     error
	setStarted error
	setFinished error
	updated    *models.BuildStep
	buildSteps []*models.BuildStep
}

func (m *mockStepRepo) Create(context.Context, *models.BuildStep) error                    { return nil }
func (m *mockStepRepo) CreateBatch(context.Context, []*models.BuildStep) error              { return nil }
func (m *mockStepRepo) ListWaitingApproval(context.Context, int64) ([]*models.BuildStep, error) {
	return nil, nil
}
func (m *mockStepRepo) UpdateStatus(context.Context, int64, models.StepStatus) error { return nil }
func (m *mockStepRepo) SetApproval(context.Context, int64, int64) error              { return nil }
func (m *mockStepRepo) AddDependency(context.Context, int64, int64) error            { return nil }
func (m *mockStepRepo) GetDependencies(context.Context, int64) ([]*models.BuildStep, error) {
	return nil, nil
}
func (m *mockStepRepo) UpdateReadySteps(context.Context, int64) (int64, error)     { return 0, nil }
func (m *mockStepRepo) SkipDependentSteps(context.Context, int64) (int64, error)   { return 0, nil }
func (m *mockStepRepo) CancelBuildSteps(context.Context, int64) (int64, error)     { return 0, nil }
func (m *mockStepRepo) ResetStepsForWorker(context.Context, string) error          { return nil }

func (m *mockStepRepo) ListReady(_ context.Context) ([]*models.BuildStep, error) {
	return m.readySteps, m.listErr
}

func (m *mockStepRepo) GetByID(_ context.Context, id int64) (*models.BuildStep, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.step != nil {
		return m.step, nil
	}
	return &models.BuildStep{ID: id, BuildID: 10, Name: "test"}, nil
}

func (m *mockStepRepo) ListByBuild(_ context.Context, _ int64) ([]*models.BuildStep, error) {
	if m.buildSteps != nil {
		return m.buildSteps, nil
	}
	return []*models.BuildStep{}, nil
}

func (m *mockStepRepo) SetStarted(_ context.Context, _ int64, _ string) error {
	return m.setStarted
}

func (m *mockStepRepo) SetLogPath(_ context.Context, _ int64, _ string) error {
	return nil
}

func (m *mockStepRepo) SetFinished(_ context.Context, _ int64, _ models.StepStatus, _ *int, _ string) error {
	return m.setFinished
}

func (m *mockStepRepo) Update(_ context.Context, step *models.BuildStep) error {
	m.updated = step
	return nil
}

type mockBuildRepo struct {
	build      *models.Build
	getErr     error
	startedErr error
}

func (m *mockBuildRepo) Create(context.Context, *models.Build) error                          { return nil }
func (m *mockBuildRepo) GetByNumber(context.Context, int64, int) (*models.Build, error)       { return nil, nil }
func (m *mockBuildRepo) ListByProject(context.Context, int64, int, int) ([]*models.Build, error) { return nil, nil }
func (m *mockBuildRepo) ListByUser(context.Context, int64, int, int) ([]*models.Build, error) { return nil, nil }
func (m *mockBuildRepo) ListPending(context.Context) ([]*models.Build, error)                 { return nil, nil }
func (m *mockBuildRepo) Update(context.Context, *models.Build) error                          { return nil }
func (m *mockBuildRepo) UpdateStatus(context.Context, int64, models.BuildStatus) error        { return nil }
func (m *mockBuildRepo) GetNextBuildNumber(context.Context, int64) (int, error)               { return 0, nil }
func (m *mockBuildRepo) SetFinished(context.Context, int64, models.BuildStatus) error         { return nil }
func (m *mockBuildRepo) CancelBuild(context.Context, int64) error                             { return nil }
func (m *mockBuildRepo) CountByProject(context.Context, int64) (int, error)                   { return 0, nil }
func (m *mockBuildRepo) Count(context.Context) (int, error)                                   { return 0, nil }

func (m *mockBuildRepo) GetByID(_ context.Context, _ int64) (*models.Build, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.build != nil {
		return m.build, nil
	}
	return &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning}, nil
}

func (m *mockBuildRepo) SetStarted(_ context.Context, _ int64) error {
	return m.startedErr
}

type mockProjectRepo struct {
	project *models.Project
	getErr  error
}

func (m *mockProjectRepo) Create(context.Context, *models.Project) error                    { return nil }
func (m *mockProjectRepo) GetByFullName(context.Context, string, string) (*models.Project, error) { return nil, nil }
func (m *mockProjectRepo) List(context.Context) ([]*models.Project, error)                  { return nil, nil }
func (m *mockProjectRepo) ListWithStatus(context.Context) ([]*models.ProjectWithStatus, error) { return nil, nil }
func (m *mockProjectRepo) Update(context.Context, *models.Project) error                    { return nil }
func (m *mockProjectRepo) Delete(context.Context, int64) error                              { return nil }
func (m *mockProjectRepo) CountAll(context.Context) (int, error)                            { return 0, nil }

func (m *mockProjectRepo) GetByID(_ context.Context, _ int64) (*models.Project, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.project != nil {
		return m.project, nil
	}
	return &models.Project{ID: 1, Provider: "github", FullName: "org/repo"}, nil
}

type mockWorkerRepo struct {
	registerErr   error
	heartbeatErr  error
	updateStatErr error
	offlineErr    error
}

func (m *mockWorkerRepo) Register(_ context.Context, _ *models.Worker) error { return m.registerErr }
func (m *mockWorkerRepo) UpdateHeartbeat(_ context.Context, _ string) error  { return m.heartbeatErr }
func (m *mockWorkerRepo) UpdateStatus(_ context.Context, _ string, _ models.WorkerStatus, _ *int64) error {
	return m.updateStatErr
}
func (m *mockWorkerRepo) SetOffline(_ context.Context, _ string) error                       { return m.offlineErr }
func (m *mockWorkerRepo) ListStale(_ context.Context, _ time.Duration) ([]*models.Worker, error) { return nil, nil }
func (m *mockWorkerRepo) List(context.Context) ([]*models.Worker, error)                     { return nil, nil }
func (m *mockWorkerRepo) CountActive(context.Context) (int, error)                           { return 0, nil }
func (m *mockWorkerRepo) PurgeOffline(_ context.Context, _ time.Duration) (int64, error)     { return 0, nil }

type mockSecretDecrypter struct {
	secrets map[string]string
	err     error
}

func (m *mockSecretDecrypter) GetDecryptedSecrets(_ context.Context, _ int64) (map[string]string, error) {
	return m.secrets, m.err
}

type mockTokenProvider struct {
	token string
	err   error
}

func (m *mockTokenProvider) TokenForProject(_ context.Context, _ int64) (string, error) {
	return m.token, m.err
}

type mockStepStatusPoster struct{}

func (m *mockStepStatusPoster) PostBuildStatus(context.Context, *models.Project, *models.Build) {}
func (m *mockStepStatusPoster) PostStepStatus(context.Context, *models.Project, *models.Build, string, models.StepStatus) {
}

// --- Helper ---

func newTestHandler(t *testing.T) (*WorkerAPIHandler, *mockStepRepo, *mockBuildRepo, *mockProjectRepo, *mockWorkerRepo) {
	t.Helper()
	stepRepo := &mockStepRepo{}
	buildRepo := &mockBuildRepo{}
	projectRepo := &mockProjectRepo{}
	workerRepo := &mockWorkerRepo{}
	secrets := &mockSecretDecrypter{secrets: map[string]string{"KEY": "value"}}
	tokens := &mockTokenProvider{token: "ghp_abc123"}
	poster := &mockStepStatusPoster{}

	advancer := services.NewBuildAdvancer(
		// Use minimal mock repos for the advancer since we're testing the handler, not the advancer
		&mockAdvancerStepRepo{
			steps:       []*models.BuildStep{},
			skipResults: []int64{0},
		},
		&mockAdvancerBuildRepo{
			build: &models.Build{ID: 10, ProjectID: 1, Status: models.BuildStatusRunning},
		},
		&mockAdvancerProjectRepo{
			project: &models.Project{ID: 1},
		},
		nil, nil, nil,
	)

	h := NewWorkerAPIHandler(stepRepo, buildRepo, projectRepo, workerRepo, secrets, tokens, poster, advancer, t.TempDir(), nil)
	return h, stepRepo, buildRepo, projectRepo, workerRepo
}

// Use Go 1.22+ ServeMux path patterns for {id} matching
func serveMux(h *WorkerAPIHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/worker/steps/ready", h.ListReadySteps)
	mux.HandleFunc("POST /api/worker/steps/{id}/claim", h.ClaimStep)
	mux.HandleFunc("POST /api/worker/steps/{id}/complete", h.CompleteStep)
	mux.HandleFunc("POST /api/worker/steps/{id}/log", h.UploadLog)
	mux.HandleFunc("GET /api/worker/builds/{id}", h.GetBuild)
	mux.HandleFunc("GET /api/worker/builds/{id}/steps", h.ListBuildSteps)
	mux.HandleFunc("POST /api/worker/builds/{id}/started", h.BuildStarted)
	mux.HandleFunc("GET /api/worker/projects/{id}", h.GetProject)
	mux.HandleFunc("GET /api/worker/projects/{id}/secrets", h.GetProjectSecrets)
	mux.HandleFunc("GET /api/worker/projects/{id}/token", h.GetProjectToken)
	mux.HandleFunc("POST /api/worker/register", h.Register)
	mux.HandleFunc("POST /api/worker/heartbeat", h.Heartbeat)
	mux.HandleFunc("POST /api/worker/status", h.UpdateStatus)
	mux.HandleFunc("POST /api/worker/offline", h.SetOffline)
	return mux
}

// --- Tests ---

func TestListReadySteps(t *testing.T) {
	h, stepRepo, _, _, _ := newTestHandler(t)
	img := "golang:1.22"
	stepRepo.readySteps = []*models.BuildStep{
		{ID: 1, BuildID: 10, Name: "build", Image: &img, Status: models.StepStatusReady, Commands: []string{"go build"}},
		{ID: 2, BuildID: 10, Name: "test", Status: models.StepStatusReady, Commands: []string{"go test"}},
	}

	mux := serveMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/worker/steps/ready", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Steps []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(resp.Steps))
	}
}

func TestClaimStep_Valid(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{"worker_id":"w1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/steps/1/claim", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestClaimStep_MissingWorkerID(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/steps/1/claim", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestClaimStep_NotFound(t *testing.T) {
	h, stepRepo, _, _, _ := newTestHandler(t)
	stepRepo.setStarted = models.ErrNotFound
	mux := serveMux(h)

	body := `{"worker_id":"w1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/steps/999/claim", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestCompleteStep(t *testing.T) {
	h, stepRepo, _, _, _ := newTestHandler(t)
	stepRepo.step = &models.BuildStep{ID: 1, BuildID: 10, Name: "test", Status: models.StepStatusRunning}
	mux := serveMux(h)

	body := `{"status":"success","exit_code":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/steps/1/complete", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadLog(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	logContent := "step output line 1\nstep output line 2\n"
	req := httptest.NewRequest(http.MethodPost, "/api/worker/steps/42/log", strings.NewReader(logContent))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["status"] != "uploaded" {
		t.Errorf("expected status=uploaded, got %s", resp["status"])
	}
	if resp["path"] == "" {
		t.Error("expected non-empty path in response")
	}
}

func TestGetBuild(t *testing.T) {
	h, _, buildRepo, _, _ := newTestHandler(t)
	branch := "main"
	buildRepo.build = &models.Build{
		ID: 10, ProjectID: 1, BuildNumber: 5,
		CommitSHA: "abc123", Branch: &branch,
		Status: models.BuildStatusRunning,
	}
	mux := serveMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/builds/10", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.ID != 10 {
		t.Errorf("expected build ID 10, got %d", resp.ID)
	}
}

func TestGetBuild_NotFound(t *testing.T) {
	h, _, buildRepo, _, _ := newTestHandler(t)
	buildRepo.getErr = models.ErrNotFound
	mux := serveMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/builds/999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetProject(t *testing.T) {
	h, _, _, projectRepo, _ := newTestHandler(t)
	projectRepo.project = &models.Project{
		ID: 1, Provider: "github", Namespace: "org", Name: "repo",
		FullName: "org/repo", CloneURL: "https://github.com/org/repo.git",
		DefaultBranch: "main",
	}
	mux := serveMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/projects/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		FullName string `json:"full_name"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.FullName != "org/repo" {
		t.Errorf("expected full_name org/repo, got %s", resp.FullName)
	}
}

func TestGetProjectSecrets(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/projects/1/secrets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Secrets map[string]string `json:"secrets"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Secrets["KEY"] != "value" {
		t.Errorf("expected secret KEY=value, got %v", resp.Secrets)
	}
}

func TestGetProjectToken(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/projects/1/token", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Token string `json:"token"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Token != "ghp_abc123" {
		t.Errorf("expected token ghp_abc123, got %s", resp.Token)
	}
}

func TestRegister(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{"id":"worker-1","name":"Worker One"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegister_MissingID(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{"name":"Worker One"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHeartbeat(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{"id":"worker-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHeartbeat_MissingID(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpdateStatus(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	stepID := int64(42)
	reqBody := map[string]any{
		"id":              "worker-1",
		"status":          "busy",
		"current_step_id": stepID,
	}
	data, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/worker/status", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetOffline(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{"id":"worker-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/offline", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetOffline_MissingID(t *testing.T) {
	h, _, _, _, _ := newTestHandler(t)
	mux := serveMux(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/worker/offline", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- Advancer mock types for the handler test (separate package) ---

type mockAdvancerStepRepo struct {
	steps       []*models.BuildStep
	skipResults []int64
	skipCount   int
}

func (m *mockAdvancerStepRepo) ListByBuild(context.Context, int64) ([]*models.BuildStep, error) {
	return m.steps, nil
}
func (m *mockAdvancerStepRepo) UpdateReadySteps(context.Context, int64) (int64, error) { return 0, nil }
func (m *mockAdvancerStepRepo) SkipDependentSteps(context.Context, int64) (int64, error) {
	idx := m.skipCount
	m.skipCount++
	if idx < len(m.skipResults) {
		return m.skipResults[idx], nil
	}
	return 0, nil
}

type mockAdvancerBuildRepo struct {
	build *models.Build
}

func (m *mockAdvancerBuildRepo) GetByID(_ context.Context, _ int64) (*models.Build, error) {
	b := *m.build
	return &b, nil
}
func (m *mockAdvancerBuildRepo) SetFinished(context.Context, int64, models.BuildStatus) error {
	return nil
}
func (m *mockAdvancerBuildRepo) UpdateStatus(context.Context, int64, models.BuildStatus) error {
	return nil
}

type mockAdvancerProjectRepo struct {
	project *models.Project
}

func (m *mockAdvancerProjectRepo) GetByID(context.Context, int64) (*models.Project, error) {
	return m.project, nil
}
