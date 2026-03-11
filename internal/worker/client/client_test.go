package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/featherci/featherci/internal/models"
)

// mockMasterAPI sets up an httptest server that mimics the master worker API.
func mockMasterAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/worker/steps/ready", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		img := "golang:1.22"
		json.NewEncoder(w).Encode(map[string]any{
			"steps": []map[string]any{
				{"id": 1, "build_id": 10, "name": "build", "image": img, "status": "ready", "commands": []string{"go build"}, "env": map[string]string{}, "depends_on": []string{}, "working_dir": "", "timeout_minutes": 30},
				{"id": 2, "build_id": 10, "name": "test", "status": "ready", "commands": []string{"go test"}, "env": map[string]string{}, "depends_on": []string{"build"}, "working_dir": "", "timeout_minutes": 30},
			},
		})
	})

	mux.HandleFunc("POST /api/worker/steps/{id}/claim", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["worker_id"] == "" {
			http.Error(w, `{"error":"worker_id required"}`, http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "claimed"})
	})

	mux.HandleFunc("POST /api/worker/steps/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
	})

	mux.HandleFunc("POST /api/worker/steps/{id}/log", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		body, _ := io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]string{"status": "uploaded", "size": string(body)})
	})

	mux.HandleFunc("GET /api/worker/builds/{id}", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		branch := "main"
		msg := "test commit"
		json.NewEncoder(w).Encode(map[string]any{
			"id": 10, "project_id": 1, "build_number": 5,
			"commit_sha": "abc123", "commit_message": msg,
			"branch": branch, "status": "running",
		})
	})

	mux.HandleFunc("GET /api/worker/builds/{id}/steps", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]any{
			"steps": []map[string]any{
				{"id": 1, "build_id": 10, "name": "build", "status": "success", "commands": []string{}, "env": map[string]string{}, "depends_on": []string{}, "working_dir": "", "timeout_minutes": 30},
			},
		})
	})

	mux.HandleFunc("POST /api/worker/builds/{id}/started", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	})

	mux.HandleFunc("GET /api/worker/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "provider": "github", "namespace": "org", "name": "repo",
			"full_name": "org/repo", "clone_url": "https://github.com/org/repo.git",
			"default_branch": "main",
		})
	})

	mux.HandleFunc("GET /api/worker/projects/{id}/secrets", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]any{
			"secrets": map[string]string{"DB_PASS": "secret123"},
		})
	})

	mux.HandleFunc("GET /api/worker/projects/{id}/token", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"token": "ghp_testtoken"})
	})

	mux.HandleFunc("POST /api/worker/register", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["id"] == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
	})

	mux.HandleFunc("POST /api/worker/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/worker/status", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/worker/offline", func(w http.ResponseWriter, r *http.Request) {
		checkAuth(t, r)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return httptest.NewServer(mux)
}

func checkAuth(t *testing.T, r *http.Request) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	if auth != "Bearer test-secret" {
		t.Errorf("expected auth header 'Bearer test-secret', got %q", auth)
	}
}

