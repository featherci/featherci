package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/featherci/featherci/internal/models"
)

// stepLogProjectRepo resolves any full name to a fixed project.
type stepLogProjectRepo struct {
	mockProjectRepo
	project *models.Project
}

func (m *stepLogProjectRepo) GetByFullName(context.Context, string, string) (*models.Project, error) {
	return m.project, nil
}

// stepLogBuildRepo resolves any build number to a fixed build.
type stepLogBuildRepo struct {
	mockBuildRepo
	build *models.Build
}

func (m *stepLogBuildRepo) GetByNumber(context.Context, int64, int) (*models.Build, error) {
	return m.build, nil
}

// writeLogFile writes n numbered lines ("line 1" .. "line n") and returns the path.
func writeLogFile(t *testing.T, n int) string {
	t.Helper()
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	path := filepath.Join(t.TempDir(), "step.log")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}
	return path
}

// newStepLogHandler builds a BuildHandler serving a single step with the given
// status and log path. logPath may be empty to simulate a step without logs.
func newStepLogHandler(status models.StepStatus, logPath string) *BuildHandler {
	step := &models.BuildStep{ID: 7, BuildID: 10, Name: "test", Status: status}
	if logPath != "" {
		step.LogPath = &logPath
	}
	return NewBuildHandler(
		&stepLogProjectRepo{project: &models.Project{ID: 1, FullName: "acme/app"}},
		&stepLogBuildRepo{build: &models.Build{ID: 10, ProjectID: 1, BuildNumber: 24}},
		&mockStepRepo{step: step},
		nil, nil, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

// getStepLog calls StepLog for step 7 of build 24 at the given offset.
func getStepLog(t *testing.T, h *BuildHandler, offset int) StepLogResponse {
	t.Helper()
	url := "/projects/acme/app/builds/24/steps/7/log?offset=" + strconv.Itoa(offset)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("namespace", "acme")
	req.SetPathValue("name", "app")
	req.SetPathValue("number", "24")
	req.SetPathValue("stepID", "7")
	rec := httptest.NewRecorder()

	h.StepLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp StepLogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return resp
}

func TestStepLog_Pagination(t *testing.T) {
	tests := []struct {
		name      string
		status    models.StepStatus
		lines     int
		offset    int
		wantLines int
		wantFirst string
		wantTotal int
		wantDone  bool
	}{
		{
			// Regression: a finished step with more than one page used to
			// report done=true on the first page, so the UI stopped at line 500.
			name:      "finished step, first page of a long log is not done",
			status:    models.StepStatusFailure,
			lines:     773,
			offset:    0,
			wantLines: defaultLogLimit,
			wantFirst: "line 1",
			wantTotal: 773,
			wantDone:  false,
		},
		{
			name:      "finished step, last page of a long log is done",
			status:    models.StepStatusFailure,
			lines:     773,
			offset:    defaultLogLimit,
			wantLines: 273,
			wantFirst: "line 501",
			wantTotal: 773,
			wantDone:  true,
		},
		{
			name:      "finished step, log exactly one page is done",
			status:    models.StepStatusSuccess,
			lines:     defaultLogLimit,
			offset:    0,
			wantLines: defaultLogLimit,
			wantFirst: "line 1",
			wantTotal: defaultLogLimit,
			wantDone:  true,
		},
		{
			name:      "finished step, short log is done in one page",
			status:    models.StepStatusSuccess,
			lines:     42,
			offset:    0,
			wantLines: 42,
			wantFirst: "line 1",
			wantTotal: 42,
			wantDone:  true,
		},
		{
			name:      "finished step, offset past the end is done",
			status:    models.StepStatusSuccess,
			lines:     42,
			offset:    100,
			wantLines: 0,
			wantTotal: 42,
			wantDone:  true,
		},
		{
			name:      "running step is never done, even when caught up",
			status:    models.StepStatusRunning,
			lines:     42,
			offset:    42,
			wantLines: 0,
			wantTotal: 42,
			wantDone:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newStepLogHandler(tt.status, writeLogFile(t, tt.lines))

			resp := getStepLog(t, h, tt.offset)

			if len(resp.Lines) != tt.wantLines {
				t.Errorf("lines: got %d, want %d", len(resp.Lines), tt.wantLines)
			}
			if tt.wantFirst != "" && len(resp.Lines) > 0 && resp.Lines[0] != tt.wantFirst {
				t.Errorf("first line: got %q, want %q", resp.Lines[0], tt.wantFirst)
			}
			if resp.Offset != tt.offset {
				t.Errorf("offset: got %d, want %d", resp.Offset, tt.offset)
			}
			if resp.Total != tt.wantTotal {
				t.Errorf("total: got %d, want %d", resp.Total, tt.wantTotal)
			}
			if resp.Done != tt.wantDone {
				t.Errorf("done: got %v, want %v", resp.Done, tt.wantDone)
			}
		})
	}
}

func TestStepLog_FinishedStepWithoutLogIsDone(t *testing.T) {
	h := newStepLogHandler(models.StepStatusSkipped, "")

	resp := getStepLog(t, h, 0)

	if len(resp.Lines) != 0 || resp.Total != 0 || !resp.Done {
		t.Errorf("got lines=%d total=%d done=%v, want 0/0/true", len(resp.Lines), resp.Total, resp.Done)
	}
}

// TestStepLog_ClientLoopReadsWholeLog follows the same contract as the build
// page: request offset+len(lines) until done. It must receive every line once.
func TestStepLog_ClientLoopReadsWholeLog(t *testing.T) {
	const total = 1234
	h := newStepLogHandler(models.StepStatusFailure, writeLogFile(t, total))

	var got []string
	offset, requests := 0, 0
	for {
		requests++
		if requests > 10 {
			t.Fatalf("client did not finish after %d requests", requests-1)
		}
		resp := getStepLog(t, h, offset)
		got = append(got, resp.Lines...)
		offset += len(resp.Lines)
		if resp.Done {
			break
		}
	}

	if len(got) != total {
		t.Fatalf("received %d lines, want %d", len(got), total)
	}
	for i, line := range got {
		if want := fmt.Sprintf("line %d", i+1); line != want {
			t.Fatalf("line %d: got %q, want %q", i+1, line, want)
		}
	}
	if wantRequests := (total + defaultLogLimit - 1) / defaultLogLimit; requests != wantRequests {
		t.Errorf("requests: got %d, want %d", requests, wantRequests)
	}
}

func TestStepLog_StepFromAnotherBuildIsNotFound(t *testing.T) {
	h := newStepLogHandler(models.StepStatusSuccess, writeLogFile(t, 3))
	h.steps = &mockStepRepo{step: &models.BuildStep{ID: 7, BuildID: 99, Status: models.StepStatusSuccess}}

	req := httptest.NewRequest(http.MethodGet, "/projects/acme/app/builds/24/steps/7/log", nil)
	req.SetPathValue("namespace", "acme")
	req.SetPathValue("name", "app")
	req.SetPathValue("number", "24")
	req.SetPathValue("stepID", "7")
	rec := httptest.NewRecorder()

	h.StepLog(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}