func TestStepClient_ListReady(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	steps, err := c.StepClient().ListReady(context.Background())
	if err != nil {
		t.Fatalf("ListReady() error = %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Name != "build" {
		t.Errorf("expected step name 'build', got %q", steps[0].Name)
	}
	if steps[1].DependsOn[0] != "build" {
		t.Errorf("expected depends_on[0]='build', got %q", steps[1].DependsOn[0])
	}
}

func TestStepClient_SetStarted(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	err := c.StepClient().SetStarted(context.Background(), 1, "worker-1")
	if err != nil {
		t.Fatalf("SetStarted() error = %v", err)
	}
}

func TestStepClient_SetFinished(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	exitCode := 0
	err := c.StepClient().SetFinished(context.Background(), 1, models.StepStatusSuccess, &exitCode, "/tmp/log")
	if err != nil {
		t.Fatalf("SetFinished() error = %v", err)
	}
}

func TestStepClient_ListByBuild(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	steps, err := c.StepClient().ListByBuild(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListByBuild() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Status != models.StepStatusSuccess {
		t.Errorf("expected status success, got %s", steps[0].Status)
	}
}

func TestStepClient_NoopMethods(t *testing.T) {
	c := New("http://unused", "secret")
	sc := c.StepClient()

	n, err := sc.UpdateReadySteps(context.Background(), 1)
	if err != nil || n != 0 {
		t.Errorf("UpdateReadySteps should be no-op, got n=%d err=%v", n, err)
	}

	n, err = sc.SkipDependentSteps(context.Background(), 1)
	if err != nil || n != 0 {
		t.Errorf("SkipDependentSteps should be no-op, got n=%d err=%v", n, err)
	}
}

func TestBuildClient_GetByID(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	build, err := c.BuildClient().GetByID(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if build.ID != 10 {
		t.Errorf("expected build ID 10, got %d", build.ID)
	}
	if build.CommitSHA != "abc123" {
		t.Errorf("expected commit sha abc123, got %s", build.CommitSHA)
	}
	if build.Status != models.BuildStatusRunning {
		t.Errorf("expected status running, got %s", build.Status)
	}
}

func TestBuildClient_SetStarted(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	err := c.BuildClient().SetStarted(context.Background(), 10)
	if err != nil {
		t.Fatalf("SetStarted() error = %v", err)
	}
}

func TestBuildClient_NoopMethods(t *testing.T) {
	c := New("http://unused", "secret")
	bc := c.BuildClient()

	if err := bc.SetFinished(context.Background(), 1, models.BuildStatusSuccess); err != nil {
		t.Errorf("SetFinished should be no-op, got %v", err)
	}
	if err := bc.UpdateStatus(context.Background(), 1, models.BuildStatusRunning); err != nil {
		t.Errorf("UpdateStatus should be no-op, got %v", err)
	}
}

func TestProjectClient_GetByID(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	project, err := c.ProjectClient().GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if project.FullName != "org/repo" {
		t.Errorf("expected full_name org/repo, got %s", project.FullName)
	}
	if project.Provider != "github" {
		t.Errorf("expected provider github, got %s", project.Provider)
	}
}

func TestWorkerClient_Register(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	err := c.WorkerClient().Register(context.Background(), &models.Worker{
		ID: "w1", Name: "Worker 1", Status: models.WorkerStatusIdle,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestWorkerClient_UpdateHeartbeat(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	err := c.WorkerClient().UpdateHeartbeat(context.Background(), "w1")
	if err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}
}

func TestWorkerClient_UpdateStatus(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	stepID := int64(42)
	err := c.WorkerClient().UpdateStatus(context.Background(), "w1", models.WorkerStatusBusy, &stepID)
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
}

func TestWorkerClient_SetOffline(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	err := c.WorkerClient().SetOffline(context.Background(), "w1")
	if err != nil {
		t.Fatalf("SetOffline() error = %v", err)
	}
}

func TestClient_GetDecryptedSecrets(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	secrets, err := c.GetDecryptedSecrets(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetDecryptedSecrets() error = %v", err)
	}
	if secrets["DB_PASS"] != "secret123" {
		t.Errorf("expected DB_PASS=secret123, got %v", secrets)
	}
}

func TestClient_TokenForProject(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	c := New(srv.URL, "test-secret")
	token, err := c.TokenForProject(context.Background(), 1)
	if err != nil {
		t.Fatalf("TokenForProject() error = %v", err)
	}
	if token != "ghp_testtoken" {
		t.Errorf("expected ghp_testtoken, got %s", token)
	}
}

func TestClient_UploadLog(t *testing.T) {
	srv := mockMasterAPI(t)
	defer srv.Close()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "step.log")
	if err := os.WriteFile(logPath, []byte("log output here"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	c := New(srv.URL, "test-secret")
	err := c.UploadLog(context.Background(), 42, logPath)
	if err != nil {
		t.Fatalf("UploadLog() error = %v", err)
	}
}

func TestClient_AuthHeader(t *testing.T) {
	// Verify the auth header is set on every request
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]string{"token": "x"})
	}))
	defer srv.Close()

	c := New(srv.URL, "my-worker-secret")
	_, _ = c.TokenForProject(context.Background(), 1)

	if gotAuth != "Bearer my-worker-secret" {
		t.Errorf("expected 'Bearer my-worker-secret', got %q", gotAuth)
	}
}

func TestClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	_, err := c.TokenForProject(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
